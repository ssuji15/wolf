package config

import (
	"log"
	"os"
	"strconv"
)

type Config struct {
	BackendAddress      string
	PostgresURL         string
	SocketDir           string
	JetstreamURL        string
	CacheType           string
	CacheByteSize       int
	CacheTTL            int
	CacheClientPassword string
	CacheURL            string
	MinioURL            string
	MinioBucket         string
	MinioAccessKey      string
	MinioSecretKey      string
	MaxWorker           int
	ServiceName         string
	AppArmorProfile     string
	SeccompProfile      string
	LauncherType        string
}

func Load() *Config {
	return &Config{
		PostgresURL:         env("POSTGRES_URL"),
		SocketDir:           env("SOCKET_DIR"),
		JetstreamURL:        env("JETSTREAM_URL"),
		CacheType:           env("CACHE_TYPE"),
		CacheByteSize:       convertStringToInt(env("CACHE_BYTE_SIZE"), "CACHE_BYTE_SIZE"),
		CacheTTL:            convertStringToInt(env("CACHE_TTL"), "CACHE_TTL"),
		CacheURL:            env("CACHE_ENDPOINT"),
		CacheClientPassword: env("CACHE_PASSWORD"),
		MinioURL:            env("MINIO_ENDPOINT"),
		MinioBucket:         env("MINIO_BUCKET"),
		MinioAccessKey:      env("MINIO_ACCESS_KEY"),
		MinioSecretKey:      env("MINIO_SECRET_KEY"),
		MaxWorker:           convertStringToInt(env("MAX_WORKER"), "MAX_WORKER"),
		ServiceName:         env("SERVICE_NAME"),
		AppArmorProfile:     env("APPARMOR_PROFILE"),
		SeccompProfile:      env("SECCOMP_PROFILE"),
		LauncherType:        env("LAUNCHER_TYPE"),
	}
}

func env(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("Error initializing config: %s", key)
	}
	return v
}

func convertStringToInt(s string, key string) int {
	sInt, err := strconv.Atoi(s)
	if err != nil {
		log.Fatalf("Error initializing config with key: %s, err: %v", key, err)
	}
	return sInt
}
