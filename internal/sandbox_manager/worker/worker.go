package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/ssuji15/wolf/internal/config"
	"github.com/ssuji15/wolf/internal/sandbox_manager/raven"
	"github.com/ssuji15/wolf/internal/sandbox_manager/raven/grpc/transport"
	"github.com/ssuji15/wolf/internal/util"
)

const WORKER_LISTEN_PORT string = "8989"
const WORKER_TRANSPORT_ENV string = "WORKER_TRANSPORT"

type JobType string

const (
	JobCodeExecution JobType = "codeExecution"
)

var validJobType = map[JobType]struct{}{
	JobCodeExecution: {},
}

func IsValidJobType(jt string) bool {
	_, ok := validJobType[JobType(jt)]
	return ok
}

type WorkerManagerType string

const (
	WorkerManagerDocker     WorkerManagerType = "docker"
	WorkerManagerContainerd WorkerManagerType = "containerd"
)

var validWorkerManager = map[WorkerManagerType]struct{}{
	WorkerManagerDocker:     {},
	WorkerManagerContainerd: {},
}

func IsValidWorker(w string) bool {
	_, ok := validWorkerManager[WorkerManagerType(w)]
	return ok
}

type RuntimeType string

const (
	RuntimeDockerCRun  RuntimeType = "runc"
	RuntimeDockerRunSc RuntimeType = "runsc"
	RuntimeCRun        RuntimeType = "io.containerd.runc.v2"
	RuntimeRunSc       RuntimeType = "io.containerd.runsc.v1"
)

var validRuntime = map[WorkerManagerType]map[RuntimeType]struct{}{
	WorkerManagerDocker: {
		RuntimeDockerCRun:  {},
		RuntimeDockerRunSc: {},
	},
	WorkerManagerContainerd: {
		RuntimeCRun:  {},
		RuntimeRunSc: {},
	},
}

func IsValidRuntime(w string, r string) bool {
	rt, ok := validRuntime[WorkerManagerType(w)]
	if !ok {
		return ok
	}
	_, ok = rt[RuntimeType(r)]
	return ok
}

var validRaven = map[raven.Type]struct{}{
	raven.RavenGRPC: {},
	raven.RavenFile: {},
}

func IsValidRaven(r string) bool {
	_, ok := validRaven[raven.Type(r)]
	return ok
}

var validTransportType = map[RuntimeType]map[transport.TransportType]struct{}{
	RuntimeDockerCRun: {
		transport.TCP: {},
		transport.UDS: {},
	},
	RuntimeDockerRunSc: {
		transport.TCP: {},
	},
	RuntimeCRun: {
		transport.TCP: {},
		transport.UDS: {},
	},
	RuntimeRunSc: {
		transport.TCP: {},
	},
}

func isValidTransportType(rt string, t string) bool {
	tt, ok := validTransportType[RuntimeType(rt)]
	if !ok {
		return false
	}
	_, ok = tt[transport.TransportType(t)]
	return ok
}

type WorkerManager interface {
	Launch(ctx context.Context, w *WorkerMetadata) (string, error)
	Wait(ctx context.Context, w *WorkerMetadata) (exitCode int64, err error)
	Destroy(ctx context.Context, id string) error
	IsHealthy(ctx context.Context, id string) (bool, error)
	GetIP(context.Context, string) (string, error)
}

type WorkerState string

const (
	WorkerStateCreating  WorkerState = "CREATING"
	WorkerStateReady     WorkerState = "READY"
	WorkerStateRunning   WorkerState = "RUNNING"
	WorkerStateExited    WorkerState = "EXITED"
	WorkerStateDestroyed WorkerState = "DESTROYED"
)

type Workspace struct {
	WorkDir string
}

type WorkerMetadata struct {
	ID                string
	Name              string
	Image             string
	CPUQuota          int64
	MemoryLimit       int64
	CreatedAt         time.Time
	UpdatedAt         time.Time
	Status            WorkerState
	JobID             string
	Workspace         Workspace
	Raven             raven.Raven
	Manager           WorkerManager
	JobType           JobType
	WorkerManagerType WorkerManagerType
	IP                string
	EnvVars           map[string]string
}

type WorkerOption struct {
	RavenType   raven.Type
	RavenOption raven.Option

	JobType JobType
	Image   string

	WorkerManagerType WorkerManagerType
	EnvVars           map[string]string
}

func (w *WorkerMetadata) Launch() error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(1*time.Second))
	defer cancel()
	id, err := w.Manager.Launch(ctx, w)
	if err != nil {
		return err
	}
	w.ID = id

	ip, err := w.Manager.GetIP(ctx, w.ID)
	if err != nil {
		return err
	}
	w.IP = ip

	return nil
}

