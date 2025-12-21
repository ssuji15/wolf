package cache

import "context"

type Cache interface {
	Put(context.Context, string, interface{}, int) error
	Get(context.Context, string, interface{}) error
	GetDefaultTTL() int
}
