package sandbox_manager

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/ssuji15/wolf/internal/job_tracer"
	"github.com/ssuji15/wolf/internal/sandbox_manager/worker"
	jobservice "github.com/ssuji15/wolf/internal/service/job_service"
	"github.com/ssuji15/wolf/internal/service/logger"
	"github.com/ssuji15/wolf/internal/util"
	"github.com/ssuji15/wolf/model"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func (m *SandboxManager) dispatchJob(ctx context.Context, j *model.Job, worker *worker.WorkerMetadata) error {
	tracer := job_tracer.GetTracer()
	span := trace.SpanFromContext(ctx)
	defer func() {
		go m.shutdownWorker(worker)
	}()

	if v, err := worker.Validate(ctx); !v || err != nil {
		err := fmt.Errorf("invalid worker. please try again: %v", err)
		util.RecordSpanError(span, err)
		return err
	}
	span.AddEvent("job.context",
		trace.WithAttributes(attribute.String("container_id", worker.ID)),
	)

	sourceCode, err := m.jobService.GetCode(ctx, j)
	if err != nil {
		return err
	}

	_, dspan := tracer.Start(ctx, "ExecuteCode")
	defer dspan.End()
	go func() {
		defer dspan.End()
		_, err := worker.Wait(ctx)
		if err != nil {
			util.RecordSpanError(dspan, err)
			return
		}
	}()

	now := time.Now().UTC()
	j.StartTime = &now
	err = worker.Raven.Send(ctx, j, sourceCode)
	if err != nil {
		util.RecordSpanError(dspan, err)
		return err
	}
	err = m.sendResult(ctx, j, worker)
	if err != nil {
		util.RecordSpanError(span, err)
		return err
	}
	return nil
}

func (m *SandboxManager) sendResult(ctx context.Context, j *model.Job, w *worker.WorkerMetadata) error {

	data, err := w.Raven.Receive(ctx)
	if err != nil {
		return err
	}

	tracer := job_tracer.GetTracer()
	ctx, span := tracer.Start(ctx, "PersistOutput")
	defer span.End()

	hashBytes := sha256.Sum256(data)
	oHash := fmt.Sprintf("%x", hashBytes[:])
	objectPath := util.GetOutputPath(oHash)
	if err := m.storageClient.Upload(ctx, m.storageClient.GetJobsBucket(), objectPath, data); err != nil {
		return fmt.Errorf("failed to upload code: %v", err)
	}

	now := time.Now().UTC()
	j.OutputHash = oHash
	j.Status = string(jobservice.JOB_COMPLETED)
	j.EndTime = &now
	err = m.jobService.UpdateJob(ctx, j)
	if err != nil {
		util.RecordSpanError(span, err)
		return err
	}

	err = m.jobService.CacheOutputHash(ctx, j)
	if err != nil {
		util.RecordSpanError(span, err)
		logger.Log.Err(err).Msg("unable to cache job output")
	}

	return nil
}
