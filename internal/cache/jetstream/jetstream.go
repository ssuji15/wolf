package jetstream

import (
	"context"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/ssuji15/wolf/internal/cache"
	"github.com/ssuji15/wolf/internal/config"
	"github.com/ssuji15/wolf/internal/job_tracer"
	"github.com/ssuji15/wolf/internal/service/logger"
	"github.com/ssuji15/wolf/internal/util"
	"github.com/vmihailenco/msgpack/v5"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type JetStreamClient struct {
	connection *nats.Conn
	context    nats.JetStreamContext
	bucket     nats.ObjectStore
	ttl        int
}

func NewJetStreamClient(ctx context.Context, cfg *config.Config) (cache.Cache, error) {
	nc, err := nats.Connect(cfg.JetstreamURL,
		nats.MaxReconnects(-1),            // infinite retries
		nats.ReconnectWait(1*time.Second), // backoff
		nats.Name("wolf-cache"),
		nats.ReconnectErrHandler(func(nc *nats.Conn, err error) {
			logger.Log.Error().Err(err).Msg("NATs reconnected")
		}),
		nats.DisconnectErrHandler(func(nc *nats.Conn, err error) {
			logger.Log.Error().Err(err).Msg("NATs disconnected")
		}),
		nats.ClosedHandler(func(nc *nats.Conn) {
			logger.Log.Error().Msg("NATs closed")
		}),
	)
	if err != nil {
		return nil, err
	}

	js, err := nc.JetStream()
	if err != nil {
		return nil, err
	}

	os, err := createOrGetObjectStore(js, "Default", cfg.CacheTTL)
	if err != nil {
		return nil, err
	}
	return &JetStreamClient{
		connection: nc,
		context:    js,
		bucket:     os,
	}, nil
}

func (j *JetStreamClient) Put(ctx context.Context, key string, value interface{}, ttl int) error {
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

func (j *JetStreamClient) Get(ctx context.Context, key string, value interface{}) error {
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

func (j *JetStreamClient) GetDefaultTTL() int {
	return j.ttl
}

func (r *JetStreamClient) Close() error {
	return r.connection.Drain()
}

func createOrGetObjectStore(js nats.JetStreamContext, bucket string, ttlSeconds int) (nats.ObjectStore, error) {
	var os nats.ObjectStore
	os, err := js.ObjectStore(bucket)
	if err != nil {
		if err == nats.ErrStreamNotFound {
			os, err = js.CreateObjectStore(&nats.ObjectStoreConfig{
				Bucket:      bucket,
				Description: "Bucket to store objects",
				TTL:         time.Duration(ttlSeconds) * time.Second,
				MaxBytes:    8 * 1024 * 1024 * 1024,
				Storage:     nats.FileStorage,
				Compression: false,
			})
			if err != nil {
				return nil, fmt.Errorf("could not create NATs bucket: %v", err)
			}
			return os, nil
		}
		return nil, fmt.Errorf("Error retrieving Nats bucket instance: %v", err)
	}
	return os, nil
}
