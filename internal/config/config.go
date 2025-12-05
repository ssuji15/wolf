package config

import (
	"log"
	"os"
	"strconv"
)

type Config struct {
	BackendAddress    string
	PostgresURL       string
	SocketDir         string
	JetstreamURL      string
	FreecacheByteSize int
	FreecacheTTL      int
	MinioURL          string
	MinioBucket       string
	MinioAccessKey    string
	MinioSecretKey    string
	MaxWorker         int
	ServiceName       string
}

func Load() *Config {
	return &Config{
		BackendAddress:    env("BACKEND_ADDRESS"),
		PostgresURL:       env("POSTGRES_URL"),
		SocketDir:         env("SOCKET_DIR"),
		JetstreamURL:      env("JETSTREAM_URL"),
		FreecacheByteSize: convertStringToInt(env("FREECACHE_BYTE_SIZE"), "FREECACHE_BYTE_SIZE"),
		FreecacheTTL:      convertStringToInt(env("FREECACHE_TTL"), "FREECACHE_TTL"),
		MinioURL:          env("MINIO_ENDPOINT"),
		MinioBucket:       env("MINIO_BUCKET"),
		MinioAccessKey:    env("MINIO_ACCESS_KEY"),
		MinioSecretKey:    env("MINIO_SECRET_KEY"),
		MaxWorker:         convertStringToInt(env("MAX_WORKER"), "MAX_WORKER"),
		ServiceName:       env("SERVICE_NAME"),
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
