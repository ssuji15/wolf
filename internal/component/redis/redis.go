package redis

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/ssuji15/wolf/internal/config"
)

var (
	rc        *redis.Client
	once      sync.Once
	initError error
)

func NewRedisClient(ctx context.Context) (*redis.Client, error) {

	once.Do(func() {
		config, err := config.GetRedisConfig()
		if err != nil {
			initError = err
			return
		}

		rc = redis.NewClient(&redis.Options{
			Addr:            config.URL,
			Password:        config.ClientPassword,
			DB:              0,
			PoolSize:        50,
			MinIdleConns:    10,
			PoolTimeout:     1 * time.Second,
			MinRetryBackoff: 100 * time.Millisecond,
			MaxRetryBackoff: 500 * time.Millisecond,
			ConnMaxIdleTime: 10 * time.Minute,
			ConnMaxLifetime: 30 * time.Minute,
		})

		ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()

		err = rc.Ping(ctx).Err()
		if err != nil {
			initError = fmt.Errorf("failed to connect to redis: %v", err)
			return
		}
	})

	return rc, initError
}

func ResetRedisClient() {
	rc = nil
	once = sync.Once{}
	initError = nil
}
