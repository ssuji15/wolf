package jetstream

import (
	"errors"
	"log"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/ssuji15/wolf/internal/queue"
	"github.com/ssuji15/wolf/model"
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

func (c *JetStreamClient) PublishEvent(event queue.QueueEvent, id string) error {
	_, err := c.context.Publish(string(event), []byte(id))
	return err
}

func (c *JetStreamClient) SubscribeEventToWorker(event queue.QueueEvent, handler func(id string, worker model.WorkerMetadata) error, getWorker func() model.WorkerMetadata, addWorker func(model.WorkerMetadata)) error {
	sub, err := c.context.PullSubscribe(string(event), "worker", nats.ManualAck(), nats.AckExplicit())

	if err != nil {
		return err
	}

	go func() {
		for {
			worker := getWorker()
			msgs, err := sub.Fetch(1, nats.MaxWait(30*time.Second))
			if err != nil {
				addWorker(worker)
				if errors.Is(err, nats.ErrTimeout) {
					continue
				}
				if errors.Is(err, nats.ErrConnectionClosed) {
					break
				}
				time.Sleep(time.Second)
				continue
			}

			msg := msgs[0]
			go func() {
				id := string(msg.Data)
				if err := handler(id, worker); err != nil {
					log.Printf("Failed to handle: %s, err: %v", id, err)
					msg.Nak()
					return
				}
				msg.Ack()
			}()
		}
	}()
	return nil
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
