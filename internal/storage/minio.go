package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
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
func GetMinioConfig() (MinioConfig, error) {
	endpoint := os.Getenv("MINIO_ENDPOINT")
	if endpoint == "" {
		return MinioConfig{}, fmt.Errorf("unable to retrieve minio endpoint")
	}

	bucket := os.Getenv("MINIO_BUCKET")
	if bucket == "" {
		return MinioConfig{}, fmt.Errorf("unable to retrieve minio bucket")
	}

	ak := os.Getenv("MINIO_ACCESS_KEY")
	if ak == "" {
		return MinioConfig{}, fmt.Errorf("unable to retrieve minio access key")
	}

	sk := os.Getenv("MINIO_SECRET_KEY")
	if sk == "" {
		return MinioConfig{}, fmt.Errorf("unable to retrieve minio secret key")
	}

	return MinioConfig{
		Endpoint:  endpoint,
		Bucket:    bucket,
		UseSSL:    false,
		AccessKey: ak,
		SecretKey: sk,
	}, nil
}

// Uploads files to Minio.
func (m *MinioClient) Upload(ctx context.Context, objectPath string, code []byte) error {

	// upload
	reader := bytes.NewReader(code)

	_, err := m.client.PutObject(ctx, m.cfg.Bucket, objectPath, reader, -1, minio.PutObjectOptions{})
	if err != nil {
		return err
	}

	return nil
}

// Download files to Minio
func (m *MinioClient) Download(ctx context.Context, objectPath string) ([]byte, error) {
	// Get the object
	object, err := m.client.GetObject(ctx, m.cfg.Bucket, objectPath, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	defer object.Close()

	// check if the object exists
	if _, err := object.Stat(); err != nil {
		return nil, err
	}

	// Read all bytes
	data, err := io.ReadAll(object)
	if err != nil {
		return nil, err
	}

	return data, nil
}
