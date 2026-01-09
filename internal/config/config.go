package config

import (
	"fmt"
	"os"
	"strconv"
)

type NatsConfig struct {
	URL               string
	TTL               int
	BUCKET_NAME       string
	BUCKET_SIZE_BYTES int
}

type RedisConfig struct {
	TTL            int
	ClientPassword string
	URL            string
}

type FreeCacheConfig struct {
	SIZE_BYTES int
	TTL        int
}

type MinioConfig struct {
	URL              string
	JOBS_BUCKET      string
	ACCESS_KEY       string
	SECRET_KEY       string
	TELEMETRY_BUCKET string
	USE_SSL          bool
}

type PostgresConfig struct {
	URL string
}

type SandboxManagerConfig struct {
	SOCKET_DIR       string
	MAX_WORKER       int
	APPARMOR_PROFILE string
	SECCOMP_PROFILE  string
	LAUNCHER_TYPE    string
	WORKER_IMAGE     string
}

type Config struct {
	SERVICE_NAME string
	TRACE_URL    string
	CACHE_TYPE   string
	QUEUE_TYPE   string
	STORAGE_TYPE string
}

func env(key string) string {
	v := os.Getenv(key)
	return v
}

func convertStringToInt(s string, key string) (int, error) {
	sInt, err := strconv.Atoi(s)
	if err != nil {
		return -1, fmt.Errorf("error initializing config with key: %s, err: %v", key, err)
	}
	return sInt, nil
}

func GetNatsConfig() (*NatsConfig, error) {
	ttl, err := convertStringToInt(env("JETSTREAM_TTL"), "JETSTREAM_TTL")
	if err != nil {
		return nil, err
	}
	url := env("JETSTREAM_URL")
	if url == "" {
		return nil, fmt.Errorf("KEY: JETSTREAM_URL is empty")
	}
	bn := env("JETSTREAM_BUCKET_NAME")
	if bn == "" {
		return nil, fmt.Errorf("KEY: JETSTREAM_BUCKET_NAME is empty")
	}
	bs, err := convertStringToInt(env("JETSTREAM_BUCKET_SIZE"), "JETSTREAM_BUCKET_SIZE")
	if err != nil {
		return nil, err
	}
	return &NatsConfig{
		URL:               url,
		TTL:               ttl,
		BUCKET_NAME:       bn,
		BUCKET_SIZE_BYTES: bs,
	}, nil
}

func GetRedisConfig() (*RedisConfig, error) {
	ttl, err := convertStringToInt(env("REDIS_TTL"), "REDIS_TTL")
	if err != nil {
		return nil, err
	}

	url := env("REDIS_ENDPOINT")
	if url == "" {
		return nil, fmt.Errorf("KEY: REDIS_ENDPOINT is empty")
	}

	return &RedisConfig{
		TTL:            ttl,
		ClientPassword: env("REDIS_CLIENT_PASSWORD"),
		URL:            url,
	}, nil
}

func GetFreeCacheConfig() (*FreeCacheConfig, error) {
	ttl, err := convertStringToInt(env("FREECACHE_TTL"), "FREECACHE_TTL")
	if err != nil {
		return nil, err
	}
	fs, err := convertStringToInt(env("FREECACHE_SIZE"), "FREECACHE_SIZE")
	if err != nil {
		return nil, err
	}
	return &FreeCacheConfig{
		TTL:        ttl,
		SIZE_BYTES: fs,
	}, nil
}

func GetPostgresConfig() (*PostgresConfig, error) {
	url := env("POSTGRES_URL")
	if url == "" {
		return nil, fmt.Errorf("KEY: POSTGRES_URL is empty")
	}
	return &PostgresConfig{
		URL: url,
	}, nil
}

func GetConfig() (*Config, error) {
	sn := env("SERVICE_NAME")
	if sn == "" {
		return nil, fmt.Errorf("KEY: SERVICE_NAME is empty")
	}
	turl := env("TRACE_URL")
	ct := env("CACHE_TYPE")
	if ct == "" {
		return nil, fmt.Errorf("KEY: CACHE_TYPE is empty")
	}
	qt := env("QUEUE_TYPE")
	if qt == "" {
		return nil, fmt.Errorf("KEY: QUEUE_TYPE is empty")
	}
	st := env("STORAGE_TYPE")
	if st == "" {
		return nil, fmt.Errorf("KEY: STORAGE_TYPE is empty")
	}
	return &Config{
		SERVICE_NAME: sn,
		TRACE_URL:    turl,
		CACHE_TYPE:   ct,
		QUEUE_TYPE:   qt,
		STORAGE_TYPE: st,
	}, nil
}

func GetMinioConfig() (*MinioConfig, error) {
	url := env("MINIO_ENDPOINT")
	if url == "" {
		return nil, fmt.Errorf("KEY: MINIO_ENDPOINT is empty")
	}

	jb := env("MINIO_JOBS_BUCKET")
	if jb == "" {
		return nil, fmt.Errorf("KEY: MINIO_JOBS_BUCKET is empty")
	}

	ssl := env("MINIO_USE_SSL")
	if ssl != "true" && ssl != "false" {
		return nil, fmt.Errorf("KEY: MINIO_USE_SSL is invalid")
	}

	ak := env("MINIO_ACCESS_KEY")
	if ak == "" {
		return nil, fmt.Errorf("KEY: MINIO_ACCESS_KEY is empty")
	}

	sk := env("MINIO_SECRET_KEY")
	if sk == "" {
		return nil, fmt.Errorf("KEY: MINIO_SECRET_KEY is empty")
	}

	return &MinioConfig{
		URL:         url,
		JOBS_BUCKET: jb,
		USE_SSL:     ssl == "true",
		ACCESS_KEY:  ak,
		SECRET_KEY:  sk,
	}, nil
}

func GetSandboxManagerConfig() (*SandboxManagerConfig, error) {
	sd := env("SOCKET_DIR")
	if sd == "" {
		return nil, fmt.Errorf("KEY: SOCKET_DIR is empty")
	}
	mw, err := convertStringToInt(env("MAX_WORKER"), "MAX_WORKER")
	if err != nil {
		return nil, err
	}

	ap := env("APPARMOR_PROFILE")
	if ap == "" {
		return nil, fmt.Errorf("KEY: APPARMOR_PROFILE is empty")
	}

	sp := env("SECCOMP_PROFILE")
	if sp == "" {
		return nil, fmt.Errorf("KEY: SECCOMP_PROFILE is empty")
	}

	lt := env("LAUNCHER_TYPE")
	if lt == "" {
		return nil, fmt.Errorf("KEY: LAUNCHER_TYPE is empty")
	}
	wi := env("WORKER_IMAGE")
	if wi == "" {
		return nil, fmt.Errorf("KEY: WORKER_IMAGE is empty")
	}
	return &SandboxManagerConfig{
		SOCKET_DIR:       sd,
		MAX_WORKER:       mw,
		APPARMOR_PROFILE: ap,
		SECCOMP_PROFILE:  sp,
		LAUNCHER_TYPE:    lt,
		WORKER_IMAGE:     wi,
	}, nil
}
