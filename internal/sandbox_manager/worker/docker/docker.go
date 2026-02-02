package docker

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/ssuji15/wolf/internal/sandbox_manager/worker"
	dockerservice "github.com/ssuji15/wolf/internal/service/docker_service"
	"github.com/ssuji15/wolf/model"
)

type DockerManager struct {
	dockerservice   *dockerservice.DockerService
	seccompprofile  string
	apparmorProfile string
	runtime         string
}

func NewDockerWorker(secompprofile string, apparmorProfile string, runtime string) (*DockerManager, error) {
	ds, err := dockerservice.NewDockerService()
	if err != nil {
		return nil, err
	}

	var sec []byte
	if secompprofile != "" {
		sec, err = os.ReadFile(secompprofile)
		if err != nil {
			return nil, err
		}
	}

	d := &DockerManager{
		dockerservice:   ds,
		seccompprofile:  string(sec),
		apparmorProfile: apparmorProfile,
		runtime:         runtime,
	}
	return d, nil
}

func (d *DockerManager) Launch(ctx context.Context, w *worker.WorkerMetadata) (string, error) {
	opt := model.CreateOptions{
		Name:            w.Name,
		Image:           w.Image,
		AppArmorProfile: d.apparmorProfile,
		CPUQuota:        w.CPUQuota,
		MemoryLimit:     w.MemoryLimit,
		Runtime:         d.runtime,
		WorkDir:         w.Workspace.WorkDir,
		EnvVars:         w.EnvVars,
	}
	c, err := d.dockerservice.CreateContainer(ctx, opt, d.seccompprofile)
	if err != nil {
		return "", err
	}
	return c, nil
}

func (d *DockerManager) Destroy(ctx context.Context, workerID string) error {
	_, err := d.dockerservice.StopContainer(ctx, workerID)
	if err == nil {
		d.dockerservice.RemoveContainer(ctx, workerID)
	}
	return err
}

func (d *DockerManager) IsHealthy(ctx context.Context, workerID string) (bool, error) {
	ic, err := d.dockerservice.InspectContainer(ctx, workerID)
	if err != nil {
		return false, err
	}
	return ic.Container.State.Status == container.StateRunning, nil
}

func (d *DockerManager) Wait(ctx context.Context, w *worker.WorkerMetadata) (int64, error) {
	res := d.dockerservice.ContainerWait(ctx, w.ID, container.WaitConditionNotRunning)
	timeout := 10 * time.Second
	select {
	case err := <-res.Error:
		return 0, err
	case status := <-res.Result:
		return status.StatusCode, nil
	case <-time.After(timeout):
		d.Destroy(ctx, w.ID)
		err := fmt.Errorf("killing worker: %s, executing for more than 2 seconds", w.ID)
		return 0, err
	}
}

func (d *DockerManager) GetIP(ctx context.Context, workerId string) (string, error) {
	return d.dockerservice.GetIP(ctx, workerId)
}
