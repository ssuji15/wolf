package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/ssuji15/wolf/internal/component"
	"github.com/ssuji15/wolf/internal/job_tracer"
	"github.com/ssuji15/wolf/internal/sandbox_manager/manager"
)

func main() {
	comp := component.GetNewComponents()
	ctx, cancel := context.WithCancel(context.Background())

	tracerShutdown := job_tracer.InitTracer(ctx, comp.Cfg.ServiceName, "localhost:8085")
	defer tracerShutdown()

	m, err := manager.NewSandboxManager(ctx, comp)
	m.Addwg()
	if err != nil {
		log.Fatalf("Error initialising sandbox: %v", err)
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	<-stop
	log.Println("Shutting down sandbox manager...")
	cancel()
	m.Waitwg()
}
