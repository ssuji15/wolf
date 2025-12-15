package freecache

import (
	"bytes"
	"encoding/gob"

	fc "github.com/coocood/freecache"
	"github.com/ssuji15/wolf/internal/cache"
)

type FreeCache struct {
	cache *fc.Cache
	ttl   int // seconds
}

func NewFreeCache(sizeBytes int, ttlSeconds int) cache.Cache {
	return &FreeCache{
		cache: fc.NewCache(sizeBytes),
		ttl:   ttlSeconds,
	}
}

func (c *FreeCache) Put(key string, value interface{}, ttlSeconds int) error {
	data, err := encode(value)
	if err != nil {
		return err
	}

	return c.cache.Set([]byte(key), data, ttlSeconds)
}

func (c *FreeCache) Get(key string, out interface{}) error {
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
