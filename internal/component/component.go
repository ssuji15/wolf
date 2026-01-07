package component

import (
	"context"

	"github.com/ssuji15/wolf/internal/cache"
	"github.com/ssuji15/wolf/internal/cache/freecache"
	"github.com/ssuji15/wolf/internal/cache/jetstream"
	"github.com/ssuji15/wolf/internal/cache/redis"
	"github.com/ssuji15/wolf/internal/queue"
	jq "github.com/ssuji15/wolf/internal/queue/jetstream"
	"github.com/ssuji15/wolf/internal/storage"
	"github.com/ssuji15/wolf/internal/storage/minio"
)

func GetCache(ctx context.Context, cacheType string) (cache.Cache, error) {
	switch cacheType {
	case "redis":
		return redis.NewRedisCacheClient(ctx)
	case "jetstream":
		return jetstream.NewJetStreamCacheClient()
	default:
		return freecache.NewFreeCache()
	}
}

func GetQueue(qType string) (queue.Queue, error) {
	switch qType {
	default:
		return jq.NewJetStreamQueueClient()
	}
}

func GetStorage(storageType string) (storage.Storage, error) {
	switch storageType {
	default:
		return minio.NewMinioClient()
	}
}
