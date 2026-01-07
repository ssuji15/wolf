package jobservice

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/ssuji15/wolf/internal/cache"
	"github.com/ssuji15/wolf/internal/db/repository"
	"github.com/ssuji15/wolf/internal/queue"
	"github.com/ssuji15/wolf/internal/service/logger"
	"github.com/ssuji15/wolf/internal/storage"
	"github.com/ssuji15/wolf/internal/util"
	"github.com/ssuji15/wolf/model"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

const (
	JOB_DB_CONSUMER   = "JOB_DB"
	JOB_CODE_CONSUMER = "JOB_CODE"
)

type JobService struct {
	repo    *repository.JobRepository
	cache   cache.Cache
	storage storage.Storage
	queue   queue.Queue
}

type JobEvent string

const (
	JOB_PENDING    JobEvent = "PENDING"
	JOB_COMPLETED  JobEvent = "COMPLETED"
	JOB_FAILED     JobEvent = "FAILED"
	JOB_DISPATCHED JobEvent = "DISPATCHED"
	JOB_PUBLISHED  JobEvent = "PUBLISHED"
)

var (
	jobService *JobService
	once       sync.Once
	initError  error
)

func NewJobService(ctx context.Context, c cache.Cache, s storage.Storage, q queue.Queue) (*JobService, error) {
	once.Do(func() {
		jr, err := repository.NewJobRepository(ctx)
		if err != nil {
			initError = err
			return
		}

		jobService = &JobService{
			repo:    jr,
			cache:   c,
			queue:   q,
			storage: s,
		}
	})
	return jobService, initError
}

func (s *JobService) CreateJob(ctx context.Context, input model.JobRequest) (model.Job, error) {
	// ---------- Step 1: Compute SHA256 Hash ----------
	hashBytes := sha256.Sum256(input.Code)
	codeHash := fmt.Sprintf("%x", hashBytes[:])

	jobID, err := uuid.NewV7()
	if err != nil {
		return model.Job{}, err
	}

	// ---------- Step 2: Build Job model ----------
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
		Tags:            input.Tags,
	}

	// check if the codehash is in cache, if hit, use the output.
	oh, err := s.GetOutputHashFromCache(ctx, &job)
	if err == nil && oh != "" {
		job.OutputHash = oh
		job.Status = string(JOB_COMPLETED)
		job.StartTime = &now
		job.EndTime = &now
	}

	// ---------- Step 3: Add Job to cache --------------
	err = s.cache.Put(ctx, job.ID.String(), job, s.cache.GetDefaultTTL())
	if err != nil {
		return model.Job{}, err
	}

	// ---------- Step 4: Add Code to cache --------------
	err = s.cache.Put(ctx, util.GetCodeKey(codeHash), input.Code, s.cache.GetDefaultTTL())
	if err != nil {
		return model.Job{}, err
	}

	// ---------- Step 5: Publish event to Q --------------
	err = s.queue.PublishEvent(ctx, queue.JobCreated, jobID.String())
	if err != nil {
		return model.Job{}, err
	}

	return job, nil
}

func (s *JobService) ListJobs(ctx context.Context, offset string) ([]*model.Job, error) {
	jobs, err := s.repo.ListJobs(ctx, offset)
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
	err := s.cache.Get(ctx, id, job)
	if err == nil {
		return job, nil
	}

	// 2. Retrieve Job from DB
	job, err = s.repo.GetJobByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve job %s from db: %w", id, err)
	}

	// 3. Add job to cache, ignore error
	err = s.cache.Put(ctx, id, job, s.cache.GetDefaultTTL())
	if err != nil {
		logger.Log.Error().Err(err).Msg("Unable to add job to cache")
	}
	return job, nil
}

func (s *JobService) GetCode(ctx context.Context, job *model.Job) ([]byte, error) {
	// 1. Retrieve from cache
	var codeRaw []byte
	err := s.cache.Get(ctx, util.GetCodeKey(job.CodeHash), &codeRaw)
	if err == nil {
		return codeRaw, nil
	}

	// 2. Retrieve Job from Storage
	codeRaw, err = s.storage.Download(ctx, s.storage.GetJobsBucket(), job.CodeHash)
	if err != nil {
		return []byte{}, fmt.Errorf("unable to retrieve code from storage: %w", err)
	}
	return codeRaw, nil
}

