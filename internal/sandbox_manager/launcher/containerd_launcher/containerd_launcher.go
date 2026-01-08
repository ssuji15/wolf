package containerdlauncher

import (
	"context"
	"fmt"
	"time"

	"github.com/containerd/containerd"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/ssuji15/wolf/internal/config"
	containerdservice "github.com/ssuji15/wolf/internal/service/containerd_service"
	"github.com/ssuji15/wolf/internal/util"
	"github.com/ssuji15/wolf/model"
)

type ContainerdLauncher struct {
	containerdService *containerdservice.ContainerdService
	seccompprofile    *specs.LinuxSeccomp
}

func NewContainerdLauncher(cfg *config.SandboxManagerConfig) (*ContainerdLauncher, error) {
	cs, err := containerdservice.NewContainerdService(cfg)
	if err != nil {
		return nil, err
	}
	d := &ContainerdLauncher{
		containerdService: cs,
	}
	return d, nil
}

func (c *ContainerdLauncher) LaunchWorker(ctx context.Context, opt model.CreateOptions) (model.WorkerMetadata, error) {
	con, err := c.containerdService.CreateContainer(ctx, opt, c.seccompprofile)
	if err != nil {
		return model.WorkerMetadata{}, err
	}
	return con, nil
}

func (c *ContainerdLauncher) DestroyWorker(ctx context.Context, workerID string) error {
	return c.containerdService.RemoveContainer(ctx, workerID)
}

func (c *ContainerdLauncher) IsContainerHealthy(ctx context.Context, workerID string) bool {
	ts, err := c.containerdService.GetTaskStatus(ctx, workerID)
	if err != nil {
		return false
	}
	return ts.Status == containerd.Running
}

func (c *ContainerdLauncher) DispatchJob(socketPath string, job *model.Job, code []byte) error {
	return util.DispatchJob(socketPath, job, code)
}

func (c *ContainerdLauncher) ContainerWaitTillExit(ctx context.Context, id string) (int64, error) {
	ch, err := c.containerdService.ContainerWait(ctx, id)
	if err != nil {
		return 0, err
	}
	timeout := 10 * time.Second
	select {
	case status := <-ch:
		return int64(status.ExitCode()), nil
	case <-time.After(timeout):
		c.DestroyWorker(ctx, id)
		err := fmt.Errorf("killing worker: %s, executing for more than 10 seconds", id)
		return 0, err
	}
}

func (c *ContainerdLauncher) SetSecCompProfile(profile string) error {
	sec, err := util.LoadSeccomp(profile)
	c.seccompprofile = sec
	return err
}
