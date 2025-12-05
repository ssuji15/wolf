package manager

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/ssuji15/wolf/internal/component"
	"github.com/ssuji15/wolf/internal/job_tracer"
	"github.com/ssuji15/wolf/internal/queue"
	"github.com/ssuji15/wolf/internal/sandbox_manager/manager/launcher/docker_launcher"
	"github.com/ssuji15/wolf/internal/service"
	"github.com/ssuji15/wolf/model"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
)

type SandboxManager struct {
	ctx             context.Context
	launcher        WorkerLauncher
	backendCallback string
	workers         chan model.WorkerMetadata
	maxWorkers      int
	qClient         queue.Queue
	jobService      *service.JobService
	subscription    *nats.Subscription
	wg              *sync.WaitGroup
}

func NewSandboxManager(ctx context.Context, comp *component.Components) (*SandboxManager, error) {

	launcher := docker_launcher.NewDockerLauncher(comp.DBClient, comp.Cfg)
	sub, err := comp.QClient.SubscribeEventToWorker(queue.JobCreated)
	if err != nil {
		return nil, err
	}
	m := &SandboxManager{
		ctx:             ctx,
		launcher:        launcher,
		backendCallback: comp.Cfg.BackendAddress,
		workers:         make(chan model.WorkerMetadata, comp.Cfg.MaxWorker),
		maxWorkers:      comp.Cfg.MaxWorker,
		qClient:         comp.QClient,
		jobService:      service.NewJobService(comp.DBClient, comp.StorageClient, comp.QClient, comp.LocalCache),
		subscription:    sub,
		wg:              &sync.WaitGroup{},
	}
	m.initializePool()
	go m.processRequests()
	return m, nil
}

func (m *SandboxManager) initializePool() {
	for i := 0; i < m.maxWorkers; i++ {
		m.LaunchWorker()
	}
}

func (m *SandboxManager) LaunchWorker() {
	if err := m.ctx.Err(); err != nil {
		return
	}

	c, err := m.launcher.LaunchWorker(m.ctx)
	if err != nil {
		fmt.Println("worker launch failed:", err)
		return
	}
	go func() {
		time.Sleep(300 * time.Millisecond)
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
		fmt.Printf("Could not delete worker: %s, error: %v", w.ID, err)
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
			headers := propagation.MapCarrier(natsHeaderToMapStringString(msg.Header))
			parentCtx := otel.GetTextMapPropagator().Extract(
				context.Background(),
				headers,
			)

			tracer := job_tracer.GetTracer()
			ctx, span := tracer.Start(parentCtx, "Jetstream/Subscribe")
			defer span.End()

			id := string(msg.Data)
			span.SetAttributes(
				attribute.String("id", id),
			)

			go func() {
				if err := m.dispatchJob(ctx, id, worker); err != nil {
					log.Printf("Failed to handle: %s, err: %v", id, err)
					msg.Nak()
					return
				}
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