func (s *JobService) GetOutput(ctx context.Context, job *model.Job) ([]byte, error) {
	if job.OutputHash != "" {
		o, err := s.storage.Download(ctx, s.storage.GetJobsBucket(), util.GetOutputPath(job.OutputHash))
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
	err = s.cache.Put(ctx, job.ID.String(), job, s.cache.GetDefaultTTL())
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

func (s *JobService) PersistJobsToDB(ctx context.Context) {

	err := s.queue.AddConsumer(queue.EventStream, JOB_DB_CONSUMER)
	if err != nil {
		log.Fatalf("unable to add job consumer: %v", err)
	}
	sub, err := s.queue.SubscribeEvent(queue.JobCreated, JOB_DB_CONSUMER)
	if err != nil {
		log.Fatalf("unable to subscribe to Nats events: %v", err)
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
			// should parameterise fetch and timeout numbers.
			msgs, err := sub.Fetch(ctx, 10, 250*time.Millisecond)
			if err != nil {
				if errors.Is(err, nats.ErrTimeout) || errors.Is(err, nats.ErrSubscriptionClosed) {
					continue
				}
				logger.Log.Error().Err(err).Msg("failed to fetch messages")
				continue
			}
			var jobs []*model.Job
			validMsgs := make([]queue.QMsg, 0, len(msgs))
			for _, msg := range msgs {
				id := string(msg.Data())
				j, err := s.GetJob(msg.Ctx(), id)
				if err != nil {
					logger.Log.Error().Err(err).Str("id", id).Msg("Unable to retrieve job")
					continue
				}
				jobs = append(jobs, j)
				validMsgs = append(validMsgs, msg)
			}

			if err := s.repo.CreateJobs(ctx, jobs); err != nil {
				logger.Log.Error().Err(err).Msg("failed to persist jobs to db.")
				continue
			}
			for _, msg := range validMsgs {
				msg.Ack()
			}
		}
	}
}

func (s *JobService) PersistCodeToDB(ctx context.Context) {
	err := s.queue.AddConsumer(queue.EventStream, JOB_CODE_CONSUMER)
	if err != nil {
		log.Fatalf("unable to add code consumer: %v", err)
	}
	sub, err := s.queue.SubscribeEvent(queue.JobCreated, JOB_CODE_CONSUMER)
	if err != nil {
		log.Fatalf("unable to subscribe to Nats events: %v", err)
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
			// should parameterise fetch and timeout numbers.
			msgs, err := sub.Fetch(ctx, 10, 250*time.Millisecond)
			if err != nil {
				if errors.Is(err, nats.ErrTimeout) || errors.Is(err, nats.ErrSubscriptionClosed) {
					continue
				}
				logger.Log.Error().Err(err).Msg("failed to fetch messages")
				continue
			}
			wg := &sync.WaitGroup{}
			for _, msg := range msgs {
				wg.Add(1)
				go func() {
					defer wg.Done()
					id := string(msg.Data())
					j, err := s.GetJob(msg.Ctx(), id)
					if err != nil {
						logger.Log.Error().Err(err).Str("id", id).Msg("Unable to retrieve job")
						return
					}
					if j.Status != string(JOB_PENDING) {
						msg.Ack()
						return
					}
					sourceCode, err := s.GetCode(ctx, j)
					if err != nil {
						logger.Log.Error().Err(err).Str("id", id).Msg("Unable to retrieve code")
						return
					}
					if err := s.storage.Upload(ctx, s.storage.GetJobsBucket(), j.CodeHash, sourceCode); err != nil {
						logger.Log.Error().Err(err).Str("id", id).Msg("Unable to upload code")
						return
					}
					msg.Ack()
				}()
			}
			wg.Wait()
		}
	}
}

func (s *JobService) CacheOutputHash(ctx context.Context, j *model.Job) error {
	return s.cache.Put(ctx, util.GetOutputHashKey(j.CodeHash), j.OutputHash, s.cache.GetDefaultTTL())
}

func (s *JobService) GetOutputHashFromCache(ctx context.Context, j *model.Job) (string, error) {
	var outputHash string
	err := s.cache.Get(ctx, util.GetOutputHashKey(j.CodeHash), &outputHash)
	return outputHash, err
}

func RestoreTraceContext(traceparent string, tracestate *string) context.Context {
	carrier := propagation.MapCarrier{
		"traceparent": traceparent,
	}
	if tracestate != nil && *tracestate != "" {
		carrier["tracestate"] = *tracestate
	}
	return otel.GetTextMapPropagator().Extract(context.Background(), carrier)
}
