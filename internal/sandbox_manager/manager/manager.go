package manager

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/ssuji15/wolf/internal/component"
	"github.com/ssuji15/wolf/internal/config"
	"github.com/ssuji15/wolf/internal/job_tracer"
	"github.com/ssuji15/wolf/internal/queue"
	containerdlauncher "github.com/ssuji15/wolf/internal/sandbox_manager/manager/launcher/containerd_launcher"
	"github.com/ssuji15/wolf/internal/sandbox_manager/manager/launcher/docker_launcher"
	jobservice "github.com/ssuji15/wolf/internal/service/job_service"
	"github.com/ssuji15/wolf/internal/service/logger"
	"github.com/ssuji15/wolf/internal/util"
	"github.com/ssuji15/wolf/model"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
)

type SandboxManager struct {
	ctx                  context.Context
	launcher             WorkerLauncher
	workers              chan model.WorkerMetadata
	qClient              queue.Queue
	jobService           *jobservice.JobService
	subscription         *nats.Subscription
	wg                   *sync.WaitGroup
	secCompProfile       *specs.LinuxSeccomp
	secCompProfileString string
	cfg                  *config.Config
}

func NewLauncher(cfg *config.Config) WorkerLauncher {
	switch cfg.LauncherType {
	case "docker":
		return docker_launcher.NewDockerLauncher(cfg)
	default:
		return containerdlauncher.NewContainerdLauncher(cfg)
	}
}

func NewSandboxManager(ctx context.Context, comp *component.Components) (*SandboxManager, error) {

	launcher := NewLauncher(comp.Cfg)
	sub, err := comp.QClient.SubscribeEventToWorker(queue.JobCreated)
	if err != nil {
		return nil, err
	}
	sec, err := util.LoadSeccomp(comp.Cfg.SeccompProfile)
	if err != nil {
		return nil, err
	}
	secString, err := os.ReadFile(comp.Cfg.SeccompProfile)
	if err != nil {
		return nil, err
	}
	m := &SandboxManager{
		ctx:                  ctx,
		launcher:             launcher,
		workers:              make(chan model.WorkerMetadata, comp.Cfg.MaxWorker),
		qClient:              comp.QClient,
		jobService:           jobservice.NewJobService(comp.DBClient, comp.StorageClient, comp.QClient, comp.LocalCache),
		subscription:         sub,
		wg:                   &sync.WaitGroup{},
		secCompProfile:       sec,
		secCompProfileString: string(secString),
		cfg:                  comp.Cfg,
	}
	m.initializePool()
	go m.processRequests()
	return m, nil
}

func (m *SandboxManager) initializePool() {
	for i := 0; i < m.cfg.MaxWorker; i++ {
		m.LaunchWorker()
	}
}

func (m *SandboxManager) LaunchWorker() {
	if err := m.ctx.Err(); err != nil {
		return
	}

	tracer := job_tracer.GetTracer()
	ctx, span := tracer.Start(m.ctx, "Create container")
	defer span.End()

	opt := m.GetWorkerOption()
	udsPath := fmt.Sprintf("%s/%s/socket/socket.sock", m.cfg.SocketDir, opt.Name)
	if err := util.VerifyFileDoesNotExist(udsPath); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return
	}

	outputPath := fmt.Sprintf("%s/%s/output/output.log", m.cfg.SocketDir, opt.Name)
	if err := util.VerifyFileDoesNotExist(outputPath); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return
	}

	c, err := m.launcher.LaunchWorker(ctx, opt)
	c.SocketPath = udsPath
	c.OutputPath = outputPath
	c.WorkDir = opt.WorkDir

	if err != nil {
		err := fmt.Errorf("worker launch failed: %v", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return
	}
	span.SetAttributes(attribute.String("container_id", c.ID))
	go func() {
		time.Sleep(10 * time.Millisecond)
		m.AddWorkerToPool(c)
	}()
}

func (m *SandboxManager) AddWorkerToPool(c model.WorkerMetadata) {
	m.workers <- c
}

