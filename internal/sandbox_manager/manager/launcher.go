package manager

import (
	"context"

	"github.com/ssuji15/wolf/model"
)

type WorkerLauncher interface {
	LaunchWorker(context.Context, model.CreateOptions) (model.WorkerMetadata, error)
	DestroyWorker(context.Context, string) error
	IsContainerHealthy(context.Context, string) bool
	DispatchJob(string, *model.Job, []byte) error
	ContainerWaitTillExit(context.Context, string) (int64, error)
}