func (w *WorkerMetadata) Wait(ctx context.Context) (exitCode int64, err error) {
	return w.Manager.Wait(ctx, w)
}

func (w *WorkerMetadata) Destroy(ctx context.Context) error {
	return w.Manager.Destroy(ctx, w.ID)
}

func (w *WorkerMetadata) IsHealthy(ctx context.Context) (bool, error) {
	return w.Manager.IsHealthy(ctx, w.ID)
}

func (w *WorkerMetadata) Validate(ctx context.Context) (bool, error) {
	h, err := w.IsHealthy(ctx)
	if err != nil || !h {
		return false, err
	}
	if w.ID == "" || w.Name == "" || w.Status != WorkerStateReady {
		return false, nil
	}
	return true, nil
}

func (w *WorkerMetadata) AddRaven(opt WorkerOption, path string) error {
	opt.RavenOption.GrpcTransportOption.Path = path
	r, err := raven.NewRaven(opt.RavenType, opt.RavenOption)
	if err != nil {
		return err
	}
	w.Raven = r
	return nil
}

func NewWorker(opt WorkerOption, wm WorkerManager, workDir string) (*WorkerMetadata, error) {
	n := uuid.New().String()
	var wd string
	wd = ""
	if opt.RavenOption.GrpcTransportOption.Type == transport.UDS {
		wd = fmt.Sprintf("%s/%s", workDir, n)
		err := util.EnsureDirExist(wd)
		if err != nil {
			return nil, err
		}
	}
	w := WorkerMetadata{
		Name:              n,
		Image:             opt.Image,
		CPUQuota:          100000,
		MemoryLimit:       512 * 1024 * 1024,
		CreatedAt:         time.Now(),
		Status:            WorkerStateCreating,
		Manager:           wm,
		JobType:           opt.JobType,
		WorkerManagerType: opt.WorkerManagerType,
		Workspace: Workspace{
			WorkDir: wd,
		},
		EnvVars: opt.EnvVars,
	}
	return &w, nil
}

func ValidateSandboxManagerConfig(c *config.SandboxManagerConfig) error {
	if len(c.Workers) == 0 {
		return fmt.Errorf("no workers defined")
	}
	seenTypes := make(map[string]struct{})
	for idx, wt := range c.Workers {
		if !IsValidJobType(wt.Type) {
			return fmt.Errorf("job type %s of workers[%d]: unsupported type", wt.Type, idx)
		}

		if _, ok := seenTypes[wt.Type]; ok {
			return fmt.Errorf("duplicate job type %q", wt.Type)
		}
		seenTypes[wt.Type] = struct{}{}

		if len(wt.WorkerGroups) == 0 {
			return fmt.Errorf("worker type %q has no workerGroups", wt.Type)
		}
		seenWorkerTypes := make(map[string]struct{})
		for i, wg := range wt.WorkerGroups {
			if _, ok := seenWorkerTypes[wg.Worker]; ok {
				return fmt.Errorf("duplicate worker type %q", wg.Worker)
			}
			if err := ValidateWorkerGroup(wt.Type, i, wg); err != nil {
				return err
			}
			seenWorkerTypes[wg.Worker] = struct{}{}
		}
	}
	return nil
}

func ValidateWorkerGroup(workerType string, index int, wg config.WorkerGroupConfig) error {
	if wg.Count <= 0 {
		return fmt.Errorf("worker type %q group[%d]: count must be > 0", workerType, index)
	}

	if !IsValidWorker(wg.Worker) {
		return fmt.Errorf("worker type %q group[%d]: unsupported worker %q", workerType, index, wg.Worker)
	}

	if !IsValidRuntime(wg.Worker, wg.Runtime) {
		return fmt.Errorf("worker type %q group[%d]: unsupported runtime %q", workerType, index, wg.Runtime)
	}

	if wg.Image == "" {
		return fmt.Errorf(
			"worker type %q group[%d]: image must not be empty",
			workerType, index,
		)
	}

	if !IsValidRaven(wg.Raven.Type) {
		return fmt.Errorf(
			"worker type %q group[%d]: unsupported raven type %s",
			wg.Runtime, index, wg.Raven.Type,
		)
	}

	if !isValidTransportType(wg.Runtime, wg.Raven.TransportConfig.Type) {
		return fmt.Errorf(
			"worker type %q group[%d] runtime %s: unsupported transport type %s",
			wg.Runtime, index, wg.Runtime, wg.Raven.TransportConfig.Type,
		)
	}

	return nil
}
