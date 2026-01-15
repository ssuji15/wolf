package minio

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	minioSDK "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func SetupContainer(ctx context.Context) (testcontainers.Container, string) {
	req := testcontainers.ContainerRequest{
		Image:        "minio/minio:latest",
		ExposedPorts: []string{"9000/tcp"},
		Env: map[string]string{
			"MINIO_ROOT_USER":     "minioadmin",
			"MINIO_ROOT_PASSWORD": "minioadmin",
		},
		Cmd: []string{"server", "/data"},
		WaitingFor: wait.ForHTTP("/minio/health/ready").
			WithPort("9000").
			WithStartupTimeout(30 * time.Second),
	}

	var err error
	minioContainer, err := testcontainers.GenericContainer(
		ctx,
		testcontainers.GenericContainerRequest{
			ContainerRequest: req,
			Started:          true,
		},
	)
	if err != nil {
		panic(err)
	}

	host, _ := minioContainer.Host(ctx)
	port, _ := minioContainer.MappedPort(ctx, "9000")

	MINIO_ENDPOINT := fmt.Sprintf("%s:%s", host, port.Port())
	return minioContainer, MINIO_ENDPOINT
}

func CreateJobsBucket(t *testing.T, bucket string, endpoint string) {
	t.Helper()

	client, err := minioSDK.New(
		endpoint,
		&minioSDK.Options{
			Creds:  credentials.NewStaticV4("minioadmin", "minioadmin", ""),
			Secure: false,
		},
	)
	require.NoError(t, err)

	exists, err := client.BucketExists(context.Background(), bucket)
	require.NoError(t, err)

	if !exists {
		require.NoError(t, client.MakeBucket(context.Background(), bucket, minioSDK.MakeBucketOptions{}))
	}
}

func SetMinioEnv(endpoint string) {
	os.Setenv("MINIO_ENDPOINT", endpoint)
	os.Setenv("MINIO_ACCESS_KEY", "minioadmin")
	os.Setenv("MINIO_SECRET_KEY", "minioadmin")
	os.Setenv("MINIO_USE_SSL", "false")
	os.Setenv("MINIO_JOBS_BUCKET", "jobs")
}
