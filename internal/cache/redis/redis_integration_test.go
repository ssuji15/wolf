//go:build integration
// +build integration

package redis

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/ssuji15/wolf/internal/component/redis"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

var (
	redisContainer testcontainers.Container
	REDIS_ENDPOINT string
)

// ------------------------
// TestMain: spin up Redis container
// ------------------------
func TestMain(m *testing.M) {
	flag.Parse()

	if testing.Short() {
		fmt.Println("skipping integration tests")
		os.Exit(0)
	}

	ctx := context.Background()
	req := testcontainers.ContainerRequest{
		Image:        "redis:latest",
		ExposedPorts: []string{"6379/tcp"},
		WaitingFor:   wait.ForListeningPort("6379/tcp").WithStartupTimeout(30 * time.Second),
	}

	var err error
	redisContainer, err = testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		panic(err)
	}

	host, err := redisContainer.Host(ctx)
	if err != nil {
		panic(err)
	}

	port, err := redisContainer.MappedPort(ctx, "6379")
	if err != nil {
		panic(err)
	}

	REDIS_ENDPOINT = fmt.Sprintf("%s:%s", host, port.Port())

	code := m.Run()
	_ = redisContainer.Terminate(ctx)
	os.Exit(code)
}

// ------------------------
// Singleton reset
// ------------------------
func resetRedisSingleton() {
	rcc = nil
	initError = nil
	once = sync.Once{}
}

// ------------------------
// Env setup
// ------------------------
func setRedisEnv() {
	os.Setenv("REDIS_ENDPOINT", REDIS_ENDPOINT)
	os.Setenv("REDIS_TTL", "2") // short TTL for tests
}

// ------------------------
// 1. NewRedisCacheClient tests
// ------------------------
func TestNewRedisCacheClient(t *testing.T) {
	tests := []struct {
		name      string
		unsetEnv  string
		expectErr bool
	}{
		{"All env set succeeds", "", false},
		{"Missing REDIS_ENDPOINT fails", "REDIS_ENDPOINT", true},
		{"Invalid TTL fails", "REDIS_TTL", true}, // we'll unset then set invalid TTL
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			resetRedisSingleton()
			setRedisEnv()
			redis.ResetRedisClient()
			if tt.unsetEnv != "" {
				os.Unsetenv(tt.unsetEnv)
				if tt.name == "Invalid TTL fails" {
					os.Setenv("REDIS_TTL", "invalidTTL")
				}
			}

			ctx := context.Background()
			client, err := NewRedisCacheClient(ctx)
			if tt.expectErr {
				require.Error(t, err)
				require.Nil(t, client)
			} else {
				require.NoError(t, err)
				require.NotNil(t, client)
				require.Equal(t, 2, client.GetDefaultTTL())
			}
		})
	}
}

// ------------------------
// 2. Put tests
// ------------------------
func TestRedisCacheClient_Put(t *testing.T) {
	resetRedisSingleton()
	setRedisEnv()

	ctx := context.Background()
	c, err := NewRedisCacheClient(ctx)
	require.NoError(t, err)

	tests := []struct {
		name      string
		key       string
		value     interface{}
		expectErr bool
	}{
		{"Empty key fails", "", "value", true},
		{"Nil value fails", "nil_val", nil, true},
		{"Valid string succeeds", "key1", "value1", false},
		{"Struct value succeeds", "user:1", struct{ Name string }{Name: "Alice"}, false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := c.Put(ctx, tt.key, tt.value, c.GetDefaultTTL())
			if tt.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// ------------------------
// 3. Get tests
// ------------------------
func TestRedisCacheClient_Get(t *testing.T) {
	resetRedisSingleton()
	setRedisEnv()

	ctx := context.Background()
	c, err := NewRedisCacheClient(ctx)
	require.NoError(t, err)

	// seed values
	require.NoError(t, c.Put(ctx, "key1", "value1", c.GetDefaultTTL()))
	require.NoError(t, c.Put(ctx, "user:1", struct{ Name string }{Name: "Alice"}, c.GetDefaultTTL()))

	tests := []struct {
		name      string
		key       string
		expected  interface{}
		expectErr bool
	}{
		{"Empty key fails", "", nil, true},
		{"Key not present fails", "missing", nil, true},
		{"Get string succeeds", "key1", "value1", false},
		{"Get struct succeeds", "user:1", struct{ Name string }{Name: "Alice"}, false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			var out interface{}
			switch tt.expected.(type) {
			case string:
				var s string
				err := c.Get(ctx, tt.key, &s)
				out = s
				if tt.expectErr {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
					require.Equal(t, tt.expected, out)
				}
			case struct{ Name string }:
				var u struct{ Name string }
				err := c.Get(ctx, tt.key, &u)
				out = u
				if tt.expectErr {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
					require.Equal(t, tt.expected, out)
				}
			default:
				if tt.expectErr {
					var tmp string
					err := c.Get(ctx, tt.key, &tmp)
					require.Error(t, err)
				}
			}
		})
	}
}

// ------------------------
// 4. TTL tests
// ------------------------
func TestRedisCacheClient_TTL(t *testing.T) {
	resetRedisSingleton()
	setRedisEnv()

	ctx := context.Background()
	c, err := NewRedisCacheClient(ctx)
	require.NoError(t, err)

	ttl := c.GetDefaultTTL()
	require.Equal(t, 2, ttl)

	tests := []struct {
		name        string
		key         string
		value       string
		sleepBefore time.Duration
		expectErr   bool
	}{
		{"Value expires after TTL", "temp", "shortlived", time.Duration(ttl+1) * time.Second, true},
		{"Value accessible before TTL", "persistent", "longlived", time.Duration(ttl/2) * time.Second, false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := c.Put(ctx, tt.key, tt.value, ttl)
			require.NoError(t, err)

			time.Sleep(tt.sleepBefore)

			var out string
			err = c.Get(ctx, tt.key, &out)
			if tt.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.value, out)
			}
		})
	}
}

// ------------------------
// 5. Shutdown tests
// ------------------------
func TestRedisCacheClient_Shutdown_TableDriven(t *testing.T) {
	resetRedisSingleton()
	setRedisEnv()

	ctx := context.Background()
	c, _ := NewRedisCacheClient(ctx)

	// prepopulate keys
	require.NoError(t, c.Put(ctx, "key1", "value1", c.GetDefaultTTL()))
	require.NoError(t, c.Put(ctx, "key2", "value2", c.GetDefaultTTL()))

	c.ShutDown(ctx)

	tests := []struct {
		name string
		key  string
	}{
		{"Key1 fails after shutdown", "key1"},
		{"Key2 fails after shutdown", "key2"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			var out string
			err := c.Get(ctx, tt.key, &out)
			require.Error(t, err)
		})
	}
}
