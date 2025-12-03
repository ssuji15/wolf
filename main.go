package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	pb "github.com/ssuji15/wolf-worker/agent"
	"github.com/ssuji15/wolf/internal/component"
	"github.com/ssuji15/wolf/internal/web"
	"google.golang.org/grpc"
)

func main() {
	comp := component.GetNewComponents()

	defer comp.DBClient.Close()
	defer comp.QClient.Shutdown()

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

	grpcServer := grpc.NewServer()
	pb.RegisterWorkerAgentServer(grpcServer, &web.BackendReceiver{})

	go func() {
		lis, err := net.Listen("tcp", ":8081")
		if err != nil {
			log.Fatalf("grpc listener error: %v", err)
		}
		log.Println("GRPC server started on :8081")
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("grpc server error: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	<-stop
	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Graceful shutdown failed: %v", err)
	}

	grpcServer.GracefulStop()

	log.Println("Server stopped gracefully.")
}
