package manager

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
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
	"go.opentelemetry.io/otel/trace"
)

type SandboxManager struct {
	ctx        context.Context
	launcher   WorkerLauncher
	workers    chan model.WorkerMetadata
	qClient    queue.Queue
	jobService *jobservice.JobService
	wg         *sync.WaitGroup
	cfg        *config.Config
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
	err := launcher.SetSecCompProfile(comp.Cfg.SeccompProfile)
	if err != nil {
		return nil, err
	}
	m := &SandboxManager{
		ctx:        ctx,
		launcher:   launcher,
		workers:    make(chan model.WorkerMetadata, comp.Cfg.MaxWorker),
		qClient:    comp.QClient,
		jobService: jobservice.NewJobService(comp.DBClient, comp.StorageClient, comp.QClient, comp.LocalCache),
		wg:         &sync.WaitGroup{},
		cfg:        comp.Cfg,
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
	span.AddEvent("Worker_Launch",
		trace.WithAttributes(attribute.String("container_id", c.ID)),
	)
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
	sub, err := m.qClient.SubscribeEvent(queue.JobCreated)
	if err != nil {
		log.Fatalf("unable to subscribe to Nats events: %v", err)
	}
	for {
		select {
		case <-m.ctx.Done():
			err := m.qClient.Shutdown()
			if err != nil {
				logger.Log.Error().Err(err).Msg("queue drain failed")
			}
			m.shutdownAllWorkers()
			comp := component.GetComponent()
			comp.DBClient.Close()
			m.wg.Done()
			return
		default:
			worker := m.getIdleWorker()
			msg, err := sub.Fetch(1, 30*time.Second)
			if err != nil {
				m.AddWorkerToPool(worker)
				if errors.Is(err, nats.ErrTimeout) {
					continue
				}
				time.Sleep(time.Second)
				continue
			}

			d := time.Since(msg.PublishedAt())
			latency.Record(context.Background(), d.Seconds())

			id := string(msg.Data())
			go func() {
				if err := m.dispatchJob(msg.Ctx(), id, worker); err != nil {
					logger.Log.Error().Err(err).Str("id", id).Msg("failed to execute job")
					if msg.RetryCount() == queue.MaxDeliver {
						logger.Log.Error().Err(fmt.Errorf("max delivery reached for job")).Str("id", id).Msg("sending job to DLQ")
						m.qClient.PublishEvent(msg.Ctx(), queue.DeadLetterQueue, id)
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
		AppArmorProfile: m.cfg.AppArmorProfile,
		WorkDir:         fmt.Sprintf("%s/%s", m.cfg.SocketDir, n),
	}
}
