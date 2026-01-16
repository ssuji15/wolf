package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/ssuji15/wolf/internal/cache"
	rclient "github.com/ssuji15/wolf/internal/component/redis"
	"github.com/ssuji15/wolf/internal/config"
	"github.com/ssuji15/wolf/internal/job_tracer"
	"github.com/ssuji15/wolf/internal/service/logger"
	"github.com/ssuji15/wolf/internal/util"
	"github.com/vmihailenco/msgpack/v5"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type RedisCacheClient struct {
	client *redis.Client
	ttl    int
}

var (
	rcc *RedisCacheClient
)

func NewRedisCacheClient(ctx context.Context) (cache.Cache, error) {
	if rcc != nil {
		return rcc, nil
	}
	rc, err := rclient.NewRedisClient(ctx)
	if err != nil {
		return nil, err
	}
	cfg, err := config.GetRedisCacheConfig()
	if err != nil {
		return nil, err
	}
	rcc = &RedisCacheClient{
		client: rc,
		ttl:    cfg.TTL,
	}
	return rcc, nil
}

func (r *RedisCacheClient) Put(ctx context.Context, key string, value interface{}, ttl int) error {
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
func (r *RedisCacheClient) Get(ctx context.Context, key string, value interface{}) error {
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

func (r *RedisCacheClient) GetDefaultTTL() int {
	return r.ttl
}

func (r *RedisCacheClient) Close() error {
	return r.client.Close()
}

func (r *RedisCacheClient) ShutDown(ctx context.Context) {
	done := make(chan struct{})

	go func() {
		if err := r.Close(); err != nil {
			logger.Log.Err(err).Msg("unable to close redis")
		}
		close(done)
	}()

	select {
	case <-done:
		return
	case <-ctx.Done():
		return
	}
}
