package config

import (
	"os"
	"reflect"
	"testing"
)

func withEnv(t *testing.T, envs map[string]string) {
	t.Helper()

	original := make(map[string]string)
	for k := range envs {
		original[k] = os.Getenv(k)
	}

	for k, v := range envs {
		_ = os.Setenv(k, v)
	}

	t.Cleanup(func() {
		for k, v := range original {
			if v == "" {
				_ = os.Unsetenv(k)
			} else {
				_ = os.Setenv(k, v)
			}
		}
	})
}

func TestGetNatsConfig(t *testing.T) {
	tests := []struct {
		name      string
		envs      map[string]string
		expected  *NatsConfig
		shouldErr bool
	}{
		{
			name: "valid nats config",
			envs: map[string]string{
				"JETSTREAM_URL": "nats://localhost:4222",
			},
			expected: &NatsConfig{
				URL: "nats://localhost:4222",
			},
		},
		{
			name:      "invalid nats config: missing url",
			envs:      map[string]string{},
			shouldErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withEnv(t, tt.envs)

			cfg, err := GetNatsConfig()
			if tt.shouldErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !reflect.DeepEqual(cfg, tt.expected) {
				t.Fatalf("got %+v, want %+v", cfg, tt.expected)
			}
		})
	}
}

func TestGetNatsCacheConfig(t *testing.T) {
	tests := []struct {
		name      string
		envs      map[string]string
		expected  *NatsCacheConfig
		shouldErr bool
	}{
		{
			name: "valid nats cache config",
			envs: map[string]string{
				"JETSTREAM_TTL":         "30",
				"JETSTREAM_BUCKET_NAME": "cache",
				"JETSTREAM_BUCKET_SIZE": "1024",
			},
			expected: &NatsCacheConfig{
				TTL:               30,
				BUCKET_NAME:       "cache",
				BUCKET_SIZE_BYTES: 1024,
			},
		},
		{
			name: "invalid nats cache config: invalid ttl",
			envs: map[string]string{
				"JETSTREAM_TTL":         "abc",
				"JETSTREAM_BUCKET_NAME": "cache",
				"JETSTREAM_BUCKET_SIZE": "1024",
			},
			shouldErr: true,
		},
		{
			name: "invalid nats cache config: missing bucket name",
			envs: map[string]string{
				"JETSTREAM_TTL":         "10",
				"JETSTREAM_BUCKET_SIZE": "1024",
			},
			shouldErr: true,
		},
		{
			name: "invalid nats cache config: invalid bucket size",
			envs: map[string]string{
				"JETSTREAM_TTL":         "30",
				"JETSTREAM_BUCKET_NAME": "cache",
				"JETSTREAM_BUCKET_SIZE": "xyz",
			},
			shouldErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withEnv(t, tt.envs)

			cfg, err := GetNatsCacheConfig()
			if tt.shouldErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !reflect.DeepEqual(cfg, tt.expected) {
				t.Fatalf("got %+v, want %+v", cfg, tt.expected)
			}
		})
	}
}

func TestGetNatsQueueConfig(t *testing.T) {
	tests := []struct {
		name      string
		envs      map[string]string
		expected  *NatsQueueConfig
		shouldErr bool
	}{
		{
			name: "valid nats queue config",
			envs: map[string]string{
				"MAX_MESSAGES_JOB_QUEUE": "100",
			},
			expected: &NatsQueueConfig{
				MAX_MESSAGES_JOB_QUEUE: 100,
			},
		},
		{
			name: "invalid nats queue config: invalid number for job queue",
			envs: map[string]string{
				"MAX_MESSAGES_JOB_QUEUE": "x",
			},
			shouldErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withEnv(t, tt.envs)

			cfg, err := GetNatsQueueConfig()
			if tt.shouldErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !reflect.DeepEqual(cfg, tt.expected) {
				t.Fatalf("got %+v, want %+v", cfg, tt.expected)
			}
		})
	}
}

