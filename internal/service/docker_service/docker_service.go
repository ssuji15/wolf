package dockerservice

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/ssuji15/wolf/internal/config"
	"github.com/ssuji15/wolf/internal/util"
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

type CreateOptions struct {
	Name            string
	Image           string
	SeccompProfile  string
	AppArmorProfile string
	CPUQuota        int64
	MemoryLimit     int64
	Labels          map[string]string
}

func (d *DockerService) CreateContainer(ctx context.Context, opts CreateOptions) (model.WorkerMetadata, error) {

	// Pull image if missing
	// resp, err := d.docker.ImagePull(ctx, opts.Image, client.ImagePullOptions{})
	// if err != nil {
	// 	return "", fmt.Errorf("image pull: %w", err)
	// }

	// err = resp.Wait(ctx)
	// if err != nil {
	// 	return "", fmt.Errorf("image pull: %w", err)
	// }

	// Create UDS path
	rootDir := fmt.Sprintf("%s/%s", d.cfg.SocketDir, opts.Name)
	udsPath := fmt.Sprintf("%s/%s/socket/socket.sock", d.cfg.SocketDir, opts.Name)
	if err := util.VerifyPath(udsPath); err != nil {
		return model.WorkerMetadata{}, err
	}

	outputPath := fmt.Sprintf("%s/%s/output/output.log", d.cfg.SocketDir, opts.Name)
	if err := util.VerifyPath(outputPath); err != nil {
		return model.WorkerMetadata{}, err
	}

	hostCfg := &container.HostConfig{
		NetworkMode: network.NetworkNone,
		SecurityOpt: []string{
			"seccomp=" + opts.SeccompProfile,
			"apparmor=" + opts.AppArmorProfile,
		},
		Resources: container.Resources{
			CPUQuota: opts.CPUQuota,
			Memory:   opts.MemoryLimit,
		},
		Mounts: []mount.Mount{
			{
				Type:   mount.TypeBind,
				Source: rootDir,
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
		ID:         created.ID,
		Name:       opts.Name,
		WorkDir:    rootDir,
		SocketPath: udsPath,
		OutputPath: outputPath,
		Status:     "created",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
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
