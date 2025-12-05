package jetstream

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/ssuji15/wolf/internal/job_tracer"
	"github.com/ssuji15/wolf/internal/queue"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
)

type JetStreamClient struct {
	connection *nats.Conn
	context    nats.JetStreamContext
}

func NewJetStreamClient(url string) (queue.Queue, error) {
	nc, err := nats.Connect(url,
		nats.MaxReconnects(-1),            // infinite retries
		nats.ReconnectWait(2*time.Second), // backoff
		nats.Name("wolf"),
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
		AckWait:    20 * time.Second, // retry every 20s
		MaxDeliver: 5,                // stop retrying after 5 attempts
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
	tracer := job_tracer.GetTracer()
	_, span := tracer.Start(pctx, "Jetstream/Publish")
	defer span.End()

	span.SetAttributes(
		attribute.String("id", id),
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
		span.RecordError(err)
	}
	return err
}

func (c *JetStreamClient) SubscribeEventToWorker(event queue.QueueEvent) (*nats.Subscription, error) {
	return c.context.PullSubscribe(string(event), "worker", nats.ManualAck(), nats.AckExplicit())
}

func (c *JetStreamClient) Shutdown() {
	c.connection.Drain() // flush + stop new messages
	c.connection.Close()
}

func (c *JetStreamClient) GetPendingMessagesForConsumer(stream queue.QueueEvent, consumer string) (uint64, error) {
	consumerInfo, err := c.context.ConsumerInfo(string(stream), consumer)
	if err != nil {
		return 0, err
	}
	return consumerInfo.NumPending, nil
}
