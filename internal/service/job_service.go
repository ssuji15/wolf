package service

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
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

	// ---------- Step 1: Decode Base64 ----------
	decoded, err := base64.StdEncoding.DecodeString(input.CodeBase64)
	if err != nil {
		return model.Job{}, fmt.Errorf("invalid base64 code: %w", err)
	}

	// ---------- Step 2: Compute SHA256 Hash ----------
	hashBytes := sha256.Sum256(decoded)
	codeHash := fmt.Sprintf("%x", hashBytes[:])

	// ---------- Step 3: Upload to MinIO (S3) ----------
	jobID := uuid.New()
	objectPath := fmt.Sprintf("jobs/%s/code.bin", jobID.String())

	if err := s.storageClient.Upload(ctx, objectPath, decoded); err != nil {
		return model.Job{}, fmt.Errorf("failed to upload code to minio: %w", err)
	}

	// ---------- Step 4: Build Job model ----------
	now := time.Now()

	job := model.Job{
		ID:              jobID,
		ExecutionEngine: input.ExecutionEngine,
		Code:            string(decoded),
		CodePath:        objectPath,
		CodeHash:        codeHash,
		Status:          "Pending",
		OutputPath:      "", // will be set when execution completes
		CreationTime:    &now,
		StartTime:       nil,
		EndTime:         nil,
		RetryCount:      0,
		OutputHash:      "",
		Output:          "",
	}

	// ---------- Step 5: Insert into DB ----------
	_, err = s.repo.CreateJob(ctx, job, input.Tags)
	if err != nil {
		return model.Job{}, fmt.Errorf("db insert failed: %w", err)
	}

	// ---------- Step 6: Add Job to Redis cache --------------

	// ---------- Step 7: Add Job to local cache --------------
	err = s.jobLocalCache.Put(job.ID.String(), job)

	if err != nil {
		return model.Job{}, fmt.Errorf("writing to cache failed: %w", err)
	}

	// ---------- Step 8: Publish Create Event ----------
	err = s.qClient.PublishEvent(queue.JobCreated, job.ID.String())
	if err != nil {
		return model.Job{}, fmt.Errorf("publishing to queue failed: %w", err)
	}

	return job, nil
}

func (s *JobService) ListJobs(ctx context.Context) ([]*model.Job, error) {
	jobs, err := s.repo.ListJobs(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve from db: %w", err)
	}
	for _, j := range jobs {
		s.retrieveCodeAndOutput(ctx, j)
		if err != nil {
			return nil, fmt.Errorf("unable to retrieve from db: %w", err)
		}
	}
	return jobs, nil
}

func (s *JobService) GetJob(ctx context.Context, id string) (*model.Job, error) {

	if id == "" {
		return nil, fmt.Errorf("id cannot be empty")
	}

	// 1. Retrieve from local cache
	job := &model.Job{}
	s.jobLocalCache.Get(id, job)

	if job.ID != uuid.Nil {
		return job, nil
	}

	// 2. Retrieve from Redis

	// 3. Retrieve Job from DB
	job, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve job %s from db: %w", id, err)
	}
	s.retrieveCodeAndOutput(ctx, job)
	return job, nil
}

func (s *JobService) retrieveCodeAndOutput(ctx context.Context, job *model.Job) error {

	// 1. Retrieve Code from storage
	codeRaw, err := s.storageClient.Download(ctx, job.CodePath)
	if err != nil {
		return fmt.Errorf("unable to retrieve code from storage: %w", err)
	}

	// 2. Retrieve output from storage if it exist
	outputRaw := ""
	if job.OutputPath != "" {
		o, err := s.storageClient.Download(ctx, job.OutputPath)
		if err != nil {
			return fmt.Errorf("unable to retrieve output from storage: %w", err)
		}
		outputRaw = string(o)
	}

	job.Code = string(codeRaw)
	job.Output = outputRaw

	return nil
}

func (s *JobService) UpdateJob(ctx context.Context, job *model.Job) error {
	// 1. Update database
	_, err := s.repo.UpdateJob(ctx, job)
	if err != nil {
		return fmt.Errorf("db update failed: %w", err)
	}

	// 2. Update local cache
	err = s.jobLocalCache.Put(job.ID.String(), job)
	if err != nil {
		return fmt.Errorf("local cache update failed: %w", err)
	}

	return nil
}
