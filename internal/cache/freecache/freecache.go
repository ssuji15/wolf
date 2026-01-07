package freecache

import (
	"bytes"
	"context"
	"encoding/gob"
	"fmt"
	"sync"

	fc "github.com/coocood/freecache"
	"github.com/ssuji15/wolf/internal/cache"
	"github.com/ssuji15/wolf/internal/config"
)

type FreeCache struct {
	cache *fc.Cache
	ttl   int // seconds
}

var (
	fcc       *FreeCache
	once      sync.Once
	initError error
)

func NewFreeCache() (cache.Cache, error) {
	once.Do(func() {
		cfg, err := config.GetFreeCacheConfig()
		if err != nil {
			initError = err
			return
		}
		fcc = &FreeCache{
			cache: fc.NewCache(cfg.SIZE_BYTES),
			ttl:   cfg.TTL,
		}
	})
	return fcc, initError
}

func (c *FreeCache) Put(ctx context.Context, key string, value interface{}, ttlSeconds int) error {
	if key == "" {
		return fmt.Errorf("key cannot be empty")
	}
	if value == nil {
		return fmt.Errorf("value cannot be nil")
	}
	data, err := encode(value)
	if err != nil {
		return err
	}

	return c.cache.Set([]byte(key), data, ttlSeconds)
}

func (c *FreeCache) Get(ctx context.Context, key string, out interface{}) error {
	if key == "" {
		return fmt.Errorf("key cannot be empty")
	}
	data, err := c.cache.Get([]byte(key))
	if err != nil {
		return err
	}
	return decode(data, out)
}

func encode(value interface{}) ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(value); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func decode(data []byte, out interface{}) error {
	buf := bytes.NewBuffer(data)
	dec := gob.NewDecoder(buf)
	return dec.Decode(out)
}

func (c *FreeCache) GetDefaultTTL() int {
	return c.ttl
}

func (c *FreeCache) ShutDown(ctx context.Context) {
	c.cache.Clear()
}
