package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/ssuji15/wolf/internal/cache"
	"github.com/ssuji15/wolf/internal/config"
	"github.com/vmihailenco/msgpack/v5"
)

type RedisClient struct {
	client *redis.Client
	ttl    int
}

func NewRedisClient(ctx context.Context, cfg *config.Config) (cache.Cache, error) {
	rc := redis.NewClient(&redis.Options{
		Addr:            cfg.CacheURL,
		Password:        cfg.CacheClientPassword,
		DB:              0,
		PoolSize:        5,
		MinIdleConns:    2,
		PoolTimeout:     1 * time.Second,
		MinRetryBackoff: 100 * time.Millisecond,
		MaxRetryBackoff: 500 * time.Millisecond,
		ConnMaxIdleTime: 5 * time.Minute,
		ConnMaxLifetime: 30 * time.Minute,
	})

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	if err := rc.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %v", err)
	}

	return &RedisClient{
		client: rc,
		ttl:    cfg.CacheTTL,
	}, nil
}

func (r *RedisClient) Put(ctx context.Context, key string, value interface{}, ttl int) error {
	if key == "" {
		return fmt.Errorf("key cannot be empty")
	}
	if value == nil {
		return fmt.Errorf("value cannot be nil")
	}
	b, err := msgpack.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value for key %s: %w", key, err)
	}
	err = r.client.Set(ctx, key, b, time.Duration(ttl)*time.Second).Err()
	if err != nil {
		return err
	}
	return nil
}

// value must be non-nil pointer to destination type
func (r *RedisClient) Get(ctx context.Context, key string, value interface{}) error {
	if key == "" {
		return fmt.Errorf("key cannot be empty")
	}
	val, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		return fmt.Errorf("failed to retrieve value for key %s: %w", key, err)
	}
	err = msgpack.Unmarshal(val, value)
	if err != nil {
		return fmt.Errorf("failed to unmarshal value for key %s: %w", key, err)
	}
	return nil
}

func (r *RedisClient) GetDefaultTTL() int {
	return r.ttl
}

func (r *RedisClient) Close() error {
	return r.client.Close()
}
