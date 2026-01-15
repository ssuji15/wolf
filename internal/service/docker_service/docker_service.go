package dockerservice

import (
	"context"
	"fmt"
	"time"

	"github.com/ssuji15/wolf/internal/config"
	"github.com/ssuji15/wolf/model"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
)

type DockerService struct {
	docker *client.Client
	cfg    *config.SandboxManagerConfig
}

func NewDockerService(cfg *config.SandboxManagerConfig) (*DockerService, error) {
	dc, err := NewDockerClient()
	if err != nil {
		return nil, fmt.Errorf("unable to initialise docker")
	}
	return &DockerService{
		docker: dc,
		cfg:    cfg,
	}, nil
}

func (d *DockerService) CreateContainer(ctx context.Context, opts model.CreateOptions, secompprofile string) (model.WorkerMetadata, error) {

	// Pull image if missing
	// resp, err := d.docker.ImagePull(ctx, opts.Image, client.ImagePullOptions{})
	// if err != nil {
	// 	return "", fmt.Errorf("image pull: %w", err)
	// }

	// err = resp.Wait(ctx)
	// if err != nil {
	// 	return "", fmt.Errorf("image pull: %w", err)
	// }

	hostCfg := &container.HostConfig{
		NetworkMode: network.NetworkNone,
		SecurityOpt: []string{
			//"seccomp=" + secompprofile,
			"apparmor=" + opts.AppArmorProfile,
		},
		Resources: container.Resources{
			CPUPeriod: opts.CPUQuota,
			CPUQuota:  opts.CPUQuota,
			Memory:    opts.MemoryLimit,
		},
		Mounts: []mount.Mount{
			{
				Type:   mount.TypeBind,
				Source: opts.WorkDir,
				Target: "/job",
			},
		},
	}
	cfg := &container.Config{
		Image:  opts.Image,
		Labels: opts.Labels,
	}
	networkCfg := &network.NetworkingConfig{}

	created, err := d.docker.ContainerCreate(ctx, client.ContainerCreateOptions{
		Config:           cfg,
		HostConfig:       hostCfg,
		NetworkingConfig: networkCfg,
		Name:             opts.Name,
	})
	if err != nil {
		return model.WorkerMetadata{}, err
	}

	if _, err := d.docker.ContainerStart(ctx, created.ID, client.ContainerStartOptions{}); err != nil {
		d.RemoveContainer(ctx, created.ID)
		return model.WorkerMetadata{}, err
	}

	meta := model.WorkerMetadata{
		ID:        created.ID,
		Name:      opts.Name,
		Status:    "created",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	return meta, nil
}

func (d *DockerService) StopContainer(ctx context.Context, id string) (client.ContainerStopResult, error) {
	timeout := 0
	return d.docker.ContainerStop(ctx, id, client.ContainerStopOptions{Timeout: &timeout})
}

func (d *DockerService) RestartContainer(ctx context.Context, id string) (client.ContainerRestartResult, error) {
	timeout := 0
	return d.docker.ContainerRestart(ctx, id, client.ContainerRestartOptions{Timeout: &timeout})
}

func (d *DockerService) RemoveContainer(ctx context.Context, id string) (client.ContainerRemoveResult, error) {
	return d.docker.ContainerRemove(ctx, id, client.ContainerRemoveOptions{
		Force: true,
	})
}

func (d *DockerService) InspectContainer(ctx context.Context, id string) (client.ContainerInspectResult, error) {
	return d.docker.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
}

func (d *DockerService) ContainerWait(ctx context.Context, id string, cond container.WaitCondition) client.ContainerWaitResult {
	return d.docker.ContainerWait(ctx, id, client.ContainerWaitOptions{
		Condition: cond,
	})
}
