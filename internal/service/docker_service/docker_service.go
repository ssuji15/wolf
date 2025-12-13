package dockerservice

import (
	"context"
	"fmt"
	"log"
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
	cfg    *config.Config
}

func NewDockerService(cfg *config.Config) *DockerService {
	dc, err := NewDockerClient()
	if err != nil {
		log.Fatalf("Unable to initialise Docker")
	}
	return &DockerService{
		docker: dc,
		cfg:    cfg,
	}
}

func (d *DockerService) CreateContainer(ctx context.Context, opts model.CreateOptions) (model.WorkerMetadata, error) {

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
			//"seccomp=" + opts.SecCompProfileString,
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
		return model.WorkerMetadata{}, fmt.Errorf("container create: %w", err)
	}

	if _, err := d.docker.ContainerStart(ctx, created.ID, client.ContainerStartOptions{}); err != nil {
		return model.WorkerMetadata{}, fmt.Errorf("container start: %w", err)
	}

	meta := model.WorkerMetadata{
		ID:        created.ID,
		Name:      opts.Name,
		Status:    "created",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	return meta, nil
}

func (d *DockerService) StopContainer(ctx context.Context, id string) (client.ContainerStopResult, error) {
	return d.docker.ContainerStop(ctx, id, client.ContainerStopOptions{})
}

func (d *DockerService) RestartContainer(ctx context.Context, id string) (client.ContainerRestartResult, error) {
	return d.docker.ContainerRestart(ctx, id, client.ContainerRestartOptions{})
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
