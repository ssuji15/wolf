package queue

type Queue interface {
	PublishEvent(QueueEvent, string) error
	SubscribeEvent(QueueEvent, func(string) error) error
	Shutdown()
}

type QueueEvent string

const (
	JobCreated QueueEvent = "events.job.created"
)