func TestGetRedisCacheConfig(t *testing.T) {
	tests := []struct {
		name      string
		envs      map[string]string
		expected  *RedisCacheConfig
		shouldErr bool
	}{
		{
			name: "valid redis cache config",
			envs: map[string]string{
				"REDIS_TTL": "60",
			},
			expected: &RedisCacheConfig{
				TTL: 60,
			},
		},
		{
			name: "invalid redis cache config: invalid ttl",
			envs: map[string]string{
				"REDIS_TTL": "bad",
			},
			shouldErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withEnv(t, tt.envs)

			cfg, err := GetRedisCacheConfig()
			if tt.shouldErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !reflect.DeepEqual(cfg, tt.expected) {
				t.Fatalf("got %+v, want %+v", cfg, tt.expected)
			}
		})
	}
}

func TestGetRedisConfig(t *testing.T) {
	tests := []struct {
		name      string
		envs      map[string]string
		expected  *RedisConfig
		shouldErr bool
	}{
		{
			name: "valid redis config",
			envs: map[string]string{
				"REDIS_ENDPOINT":        "localhost:6379",
				"REDIS_CLIENT_PASSWORD": "pwd",
			},
			expected: &RedisConfig{
				URL:            "localhost:6379",
				ClientPassword: "pwd",
			},
		},
		{
			name: "invalid redis config: missing endpoint",
			envs: map[string]string{
				"REDIS_CLIENT_PASSWORD": "pwd",
			},
			shouldErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withEnv(t, tt.envs)

			cfg, err := GetRedisConfig()
			if tt.shouldErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !reflect.DeepEqual(cfg, tt.expected) {
				t.Fatalf("got %+v, want %+v", cfg, tt.expected)
			}
		})
	}
}

func TestGetFreeCacheConfig(t *testing.T) {
	tests := []struct {
		name      string
		envs      map[string]string
		expected  *FreeCacheConfig
		shouldErr bool
	}{
		{
			name: "valid freecache config",
			envs: map[string]string{
				"FREECACHE_TTL":  "10",
				"FREECACHE_SIZE": "2048",
			},
			expected: &FreeCacheConfig{
				TTL:        10,
				SIZE_BYTES: 2048,
			},
		},
		{
			name: "invalid freecache config: invalid size",
			envs: map[string]string{
				"FREECACHE_TTL":  "10",
				"FREECACHE_SIZE": "bad",
			},
			shouldErr: true,
		},
		{
			name: "invalid freecache config: invalid ttl",
			envs: map[string]string{
				"FREECACHE_TTL":  "bad",
				"FREECACHE_SIZE": "2048",
			},
			shouldErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withEnv(t, tt.envs)

			cfg, err := GetFreeCacheConfig()
			if tt.shouldErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !reflect.DeepEqual(cfg, tt.expected) {
				t.Fatalf("got %+v, want %+v", cfg, tt.expected)
			}
		})
	}
}

/*
	GetPostgresConfig
*/

func TestGetPostgresConfig(t *testing.T) {
	tests := []struct {
		name      string
		envs      map[string]string
		expected  *PostgresConfig
		shouldErr bool
	}{
		{
			name: "valid postgres config",
			envs: map[string]string{
				"POSTGRES_URL": "postgres://localhost/db",
			},
			expected: &PostgresConfig{
				URL: "postgres://localhost/db",
			},
		},
		{
			name:      "invalid postgres config: missing url",
			envs:      map[string]string{},
			shouldErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withEnv(t, tt.envs)

			cfg, err := GetPostgresConfig()
			if tt.shouldErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !reflect.DeepEqual(cfg, tt.expected) {
				t.Fatalf("got %+v, want %+v", cfg, tt.expected)
			}
		})
	}
}

/*
	GetMinioConfig
*/