func (m *SandboxManager) getIdleWorker() model.WorkerMetadata {
	for w := range m.workers {
		if m.launcher.IsContainerHealthy(m.ctx, w.ID) {
			return w
		}
		go func() {
			m.shutdownWorker(w)
		}()
	}
	return model.WorkerMetadata{}
}

func (m *SandboxManager) shutdownWorker(w model.WorkerMetadata) {
	err := m.launcher.DestroyWorker(context.Background(), w.ID)
	if err != nil {
		logger.Log.Error().Err(err).Str("workerID", w.ID).Msg("could not delete worker")
	}
	go m.cleanWorkerSpace(w)
	m.LaunchWorker()
}

func (m *SandboxManager) shutdownAllWorkers() {
	for {
		select {
		case w := <-m.workers:
			m.shutdownWorker(w)
		default:
			close(m.workers)
			return
		}
	}
}

func (m *SandboxManager) cleanWorkerSpace(w model.WorkerMetadata) {
	os.RemoveAll(w.WorkDir)
}

func (m *SandboxManager) processRequests() {
	meter := otel.Meter("sandboxmanager")
	latency, _ := meter.Float64Histogram("job_queue_duration_seconds")
	for {
		select {
		case <-m.ctx.Done():
			m.subscription.Drain()
			time.Sleep(5 * time.Second)
			m.shutdownAllWorkers()
			comp := component.GetComponent()
			comp.DBClient.Close()
			comp.QClient.Shutdown()
			m.wg.Done()
			return
		default:
			worker := m.getIdleWorker()
			msgs, err := m.subscription.Fetch(1, nats.MaxWait(30*time.Second))
			if err != nil {
				m.AddWorkerToPool(worker)
				if errors.Is(err, nats.ErrTimeout) {
					continue
				}
				time.Sleep(time.Second)
				continue
			}

			msg := msgs[0]
			meta, _ := msg.Metadata()

			d := time.Since(meta.Timestamp)
			latency.Record(context.Background(), d.Seconds())

			headers := propagation.MapCarrier(natsHeaderToMapStringString(msg.Header))
			parentCtx := otel.GetTextMapPropagator().Extract(
				context.Background(),
				headers,
			)
			id := string(msg.Data)
			go func() {
				if err := m.dispatchJob(parentCtx, id, worker); err != nil {
					logger.Log.Error().Err(err).Str("id", id).Msg("failed to execute job")
					if meta.NumDelivered == uint64(queue.MaxDeliver) {
						logger.Log.Error().Err(fmt.Errorf("max delivery reached for job")).Str("id", string(msg.Data)).Msg("sending job to DLQ")
						m.qClient.PublishEvent(parentCtx, queue.DeadLetterQueue, string(msg.Data))
						msg.Term()
					}
					return
				}
				logger.Log.Info().Str("id", id).Msg("job processed successfully")
				msg.Ack()
			}()
		}
	}
}

func natsHeaderToMapStringString(h nats.Header) map[string]string {
	result := make(map[string]string)

	for key, values := range h {
		// We only take the first value encountered for that key.
		if len(values) > 0 {
			result[key] = values[0]
		}
	}
	return result
}

func (m *SandboxManager) Addwg() {
	m.wg.Add(1)
}

func (m *SandboxManager) Waitwg() {
	m.wg.Wait()
}

func (m *SandboxManager) GetWorkerOption() model.CreateOptions {
	n := uuid.New().String()
	return model.CreateOptions{
		Name:        n,
		Image:       "docker.io/library/worker:latest",
		CPUQuota:    100000,
		MemoryLimit: 512 * 1024 * 1024,
		Labels: map[string]string{
			"id": "worker",
		},
		AppArmorProfile:      m.cfg.AppArmorProfile,
		SeccompProfile:       m.secCompProfile,
		SecCompProfileString: m.secCompProfileString,
		WorkDir:              fmt.Sprintf("%s/%s", m.cfg.SocketDir, n),
	}
}
