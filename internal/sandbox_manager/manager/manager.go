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
	err := m.qClient.SubscribeEvent(queue.JobCreated, m.dispatchJob)
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
	m.workersCount++
	m.workersMu.Unlock()

	c, err := m.launcher.LaunchWorker(m.ctx)
	if err != nil {
		fmt.Println("worker launch failed:", err)
		return
	}

	go func() {
		time.Sleep(300 * time.Millisecond)
		m.workers <- c
	}()
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

func (m *SandboxManager) scaleWorkers() {
	ticker := time.NewTicker(5 * time.Second) // check every 5 seconds
	defer ticker.Stop()

	for range ticker.C {
		m.workersMu.Lock()
		excess := len(m.workers) + (m.workersCount - len(m.workers)) - m.minWorkers
		// len(m.workers) = idle workers
		// m.workersCount - len(m.workers) = busy workers
		if excess > 0 {
			for i := 0; i < excess; i++ {
				select {
				case w := <-m.workers:
					// remove idle worker
					m.launcher.DestroyWorker(m.ctx, w.ID)
					m.workersCount--
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

func (m *SandboxManager) cleanWorkerSpace(w model.WorkerMetadata) {
	os.RemoveAll(w.WorkDir)
}