func TestGetMinioConfig(t *testing.T) {
	tests := []struct {
		name      string
		envs      map[string]string
		expected  *MinioConfig
		shouldErr bool
	}{
		{
			name: "valid minio config",
			envs: map[string]string{
				"MINIO_ENDPOINT":    "localhost:9000",
				"MINIO_JOBS_BUCKET": "jobs",
				"MINIO_USE_SSL":     "true",
				"MINIO_ACCESS_KEY":  "ak",
				"MINIO_SECRET_KEY":  "sk",
			},
			expected: &MinioConfig{
				URL:         "localhost:9000",
				JOBS_BUCKET: "jobs",
				USE_SSL:     true,
				ACCESS_KEY:  "ak",
				SECRET_KEY:  "sk",
			},
		},
		{
			name: "invalid minio config: invalid ssl value",
			envs: map[string]string{
				"MINIO_ENDPOINT":    "localhost",
				"MINIO_JOBS_BUCKET": "jobs",
				"MINIO_USE_SSL":     "yes",
			},
			shouldErr: true,
		},
		{
			name: "invalid minio config: endpoint empty",
			envs: map[string]string{
				"MINIO_ENDPOINT":    "",
				"MINIO_JOBS_BUCKET": "jobs",
				"MINIO_USE_SSL":     "true",
				"MINIO_ACCESS_KEY":  "ak",
				"MINIO_SECRET_KEY":  "sk",
			},
			shouldErr: true,
		},
		{
			name: "invalid minio config: bucket empty",
			envs: map[string]string{
				"MINIO_ENDPOINT":    "localhost:9000",
				"MINIO_JOBS_BUCKET": "",
				"MINIO_USE_SSL":     "true",
				"MINIO_ACCESS_KEY":  "ak",
				"MINIO_SECRET_KEY":  "sk",
			},
			shouldErr: true,
		},
		{
			name: "invalid minio config: accesskey empty",
			envs: map[string]string{
				"MINIO_ENDPOINT":    "localhost:9000",
				"MINIO_JOBS_BUCKET": "jobs",
				"MINIO_USE_SSL":     "true",
				"MINIO_ACCESS_KEY":  "",
				"MINIO_SECRET_KEY":  "sk",
			},
			shouldErr: true,
		},
		{
			name: "invalid minio config: secretkey empty",
			envs: map[string]string{
				"MINIO_ENDPOINT":    "localhost:9000",
				"MINIO_JOBS_BUCKET": "jobs",
				"MINIO_USE_SSL":     "true",
				"MINIO_ACCESS_KEY":  "ak",
				"MINIO_SECRET_KEY":  "",
			},
			shouldErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withEnv(t, tt.envs)

			cfg, err := GetMinioConfig()
			if tt.shouldErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !reflect.DeepEqual(cfg, tt.expected) {
				t.Fatalf("got %+v, want %+v", cfg, tt.expected)
			}
		})
	}
}

/*
	GetSandboxManagerConfig
*/

