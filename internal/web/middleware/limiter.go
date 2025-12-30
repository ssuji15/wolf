package middleware

import (
	"net/http"
)

type job struct {
	w    http.ResponseWriter
	r    *http.Request
	next http.Handler
	done chan struct{}
}

type Limiter struct {
	queue    chan job
	inflight chan struct{}
}

func NewLimiter(queueSize, maxInflight int) *Limiter {
	l := &Limiter{
		queue:    make(chan job, queueSize),
		inflight: make(chan struct{}, maxInflight),
	}

	go l.dispatch()

	return l
}

func (l *Limiter) dispatch() {
	for j := range l.queue {
		// acquire inflight slot (blocks if full)
		l.inflight <- struct{}{}

		go func(j job) {
			defer func() {
				<-l.inflight // release slot
				close(j.done)
			}()

			j.next.ServeHTTP(j.w, j.r)
		}(j)
	}
}

func (l *Limiter) Limit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		j := job{
			w:    w,
			r:    r,
			next: next,
			done: make(chan struct{}),
		}

		// Try to enqueue
		select {
		case l.queue <- j:
			// Wait until request is processed or context is cancelled
			select {
			case <-j.done:
			case <-r.Context().Done():
				http.Error(w, "request canceled or timed out", http.StatusGatewayTimeout)
				return
			}
		default:
			// Queue full
			http.Error(w, "server busy", http.StatusServiceUnavailable)
			return
		}
	})
}
