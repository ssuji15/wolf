package component

import (
	"context"
	"log"

	"github.com/ssuji15/wolf/internal/cache"
	"github.com/ssuji15/wolf/internal/cache/freecache"
	"github.com/ssuji15/wolf/internal/config"
	"github.com/ssuji15/wolf/internal/db"
	"github.com/ssuji15/wolf/internal/queue"
	"github.com/ssuji15/wolf/internal/queue/jetstream"
	"github.com/ssuji15/wolf/internal/storage"
	"github.com/ssuji15/wolf/internal/storage/minio"
)

type Components struct {
	Cfg           *config.Config
	DBClient      *db.DB
	StorageClient storage.Storage
	QClient       queue.Queue
	LocalCache    cache.Cache
}

var component *Components

func GetNewComponents(ctx context.Context) *Components {
	cfg := config.Load()

	// ---- Step 1: Initialize Postgres ----
	dbClient, err := db.New(ctx, *cfg)
	if err != nil {
		log.Fatalf("failed to initialize database: %v", err)
	}

	// ---- Step 2: Initialize MinIO ----
	minioClient, err := minio.NewMinioClient(minio.GetMinioConfig(*cfg))
	if err != nil {
		log.Fatalf("failed to initialize minio: %v", err)
	}

	// ---- Step 3: Initialize Jetstream ----
	jetstreamClient, err := jetstream.NewJetStreamClient(cfg.JetstreamURL)
	if err != nil {
		log.Fatalf("failed to initialize jetstream: %v", err)
	}

	// ---- Step 4: Initialize Cache ----
	cache := freecache.NewFreeCache(cfg.FreecacheByteSize, cfg.FreecacheTTL)

	component = &Components{
		Cfg:           cfg,
		DBClient:      dbClient,
		StorageClient: minioClient,
		QClient:       jetstreamClient,
		LocalCache:    cache,
	}

	return component
}

func GetComponent() *Components {
	return component
}
