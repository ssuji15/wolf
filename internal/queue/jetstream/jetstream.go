package jetstream

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/ssuji15/wolf/internal/component/jetstream"
	"github.com/ssuji15/wolf/internal/job_tracer"
	"github.com/ssuji15/wolf/internal/queue"
	"github.com/ssuji15/wolf/internal/service/logger"
	"github.com/ssuji15/wolf/internal/util"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
)

type JetStreamQueueClient struct {
	connection *nats.Conn
	context    nats.JetStreamContext
}

var (
	jqc       *JetStreamQueueClient
	once      sync.Once
	initError error
)

type NatsSubscription struct {
	sub *nats.Subscription
}

type NatsData struct {
	msg  *nats.Msg
	meta *nats.MsgMetadata
	ctx  context.Context
}

func NewJetStreamQueueClient() (queue.Queue, error) {
	once.Do(func() {
		nc, err := jetstream.NewJetStreamClient()
		if err != nil {
			initError = err
			return
		}
		js, err := nc.JetStream()
		if err != nil {
			initError = err
			return
		}
		_, err = js.AddStream(&nats.StreamConfig{
			Name:     string(queue.EventStream),
			Subjects: []string{"events.>"},
		})
		if err != nil && !errors.Is(err, nats.ErrStreamNameAlreadyInUse) {
			initError = err
			return
		}
		jqc = &JetStreamQueueClient{
			connection: nc,
			context:    js,
		}
	})
	return jqc, initError
}

func (j *JetStreamQueueClient) AddConsumer(stream queue.QueueEvent, consumer string) error {
	_, err := j.context.AddConsumer(string(stream), &nats.ConsumerConfig{
		Durable:    consumer,
		AckPolicy:  nats.AckExplicitPolicy,
		AckWait:    2 * time.Second,
		MaxDeliver: queue.MaxDeliver,
		BackOff: []time.Duration{
			1 * time.Second,
			3 * time.Second,
			5 * time.Second,
		},
		DeliverPolicy: nats.DeliverNewPolicy,
	})
	if err != nil && !strings.Contains(err.Error(), "consumer name already in use") {
		return err
	}
	return nil
}

func (j *JetStreamQueueClient) PublishEvent(pctx context.Context, event queue.QueueEvent, id string) error {
	if id == "" {
		return fmt.Errorf("id cannot be empty")
	}
	tracer := job_tracer.GetTracer()
	_, span := tracer.Start(pctx, "Jetstream/Publish")
	defer span.End()

	span.SetAttributes(
		attribute.String("event", string(event)),
	)

	headers := nats.Header{}
	propagation.TraceContext{}.Inject(pctx, propagation.HeaderCarrier(headers))

	msg := &nats.Msg{
		Subject: string(event),
		Header:  headers,
		Data:    []byte(id),
	}

	_, err := j.context.PublishMsg(msg)
	if err != nil {
		util.RecordSpanError(span, err)
	}
	return err
}

func (j *JetStreamQueueClient) SubscribeEvent(event queue.QueueEvent, consumer string) (queue.Subscription, error) {
	sub, err := j.context.PullSubscribe(string(event), consumer, nats.ManualAck(), nats.AckExplicit())
	if err != nil {
		return nil, err
	}
	return &NatsSubscription{
		sub: sub,
	}, nil
}

func (j *JetStreamQueueClient) Close() error {
	return j.connection.Drain()
}

func (j *JetStreamQueueClient) ShutDown(ctx context.Context) {
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

func (j *JetStreamQueueClient) GetPendingMessagesForConsumer(stream queue.QueueEvent, consumer string) (uint64, error) {
	consumerInfo, err := j.context.ConsumerInfo(string(stream), consumer)
	if err != nil {
		return 0, err
	}
	return consumerInfo.NumPending, nil
}

func (s *NatsSubscription) Fetch(ctx context.Context, count int, timeout time.Duration) ([]queue.QMsg, error) {
	msgs, err := s.sub.Fetch(count, nats.MaxWait(timeout))
	if err != nil {
		return nil, err
	}

	var qMsg []queue.QMsg

	for _, msg := range msgs {
		meta, err := msg.Metadata()
		if err != nil {
			return nil, err
		}
		qMsg = append(qMsg, &NatsData{msg: msg, meta: meta, ctx: getCtx(ctx, msg)})
	}
	return qMsg, nil
}

func (d *NatsData) Data() []byte {
	return d.msg.Data
}

func (d *NatsData) Ack() error {
	return d.msg.Ack()
}

func (d *NatsData) PublishedAt() time.Time {
	return d.meta.Timestamp
}

func (d *NatsData) Term() error {
	return d.msg.Term()
}

func (d *NatsData) Ctx() context.Context {
	return d.ctx
}

func getCtx(ctx context.Context, msg *nats.Msg) context.Context {
	headers := propagation.MapCarrier(natsHeaderToMapStringString(msg.Header))
	return otel.GetTextMapPropagator().Extract(
		ctx,
		headers,
	)
}

func (d *NatsData) RetryCount() int {
	return int(d.meta.NumDelivered)
}

func natsHeaderToMapStringString(h nats.Header) map[string]string {
	result := make(map[string]string)

	for key, values := range h {
		// We only take the first value encountered for that key.
		if len(values) > 0 {
			result[key] = values[0]
		}
	}
	return result
}
