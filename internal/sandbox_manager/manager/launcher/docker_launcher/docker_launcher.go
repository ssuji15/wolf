package docker_launcher

import (
	"context"
	"net"

	"github.com/google/uuid"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	pb "github.com/ssuji15/wolf-worker/agent"
	"github.com/ssuji15/wolf/internal/config"
	"github.com/ssuji15/wolf/internal/db"
	dockerservice "github.com/ssuji15/wolf/internal/service/docker_service"
	"github.com/ssuji15/wolf/model"
	"google.golang.org/grpc"
)

type DockerLauncher struct {
	dockerservice *dockerservice.DockerService
}

func NewDockerLauncher(dbClient *db.DB, cfg *config.Config) *DockerLauncher {
	d := &DockerLauncher{
		dockerservice: dockerservice.NewDockerService(cfg),
	}
	return d
}

func (d *DockerLauncher) LaunchWorker(ctx context.Context) (model.WorkerMetadata, error) {
	c, err := d.dockerservice.CreateContainer(ctx, dockerservice.CreateOptions{
		Name:        uuid.New().String(),
		Image:       "worker",
		CPUQuota:    100000,
		MemoryLimit: 512 * 1024 * 1024,
		Labels: map[string]string{
			"id": "worker",
		},
	})

	if err != nil {
		return model.WorkerMetadata{}, err
	}

	return c, nil
}

func (d *DockerLauncher) DestroyWorker(ctx context.Context, workerID string) error {
	_, err := d.dockerservice.StopContainer(ctx, workerID)
	if err == nil {
		d.dockerservice.RemoveContainer(ctx, workerID)
	}
	return err
}

func (d *DockerLauncher) IsContainerHealthy(ctx context.Context, workerID string) bool {
	ic, err := d.dockerservice.InspectContainer(ctx, workerID)
	if err != nil {
		return false
	}
	return ic.Container.State.Status == container.StateRunning
}

func (d *DockerLauncher) DispatchJob(socketPath string, job *model.Job) error {
	dialer := func(ctx context.Context, addr string) (net.Conn, error) {
		return net.Dial("unix", socketPath)
	}

	conn, err := grpc.Dial(
		"unix://"+socketPath,
		grpc.WithInsecure(),
		grpc.WithContextDialer(dialer),
	)
	if err != nil {
		return err
	}
	defer conn.Close()

	client := pb.NewWorkerAgentClient(conn)

	_, err = client.StartJob(context.Background(), &pb.JobRequest{
		Engine: job.ExecutionEngine,
		Code:   job.Code,
	})

	return err
}

func (d *DockerLauncher) ContainerWait(ctx context.Context, id string, cond container.WaitCondition) client.ContainerWaitResult {
	return d.dockerservice.ContainerWait(ctx, id, cond)
}
