package manager

import (
	"context"
	"fmt"
	"os"

	pb "github.com/ssuji15/wolf-worker/agent"
	"github.com/ssuji15/wolf/internal/job_tracer"
	"github.com/ssuji15/wolf/model"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"google.golang.org/grpc"
)

func (m *SandboxManager) dispatchJob(ctx context.Context, id string, worker model.WorkerMetadata) error {
	tracer := job_tracer.GetTracer()
	ctx, span := tracer.Start(ctx, "ProcessJob")
	defer m.shutdownWorker(worker)
	if worker.ID == "" || worker.SocketPath == "" {
		err := fmt.Errorf("invalid worker. please try again")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	j, err := m.jobService.GetJob(ctx, id)
	if err != nil {
		return err
	}

	code, err := m.jobService.GetCode(ctx, j)
	if err != nil {
		return err
	}

	_, dspan := tracer.Start(ctx, "ExecuteCode")
	dspan.SetAttributes(attribute.String("container_id", worker.ID))
	err = m.launcher.DispatchJob(worker.SocketPath, j, code)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	statusCode, err := m.launcher.ContainerWaitTillExit(ctx, worker.ID)
	if err != nil {
		dspan.RecordError(err)
		return err
	}
	if statusCode != 0 {
		err = fmt.Errorf("container exited with non zero exit code")
		dspan.RecordError(err)
		return err
	}
	dspan.End()
	err = m.sendResult(ctx, j, worker)
	if err != nil {
		return err
	}
	span.End()
	return nil
}

func (m *SandboxManager) sendResult(ctx context.Context, j *model.Job, w model.WorkerMetadata) error {

	tracer := job_tracer.GetTracer()
	ctx, span := tracer.Start(ctx, "UploadOutput")
	defer span.End()

	data, err := os.ReadFile(w.OutputPath)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	conn, err := grpc.DialContext(ctx,
		m.cfg.BackendAddress,
		grpc.WithInsecure(),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
		grpc.WithBlock())
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	defer conn.Close()

	client := pb.NewWorkerAgentClient(conn)
	_, err = client.UploadResult(ctx, &pb.JobResponse{
		JobId:  j.ID.String(),
		Output: string(data),
	})
	if err != nil {
		return err
	}
	return nil
}
