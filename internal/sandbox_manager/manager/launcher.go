package manager

import (
	"context"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/ssuji15/wolf/model"
)

type WorkerLauncher interface {
	LaunchWorker(context.Context) (model.WorkerMetadata, error)
	DestroyWorker(context.Context, string) error
	IsContainerHealthy(context.Context, string) bool
	DispatchJob(string, *model.Job) error
	ContainerWait(context.Context, string, container.WaitCondition) client.ContainerWaitResult
}
