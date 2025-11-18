package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/ssuji15/wolf/internal/cache/freecache"
	"github.com/ssuji15/wolf/internal/db"
	"github.com/ssuji15/wolf/internal/queue/jetstream"
	"github.com/ssuji15/wolf/internal/storage"
	"github.com/ssuji15/wolf/internal/web"
)

func main() {
	// ---- Step 1: Initialize Postgres ----
	dbClient, err := db.New()
	if err != nil {
		log.Fatalf("failed to initialize database: %v", err)
	}
	defer dbClient.Close()

	// ---- Step 2: Initialize MinIO ----
	minioConfig, err := storage.GetMinioConfig()
	if err != nil {
		log.Fatalf("failed to initialize minio: %v", err)
	}

	minioClient, err := storage.NewMinioClient(minioConfig)
	if err != nil {
		log.Fatalf("failed to initialize minio: %v", err)
	}

	jsURL := os.Getenv("JETSTREAM_URL")
	if jsURL == "" {
		log.Fatalf("unable to retrieve JETSTREAM_URL")
	}

	jetstreamClient, err := jetstream.NewJetStreamClient(jsURL)
	if err != nil {
		log.Fatalf("failed to initialize jetstream: %v", err)
	}

	defer jetstreamClient.Shutdown()

	cacheSizeBytes := os.Getenv("FREECACHE_BYTE_SIZE")
	cacheSizeBytesInt, err := strconv.Atoi(cacheSizeBytes)
	if err != nil {
		log.Fatalf("Invalid FREECACHE_BYTE_SIZE: %v", err)
	}

	ttl := os.Getenv("FREECACHE_TTL")
	ttlInt, err := strconv.Atoi(ttl)
	if err != nil {
		log.Fatalf("Invalid FREECACHE_TTL: %v", err)
	}

	cache := freecache.NewFreeCache(cacheSizeBytesInt, ttlInt)

	// ---- Step 3: Initialize Web Server ----
	server := web.NewServer(dbClient, minioClient, jetstreamClient, cache)

	srv := &http.Server{
		Addr:              ":8080",
		Handler:           server.Router(),
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
	}

	// ---- Step 4: Graceful Shutdown ----
	go func() {
		log.Println("HTTP server started on :8080")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
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

	log.Println("Server stopped gracefully.")
}
