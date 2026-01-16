package jetstream

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/ssuji15/wolf/internal/component/jetstream"
	"github.com/ssuji15/wolf/internal/config"
	"github.com/ssuji15/wolf/internal/job_tracer"
	"github.com/ssuji15/wolf/internal/queue"
	"github.com/ssuji15/wolf/internal/service/logger"
	"github.com/ssuji15/wolf/internal/util"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

type JetStreamQueueClient struct {
	connection *nats.Conn
	context    nats.JetStreamContext
}

var (
	jqc *JetStreamQueueClient
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
	if jqc != nil {
		return jqc, nil
	}
	nc, err := jetstream.NewJetStreamClient()
	if err != nil {
		return nil, err
	}
	js, err := nc.JetStream()
	if err != nil {
		return nil, err
	}
	jqc = &JetStreamQueueClient{
		connection: nc,
		context:    js,
	}
	err = jqc.init()
	if err != nil {
		jqc = nil
		return nil, err
	}
	return jqc, nil
}

func (j *JetStreamQueueClient) AddStream(name string, subjects []string, maxMsg int) error {
	if maxMsg < 1 {
		return fmt.Errorf("invalid maxMsg")
	}
	_, err := j.context.AddStream(&nats.StreamConfig{
		Name:      name,
		Subjects:  subjects,
		MaxMsgs:   int64(maxMsg),
		Retention: nats.InterestPolicy,
		Discard:   nats.DiscardNew,
	})
	if err != nil && !errors.Is(err, nats.ErrStreamNameAlreadyInUse) {
		return err
	}
	return nil
}

func (j *JetStreamQueueClient) AddConsumer(stream queue.QueueEvent, consumer string, backoff []time.Duration, maxDeliver int) error {
	if consumer == "" {
		return fmt.Errorf("consumer is empty")
	}
	if maxDeliver <= 0 {
		return fmt.Errorf("invalid maxDeliver value")
	}
	_, err := j.context.AddConsumer(string(stream), &nats.ConsumerConfig{
		Durable:       consumer,
		AckPolicy:     nats.AckExplicitPolicy,
		MaxDeliver:    maxDeliver,
		BackOff:       backoff,
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
	ctx, span := tracer.Start(pctx, "Jetstream/Publish")
	defer span.End()

	span.AddEvent("Jetstream/Publish",
		trace.WithAttributes(attribute.String("event", string(event))),
	)

	headers := nats.Header{}
	propagation.TraceContext{}.Inject(ctx, propagation.HeaderCarrier(headers))

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

func (j *JetStreamQueueClient) init() error {
	cfg, err := config.GetNatsQueueConfig()
	if err != nil {
		return err
	}
	err = j.AddStream(string(queue.EventStream), []string{"events.>"}, cfg.MAX_MESSAGES_JOB_QUEUE)
	if err != nil {
		return err
	}

	err = j.AddConsumer(queue.EventStream, queue.WORKER_CONSUMER, []time.Duration{
		2 * time.Second,
		4 * time.Second,
		30 * time.Second,
	}, 5)
	if err != nil {
		return err
	}

	err = j.AddConsumer(queue.EventStream, queue.JOB_DB_CONSUMER, []time.Duration{
		1 * time.Second,
		3 * time.Second,
		15 * time.Second,
		30 * time.Second,
	}, 5)
	if err != nil {
		return err
	}

	err = j.AddConsumer(queue.EventStream, queue.JOB_CODE_CONSUMER, []time.Duration{
		1 * time.Second,
		3 * time.Second,
		15 * time.Second,
		30 * time.Second,
	}, 5)
	if err != nil {
		return err
	}

	return nil
}

func (s *NatsSubscription) Fetch(count int, timeout time.Duration) ([]queue.QMsg, error) {
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
		qMsg = append(qMsg, &NatsData{msg: msg, meta: meta, ctx: getCtx(msg)})
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

func getCtx(msg *nats.Msg) context.Context {
	headers := propagation.MapCarrier(natsHeaderToMapStringString(msg.Header))
	return otel.GetTextMapPropagator().Extract(
		context.Background(),
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
