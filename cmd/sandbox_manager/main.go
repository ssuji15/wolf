package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ssuji15/wolf/internal/component"
	"github.com/ssuji15/wolf/internal/config"
	"github.com/ssuji15/wolf/internal/db"
	"github.com/ssuji15/wolf/internal/job_tracer"
	"github.com/ssuji15/wolf/internal/sandbox_manager"
	"github.com/ssuji15/wolf/internal/service/logger"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := config.GetConfig()
	if err != nil {
		log.Fatalf("Initialization error: %v", err)
	}
	logger.Init(cfg.SERVICE_NAME)

	if cfg.TRACE_URL != "" {
		tp, err := job_tracer.InitTracer(ctx, cfg.SERVICE_NAME, cfg.TRACE_URL)
		if err != nil {
			log.Fatalf("error initialising trace: %v", err)
		}
		defer tp.Shutdown(ctx)
	}

	d, err := db.New(ctx)
	if err != nil {
		log.Fatalf("db initialization error: %v", err)
	}

	cache, err := component.GetCache(ctx, cfg.CACHE_TYPE)
	if err != nil {
		log.Fatalf("cache initialization error: %v", err)
	}
	storage, err := component.GetStorage(cfg.STORAGE_TYPE)
	if err != nil {
		log.Fatalf("storage initialization error: %v", err)
	}
	queue, err := component.GetQueue(cfg.QUEUE_TYPE)
	if err != nil {
		log.Fatalf("queue initialization error: %v", err)
	}

	m, err := sandbox_manager.NewSandboxManager(ctx, cache, queue, storage, d)
	if err != nil {
		log.Fatalf("error initialising sandbox: %v", err)
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	<-stop
	logger.Log.Info().Msg("trying to shut down sandbox gracefully..")
	cancel()

	ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	m.ShutdownAllWorkers(ctx)

	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	shutdown := func(fn func(context.Context)) {
		m.Addwg()
		go func() {
			defer m.Donewg()
			fn(ctx)
		}()
	}
	done := make(chan struct{})

	shutdown(d.Close)
	shutdown(cache.ShutDown)
	shutdown(storage.ShutDown)
	shutdown(queue.ShutDown)
	go func() {
		m.Waitwg()
		close(done)
	}()

	select {
	case <-done:
		logger.Log.Info().Msg("sandbox shutdown gracefully.")
	case <-ctx.Done():
		logger.Log.Info().Msg("sandbox graceful shutdown timedout..")
	}
}
