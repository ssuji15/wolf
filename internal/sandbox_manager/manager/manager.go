package manager

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/ssuji15/wolf/internal/component"
	"github.com/ssuji15/wolf/internal/queue"
	"github.com/ssuji15/wolf/internal/sandbox_manager/manager/launcher/docker_launcher"
	"github.com/ssuji15/wolf/internal/service"
	"github.com/ssuji15/wolf/model"
)

type SandboxManager struct {
	ctx             context.Context
	launcher        WorkerLauncher
	backendCallback string
	workers         chan model.WorkerMetadata
	workersMu       sync.Mutex
	workersCount    int
	minWorkers      int
	maxWorkers      int
	qClient         queue.Queue
	jobService      *service.JobService
}

func NewSandboxManager() (*SandboxManager, error) {

	comp := component.GetNewComponents()
	launcher := docker_launcher.NewDockerLauncher(comp.DBClient, comp.Cfg)

	m := &SandboxManager{
		ctx:             context.Background(),
		launcher:        launcher,
		backendCallback: comp.Cfg.BackendAddress,
		workers:         make(chan model.WorkerMetadata, comp.Cfg.MaxWorker),
		workersCount:    0,
		minWorkers:      comp.Cfg.MinWorker,
		maxWorkers:      comp.Cfg.MaxWorker,
		qClient:         comp.QClient,
		jobService:      service.NewJobService(comp.DBClient, comp.StorageClient, comp.QClient, comp.LocalCache),
	}
	m.initializePool()
	err := m.qClient.SubscribeEventToWorker(queue.JobCreated, m.dispatchJob, m.getIdleWorker, m.AddWorker)
	if err != nil {
		return nil, err
	}
	go m.scaleWorkers()
	return m, nil
}

func (m *SandboxManager) initializePool() {
	for i := 0; i < m.minWorkers; i++ {
		m.LaunchReplacement()
	}
}

func (m *SandboxManager) LaunchReplacement() {
	m.workersMu.Lock()
	if m.workersCount == m.maxWorkers {
		m.workersMu.Unlock()
		return
	}
	m.workersMu.Unlock()

	c, err := m.launcher.LaunchWorker(m.ctx)
	if err != nil {
		fmt.Println("worker launch failed:", err)
		return
	}
	m.increaseWorkerCount(1)
	go func() {
		time.Sleep(300 * time.Millisecond)
		m.workers <- c
	}()
}

func (m *SandboxManager) AddWorker(c model.WorkerMetadata) {
	m.workers <- c
}

func (m *SandboxManager) getIdleWorker() model.WorkerMetadata {
	for w := range m.workers {
		if m.launcher.IsContainerHealthy(m.ctx, w.ID) {
			m.LaunchReplacement()
			return w
		}
		go func() {
			m.shutdownWorker(w)
			m.LaunchReplacement()
		}()
	}
	return model.WorkerMetadata{}
}

func (m *SandboxManager) shutdownWorker(w model.WorkerMetadata) {
	m.launcher.DestroyWorker(m.ctx, w.ID)
	m.reduceWorkerCount()
	m.cleanWorkerSpace(w)
}

/*
Its still not fully correct. At the moment using workerscount to find the number of containers
which is naive. Should use docker API to get the current containers and scale up/down accordingly.

Scale Down will work as expected. Scale Up does not, which we will fix along with above change.
*/
func (m *SandboxManager) scaleWorkers() {
	ticker := time.NewTicker(30 * time.Second) // check every 30 seconds
	defer ticker.Stop()

	for range ticker.C {
		pending, err := m.qClient.GetPendingMessagesForConsumer(queue.JobCreated, "worker")
		if err != nil || pending > 0 {
			fmt.Println("Skipping scaling..")
			continue
		}
		m.workersMu.Lock()
		excess := len(m.workers) + (m.workersCount - len(m.workers)) - m.minWorkers
		// len(m.workers) = idle workers
		// m.workersCount - len(m.workers) = busy workers
		if excess > 0 {
			fmt.Printf("excess workers: %d, cleaning..\n", excess)
			for i := 0; i < excess; i++ {
				select {
				case w := <-m.workers:
					// remove idle worker
					m.shutdownWorker(w)
				default:
					// no more idle workers to remove
				}
			}
		} else if excess < 0 {
			for i := excess; i < 0; i++ {
				go m.LaunchReplacement()
			}
		}
		m.workersMu.Unlock()
	}
}

func (m *SandboxManager) reduceWorkerCount() {
	m.workersMu.Lock()
	m.workersCount--
	m.workersMu.Unlock()
}

func (m *SandboxManager) increaseWorkerCount(count int) {
	m.workersMu.Lock()
	m.workersCount += count
	m.workersMu.Unlock()
}

func (m *SandboxManager) cleanWorkerSpace(w model.WorkerMetadata) {
	os.RemoveAll(w.WorkDir)
}