func TestGetSandboxManagerConfig(t *testing.T) {
	tests := []struct {
		name      string
		envs      map[string]string
		expected  *SandboxManagerConfig
		shouldErr bool
	}{
		{
			name: "valid sandbox manager config",
			envs: map[string]string{
				"SOCKET_DIR":       "/tmp",
				"MAX_WORKER":       "4",
				"APPARMOR_PROFILE": "profile",
				"SECCOMP_PROFILE":  "profile",
				"LAUNCHER_TYPE":    "docker",
				"WORKER_IMAGE":     "worker:latest",
			},
			expected: &SandboxManagerConfig{
				SOCKET_DIR:       "/tmp",
				MAX_WORKER:       4,
				APPARMOR_PROFILE: "profile",
				SECCOMP_PROFILE:  "profile",
				LAUNCHER_TYPE:    "docker",
				WORKER_IMAGE:     "worker:latest",
			},
		},
		{
			name: "invalid sandbox manager config: invalid max worker",
			envs: map[string]string{
				"SOCKET_DIR": "/tmp",
				"MAX_WORKER": "bad",
			},
			shouldErr: true,
		},
		{
			name: "invalid sandbox manager config: invalid socket dir",
			envs: map[string]string{
				"SOCKET_DIR":       "",
				"MAX_WORKER":       "4",
				"APPARMOR_PROFILE": "profile",
				"SECCOMP_PROFILE":  "profile",
				"LAUNCHER_TYPE":    "docker",
				"WORKER_IMAGE":     "worker:latest",
			},
			shouldErr: true,
		},
		{
			name: "invalid sandbox manager config: invalid app armour",
			envs: map[string]string{
				"SOCKET_DIR":       "/tmp",
				"MAX_WORKER":       "4",
				"APPARMOR_PROFILE": "",
				"SECCOMP_PROFILE":  "profile",
				"LAUNCHER_TYPE":    "docker",
				"WORKER_IMAGE":     "worker:latest",
			},
			shouldErr: true,
		},
		{
			name: "invalid sandbox manager config: invalid sec comp",
			envs: map[string]string{
				"SOCKET_DIR":       "/tmp",
				"MAX_WORKER":       "4",
				"APPARMOR_PROFILE": "profile",
				"SECCOMP_PROFILE":  "",
				"LAUNCHER_TYPE":    "docker",
				"WORKER_IMAGE":     "worker:latest",
			},
			shouldErr: true,
		},
		{
			name: "invalid sandbox manager config: invalid launcher type",
			envs: map[string]string{
				"SOCKET_DIR":       "/tmp",
				"MAX_WORKER":       "4",
				"APPARMOR_PROFILE": "profile",
				"SECCOMP_PROFILE":  "profile",
				"LAUNCHER_TYPE":    "",
				"WORKER_IMAGE":     "worker:latest",
			},
			shouldErr: true,
		},
		{
			name: "invalid sandbox manager config: invalid worker image",
			envs: map[string]string{
				"SOCKET_DIR":       "/tmp",
				"MAX_WORKER":       "4",
				"APPARMOR_PROFILE": "profile",
				"SECCOMP_PROFILE":  "profile",
				"LAUNCHER_TYPE":    "docker",
				"WORKER_IMAGE":     "",
			},
			shouldErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withEnv(t, tt.envs)

			cfg, err := GetSandboxManagerConfig()
			if tt.shouldErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !reflect.DeepEqual(cfg, tt.expected) {
				t.Fatalf("got %+v, want %+v", cfg, tt.expected)
			}
		})
	}
}

func TestGetConfig(t *testing.T) {
	tests := []struct {
		name      string
		envs      map[string]string
		expected  *Config
		shouldErr bool
	}{
		{
			name: "valid config",
			envs: map[string]string{
				"SERVICE_NAME": "svc",
				"TRACE_URL":    "http://trace",
				"CACHE_TYPE":   "redis",
				"QUEUE_TYPE":   "nats",
				"STORAGE_TYPE": "minio",
			},
			expected: &Config{
				SERVICE_NAME: "svc",
				TRACE_URL:    "http://trace",
				CACHE_TYPE:   "redis",
				QUEUE_TYPE:   "nats",
				STORAGE_TYPE: "minio",
			},
		},
		{
			name:      "invalid config: missing required",
			envs:      map[string]string{},
			shouldErr: true,
		},
		{
			name: "invalid config: missing cache type",
			envs: map[string]string{
				"SERVICE_NAME": "svc",
				"TRACE_URL":    "http://trace",
				"CACHE_TYPE":   "",
				"QUEUE_TYPE":   "nats",
				"STORAGE_TYPE": "minio",
			},
			shouldErr: true,
		},
		{
			name: "invalid config: missing queue type",
			envs: map[string]string{
				"SERVICE_NAME": "svc",
				"TRACE_URL":    "http://trace",
				"CACHE_TYPE":   "redis",
				"QUEUE_TYPE":   "",
				"STORAGE_TYPE": "minio",
			},
			shouldErr: true,
		},
		{
			name: "invalid config: missing storage type",
			envs: map[string]string{
				"SERVICE_NAME": "svc",
				"TRACE_URL":    "http://trace",
				"CACHE_TYPE":   "redis",
				"QUEUE_TYPE":   "nats",
				"STORAGE_TYPE": "",
			},
			shouldErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withEnv(t, tt.envs)

			cfg, err := GetConfig()
			if tt.shouldErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !reflect.DeepEqual(cfg, tt.expected) {
				t.Fatalf("got %+v, want %+v", cfg, tt.expected)
			}
		})
	}
}
