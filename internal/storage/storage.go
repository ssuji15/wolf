package storage

import (
	"context"
	"errors"
)

type Storage interface {
	Upload(context.Context, string, string, []byte) error
	Download(context.Context, string, string) ([]byte, error)
	GetJobsBucket() string
	ShutDown(context.Context)
	Close()
}

var (
	ErrNotfound = errors.New("invalid path")
)
