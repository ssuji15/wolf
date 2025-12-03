package manager

import (
	"sync"

	"github.com/ssuji15/wolf/model"
)

type WorkerWrapper struct {
	W   *model.WorkerMetadata
	Mux sync.Mutex
}

func (w *WorkerWrapper) SetStatus(s model.WorkerStatus) {
	w.Mux.Lock()
	w.W.Status = string(s)
	w.Mux.Unlock()
}

func (w *WorkerWrapper) GetStatus() model.WorkerStatus {
	w.Mux.Lock()
	s := w.W.Status
	w.Mux.Unlock()
	return model.WorkerStatus(s)
}
