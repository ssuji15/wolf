package queue

import "github.com/ssuji15/wolf/model"

type Queue interface {
	PublishEvent(QueueEvent, string) error
	SubscribeEventToWorker(QueueEvent, func(string, model.WorkerMetadata) error, func() model.WorkerMetadata, func(model.WorkerMetadata)) error
	GetPendingMessagesForConsumer(QueueEvent, string) (uint64, error)
	Shutdown()
}

type QueueEvent string

const (
	JobCreated QueueEvent = "events.job.created"
)
