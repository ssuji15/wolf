package sandbox_manager

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"time"

	"github.com/ssuji15/wolf/internal/job_tracer"
	jobservice "github.com/ssuji15/wolf/internal/service/job_service"
	"github.com/ssuji15/wolf/internal/service/logger"
	"github.com/ssuji15/wolf/internal/util"
	"github.com/ssuji15/wolf/model"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func (m *SandboxManager) dispatchJob(ctx context.Context, j *model.Job, worker model.WorkerMetadata) error {
	tracer := job_tracer.GetTracer()
	ctx, span := tracer.Start(ctx, "ProcessJob")
	defer span.End()
	defer func() {
		go m.shutdownWorker(worker)
	}()

	if worker.ID == "" || worker.SocketPath == "" {
		err := fmt.Errorf("invalid worker. please try again")
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

	now := time.Now().UTC()
	j.StartTime = &now
	err = m.launcher.DispatchJob(worker.SocketPath, j, sourceCode)
	if err != nil {
		util.RecordSpanError(dspan, err)
		return err
	}
	statusCode, err := m.launcher.ContainerWaitTillExit(ctx, worker.ID)
	if err != nil {
		util.RecordSpanError(dspan, err)
		return err
	}
	if statusCode != 0 {
		err = fmt.Errorf("container exited with status code %d", statusCode)
		util.RecordSpanError(dspan, err)
		return err
	}
	dspan.End()
	err = m.sendResult(ctx, j, worker)
	if err != nil {
		util.RecordSpanError(span, err)
		return err
	}
	return nil
}

func (m *SandboxManager) sendResult(ctx context.Context, j *model.Job, w model.WorkerMetadata) error {

	tracer := job_tracer.GetTracer()
	ctx, span := tracer.Start(ctx, "UploadOutput")
	defer span.End()

	data, err := os.ReadFile(w.OutputPath)
	if err != nil {
		util.RecordSpanError(span, err)
		return err
	}
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
