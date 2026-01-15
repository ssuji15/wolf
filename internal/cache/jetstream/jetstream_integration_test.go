//go:build integration
// +build integration

package jetstream

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/ssuji15/wolf/internal/component/jetstream"
	tjetstream "github.com/ssuji15/wolf/tests/integration_test/infra/jetstream"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
)

var (
	natsContainer testcontainers.Container
	JETSTREAM_URL string
)

func TestMain(m *testing.M) {
	flag.Parse()
	if testing.Short() {
		fmt.Println("skipping integration tests")
		os.Exit(0)
	}
	ctx := context.Background()
	natsContainer, JETSTREAM_URL = tjetstream.SetupContainer(ctx)
	code := m.Run()
	_ = natsContainer.Terminate(ctx)
	os.Exit(code)
}

// ------------------------
// Singleton reset helper
// ------------------------
func resetJetStreamSingleton() {
	jcc = nil
	initError = nil
	once = sync.Once{}
}

// ------------------------
// Environment setup
// ------------------------
func setJetStreamEnv() {
	os.Setenv("JETSTREAM_TTL", "2")
	os.Setenv("JETSTREAM_BUCKET_NAME", "TEST_CACHE")
	os.Setenv("JETSTREAM_BUCKET_SIZE", "1048576")
	os.Setenv("JETSTREAM_URL", JETSTREAM_URL)
}

// ------------------------
// 1. NewJetStreamCacheClient tests
// ------------------------
func TestNewJetStreamCacheClient(t *testing.T) {
	resetJetStreamSingleton()
	setJetStreamEnv()

	tests := []struct {
		name      string
		unsetEnv  string
		expectErr bool
	}{
		{"All env set succeeds", "", false},
		{"Missing JETSTREAM_URL fails", "JETSTREAM_URL", true},
		{"Missing BUCKET_NAME fails", "JETSTREAM_BUCKET_NAME", true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			jetstream.ResetJetStreamClient()
			resetJetStreamSingleton()
			setJetStreamEnv()
			if tt.unsetEnv != "" {
				os.Unsetenv(tt.unsetEnv)
			}
			c, err := NewJetStreamCacheClient()
			if tt.expectErr {
				require.Error(t, err)
				require.Nil(t, c)
			} else {
				require.NoError(t, err)
				require.NotNil(t, c)
			}
		})
	}
}

// ------------------------
// 2. Put tests
// ------------------------
func TestJetStreamCacheClient_Put(t *testing.T) {
	resetJetStreamSingleton()
	setJetStreamEnv()

	ctx := context.Background()
	c, err := NewJetStreamCacheClient()
	require.NoError(t, err)

	tests := []struct {
		name      string
		key       string
		value     interface{}
		expectErr bool
	}{
		{"Empty key fails", "", "value", true},
		{"Nil value fails", "nil_val", nil, true},
		{"String value succeeds", "key1", "value1", false},
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
func TestJetStreamCacheClient_Get(t *testing.T) {
	resetJetStreamSingleton()
	setJetStreamEnv()

	ctx := context.Background()
	c, err := NewJetStreamCacheClient()
	require.NoError(t, err)

	// Prepopulate cache
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
func TestJetStreamCacheClient_TTL(t *testing.T) {
	resetJetStreamSingleton()
	setJetStreamEnv()

	ctx := context.Background()
	c, err := NewJetStreamCacheClient()
	require.NoError(t, err)

	// TTL is fixed per bucket, get it from the client
	bucketTTL := c.GetDefaultTTL()

	tests := []struct {
		name        string
		key         string
		value       string
		sleepBefore time.Duration
		expectErr   bool
	}{
		{
			name:        "Value expires after bucket TTL",
			key:         "temp",
			value:       "shortlived",
			sleepBefore: time.Duration(bucketTTL+1) * time.Second, // wait longer than bucket TTL
			expectErr:   true,
		},
		{
			name:        "Value accessible before TTL",
			key:         "persistent",
			value:       "longlived",
			sleepBefore: time.Duration(bucketTTL/2) * time.Second, // wait less than TTL
			expectErr:   false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			// Put ignores the ttl parameter because bucket TTL is fixed
			err := c.Put(ctx, tt.key, tt.value, bucketTTL)
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
func TestJetStreamCacheClient_Shutdown_TableDriven(t *testing.T) {
	resetJetStreamSingleton()
	setJetStreamEnv()

	ctx := context.Background()
	c, _ := NewJetStreamCacheClient()

	// Prepopulate cache
	require.NoError(t, c.Put(ctx, "key1", "value1", c.GetDefaultTTL()))
	require.NoError(t, c.Put(ctx, "key2", "value2", c.GetDefaultTTL()))

	// Shutdown
	c.ShutDown(ctx)

	tests := []struct {
		name string
		key  string
	}{
		{"Key1 cleared after shutdown", "key1"},
		{"Key2 cleared after shutdown", "key2"},
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
