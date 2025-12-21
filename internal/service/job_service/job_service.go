package jobservice

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/ssuji15/wolf/internal/component"
	"github.com/ssuji15/wolf/internal/db/repository"
	"github.com/ssuji15/wolf/internal/queue"
	"github.com/ssuji15/wolf/internal/service/logger"
	"github.com/ssuji15/wolf/internal/util"
	"github.com/ssuji15/wolf/model"
)

type JobService struct {
	repo *repository.JobRepository
	comp *component.Components
}

type JobEvent string

const (
	JOB_PENDING    JobEvent = "PENDING"
	JOB_COMPLETED  JobEvent = "COMPLETED"
	JOB_FAILED     JobEvent = "FAILED"
	JOB_DISPATCHED JobEvent = "DISPATCHED"
	JOB_PUBLISHED  JobEvent = "PUBLISHED"
)

var jobService *JobService

func NewJobService(comp *component.Components) *JobService {
	jobService = &JobService{
		comp: comp,
		repo: repository.NewJobRepository(comp.DBClient),
	}
	go publishJobsToQueue(comp.Ctx)
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

	// ---------- Step 2: Upload to MinIO (S3) ----------
	jobID, err := uuid.NewV7()
	if err != nil {
		return model.Job{}, err
	}
	if err := s.comp.StorageClient.Upload(ctx, util.GetCodePath(codeHash), code); err != nil {
		return model.Job{}, err
	}

	// ---------- Step 3: Build Job model ----------
	now := time.Now().UTC()
	job := model.Job{
		ID:              jobID,
		ExecutionEngine: input.ExecutionEngine,
		CodeHash:        codeHash,
		Status:          string(JOB_PENDING),
		CreationTime:    &now,
		StartTime:       nil,
		EndTime:         nil,
		RetryCount:      0,
		OutputHash:      "",
	}

	// ---------- Step 4: Insert into DB ----------
	err = s.repo.CreateJob(ctx, job, input.Tags)
	if err != nil {
		return model.Job{}, fmt.Errorf("db insert failed: %w", err)
	}

	// ---------- Step 5: Add Job to cache --------------
	err = s.comp.Cache.Put(ctx, job.ID.String(), job, s.comp.Cache.GetDefaultTTL())
	if err != nil {
		logger.Log.Error().Err(err).Str("id", job.ID.String()).Msg("writing to local cache failed.")
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

	// 1. Retrieve from cache
	job := &model.Job{}
	err := s.comp.Cache.Get(ctx, id, job)
	if err == nil {
		return job, nil
	}

	// 2. Retrieve Job from DB
	job, err = s.repo.GetJobByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve job %s from db: %w", id, err)
	}

	// 3. Add job to cache, ignore error
	err = s.comp.Cache.Put(ctx, id, job, s.comp.Cache.GetDefaultTTL())
	if err != nil {
		logger.Log.Error().Err(err).Msg("Unable to add job to cache")
	}
	return job, nil
}

func (s *JobService) GetCode(ctx context.Context, job *model.Job) ([]byte, error) {
	codeRaw, err := s.comp.StorageClient.Download(ctx, util.GetCodePath(job.CodeHash))
	if err != nil {
		return []byte{}, fmt.Errorf("unable to retrieve code from storage: %w", err)
	}
	return codeRaw, nil
}

func (s *JobService) GetOutput(ctx context.Context, job *model.Job) ([]byte, error) {
	if job.OutputHash != "" {
		o, err := s.comp.StorageClient.Download(ctx, util.GetOutputPath(job.OutputHash))
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

	// 2. Update cache
	err = s.comp.Cache.Put(ctx, job.ID.String(), job, s.comp.Cache.GetDefaultTTL())
	if err != nil {
		logger.Log.Error().Err(err).Msg("Unable to add job to cache")
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

func publishJobsToQueue(ctx context.Context) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			jobs, err := jobService.repo.ClaimOutboxJobs(ctx)
			if err != nil {
				logger.Log.Error().Err(err).Msg("claim outbox jobs failed")
				continue
			}

			for _, j := range jobs {
				err := jobService.comp.QClient.PublishEvent(ctx, queue.JobCreated, j)
				if err != nil {
					logger.Log.Error().Err(err).Str("id", j).Msg("publish failed")
					jobService.repo.OutboxJobFailed(ctx, j)
					continue
				}
				jobService.repo.OutboxJobPublished(ctx, j)
			}
		}
	}
}

func (s *JobService) CacheOutput(ctx context.Context, j *model.Job) error {
	return s.comp.Cache.Put(ctx, j.CodeHash, j.OutputHash, s.comp.Cache.GetDefaultTTL())
}

func (s *JobService) GetOutputHashFromCache(ctx context.Context, j *model.Job) (string, error) {
	var outputHash string
	err := s.comp.Cache.Get(ctx, j.CodeHash, &outputHash)
	return outputHash, err
}
