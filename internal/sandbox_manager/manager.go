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

	"github.com/nats-io/nats.go"
	"github.com/ssuji15/wolf/internal/cache"
	"github.com/ssuji15/wolf/internal/config"
	"github.com/ssuji15/wolf/internal/db"
	"github.com/ssuji15/wolf/internal/job_tracer"
	"github.com/ssuji15/wolf/internal/queue"
	"github.com/ssuji15/wolf/internal/sandbox_manager/raven"
	"github.com/ssuji15/wolf/internal/sandbox_manager/raven/grpc"
	"github.com/ssuji15/wolf/internal/sandbox_manager/raven/grpc/transport"
	"github.com/ssuji15/wolf/internal/sandbox_manager/worker"
	"github.com/ssuji15/wolf/internal/sandbox_manager/worker/containerd"
	"github.com/ssuji15/wolf/internal/sandbox_manager/worker/docker"
	jobservice "github.com/ssuji15/wolf/internal/service/job_service"
	"github.com/ssuji15/wolf/internal/service/logger"
	"github.com/ssuji15/wolf/internal/storage"
	"github.com/ssuji15/wolf/internal/util"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type SandboxManager struct {
	ctx               context.Context
	workers           map[worker.JobType]chan *worker.WorkerMetadata
	dworkers          chan *worker.WorkerMetadata
	qClient           queue.Queue
	storageClient     storage.Storage
	jobService        *jobservice.JobService
	wg                *sync.WaitGroup
	cfg               *config.SandboxManagerConfig
	workerGroupOption map[string]worker.WorkerOption
}

func NewWorkerManager(cfg config.WorkerGroupConfig) (worker.WorkerManager, error) {
	switch cfg.Worker {
	case "docker":
		return docker.NewDockerWorker(cfg.Secomp, cfg.AppArmor, cfg.Runtime)
	case "containerd":
		return containerd.NewContainerdWorker(cfg.Secomp, cfg.AppArmor, cfg.Runtime)
	default:
		return nil, fmt.Errorf("unsupported worker type")
	}
}

func NewSandboxManager(ctx context.Context, cache cache.Cache, queue queue.Queue, storage storage.Storage, db *db.DB) (*SandboxManager, error) {

	cfg, err := config.GetSandboxManagerConfig()
	if err != nil {
		return nil, err
	}

	err = worker.ValidateSandboxManagerConfig(cfg)
	if err != nil {
		return nil, err
	}

	js, err := jobservice.NewJobService(ctx, cache, storage, queue, db)
	if err != nil {
		return nil, err
	}

	m := &SandboxManager{
		ctx:               ctx,
		workers:           make(map[worker.JobType]chan *worker.WorkerMetadata),
		dworkers:          make(chan *worker.WorkerMetadata, 100),
		qClient:           queue,
		storageClient:     storage,
		jobService:        js,
		wg:                &sync.WaitGroup{},
		cfg:               cfg,
		workerGroupOption: make(map[string]worker.WorkerOption),
	}
	m.initWorkers(cfg)
	for i := 0; i < 5; i++ {
		go m.shutdownWorkerJob(ctx)
	}
	go m.processCodeRequests()
	return m, nil
}

func (m *SandboxManager) initWorkers(cfg *config.SandboxManagerConfig) {
	for _, w := range cfg.Workers {
		m.workers[worker.JobType(w.Type)] = make(chan *worker.WorkerMetadata, 100)
		for _, wc := range w.WorkerGroups {
			gn := w.Type + wc.Worker
			wm, err := NewWorkerManager(wc)
			if err != nil {
				logger.Log.Error().Err(err).Str("type", wc.Worker).Msg("error initialising worker")
				continue
			}
			opt := worker.WorkerOption{
				RavenType: raven.Type(wc.Raven.Type),
				RavenOption: raven.Option{
					GrpcJobType: grpc.GRPCRavenJobType(w.Type),
					GrpcTransportOption: transport.Option{
						Type: transport.TransportType(wc.Raven.TransportConfig.Type),
					},
				},
				JobType:           worker.JobType(w.Type),
				Image:             wc.Image,
				WorkerManagerType: worker.WorkerManagerType(wc.Worker),
				EnvVars: map[string]string{
					worker.WORKER_TRANSPORT_ENV: wc.Raven.TransportConfig.Type,
				},
			}
			m.workerGroupOption[gn] = opt
			for i := 0; i < wc.Count; i++ {
				_, err := m.LaunchWorker(opt, wm)
				if err != nil {
					logger.Log.Error().Err(err).Msg("worker creation error")
				}
			}
		}
	}
}

