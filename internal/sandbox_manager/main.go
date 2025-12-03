package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ssuji15/wolf/internal/component"
	"github.com/ssuji15/wolf/internal/sandbox_manager/manager"
)

func main() {
	_, err := manager.NewSandboxManager()
	if err != nil {
		log.Fatalf("Error initialising sandbox: %v", err)
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	<-stop
	log.Println("Shutting down sandbox manager...")

	_, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	comp := component.GetComponent()
	defer comp.DBClient.Close()
	defer comp.QClient.Shutdown()
}
