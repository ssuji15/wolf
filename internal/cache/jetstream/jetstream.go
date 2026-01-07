package jetstream

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/ssuji15/wolf/internal/cache"
	"github.com/ssuji15/wolf/internal/component/jetstream"
	"github.com/ssuji15/wolf/internal/config"
	"github.com/ssuji15/wolf/internal/job_tracer"
	"github.com/ssuji15/wolf/internal/service/logger"
	"github.com/ssuji15/wolf/internal/util"
	"github.com/vmihailenco/msgpack/v5"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type JetStreamCacheClient struct {
	connection *nats.Conn
	jContext   nats.JetStreamContext
	bucket     nats.ObjectStore
	ttl        int
}

var (
	jcc       *JetStreamCacheClient
	once      sync.Once
	initError error
)

func NewJetStreamCacheClient() (cache.Cache, error) {
	once.Do(func() {
		nc, err := jetstream.NewJetStreamClient()
		if err != nil {
			initError = err
			return
		}
		cfg, err := config.GetNatsConfig()
		if err != nil {
			initError = err
			return
		}
		js, err := nc.JetStream()
		if err != nil {
			initError = err
			return
		}
		os, err := createOrGetObjectStore(js, cfg.BUCKET_NAME, cfg.TTL, cfg.BUCKET_SIZE_BYTES)
		if err != nil {
			initError = err
			return
		}
		jcc = &JetStreamCacheClient{
			connection: nc,
			jContext:   js,
			bucket:     os,
			ttl:        cfg.TTL,
		}
	})
	return jcc, initError
}

func (j *JetStreamCacheClient) Put(ctx context.Context, key string, value interface{}, ttl int) error {
	tracer := job_tracer.GetTracer()
	ctx, span := tracer.Start(ctx, "Nats/Put")
	defer span.End()

	if key == "" {
		err := fmt.Errorf("key cannot be empty")
		util.RecordSpanError(span, err)
		return err
	}
	span.AddEvent("nats.context",
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

	_, err = j.bucket.PutBytes(key, b)
	if err != nil {
		util.RecordSpanError(span, err)
		return err
	}
	return nil
}

func (j *JetStreamCacheClient) Get(ctx context.Context, key string, value interface{}) error {
	tracer := job_tracer.GetTracer()
	ctx, span := tracer.Start(ctx, "nats/Get")
	defer span.End()
	if key == "" {
		err := fmt.Errorf("key cannot be empty")
		util.RecordSpanError(span, err)
		return err
	}
	span.AddEvent("nats.context",
		trace.WithAttributes(attribute.String("key", key)),
	)

	b, err := j.bucket.GetBytes(key)
	if err != nil {
		err := fmt.Errorf("failed to retrieve value for key %s: %w", key, err)
		util.RecordSpanError(span, err)
		return err
	}
	err = msgpack.Unmarshal(b, value)
	if err != nil {
		err := fmt.Errorf("failed to unmarshal value for key %s: %w", key, err)
		util.RecordSpanError(span, err)
		return err
	}
	return nil
}

func (j *JetStreamCacheClient) GetDefaultTTL() int {
	return j.ttl
}

func (j *JetStreamCacheClient) Close() error {
	return j.connection.Drain()
}

func createOrGetObjectStore(js nats.JetStreamContext, bucket string, ttlSeconds int, bucketSizeBytes int) (nats.ObjectStore, error) {
	var os nats.ObjectStore
	os, err := js.ObjectStore(bucket)
	if err != nil {
		if err == nats.ErrStreamNotFound {
			os, err = js.CreateObjectStore(&nats.ObjectStoreConfig{
				Bucket:      bucket,
				Description: "Bucket to store objects",
				TTL:         time.Duration(ttlSeconds) * time.Second,
				MaxBytes:    int64(bucketSizeBytes),
				Storage:     nats.FileStorage,
				Compression: false,
			})
			if err != nil {
				return nil, fmt.Errorf("could not create nats bucket: %v", err)
			}
			return os, nil
		}
		return nil, fmt.Errorf("error retrieving nats bucket instance: %v", err)
	}
	return os, nil
}

func (j *JetStreamCacheClient) ShutDown(ctx context.Context) {
	done := make(chan struct{})
	j.connection.SetClosedHandler(func(_ *nats.Conn) {
		close(done)
	})

	if err := j.Close(); err != nil {
		logger.Log.Err(err).Msg("unable to close nats connection")
	}

	select {
	case <-done:
		return
	case <-ctx.Done():
		j.connection.Close()
	}
}
