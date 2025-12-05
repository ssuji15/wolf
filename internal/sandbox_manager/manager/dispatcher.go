package manager

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/moby/moby/api/types/container"
	pb "github.com/ssuji15/wolf-worker/agent"
	"github.com/ssuji15/wolf/internal/job_tracer"
	"github.com/ssuji15/wolf/model"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
)

func (m *SandboxManager) dispatchJob(ctx context.Context, id string, worker model.WorkerMetadata) error {
	tracer := job_tracer.GetTracer()
	ctx, span := tracer.Start(ctx, "ExecuteCode")
	defer span.End()

	if worker.ID == "" || worker.SocketPath == "" {
		err := fmt.Errorf("invalid worker. please try again")
		span.RecordError(err)
		return err
	}

	j, err := m.jobService.GetJob(ctx, id)
	if err != nil {
		span.RecordError(err)
		fmt.Printf("Error retrieving job: %s, err: %v\n", id, err)
		return err
	}

	err = m.launcher.DispatchJob(worker.SocketPath, j)
	if err != nil {
		span.RecordError(err)
		fmt.Printf("Error sending job: %s, err: %v\n", id, err)
		return err
	}
	timeout := 10 * time.Second
	res := m.launcher.ContainerWait(ctx, worker.ID, container.WaitConditionNotRunning)
	select {
	case err := <-res.Error:
		span.RecordError(err)
		return err
	case status := <-res.Result:
		fmt.Printf("worker: %s with job: %s exited with status code: %d\n", worker.ID, j.ID, status.StatusCode)
		// Add retry mechanism when status code is not 0. Or handle it as necessary
		m.sendResult(ctx, j, worker)
		m.shutdownWorker(worker)
		return nil
	case <-time.After(timeout):
		fmt.Printf("killing worker: %s with job: %s, executing for more than 10 seconds\n", worker.ID, j.ID)
		m.shutdownWorker(worker)
		err := fmt.Errorf("killing worker: %s with job: %s, executing for more than 10 seconds", worker.ID, j.ID)
		span.RecordError(err)
		return err
	}
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
		m.backendCallback,
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
