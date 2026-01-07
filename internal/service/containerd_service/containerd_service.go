package containerdservice

import (
	"context"
	"fmt"
	"strings"
	"syscall"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/containers"

	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/oci"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/ssuji15/wolf/internal/config"
	"github.com/ssuji15/wolf/model"
)

type ContainerdService struct {
	containerd *containerd.Client
	cfg        *config.SandboxManagerConfig
}

func NewContainerdService(cfg *config.SandboxManagerConfig) (*ContainerdService, error) {
	cc, err := NewContainerdClient()
	if err != nil {
		return nil, fmt.Errorf("Unable to initialise Containerd: %v", err)
	}
	return &ContainerdService{
		containerd: cc,
		cfg:        cfg,
	}, nil
}

func (d *ContainerdService) CreateContainer(ctx context.Context, opts model.CreateOptions, seccompprofile *specs.LinuxSeccomp) (model.WorkerMetadata, error) {
	client := d.containerd // containerd.Client

	image, err := client.GetImage(ctx, opts.Image)
	if err != nil {
		return model.WorkerMetadata{}, err
	}

	container, err := client.NewContainer(
		ctx,
		opts.Name,

		containerd.WithImage(image),
		containerd.WithSnapshotter("overlayfs"),
		containerd.WithNewSnapshot(opts.Name, image),
		containerd.WithRuntime("io.containerd.runc.v2", nil),
		containerd.WithNewSpec(
			oci.WithImageConfig(image),
			oci.WithProcessArgs("./worker"),
			oci.WithProcessCwd("/usr/local/bin"),
			oci.WithUser("0"),
			oci.WithCPUCFS(opts.CPUQuota, uint64(opts.CPUQuota)),
			//oci.WithCPUShares(uint64(opts.CPUQuota)),
			oci.WithMemoryLimit(uint64(opts.MemoryLimit)),
			oci.WithApparmorProfile(opts.AppArmorProfile),
			WithSeccompProfile(seccompprofile),
			oci.WithPidsLimit(10),
			oci.WithMounts([]specs.Mount{
				{
					Type:        "bind",
					Source:      opts.WorkDir,
					Destination: "/job",
					Options:     []string{"rbind", "rw"},
				},
				// {
				// 	Type:        "tmpfs",
				// 	Destination: "/tmp",
				// 	Options: []string{
				// 		"nosuid",
				// 		"nodev",
				// 		"size=256m",
				// 	},
				// },
			}),
		),
		containerd.WithAdditionalContainerLabels(opts.Labels),
	)

	if err != nil {
		return model.WorkerMetadata{}, err
	}

	// === Create task (similar to ctr run) ===
	task, err := container.NewTask(ctx, cio.NullIO)
	if err != nil {
		return model.WorkerMetadata{}, err
	}

	if err := task.Start(ctx); err != nil {
		return model.WorkerMetadata{}, err
	}

	meta := model.WorkerMetadata{
		ID:        container.ID(),
		Name:      opts.Name,
		Status:    "created",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	return meta, nil
}

func (d *ContainerdService) StopContainer(ctx context.Context, id string) error {
	container, err := d.containerd.LoadContainer(ctx, id)
	if err != nil {
		return err
	}
	return d.stopContainer(ctx, container)
}

func (d *ContainerdService) RemoveContainer(ctx context.Context, id string) error {

	container, err := d.containerd.LoadContainer(ctx, id)
	if err != nil {
		return err
	}

	err = d.stopContainer(ctx, container)
	if err != nil {
		return err
	}

	// Delete container (force)
	if err := container.Delete(ctx, containerd.WithSnapshotCleanup); err != nil {
		return err
	}

	return nil
}

func (c *ContainerdService) InspectContainer(ctx context.Context, id string) (*containers.Container, error) {
	container, err := c.containerd.LoadContainer(ctx, id)
	if err != nil {
		return nil, err
	}

	info, err := container.Info(ctx)
	if err != nil {
		return nil, err
	}

	return &info, nil
}

func (c *ContainerdService) GetTaskStatus(ctx context.Context, id string) (containerd.Status, error) {
	container, err := c.containerd.LoadContainer(ctx, id)
	if err != nil {
		return containerd.Status{}, err
	}

	task, err := container.Task(ctx, nil)
	if err != nil {
		return containerd.Status{}, err
	}
	return task.Status(ctx)
}

func (c *ContainerdService) ContainerWait(ctx context.Context, id string) (<-chan containerd.ExitStatus, error) {
	container, err := c.containerd.LoadContainer(ctx, id)
	if err != nil {
		return nil, err
	}

	task, err := container.Task(ctx, nil)
	if err != nil {
		return nil, err
	}

	return task.Wait(ctx)
}

func (c *ContainerdService) stopContainer(ctx context.Context, container containerd.Container) error {
	task, err := container.Task(ctx, nil)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return nil // container already stopped
		}
		return err
	}

	// Send SIGTERM and wait up to 3s
	if err := task.Kill(ctx, syscall.SIGTERM); err != nil {
		if errdefs.IsNotFound(err) ||
			strings.Contains(err.Error(), "process already finished") ||
			strings.Contains(err.Error(), "not found") {
			// Task already stopped — ignore
		} else {
			return err
		}
	}
	exitC, err := task.Wait(ctx)
	if err != nil {
		return err
	}
	var status containerd.ExitStatus
	select {
	case status = <-exitC:
	case <-time.After(time.Second * 3):
		status = *containerd.NewExitStatus(1, time.Now().UTC(), fmt.Errorf("could not kill task... timedout"))
	}

	_, _, err = status.Result()
	if err != nil {
		return err
	}

	// Delete task after stop
	_, err = task.Delete(ctx)
	if err != nil {
		return err
	}
	return nil
}

func WithSeccompProfile(sec *specs.LinuxSeccomp) oci.SpecOpts {
	return func(ctx context.Context, client oci.Client, c *containers.Container, s *specs.Spec) error {
		if s.Linux == nil {
			s.Linux = &specs.Linux{}
		}
		s.Linux.Seccomp = sec
		return nil
	}
}
