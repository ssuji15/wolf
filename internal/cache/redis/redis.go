package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/ssuji15/wolf/internal/cache"
	"github.com/ssuji15/wolf/internal/config"
	"github.com/ssuji15/wolf/internal/job_tracer"
	"github.com/ssuji15/wolf/internal/util"
	"github.com/vmihailenco/msgpack/v5"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
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
		PoolSize:        50,
		MinIdleConns:    10,
		PoolTimeout:     1 * time.Second,
		MinRetryBackoff: 100 * time.Millisecond,
		MaxRetryBackoff: 500 * time.Millisecond,
		ConnMaxIdleTime: 10 * time.Minute,
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
	tracer := job_tracer.GetTracer()
	ctx, span := tracer.Start(ctx, "Redis/Put")
	defer span.End()
	if key == "" {
		err := fmt.Errorf("key cannot be empty")
		util.RecordSpanError(span, err)
		return err
	}
	span.AddEvent("redis.context",
		trace.WithAttributes(attribute.String("key", key)),
	)
	if value == nil {
		err := fmt.Errorf("value cannot be nil")
		util.RecordSpanError(span, err)
		return err
	}
	b, err := msgpack.Marshal(value)
	if err != nil {
		err := fmt.Errorf("failed to marshal value for key %s: %w", key, err)
		util.RecordSpanError(span, err)
		return err
	}
	err = r.client.Set(ctx, key, b, time.Duration(ttl)*time.Second).Err()
	if err != nil {
		util.RecordSpanError(span, err)
		return err
	}
	return nil
}

// value must be non-nil pointer to destination type
func (r *RedisClient) Get(ctx context.Context, key string, value interface{}) error {
	tracer := job_tracer.GetTracer()
	ctx, span := tracer.Start(ctx, "Redis/Get")
	defer span.End()
	if key == "" {
		err := fmt.Errorf("key cannot be empty")
		util.RecordSpanError(span, err)
		return err
	}
	span.AddEvent("redis.context",
		trace.WithAttributes(attribute.String("key", key)),
	)

	val, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		err := fmt.Errorf("failed to retrieve value for key %s: %w", key, err)
		util.RecordSpanError(span, err)
		return err
	}
	err = msgpack.Unmarshal(val, value)
	if err != nil {
		err := fmt.Errorf("failed to unmarshal value for key %s: %w", key, err)
		util.RecordSpanError(span, err)
		return err
	}
	return nil
}

func (r *RedisClient) GetDefaultTTL() int {
	return r.ttl
}

func (r *RedisClient) Close() error {
	return r.client.Close()
}
