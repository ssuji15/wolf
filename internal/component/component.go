package component

import (
	"context"
	"log"

	"github.com/ssuji15/wolf/internal/cache"
	"github.com/ssuji15/wolf/internal/cache/freecache"
	cjetstream "github.com/ssuji15/wolf/internal/cache/jetstream"
	"github.com/ssuji15/wolf/internal/cache/redis"
	"github.com/ssuji15/wolf/internal/config"
	"github.com/ssuji15/wolf/internal/db"
	"github.com/ssuji15/wolf/internal/queue"
	"github.com/ssuji15/wolf/internal/queue/jetstream"
	"github.com/ssuji15/wolf/internal/storage"
	"github.com/ssuji15/wolf/internal/storage/minio"
)

type Components struct {
	Ctx           context.Context
	Cfg           *config.Config
	DBClient      *db.DB
	StorageClient storage.Storage
	QClient       queue.Queue
	Cache         cache.Cache
}

var component *Components

func NewCacheClient(ctx context.Context, cfg *config.Config) (cache.Cache, error) {
	switch cfg.CacheType {
	case "freeCache":
		return freecache.NewFreeCache(cfg.CacheByteSize, cfg.CacheTTL), nil
	case "jetstream":
		return cjetstream.NewJetStreamClient(ctx, cfg)
	default:
		return redis.NewRedisClient(ctx, cfg)
	}
}

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
	cache, err := NewCacheClient(ctx, cfg)
	if err != nil {
		log.Fatalf("failed to initialize cache: %v", err)
	}

	component = &Components{
		Ctx:           ctx,
		Cfg:           cfg,
		DBClient:      dbClient,
		StorageClient: minioClient,
		QClient:       jetstreamClient,
		Cache:         cache,
	}

	return component
}

func GetComponent() *Components {
	return component
}
