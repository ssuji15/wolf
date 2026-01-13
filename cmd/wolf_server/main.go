package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/ssuji15/wolf/internal/component"
	"github.com/ssuji15/wolf/internal/config"
	"github.com/ssuji15/wolf/internal/db"
	"github.com/ssuji15/wolf/internal/job_tracer"
	"github.com/ssuji15/wolf/internal/service/logger"
	"github.com/ssuji15/wolf/internal/web"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	cfg, err := config.GetConfig()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}
	logger.Init(cfg.SERVICE_NAME)

	if cfg.TRACE_URL != "" {
		tp, err := job_tracer.InitTracer(ctx, cfg.SERVICE_NAME, cfg.TRACE_URL)
		if err != nil {
			log.Fatalf("error initialising trace: %v", err)
		}
		defer tp.Shutdown(ctx)
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

	server, err := web.NewServer(ctx, cache, queue, storage)
	if err != nil {
		log.Fatalf("server initialization error: %v", err)
	}

	srv := &http.Server{
		Addr:              ":8080",
		Handler:           server.Router(),
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Println("HTTP server started on :8080")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server error: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	<-stop
	log.Println("trying to shutdown server gracefully...")

	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Graceful shutdown failed: %v", err)
	}

	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var wg sync.WaitGroup

	shutdown := func(fn func(context.Context)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fn(ctx)
		}()
	}
	shutdown(db.Close)
	shutdown(cache.ShutDown)
	shutdown(storage.ShutDown)
	shutdown(queue.ShutDown)

	done := make(chan struct{})

	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		logger.Log.Info().Msg("server shutdown gracefully.")
	case <-ctx.Done():
		logger.Log.Info().Msg("server graceful shutdown timedout..")
	}
}
