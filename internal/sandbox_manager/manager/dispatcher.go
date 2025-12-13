package manager

import (
	"context"
	"fmt"
	"os"

	pb "github.com/ssuji15/wolf-worker/agent"
	"github.com/ssuji15/wolf/internal/job_tracer"
	"github.com/ssuji15/wolf/model"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
)

func (m *SandboxManager) dispatchJob(ctx context.Context, id string, worker model.WorkerMetadata) error {
	tracer := job_tracer.GetTracer()
	ctx, span := tracer.Start(ctx, "ProcessJob")
	if worker.ID == "" || worker.SocketPath == "" {
		err := fmt.Errorf("invalid worker. please try again")
		span.RecordError(err)
		return err
	}

	j, err := m.jobService.GetJob(ctx, id)
	if err != nil {
		span.RecordError(err)
		return err
	}

	_, dspan := tracer.Start(ctx, "ExecuteCode")
	err = m.launcher.DispatchJob(worker.SocketPath, j)
	if err != nil {
		span.RecordError(err)
		return err
	}

	code, err := m.launcher.ContainerWaitTillExit(ctx, worker.ID)
	if err != nil {
		span.RecordError(err)
		return err
	}
	dspan.End()
	fmt.Printf("worker: %s with job: %s exited with status code: %d\n", worker.ID, j.ID, code)
	err = m.sendResult(ctx, j, worker)
	if err != nil {
		span.RecordError(err)
		return err
	}
	span.End()
	m.shutdownWorker(worker)
	return nil
}

func (m *SandboxManager) sendResult(ctx context.Context, j *model.Job, w model.WorkerMetadata) error {

	tracer := job_tracer.GetTracer()
	ctx, span := tracer.Start(ctx, "UploadOutput")
	defer span.End()

	data, err := os.ReadFile(w.OutputPath)
	if err != nil {
		span.RecordError(err)
		return err
	}

	conn, err := grpc.DialContext(ctx,
		m.cfg.BackendAddress,
		grpc.WithInsecure(),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
		grpc.WithBlock())
	if err != nil {
		span.RecordError(err)
		return err
	}
	defer conn.Close()

	client := pb.NewWorkerAgentClient(conn)
	_, err = client.UploadResult(ctx, &pb.JobResponse{
		JobId:  j.ID.String(),
		Output: string(data),
	})
	if err != nil {
		span.RecordError(err)
		return err
	}
	return nil
}
