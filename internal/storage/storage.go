package storage

import "context"

type Storage interface {
	Upload(context.Context, string, string, []byte) error
	Download(context.Context, string, string) ([]byte, error)
	GetJobsBucket() string
	ShutDown(context.Context)
	Close()
}