func (m *SandboxManager) LaunchWorker(opt worker.WorkerOption, wm worker.WorkerManager) (*worker.WorkerMetadata, error) {
	if err := m.ctx.Err(); err != nil {
		return nil, nil
	}

	nw, err := worker.NewWorker(opt, wm, m.cfg.WorkDir)
	if err != nil {
		return nil, err
	}

	tracer := job_tracer.GetTracer()
	_, span := tracer.Start(m.ctx, "Create container")
	defer span.End()

	err = nw.Launch()
	if err != nil {
		err := fmt.Errorf("worker launch failed: %v", err)
		util.RecordSpanError(span, err)
		return nil, err
	}

	switch opt.RavenOption.GrpcTransportOption.Type {
	case transport.UDS:
		nw.AddRaven(opt, nw.Workspace.WorkDir)
	case transport.TCP:
		nw.AddRaven(opt, nw.IP+":"+worker.WORKER_LISTEN_PORT)
	}
	span.End()

	span.AddEvent("Worker_Launch",
		trace.WithAttributes(attribute.String("container_id", nw.ID)),
	)
	go func() {
		time.Sleep(10 * time.Millisecond)
		nw.Status = worker.WorkerStateReady
		m.AddWorkerToPool(nw)
	}()
	return nw, nil
}

func (m *SandboxManager) AddWorkerToPool(c *worker.WorkerMetadata) {
	m.workers[c.JobType] <- c
}

func (m *SandboxManager) getIdleWorker(ctx context.Context, t worker.JobType) (*worker.WorkerMetadata, error) {
	if err := m.ctx.Err(); err != nil {
		return nil, fmt.Errorf("unable to retrieve worker, ctx closed")
	}

	var w *worker.WorkerMetadata
	select {
	case w = <-m.workers[t]:
	case <-ctx.Done():
		return nil, fmt.Errorf("unable to retrieve worker, ctx closed")
	}

	h, err := w.IsHealthy(ctx)
	if err != nil || !h {
		return nil, fmt.Errorf("retrieved worker is unhealthy")
	}
	return w, nil
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

func (m *SandboxManager) shutdownWorker(w *worker.WorkerMetadata) {
	m.dworkers <- w
	go func() {
		for i := 0; i < 3; i++ {
			select {
			case <-m.ctx.Done():
				return
			default:
				_, err := m.LaunchWorker(m.workerGroupOption[string(w.JobType)+string(w.WorkerManagerType)], w.Manager)
				if err != nil {
					logger.Log.Error().Err(err).Msg("failed to relaunch new worker..")
					continue
				}
				return
			}
		}
	}()
}

func (m *SandboxManager) deleteWorker(w *worker.WorkerMetadata) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() {
		err := w.Destroy(ctx)
		if err != nil && !strings.Contains(err.Error(), "No such container") {
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

func (m *SandboxManager) ShutdownAllWorkers(ctx context.Context, t worker.JobType) {
	for {
		select {
		case w := <-m.workers[t]:
			go w.Destroy(ctx)
		case w := <-m.dworkers:
			go w.Destroy(ctx)
		case <-ctx.Done():
			close(m.workers[t])
			close(m.dworkers)
			return
		}
	}
}

func (m *SandboxManager) cleanWorkerSpace(w *worker.WorkerMetadata) {
	os.RemoveAll(w.Workspace.WorkDir)
}

func (m *SandboxManager) processCodeRequests() {
	meter := otel.Meter("sandboxmanager")
	latency, _ := meter.Float64Histogram("job_queue_duration_seconds")
	sub, err := m.qClient.SubscribeEvent(queue.JobCreated, queue.WORKER_CONSUMER)
	if err != nil {
		log.Fatalf("unable to subscribe to Nats events: %v", err)
	}
	for {
		select {
		case <-m.ctx.Done():
			return
		default:
			worker, err := m.getIdleWorker(m.ctx, worker.JobCodeExecution)
			if err != nil {
				logger.Log.Error().Err(err).Msg("Could not get idle worker")
				continue
			}
			msgs, err := sub.Fetch(1, 30*time.Second)
			if err != nil {
				m.AddWorkerToPool(worker)
				if errors.Is(err, nats.ErrTimeout) || errors.Is(err, nats.ErrSubscriptionClosed) {
					continue
				}
				time.Sleep(time.Second)
				continue
			}
			msg := msgs[0]

			tracer := job_tracer.GetTracer()
			ctx, span := tracer.Start(msg.Ctx(), "ProcessJob")

			d := time.Since(msg.PublishedAt())
			latency.Record(context.Background(), d.Seconds())

			id := string(msg.Data())
			j, err := m.jobService.GetJob(ctx, id)
			if err != nil {
				m.AddWorkerToPool(worker)
				continue
			}
			if j.Status == string(jobservice.JOB_COMPLETED) || j.Status == string(jobservice.JOB_FAILED) {
				msg.Ack()
				continue
			}
			j.Status = string(jobservice.JOB_DISPATCHED)

			go func() {
				defer span.End()
				if err := m.dispatchJob(ctx, j, worker); err != nil {
					j.RetryCount++
					logger.Log.Error().Err(err).Str("id", id).Msg("failed to execute job")
					if msg.RetryCount() == queue.MaxDeliver {
						j.Status = string(jobservice.JOB_FAILED)
						logger.Log.Error().Err(fmt.Errorf("max delivery reached for job")).Str("id", id).Msg("sending job to DLQ")
						m.qClient.PublishEvent(ctx, queue.DeadLetterQueue, id)
						msg.Term()
					}
					err = m.jobService.UpdateJob(ctx, j)
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
