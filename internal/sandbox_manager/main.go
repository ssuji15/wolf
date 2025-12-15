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
	"github.com/ssuji15/wolf/internal/service/logger"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	comp := component.GetNewComponents(ctx)
	logger.Init(comp.Cfg.ServiceName)

	tracerShutdown := job_tracer.InitTracer(ctx, comp.Cfg.ServiceName, "localhost:8085")
	defer tracerShutdown()

	m, err := manager.NewSandboxManager(ctx, comp)
	m.Addwg()
	if err != nil {
		log.Fatalf("error initialising sandbox: %v", err)
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	<-stop
	logger.Log.Info().Msg("shutting down sandbox gracefully..")
	cancel()
	m.Waitwg()
}
