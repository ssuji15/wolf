package queue

import (
	"context"
	"time"
)

type Queue interface {
	PublishEvent(context.Context, QueueEvent, string) error
	SubscribeEvent(QueueEvent, string) (Subscription, error)
	AddConsumer(QueueEvent, string, []time.Duration, int) error
	ShutDown(context.Context)
	Init() error
}

type QueueEvent string

const (
	EventStream       QueueEvent = "EVENTS"
	JobCreated        QueueEvent = "events.job.created"
	DeadLetterQueue   QueueEvent = "DLQ.job"
	MaxDeliver        int        = 5
	WORKER_CONSUMER   string     = "worker"
	JOB_DB_CONSUMER   string     = "JOB_DB"
	JOB_CODE_CONSUMER string     = "JOB_CODE"
)

type Subscription interface {
	Fetch(context.Context, int, time.Duration) ([]QMsg, error)
}

type QMsg interface {
	Data() []byte
	PublishedAt() time.Time
	Ctx() context.Context
	RetryCount() int
	Ack() error
	Term() error
}
