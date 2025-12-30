package minio

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/ssuji15/wolf/internal/config"
	"github.com/ssuji15/wolf/internal/job_tracer"
	"github.com/ssuji15/wolf/internal/storage"
	"github.com/ssuji15/wolf/internal/util"
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
	client    *minio.Client
	cfg       MinioConfig
	transport *http.Transport
}

// NewMinioClient initializes and returns a MinIO client.
func NewMinioClient(cfg MinioConfig) (storage.Storage, error) {

	transport := &http.Transport{
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   50,
		MaxConnsPerHost:       50,
		IdleConnTimeout:       120 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,

		DisableCompression: true,
		DisableKeepAlives:  false,
	}

	cli, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:     credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure:    cfg.UseSSL,
		Transport: transport,
	})
	if err != nil {
		return nil, err
	}

	return &MinioClient{client: cli, cfg: cfg, transport: transport}, nil
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

	// upload
	reader := bytes.NewReader(code)

	_, err := m.client.PutObject(ctx, m.cfg.Bucket, objectPath, reader, -1, minio.PutObjectOptions{})
	if err != nil {
		util.RecordSpanError(span, err)
		return err
	}

	return nil
}

// Download files to Minio
func (m *MinioClient) Download(ctx context.Context, objectPath string) ([]byte, error) {

	tracer := job_tracer.GetTracer()
	ctx, span := tracer.Start(ctx, "MinIO/Download")
	defer span.End()

	// Get the object
	object, err := m.client.GetObject(ctx, m.cfg.Bucket, objectPath, minio.GetObjectOptions{})
	if err != nil {
		util.RecordSpanError(span, err)
		return nil, err
	}
	defer object.Close()

	// check if the object exists
	if _, err := object.Stat(); err != nil {
		util.RecordSpanError(span, err)
		return nil, err
	}

	// Read all bytes
	data, err := io.ReadAll(object)
	if err != nil {
		util.RecordSpanError(span, err)
		return nil, err
	}

	return data, nil
}

func (m *MinioClient) Close() {
	m.transport.CloseIdleConnections()
}
