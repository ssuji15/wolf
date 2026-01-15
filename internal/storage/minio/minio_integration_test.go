//go:build integration
// +build integration

package minio

import (
	"context"
	"flag"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/ssuji15/wolf/tests/integration_test/infra/minio"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
)

var (
	minioContainer testcontainers.Container
	MINIO_ENDPOINT string
)

// ------------------------
// TestMain – container
// ------------------------
func TestMain(m *testing.M) {
	flag.Parse()
	if testing.Short() {
		os.Exit(0)
	}
	ctx := context.Background()
	minioContainer, MINIO_ENDPOINT = minio.SetupContainer(ctx)
	code := m.Run()
	_ = minioContainer.Terminate(ctx)
	os.Exit(code)
}

// ------------------------
// Helpers
// ------------------------
func resetMinioSingleton() {
	m = nil
	initError = nil
	once = sync.Once{}
}

func setMinioEnv() {
	minio.SetMinioEnv(MINIO_ENDPOINT)
}

func setBadMinioEnv() {
	os.Setenv("MINIO_ENDPOINT", "t//")
}

// ------------------------
// 1. NewMinioClient
// ------------------------
func TestNewMinioClient(t *testing.T) {
	tests := []struct {
		name      string
		unsetEnv  string
		setBadEnv bool
		expectErr bool
	}{
		{"Success with valid env", "", false, false},
		{"Missing URL fails", "MINIO_ENDPOINT", false, true},
		{"Missing access key fails", "MINIO_ACCESS_KEY", false, true},
		{"Bad URL fails", "", true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetMinioSingleton()
			setMinioEnv()

			if tt.unsetEnv != "" {
				os.Unsetenv(tt.unsetEnv)
			}

			if tt.setBadEnv {
				setBadMinioEnv()
			}

			c, err := NewMinioClient()
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
// 2. Upload
// ------------------------
func TestMinioClient_Upload(t *testing.T) {
	resetMinioSingleton()
	setMinioEnv()

	minio.CreateJobsBucket(t, "jobs", MINIO_ENDPOINT)

	c, err := NewMinioClient()
	require.NoError(t, err)

	ctx := context.Background()

	tests := []struct {
		name       string
		bucket     string
		objectPath string
		data       []byte
		expectErr  bool
	}{
		{"Upload small file", "jobs", "file1.txt", []byte("hello"), false},
		{"Upload empty file", "jobs", "file2.txt", []byte{}, false},
		{"Upload to wrong bucket fails", "missing-bucket", "file3.txt", []byte("oops"), true},
		{"Empty path fails", "jobs", "", []byte("hello"), true},
		{"Empty bucket fails", "", "file1.txt", []byte("hello"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := c.(*MinioClient).Upload(ctx, tt.bucket, tt.objectPath, tt.data)
			if tt.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// ------------------------
// 3. Download
// ------------------------
func TestMinioClient_Download(t *testing.T) {
	resetMinioSingleton()
	setMinioEnv()

	minio.CreateJobsBucket(t, "jobs", MINIO_ENDPOINT)

	c, err := NewMinioClient()
	require.NoError(t, err)

	ctx := context.Background()

	// Pre-upload a valid object
	content := []byte("download-me")
	require.NoError(t, c.(*MinioClient).Upload(ctx, "jobs", "file.txt", content))

	tests := []struct {
		name      string
		bucket    string
		object    string
		expectErr bool
	}{
		{"Download existing file", "jobs", "file.txt", false},
		{"Download missing file fails", "jobs", "missing.txt", true},
		{"Download from wrong bucket fails", "b", "file.txt", true},
		{"Empty object path fails", "jobs", "", true},
		{"Empty bucket fails", "", "file.txt", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := c.(*MinioClient).Download(ctx, tt.bucket, tt.object)
			if tt.expectErr {
				require.Error(t, err)
				require.Nil(t, data)
			} else {
				require.NoError(t, err)
				require.Equal(t, content, data)
			}
		})
	}
}

// ------------------------
// 4. GetJobsBucket
// ------------------------
func TestMinioClient_GetJobsBucket(t *testing.T) {
	resetMinioSingleton()
	setMinioEnv()

	c, err := NewMinioClient()
	require.NoError(t, err)

	require.Equal(t, "jobs", c.(*MinioClient).GetJobsBucket())
}

// ------------------------
// 5. ShutDown
// ------------------------
func TestMinioClient_ShutDown(t *testing.T) {

	t.Run("shutdown completes before timeout", func(t *testing.T) {
		resetMinioSingleton()
		setMinioEnv()

		c, err := NewMinioClient()
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		done := make(chan struct{})
		go func() {
			c.(*MinioClient).ShutDown(ctx)
			close(done)
		}()

		select {
		case <-done:
			// Success
		case <-time.After(3 * time.Second):
			t.Fatal("shutdown timed out")
		}
	})

	t.Run("shutdown respects context cancellation", func(t *testing.T) {
		resetMinioSingleton()
		setMinioEnv()

		c, err := NewMinioClient()
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()

		start := time.Now()
		c.(*MinioClient).ShutDown(ctx)
		elapsed := time.Since(start)
		require.Less(t, elapsed, 50*time.Millisecond)
	})
}
