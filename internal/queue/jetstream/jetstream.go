package jetstream

import (
	"time"

	"github.com/nats-io/nats.go"
	"github.com/ssuji15/wolf/internal/queue"
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

	if err != nil {
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

func (c *JetStreamClient) Shutdown() {
	c.connection.Drain() // flush + stop new messages
	c.connection.Close()
}
