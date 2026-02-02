package containerd

import (
	"context"
	"fmt"
	"time"

	"github.com/containerd/containerd"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/ssuji15/wolf/internal/sandbox_manager/worker"
	containerdservice "github.com/ssuji15/wolf/internal/service/containerd_service"
	"github.com/ssuji15/wolf/internal/util"
	"github.com/ssuji15/wolf/model"
)

type ContainerdManager struct {
	containerdService *containerdservice.ContainerdService
	seccompprofile    *specs.LinuxSeccomp
	apparmorProfile   string
	runtime           string
}

func NewContainerdWorker(secompprofile string, apparmorProfile string, runtime string) (*ContainerdManager, error) {
	cs, err := containerdservice.NewContainerdService()
	if err != nil {
		return nil, err
	}
	var sec *specs.LinuxSeccomp
	if secompprofile != "" {
		sec, err = util.LoadSeccomp(secompprofile)
	}

	d := &ContainerdManager{
		containerdService: cs,
		seccompprofile:    sec,
		apparmorProfile:   apparmorProfile,
		runtime:           runtime,
	}
	return d, nil
}

func (c *ContainerdManager) Launch(ctx context.Context, w *worker.WorkerMetadata) (string, error) {
	opt := model.CreateOptions{
		Name:            w.Name,
		Image:           w.Image,
		AppArmorProfile: c.apparmorProfile,
		CPUQuota:        w.CPUQuota,
		MemoryLimit:     w.MemoryLimit,
		Runtime:         c.runtime,
		WorkDir:         w.Workspace.WorkDir,
		EnvVars:         w.EnvVars,
	}

	id, err := c.containerdService.CreateContainer(ctx, opt, c.seccompprofile)
	if err != nil {
		return "", err
	}
	return id, nil
}

func (c *ContainerdManager) Destroy(ctx context.Context, workerID string) error {
	return c.containerdService.RemoveContainer(ctx, workerID)
}

func (c *ContainerdManager) IsHealthy(ctx context.Context, workerID string) (bool, error) {
	ts, err := c.containerdService.GetTaskStatus(ctx, workerID)
	if err != nil {
		return false, err
	}
	return ts.Status == containerd.Running, nil
}

func (c *ContainerdManager) Wait(ctx context.Context, w *worker.WorkerMetadata) (int64, error) {
	ch, err := c.containerdService.ContainerWait(ctx, w.ID)
	if err != nil {
		return 1, err
	}
	timeout := 10 * time.Second
	select {
	case status := <-ch:
		return int64(status.ExitCode()), nil
	case <-time.After(timeout):
		c.Destroy(ctx, w.ID)
		err := fmt.Errorf("killing worker: %s, executing for more than 2 seconds", w.ID)
		return 0, err
	}
}

func (c *ContainerdManager) GetIP(ctx context.Context, workerId string) (string, error) {
	return "", nil
}
