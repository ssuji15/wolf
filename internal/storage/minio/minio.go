package minio

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/ssuji15/wolf/internal/config"
	"github.com/ssuji15/wolf/internal/job_tracer"
	"github.com/ssuji15/wolf/internal/storage"
	"github.com/ssuji15/wolf/internal/util"
)

// MinioClient wraps the MinIO SDK client.
type MinioClient struct {
	client    *minio.Client
	cfg       *config.MinioConfig
	transport *http.Transport
}

var (
	m         *MinioClient
	once      sync.Once
	initError error
)

// NewMinioClient initializes and returns a MinIO client.
func NewMinioClient() (storage.Storage, error) {

	once.Do(func() {
		cfg, err := config.GetMinioConfig()
		if err != nil {
			initError = err
			return
		}

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

		cli, err := minio.New(cfg.URL, &minio.Options{
			Creds:     credentials.NewStaticV4(cfg.ACCESS_KEY, cfg.SECRET_KEY, ""),
			Secure:    cfg.USE_SSL,
			Transport: transport,
		})
		if err != nil {
			initError = err
			return
		}
		m = &MinioClient{client: cli, cfg: cfg, transport: transport}
	})

	return m, initError
}

// Uploads files to Minio.
func (m *MinioClient) Upload(ctx context.Context, bucket string, objectPath string, code []byte) error {

	tracer := job_tracer.GetTracer()
	ctx, span := tracer.Start(ctx, "MinIO/Upload")
	defer span.End()

	// upload
	reader := bytes.NewReader(code)

	_, err := m.client.PutObject(ctx, bucket, objectPath, reader, -1, minio.PutObjectOptions{})
	if err != nil {
		util.RecordSpanError(span, err)
		return err
	}

	return nil
}

// Download files to Minio
func (m *MinioClient) Download(ctx context.Context, bucket string, objectPath string) ([]byte, error) {

	tracer := job_tracer.GetTracer()
	ctx, span := tracer.Start(ctx, "MinIO/Download")
	defer span.End()

	// Get the object
	object, err := m.client.GetObject(ctx, bucket, objectPath, minio.GetObjectOptions{})
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

func (m *MinioClient) GetJobsBucket() string {
	return m.cfg.JOBS_BUCKET
}

func (m *MinioClient) ShutDown(ctx context.Context) {
	done := make(chan struct{})

	go func() {
		m.Close()
		close(done)
	}()

	select {
	case <-done:
		return
	case <-ctx.Done():
		return
	}
}
