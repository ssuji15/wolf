package dockerservice

import (
	"context"
	"fmt"

	"github.com/ssuji15/wolf/model"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
)

type DockerService struct {
	docker *client.Client
}

func NewDockerService() (*DockerService, error) {
	dc, err := NewDockerClient()
	if err != nil {
		return nil, fmt.Errorf("unable to initialise docker")
	}
	return &DockerService{
		docker: dc,
	}, nil
}

func (d *DockerService) CreateContainer(ctx context.Context, opts model.CreateOptions, secompprofile string) (string, error) {
	// Pull image if missing
	// resp, err := d.docker.ImagePull(ctx, opts.Image, client.ImagePullOptions{})
	// if err != nil {
	// 	return "", fmt.Errorf("image pull: %w", err)
	// }

	// err = resp.Wait(ctx)
	// if err != nil {
	// 	return "", fmt.Errorf("image pull: %w", err)
	// }

	networkMode := network.NetworkDefault
	var mounts []mount.Mount
	if opts.WorkDir != "" {
		networkMode = network.NetworkNone
		mounts = []mount.Mount{
			{
				Type:   mount.TypeBind,
				Source: opts.WorkDir,
				Target: "/job",
			},
		}
	}

	env := make([]string, 0, len(opts.EnvVars))
	for k, v := range opts.EnvVars {
		env = append(env, k+"="+v)
	}

	pl := int64(32)
	hostCfg := &container.HostConfig{
		Runtime:     opts.Runtime,
		NetworkMode: container.NetworkMode(networkMode),
		Resources: container.Resources{
			CPUPeriod: 100000,
			CPUQuota:  opts.CPUQuota,
			Memory:    opts.MemoryLimit,
			PidsLimit: &pl,
		},
		Tmpfs: map[string]string{
			"/tmp":     "rw,exec,nosuid,mode=0777,size=67108864",
			"/var/tmp": "rw,exec,nosuid,mode=0777,size=67108864",
		},
		Mounts: mounts,
	}
	cfg := &container.Config{
		Image:      opts.Image,
		Labels:     opts.Labels,
		User:       "1000:1000",
		Cmd:        []string{"./worker"},
		WorkingDir: "/usr/local/bin",
		Env:        env,
	}
	networkCfg := &network.NetworkingConfig{}

	created, err := d.docker.ContainerCreate(ctx, client.ContainerCreateOptions{
		Config:           cfg,
		HostConfig:       hostCfg,
		NetworkingConfig: networkCfg,
		Name:             opts.Name,
	})
	if err != nil {
		return "", err
	}

	if _, err := d.docker.ContainerStart(ctx, created.ID, client.ContainerStartOptions{}); err != nil {
		d.RemoveContainer(ctx, created.ID)
		return "", err
	}
	return created.ID, nil
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

func (d *DockerService) GetIP(ctx context.Context, id string) (string, error) {
	inspect, err := d.docker.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
	if err != nil {
		return "", err
	}

	for _, endpoint := range inspect.Container.NetworkSettings.Networks {
		ipAddress := endpoint.IPAddress.String()
		return ipAddress, nil
	}
	return "", nil
}
