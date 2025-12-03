package manager

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/moby/moby/api/types/container"
	pb "github.com/ssuji15/wolf-worker/agent"
	"github.com/ssuji15/wolf/model"
	"google.golang.org/grpc"
)

func (m *SandboxManager) dispatchJob(id string) error {

	worker := m.getIdleWorker()

	if worker.ID == "" || worker.SocketPath == "" {
		return fmt.Errorf("invalid worker. please try again")
	}

	j, err := m.jobService.GetJob(context.Background(), id)
	if err != nil {
		fmt.Printf("Error retrieving job: %s, err: %v\n", id, err)
	}

	err = m.launcher.DispatchJob(worker.SocketPath, j)
	if err != nil {
		fmt.Printf("Error sending job: %s, err: %v\n", id, err)
	}
	timeout := 10 * time.Second
	res := m.launcher.ContainerWait(m.ctx, worker.ID, container.WaitConditionNotRunning)
	select {
	case err := <-res.Error:
		return err
	case status := <-res.Result:
		fmt.Printf("worker: %s with job: %s exited with status code: %d\n", worker.ID, j.ID, status.StatusCode)
		// Add retry mechanism when status code is not 0. Or handle it as necessary
		m.sendResult(j, worker)
		m.shutdownWorker(worker)
		m.LaunchReplacement()
		return nil
	case <-time.After(timeout):
		fmt.Printf("killing worker: %s with job: %s, executing for more than 10 seconds\n", worker.ID, j.ID)
		m.shutdownWorker(worker)
		m.LaunchReplacement()
		return fmt.Errorf("killing worker: %s with job: %s, executing for more than 10 seconds", worker.ID, j.ID)
	}
}

func (m *SandboxManager) sendResult(j *model.Job, w model.WorkerMetadata) error {

	data, err := os.ReadFile(w.OutputPath)
	if err != nil {
		return err
	}

	conn, err := grpc.Dial(m.backendCallback, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		return err
	}

	defer conn.Close()

	client := pb.NewWorkerAgentClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = client.UploadResult(ctx, &pb.JobResponse{
		JobId:  j.ID.String(),
		Output: string(data),
	})
	if err != nil {
		return err
	}
	return nil
}
