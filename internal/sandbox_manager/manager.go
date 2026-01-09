package sandbox_manager

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/ssuji15/wolf/internal/cache"
	"github.com/ssuji15/wolf/internal/config"
	"github.com/ssuji15/wolf/internal/job_tracer"
	"github.com/ssuji15/wolf/internal/queue"
	containerdlauncher "github.com/ssuji15/wolf/internal/sandbox_manager/launcher/containerd_launcher"
	"github.com/ssuji15/wolf/internal/sandbox_manager/launcher/docker_launcher"
	jobservice "github.com/ssuji15/wolf/internal/service/job_service"
	"github.com/ssuji15/wolf/internal/service/logger"
	"github.com/ssuji15/wolf/internal/storage"
	"github.com/ssuji15/wolf/internal/util"
	"github.com/ssuji15/wolf/model"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const (
	WORKER_CONSUMER string = "worker"
)

type SandboxManager struct {
	ctx           context.Context
	launcher      WorkerLauncher
	workers       chan model.WorkerMetadata
	dworkers      chan model.WorkerMetadata
	qClient       queue.Queue
	storageClient storage.Storage
	jobService    *jobservice.JobService
	wg            *sync.WaitGroup
	cfg           *config.SandboxManagerConfig
}

func NewLauncher(cfg *config.SandboxManagerConfig) (WorkerLauncher, error) {
	switch cfg.LAUNCHER_TYPE {
	case "docker":
		return docker_launcher.NewDockerLauncher(cfg)
	default:
		return containerdlauncher.NewContainerdLauncher(cfg)
	}
}

func NewSandboxManager(ctx context.Context, cache cache.Cache, queue queue.Queue, storage storage.Storage) (*SandboxManager, error) {

	cfg, err := config.GetSandboxManagerConfig()
	if err != nil {
		return nil, err
	}

	launcher, err := NewLauncher(cfg)
	if err != nil {
		return nil, err
	}

	err = launcher.SetSecCompProfile(cfg.SECCOMP_PROFILE)
	if err != nil {
		return nil, err
	}

	js, err := jobservice.NewJobService(ctx, cache, storage, queue)
	if err != nil {
		return nil, err
	}

	m := &SandboxManager{
		ctx:           ctx,
		launcher:      launcher,
		workers:       make(chan model.WorkerMetadata, cfg.MAX_WORKER),
		dworkers:      make(chan model.WorkerMetadata, cfg.MAX_WORKER*2),
		qClient:       queue,
		storageClient: storage,
		jobService:    js,
		wg:            &sync.WaitGroup{},
		cfg:           cfg,
	}
	m.initializePool()
	for i := 0; i < 5; i++ {
		go m.shutdownWorkerJob(ctx)
	}
	go m.processRequests()
	return m, nil
}

func (m *SandboxManager) initializePool() {
	for i := 0; i < m.cfg.MAX_WORKER; i++ {
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
	udsPath := fmt.Sprintf("%s/%s/socket/socket.sock", m.cfg.SOCKET_DIR, opt.Name)
	if err := util.VerifyFileDoesNotExist(udsPath); err != nil {
		util.RecordSpanError(span, err)
		return
	}

	outputPath := fmt.Sprintf("%s/%s/output/output.log", m.cfg.SOCKET_DIR, opt.Name)
	if err := util.VerifyFileDoesNotExist(outputPath); err != nil {
		util.RecordSpanError(span, err)
		return
	}

	c, err := m.launcher.LaunchWorker(ctx, opt)
	c.SocketPath = udsPath
	c.OutputPath = outputPath
	c.WorkDir = opt.WorkDir

	if err != nil {
		err := fmt.Errorf("worker launch failed: %v", err)
		util.RecordSpanError(span, err)
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
	if err := m.ctx.Err(); err != nil {
		return
	}
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

func (m *SandboxManager) shutdownWorkerJob(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case w := <-m.dworkers:
			m.deleteWorker(w)
		}
	}
}

func (m *SandboxManager) shutdownWorker(w model.WorkerMetadata) {
	m.dworkers <- w
	m.LaunchWorker()
}

func (m *SandboxManager) deleteWorker(w model.WorkerMetadata) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() {
		err := m.launcher.DestroyWorker(ctx, w.ID)
		if err != nil && !strings.Contains(err.Error(), "not found") {
			logger.Log.Error().Err(err).Str("workerID", w.ID).Msg("could not delete worker")
			return
		}
		close(done)
	}()

	select {
	case <-done:
		logger.Log.Info().Str("id", w.ID).Msg("worker deleted successfully.")
		m.cleanWorkerSpace(w)
	case <-ctx.Done():
		go func() {
			time.Sleep(500 * time.Millisecond)
			m.dworkers <- w
		}()
	}
}

