package jobservice

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/ssuji15/wolf/internal/cache"
	"github.com/ssuji15/wolf/internal/db"
	"github.com/ssuji15/wolf/internal/db/repository"
	"github.com/ssuji15/wolf/internal/queue"
	"github.com/ssuji15/wolf/internal/storage"
	"github.com/ssuji15/wolf/model"
)

type JobService struct {
	repo          *repository.JobRepository
	storageClient storage.Storage
	qClient       queue.Queue
	jobLocalCache cache.Cache
}

type JobEvent string

const (
	JobCreated   JobEvent = "Created"
	JobPending   JobEvent = "Pending"
	JobCompleted JobEvent = "Completed"
)

var jobService *JobService

func NewJobService(dbClient *db.DB, storageClient_ storage.Storage, qClient_ queue.Queue, cache cache.Cache) *JobService {
	jobService = &JobService{
		repo:          repository.NewJobRepository(dbClient),
		storageClient: storageClient_,
		qClient:       qClient_,
		jobLocalCache: cache,
	}
	return jobService
}

func GetJobService() *JobService {
	return jobService
}

func (s *JobService) CreateJob(ctx context.Context, input model.JobRequest) (model.Job, error) {
	code := []byte(input.Code)
	// ---------- Step 1: Compute SHA256 Hash ----------
	hashBytes := sha256.Sum256(code)
	codeHash := fmt.Sprintf("%x", hashBytes[:])

	// ---------- Step 3: Upload to MinIO (S3) ----------
	jobID := uuid.New()
	objectPath := fmt.Sprintf("jobs/%s/code.bin", jobID.String())

	if err := s.storageClient.Upload(ctx, objectPath, code); err != nil {
		return model.Job{}, err
	}

	// ---------- Step 4: Build Job model ----------
	now := time.Now()

	job := model.Job{
		ID:              jobID,
		ExecutionEngine: input.ExecutionEngine,
		CodePath:        objectPath,
		CodeHash:        codeHash,
		Status:          string(JobPending),
		OutputPath:      "", // will be set when execution completes
		CreationTime:    &now,
		StartTime:       nil,
		EndTime:         nil,
		RetryCount:      0,
		OutputHash:      "",
	}

	// ---------- Step 5: Insert into DB ----------
	_, err := s.repo.CreateJob(ctx, job, input.Tags)
	if err != nil {
		return model.Job{}, fmt.Errorf("db insert failed: %w", err)
	}

	// ---------- Step 6: Add Job to Redis cache --------------

	// ---------- Step 7: Add Job to local cache --------------
	err = s.jobLocalCache.Put(job.ID.String(), job, s.jobLocalCache.GetDefaultTTL())

	if err != nil {
		return model.Job{}, fmt.Errorf("writing to cache failed: %w", err)
	}

	// ---------- Step 8: Publish Create Event ----------
	err = s.qClient.PublishEvent(ctx, queue.JobCreated, job.ID.String())
	if err != nil {
		return model.Job{}, fmt.Errorf("publishing to queue failed: %w", err)
	}

	return job, nil
}

func (s *JobService) ListJobs(ctx context.Context) ([]*model.Job, error) {
	jobs, err := s.repo.ListJobs(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve jobs from db: %w", err)
	}
	return jobs, nil
}

func (s *JobService) GetJob(ctx context.Context, id string) (*model.Job, error) {

	if id == "" {
		return nil, fmt.Errorf("id cannot be empty")
	}

	// 1. Retrieve from local cache
	job := &model.Job{}
	err := s.jobLocalCache.Get(id, job)
	if err == nil {
		return job, nil
	}

	// 2. Retrieve from Redis

	// 3. Retrieve Job from DB
	job, err = s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve job %s from db: %w", id, err)
	}
	return job, nil
}

func (s *JobService) GetCode(ctx context.Context, job *model.Job) ([]byte, error) {
	codeRaw, err := s.storageClient.Download(ctx, job.CodePath)
	if err != nil {
		return []byte{}, fmt.Errorf("unable to retrieve code from storage: %w", err)
	}
	return codeRaw, nil
}

func (s *JobService) GetOutput(ctx context.Context, job *model.Job) ([]byte, error) {
	if job.OutputPath != "" {
		o, err := s.storageClient.Download(ctx, job.OutputPath)
		if err != nil {
			return []byte{}, fmt.Errorf("unable to retrieve output from storage: %w", err)
		}
		return o, nil
	}
	return []byte{}, nil
}

func (s *JobService) UpdateJob(ctx context.Context, job *model.Job) error {
	// 1. Update database
	_, err := s.repo.UpdateJob(ctx, job)
	if err != nil {
		return fmt.Errorf("db update failed: %w", err)
	}

	// 2. Update local cache
	err = s.jobLocalCache.Put(job.ID.String(), job, s.jobLocalCache.GetDefaultTTL())
	if err != nil {
		return fmt.Errorf("local cache update failed: %w", err)
	}

	return nil
}

func (s *JobService) DownloadOutput(ctx context.Context, id string) ([]byte, error) {
	job, err := s.GetJob(ctx, id)
	if err != nil {
		return []byte{}, err
	}
	return s.GetOutput(ctx, job)
}

func (s *JobService) DownloadCode(ctx context.Context, id string) ([]byte, error) {
	job, err := s.GetJob(ctx, id)
	if err != nil {
		return []byte{}, err
	}
	return s.GetCode(ctx, job)
}
