package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ssuji15/wolf/internal/component"
	"github.com/ssuji15/wolf/internal/job_tracer"
	"github.com/ssuji15/wolf/internal/service/logger"
	"github.com/ssuji15/wolf/internal/web"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	comp := component.GetNewComponents(ctx)
	logger.Init(comp.Cfg.ServiceName)

	tracerShutdown := job_tracer.InitTracer(ctx, comp.Cfg.ServiceName, "localhost:8085")
	defer tracerShutdown()

	defer comp.DBClient.Close()
	defer comp.QClient.Shutdown()
	defer comp.StorageClient.Close()

	// ---- Step 5: Initialize Web Server ----
	server := web.NewServer(comp)

	srv := &http.Server{
		Addr:              ":8080",
		Handler:           server.Router(),
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
	}

	// ---- Step 6: Graceful Shutdown ----
	go func() {
		log.Println("HTTP server started on :8080")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server error: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	<-stop
	log.Println("Shutting down server...")

	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Graceful shutdown failed: %v", err)
	}

	log.Println("Server stopped gracefully.")
}
