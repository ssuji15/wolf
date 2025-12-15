package storage

import "context"

type Storage interface {
	Upload(context.Context, string, []byte) error
	Download(context.Context, string) ([]byte, error)
	Close()
}
