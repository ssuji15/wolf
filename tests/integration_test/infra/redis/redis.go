package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func SetupContainer(ctx context.Context) (testcontainers.Container, string) {
	req := testcontainers.ContainerRequest{
		Image:        "redis:latest",
		ExposedPorts: []string{"6379/tcp"},
		WaitingFor:   wait.ForListeningPort("6379/tcp").WithStartupTimeout(30 * time.Second),
	}

	var err error
	redisContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
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

	REDIS_ENDPOINT := fmt.Sprintf("%s:%s", host, port.Port())
	return redisContainer, REDIS_ENDPOINT
}
