package queue

type Queue interface {
	PublishEvent(QueueEvent, string) error
	Shutdown()
}

type QueueEvent string

const (
	JobCreated QueueEvent = "events.job.created"
)