func (m *SandboxManager) ShutdownAllWorkers(ctx context.Context) {
	for {
		select {
		case w := <-m.workers:
			go m.deleteWorker(w)
		case w := <-m.dworkers:
			go m.deleteWorker(w)
		default:
			close(m.workers)
			close(m.dworkers)
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
	m.qClient.AddConsumer(queue.EventStream, WORKER_CONSUMER)
	sub, err := m.qClient.SubscribeEvent(queue.JobCreated, WORKER_CONSUMER)
	if err != nil {
		log.Fatalf("unable to subscribe to Nats events: %v", err)
	}
	for {
		select {
		case <-m.ctx.Done():
			return
		default:
			worker := m.getIdleWorker()
			msgs, err := sub.Fetch(m.ctx, 1, 30*time.Second)
			if err != nil {
				m.AddWorkerToPool(worker)
				if errors.Is(err, nats.ErrTimeout) || errors.Is(err, nats.ErrSubscriptionClosed) {
					continue
				}
				time.Sleep(time.Second)
				continue
			}
			msg := msgs[0]

			d := time.Since(msg.PublishedAt())
			latency.Record(context.Background(), d.Seconds())

			id := string(msg.Data())
			j, err := m.jobService.GetJob(msg.Ctx(), id)
			if err != nil || j.Status == string(jobservice.JOB_COMPLETED) || j.Status == string(jobservice.JOB_FAILED) {
				m.AddWorkerToPool(worker)
				continue
			}
			j.Status = string(jobservice.JOB_DISPATCHED)

			go func() {
				if err := m.dispatchJob(msg.Ctx(), j, worker); err != nil {
					j.RetryCount++
					logger.Log.Error().Err(err).Str("id", id).Msg("failed to execute job")
					if msg.RetryCount() == queue.MaxDeliver {
						j.Status = string(jobservice.JOB_FAILED)
						logger.Log.Error().Err(fmt.Errorf("max delivery reached for job")).Str("id", id).Msg("sending job to DLQ")
						m.qClient.PublishEvent(msg.Ctx(), queue.DeadLetterQueue, id)
						msg.Term()
					}
					err = m.jobService.UpdateJob(msg.Ctx(), j)
					if err != nil {
						logger.Log.Error().Err(err).Str("id", id).Msg("failed to update job")
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

func (m *SandboxManager) Donewg() {
	m.wg.Done()
}

func (m *SandboxManager) Waitwg() {
	m.wg.Wait()
}

func (m *SandboxManager) GetWorkerOption() model.CreateOptions {
	n := uuid.New().String()
	return model.CreateOptions{
		Name:        n,
		Image:       m.cfg.WORKER_IMAGE,
		CPUQuota:    100000,
		MemoryLimit: 512 * 1024 * 1024,
		Labels: map[string]string{
			"id": "worker",
		},
		AppArmorProfile: m.cfg.APPARMOR_PROFILE,
		WorkDir:         fmt.Sprintf("%s/%s", m.cfg.SOCKET_DIR, n),
	}
}
