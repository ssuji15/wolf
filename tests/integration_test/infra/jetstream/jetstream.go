package jetstream

import (
	"context"
	"fmt"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func SetupContainer(ctx context.Context) (testcontainers.Container, string) {
	req := testcontainers.ContainerRequest{
		Image:        "nats:latest",
		ExposedPorts: []string{"4222/tcp", "8222/tcp"},
		Cmd:          []string{"-js"},
		WaitingFor:   wait.ForListeningPort("4222/tcp"),
	}

	var err error
	natsContainer, err := testcontainers.GenericContainer(
		ctx,
		testcontainers.GenericContainerRequest{
			ContainerRequest: req,
			Started:          true,
		},
	)
	if err != nil {
		panic(err)
	}

	host, err := natsContainer.Host(ctx)
	if err != nil {
		panic(err)
	}

	port, err := natsContainer.MappedPort(ctx, "4222")
	if err != nil {
		panic(err)
	}

	// ---- ENV WIRING ----
	JETSTREAM_URL := fmt.Sprintf("nats://%s:%s", host, port.Port())

	return natsContainer, JETSTREAM_URL
}
