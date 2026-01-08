package docker_launcher

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/ssuji15/wolf/internal/config"
	dockerservice "github.com/ssuji15/wolf/internal/service/docker_service"
	"github.com/ssuji15/wolf/internal/util"
	"github.com/ssuji15/wolf/model"
)

type DockerLauncher struct {
	dockerservice  *dockerservice.DockerService
	seccompprofile string
}

func NewDockerLauncher(cfg *config.SandboxManagerConfig) (*DockerLauncher, error) {
	ds, err := dockerservice.NewDockerService(cfg)
	if err != nil {
		return nil, err
	}
	d := &DockerLauncher{
		dockerservice: ds,
	}
	return d, nil
}

func (d *DockerLauncher) LaunchWorker(ctx context.Context, opt model.CreateOptions) (model.WorkerMetadata, error) {

	c, err := d.dockerservice.CreateContainer(ctx, opt, d.seccompprofile)
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

func (d *DockerLauncher) DispatchJob(socketPath string, job *model.Job, code []byte) error {
	return util.DispatchJob(socketPath, job, code)
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

func (c *DockerLauncher) SetSecCompProfile(profile string) error {
	sec, err := os.ReadFile(profile)
	if err != nil {
		return err
	}
	c.seccompprofile = string(sec)
	return err
}
