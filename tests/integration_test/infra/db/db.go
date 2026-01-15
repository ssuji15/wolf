package db

import (
	"context"
	"fmt"
	"os"

	"github.com/ssuji15/wolf/internal/db"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func SetupContainer(ctx context.Context) (testcontainers.Container, *db.DB, string) {
	req := testcontainers.ContainerRequest{
		Image:        "postgres:18",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     "wolf",
			"POSTGRES_PASSWORD": "wolf123",
			"POSTGRES_DB":       "wolf",
		},
		WaitingFor: wait.ForListeningPort("5432/tcp"),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		panic(err)
	}

	host, _ := container.Host(ctx)
	port, _ := container.MappedPort(ctx, "5432")

	POSTGRES_URL := fmt.Sprintf(
		"postgres://wolf:wolf123@%s:%s/wolf?sslmode=disable",
		host,
		port.Port(),
	)

	os.Setenv("POSTGRES_URL", POSTGRES_URL)

	db, err := db.New(ctx)
	if err != nil {
		panic(err)
	}
	return container, db, POSTGRES_URL
}
