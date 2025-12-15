package queue

import (
	"context"

	"github.com/nats-io/nats.go"
)

type Queue interface {
	PublishEvent(context.Context, QueueEvent, string) error
	SubscribeEventToWorker(QueueEvent) (*nats.Subscription, error)
	GetPendingMessagesForConsumer(QueueEvent, string) (uint64, error)
	Shutdown()
}

type QueueEvent string

const (
	JobCreated      QueueEvent = "events.job.created"
	DeadLetterQueue QueueEvent = "DLQ.job"
	MaxDeliver      int        = 3
)
