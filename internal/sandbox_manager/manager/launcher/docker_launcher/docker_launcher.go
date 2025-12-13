package docker_launcher

import (
	"context"
	"fmt"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/ssuji15/wolf/internal/config"
	dockerservice "github.com/ssuji15/wolf/internal/service/docker_service"
	"github.com/ssuji15/wolf/internal/util"
	"github.com/ssuji15/wolf/model"
)

type DockerLauncher struct {
	dockerservice *dockerservice.DockerService
	cfg           *config.Config
}

func NewDockerLauncher(cfg *config.Config) *DockerLauncher {
	d := &DockerLauncher{
		dockerservice: dockerservice.NewDockerService(cfg),
		cfg:           cfg,
	}
	return d
}

func (d *DockerLauncher) LaunchWorker(ctx context.Context, opt model.CreateOptions) (model.WorkerMetadata, error) {

	c, err := d.dockerservice.CreateContainer(ctx, opt)
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
	return util.DispatchJob(socketPath, job)
}

func (d *DockerLauncher) ContainerWaitTillExit(ctx context.Context, id string) (int64, error) {
	res := d.dockerservice.ContainerWait(ctx, id, container.WaitConditionNotRunning)
	timeout := 10 * time.Second
	select {
	case err := <-res.Error:
		return 0, err
	case status := <-res.Result:
		return status.StatusCode, nil
	case <-time.After(timeout):
		d.DestroyWorker(ctx, id)
		err := fmt.Errorf("killing worker: %s, executing for more than 10 seconds", id)
		return 0, err
	}
}
