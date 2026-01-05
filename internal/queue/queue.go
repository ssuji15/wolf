package queue

import (
	"context"
	"time"
)

type Queue interface {
	PublishEvent(context.Context, QueueEvent, string) error
	SubscribeEvent(QueueEvent, string) (Subscription, error)
	AddConsumer(QueueEvent, string) error
	GetPendingMessagesForConsumer(QueueEvent, string) (uint64, error)
	Shutdown() error
}

type QueueEvent string

const (
	EventStream     QueueEvent = "EVENTS"
	JobCreated      QueueEvent = "events.job.created"
	DeadLetterQueue QueueEvent = "DLQ.job"
	MaxDeliver      int        = 3
)

type Subscription interface {
	Fetch(int, time.Duration) ([]QMsg, error)
}

type QMsg interface {
	Data() []byte
	PublishedAt() time.Time
	Ctx() context.Context
	RetryCount() int
	Ack() error
	Term() error
}
