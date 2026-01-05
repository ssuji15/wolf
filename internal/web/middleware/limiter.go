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
	queue chan job
}

func NewLimiter(queueSize, maxInflight int) *Limiter {
	l := &Limiter{
		queue: make(chan job, queueSize),
	}

	for i := 0; i < maxInflight; i++ {
		go l.worker()
	}

	return l
}

func (l *Limiter) worker() {
	for j := range l.queue {
		if j.r.Context().Err() == nil {
			j.next.ServeHTTP(j.w, j.r)
		}
		close(j.done)
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
			<-j.done
		default:
			// Queue full
			http.Error(w, "server busy", http.StatusTooManyRequests)
			return
		}
	})
}
