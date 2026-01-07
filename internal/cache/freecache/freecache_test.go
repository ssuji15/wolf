package freecache

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func resetFreeCacheForTest() {
	fcc = nil
	initError = nil
	once = sync.Once{}
}

type User struct {
	Name string
	Age  int
}

// ------------------------
// 1. PUT tests
// ------------------------
func TestFreeCache_Put(t *testing.T) {
	resetFreeCacheForTest()
	os.Setenv("FREECACHE_TTL", "5")
	os.Setenv("FREECACHE_SIZE", "1024")

	ctx := context.Background()
	c, err := NewFreeCache()
	require.NoError(t, err)
	require.NotNil(t, c)

	tests := []struct {
		name      string
		key       string
		value     interface{}
		expectErr bool
	}{
		{"Empty key should fail", "", "value", true},
		{"Nil value should fail", "nil_value", nil, true},
		{"String value should succeed", "username", "alice", false},
		{"Struct value should succeed", "user:1", User{Name: "bob", Age: 25}, false},
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
// 2. GET tests
// ------------------------
func TestFreeCache_Get(t *testing.T) {
	resetFreeCacheForTest()
	os.Setenv("FREECACHE_TTL", "5")
	os.Setenv("FREECACHE_SIZE", "1024")

	ctx := context.Background()
	c, err := NewFreeCache()
	require.NoError(t, err)
	require.NotNil(t, c)

	// Prepopulate cache
	err = c.Put(ctx, "username", "alice", c.GetDefaultTTL())
	require.NoError(t, err)
	err = c.Put(ctx, "user:1", User{Name: "bob", Age: 25}, c.GetDefaultTTL())
	require.NoError(t, err)

	tests := []struct {
		name      string
		key       string
		expected  interface{}
		expectErr bool
	}{
		{"Empty key should fail", "", nil, true},
		{"Key not present should fail", "missing", nil, true},
		{"Get string value succeeds", "username", "alice", false},
		{"Get struct value succeeds", "user:1", User{Name: "bob", Age: 25}, false},
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
			case User:
				var u User
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
// 3. TTL tests
// ------------------------
func TestFreeCache_TTL(t *testing.T) {
	resetFreeCacheForTest()
	os.Setenv("FREECACHE_TTL", "2")
	os.Setenv("FREECACHE_SIZE", "1024")

	ctx := context.Background()
	c, _ := NewFreeCache()

	tests := []struct {
		name        string
		key         string
		value       string
		ttlSeconds  int
		sleepBefore time.Duration
		expectErr   bool
	}{
		{"Short TTL should expire", "short", "temp", 1, 2 * time.Second, true},
		{"Long TTL should survive", "long", "persistent", 5, 2 * time.Second, false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := c.Put(ctx, tt.key, tt.value, tt.ttlSeconds)
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
// 4. Shutdown tests
// ------------------------
func TestFreeCache_Shutdown(t *testing.T) {
	resetFreeCacheForTest()
	os.Setenv("FREECACHE_TTL", "5")
	os.Setenv("FREECACHE_SIZE", "1024")

	ctx := context.Background()
	c, _ := NewFreeCache()

	// Prepopulate cache
	err := c.Put(ctx, "key1", "value1", c.GetDefaultTTL())
	require.NoError(t, err)
	err = c.Put(ctx, "key2", "value2", c.GetDefaultTTL())
	require.NoError(t, err)

	tests := []struct {
		name string
		key  string
	}{
		{"Key1 should be cleared", "key1"},
		{"Key2 should be cleared", "key2"},
	}

	c.ShutDown(ctx)

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			var out string
			err := c.Get(ctx, tt.key, &out)
			require.Error(t, err)
		})
	}
}

func TestNewFreeCache(t *testing.T) {
	resetFreeCacheForTest()

	tests := []struct {
		name            string
		ttlEnv          string
		sizeEnv         string
		expectErr       bool
		expectSingleton bool
	}{
		{"Valid env vars initializes cache", "5", "1024", false, true},
		{"Missing TTL env var returns error", "", "1024", true, false},
		{"Missing SIZE env var returns error", "5", "", true, false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			// Reset singleton for each test
			resetFreeCacheForTest()

			// Set or unset env vars
			if tt.ttlEnv != "" {
				os.Setenv("FREECACHE_TTL", tt.ttlEnv)
			} else {
				os.Unsetenv("FREECACHE_TTL")
			}

			if tt.sizeEnv != "" {
				os.Setenv("FREECACHE_SIZE", tt.sizeEnv)
			} else {
				os.Unsetenv("FREECACHE_SIZE")
			}

			c, err := NewFreeCache()
			if tt.expectErr {
				require.Error(t, err)
				require.Nil(t, c)
			} else {
				require.NoError(t, err)
				require.NotNil(t, c)
				require.Equal(t, tt.expectSingleton, fcc == c) // singleton is set
			}

			// Calling NewFreeCache again should return the same instance if singleton initialized
			c2, _ := NewFreeCache()
			if !tt.expectErr {
				require.Equal(t, c, c2)
			}
		})
	}
}
