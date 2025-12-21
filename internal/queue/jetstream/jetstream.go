package jetstream

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/ssuji15/wolf/internal/job_tracer"
	"github.com/ssuji15/wolf/internal/queue"
	"github.com/ssuji15/wolf/internal/service/logger"
	"github.com/ssuji15/wolf/internal/util"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
)

type JetStreamClient struct {
	connection *nats.Conn
	context    nats.JetStreamContext
}

type NatsSubscription struct {
	sub *nats.Subscription
}

type NatsData struct {
	msg  *nats.Msg
	meta *nats.MsgMetadata
	ctx  context.Context
}

func NewJetStreamClient(url string) (queue.Queue, error) {
	nc, err := nats.Connect(url,
		nats.MaxReconnects(-1),            // infinite retries
		nats.ReconnectWait(2*time.Second), // backoff
		nats.Name("wolf"),
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

	_, err = js.AddStream(&nats.StreamConfig{
		Name:     "EVENTS",
		Subjects: []string{"events.>"},
	})

	if err != nil && !errors.Is(err, nats.ErrStreamNameAlreadyInUse) {
		return nil, err
	}

	_, err = js.AddConsumer("EVENTS", &nats.ConsumerConfig{
		Durable:    "worker",
		AckPolicy:  nats.AckExplicitPolicy,
		AckWait:    2 * time.Second,
		MaxDeliver: queue.MaxDeliver,
		BackOff: []time.Duration{
			5 * time.Second,
			15 * time.Second,
			30 * time.Second,
		},
		DeliverPolicy: nats.DeliverNewPolicy,
	})

	if err != nil && !strings.Contains(err.Error(), "consumer name already in use") {
		return nil, err
	}

	return &JetStreamClient{
		connection: nc,
		context:    js,
	}, nil
}

func (c *JetStreamClient) PublishEvent(pctx context.Context, event queue.QueueEvent, id string) error {
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

	_, err := c.context.PublishMsg(msg)
	if err != nil {
		util.RecordSpanError(span, err)
	}
	return err
}

func (c *JetStreamClient) SubscribeEvent(event queue.QueueEvent) (queue.Subscription, error) {
	sub, err := c.context.PullSubscribe(string(event), "worker", nats.ManualAck(), nats.AckExplicit())
	if err != nil {
		return nil, err
	}
	return &NatsSubscription{
		sub: sub,
	}, nil
}

func (c *JetStreamClient) Shutdown() error {
	return c.connection.Drain() // flush + stop new messages
}

func (c *JetStreamClient) GetPendingMessagesForConsumer(stream queue.QueueEvent, consumer string) (uint64, error) {
	consumerInfo, err := c.context.ConsumerInfo(string(stream), consumer)
	if err != nil {
		return 0, err
	}
	return consumerInfo.NumPending, nil
}

func (s *NatsSubscription) Fetch(count int, timeout time.Duration) (queue.QMsg, error) {
	msgs, err := s.sub.Fetch(count, nats.MaxWait(timeout))
	if err != nil {
		return nil, err
	}
	msg := msgs[0]
	meta, err := msg.Metadata()
	if err != nil {
		return nil, err
	}
	return &NatsData{msg: msg, meta: meta, ctx: getCtx(msg)}, nil
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
