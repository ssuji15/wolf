package storage

import (
	"bytes"
	"context"
	"io"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/ssuji15/wolf/internal/config"
	"github.com/ssuji15/wolf/internal/job_tracer"
	"go.opentelemetry.io/otel/attribute"
)

// MinioConfig holds S3/MinIO settings.
type MinioConfig struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	UseSSL    bool
}

// MinioClient wraps the MinIO SDK client.
type MinioClient struct {
	client *minio.Client
	cfg    MinioConfig
}

// NewMinioClient initializes and returns a MinIO client.
func NewMinioClient(cfg MinioConfig) (Storage, error) {

	cli, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, err
	}

	return &MinioClient{client: cli, cfg: cfg}, nil
}

// GetMinioConfig provides default minio config
func GetMinioConfig(cfg config.Config) MinioConfig {

	return MinioConfig{
		Endpoint:  cfg.MinioURL,
		Bucket:    cfg.MinioBucket,
		UseSSL:    false,
		AccessKey: cfg.MinioAccessKey,
		SecretKey: cfg.MinioSecretKey,
	}
}

// Uploads files to Minio.
func (m *MinioClient) Upload(ctx context.Context, objectPath string, code []byte) error {

	tracer := job_tracer.GetTracer()
	ctx, span := tracer.Start(ctx, "MinIO/Upload")
	defer span.End()

	span.SetAttributes(
		attribute.String("objectPath", objectPath),
	)

	// upload
	reader := bytes.NewReader(code)

	_, err := m.client.PutObject(ctx, m.cfg.Bucket, objectPath, reader, -1, minio.PutObjectOptions{})
	if err != nil {
		span.RecordError(err)
		return err
	}

	return nil
}

// Download files to Minio
func (m *MinioClient) Download(ctx context.Context, objectPath string) ([]byte, error) {

	tracer := job_tracer.GetTracer()
	ctx, span := tracer.Start(ctx, "MinIO/Download")
	defer span.End()

	span.SetAttributes(
		attribute.String("objectPath", objectPath),
	)

	// Get the object
	object, err := m.client.GetObject(ctx, m.cfg.Bucket, objectPath, minio.GetObjectOptions{})
	if err != nil {
		span.RecordError(err)
		return nil, err
	}
	defer object.Close()

	// check if the object exists
	if _, err := object.Stat(); err != nil {
		span.RecordError(err)
		return nil, err
	}

	// Read all bytes
	data, err := io.ReadAll(object)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	return data, nil
}
