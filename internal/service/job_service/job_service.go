package jobservice

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/ssuji15/wolf/internal/cache"
	"github.com/ssuji15/wolf/internal/db"
	"github.com/ssuji15/wolf/internal/db/repository"
	"github.com/ssuji15/wolf/internal/job_tracer"
	"github.com/ssuji15/wolf/internal/queue"
	"github.com/ssuji15/wolf/internal/service/logger"
	"github.com/ssuji15/wolf/internal/storage"
	"github.com/ssuji15/wolf/internal/util"
	"github.com/ssuji15/wolf/model"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
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
	EXECUTION_ENGINE = []string{"c++"}
)

var (
	ErrNotFound               = errors.New("Job not found")
	ErrInvalidId              = errors.New("Invalid ID")
	ErrInvalidOffset          = errors.New("invalid offset")
	ErrInvalidExecutionEngine = errors.New("invalid execution engine")
)

var (
	jobService *JobService
)

func NewJobService(ctx context.Context, c cache.Cache, s storage.Storage, q queue.Queue, db *db.DB) (*JobService, error) {
	if jobService != nil {
		return jobService, nil
	}
	jr := repository.NewJobRepository(ctx, db)

	jobService = &JobService{
		repo:    jr,
		cache:   c,
		queue:   q,
		storage: s,
	}
	return jobService, nil
}

func (s *JobService) CreateJob(ctx context.Context, input model.JobRequest) (*model.Job, error) {
	tracer := job_tracer.GetTracer()
	ctx, span := tracer.Start(ctx, "JobService/CreateJob")
	defer span.End()

	err := s.validateInputRequest(input)
	if err != nil {
		if errors.Is(err, ErrInvalidExecutionEngine) {
			return nil, ErrInvalidExecutionEngine
		}
		return nil, fmt.Errorf("validation error: %v", err)
	}

	// ---------- Step 1: Compute SHA256 Hash ----------
	hashBytes := sha256.Sum256(input.Code)
	codeHash := fmt.Sprintf("%x", hashBytes[:])

	jobID, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("err generating UUID: %v", err)
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
		return nil, err
	}

	// ---------- Step 4: Add Code to cache --------------
	err = s.cache.Put(ctx, util.GetCodeKey(codeHash), input.Code, s.cache.GetDefaultTTL())
	if err != nil {
		return nil, err
	}

	// ---------- Step 5: Publish event to Q --------------
	err = s.queue.PublishEvent(ctx, queue.JobCreated, jobID.String())
	if err != nil {
		return nil, err
	}

	return &job, nil
}

func (s *JobService) ListJobs(ctx context.Context, offset string) ([]*model.Job, error) {
	tracer := job_tracer.GetTracer()
	ctx, span := tracer.Start(ctx, "JobService/ListJobs")
	defer span.End()
	if offset != "" {
		parsed, err := uuid.Parse(offset)
		if err != nil {
			return nil, ErrInvalidOffset
		}

		if parsed.Version() != 7 {
			return nil, ErrInvalidOffset
		}
	}

	jobs, err := s.repo.ListJobs(ctx, offset)
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve jobs from db: %w", err)
	}
	return jobs, nil
}

func (s *JobService) GetJobByIdempotencyKey(ctx context.Context, key string) (*model.Job, error) {

	parsed, err := uuid.Parse(key)
	if err != nil {
		return nil, ErrInvalidId
	}

	if parsed.Version() != 4 {
		return nil, ErrInvalidId
	}

	var jobId string
	err = s.cache.Get(ctx, util.GetIdempotencyKey(key), &jobId)
	if err != nil {
		return nil, ErrNotFound
	}
	return s.GetJob(ctx, jobId)
}

func (s *JobService) GetJob(ctx context.Context, id string) (*model.Job, error) {
	tracer := job_tracer.GetTracer()
	ctx, span := tracer.Start(ctx, "JobService/GetJob")
	defer span.End()

	parsed, err := uuid.Parse(id)
	if err != nil {
		return nil, ErrInvalidId
	}

	if parsed.Version() != 7 {
		return nil, ErrInvalidId
	}

	// 1. Retrieve from cache
	job := &model.Job{}
	err = s.cache.Get(ctx, id, job)
	if err == nil {
		return job, nil
	}

	// 2. Retrieve Job from DB
	job, err = s.repo.GetJobByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("unable to retrieve job %s from db at the moment..", id)
	}

	// 3. Add job to cache, ignore error
	err = s.cache.Put(ctx, id, job, s.cache.GetDefaultTTL())
	if err != nil {
		logger.Log.Error().Err(err).Msg("Unable to add job to cache")
	}
	return job, nil
}

func (s *JobService) GetCode(ctx context.Context, job *model.Job) ([]byte, error) {
	tracer := job_tracer.GetTracer()
	ctx, span := tracer.Start(ctx, "JobService/GetCode")
	defer span.End()
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
	tracer := job_tracer.GetTracer()
	ctx, span := tracer.Start(ctx, "JobService/GetOutput")
	defer span.End()
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
	tracer := job_tracer.GetTracer()
	ctx, span := tracer.Start(ctx, "JobService/UpdateJob")
	defer span.End()
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
	tracer := job_tracer.GetTracer()
	sub, err := s.queue.SubscribeEvent(queue.JobCreated, queue.JOB_DB_CONSUMER)
	if err != nil {
		log.Fatalf("unable to subscribe to Nats events: %v", err)
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
			// should parameterise fetch and timeout numbers.
			msgs, err := sub.Fetch(10, 250*time.Millisecond)
			if err != nil {
				if errors.Is(err, nats.ErrTimeout) || errors.Is(err, nats.ErrSubscriptionClosed) {
					continue
				}
				logger.Log.Error().Err(err).Msg("failed to fetch messages")
				continue
			}
			var jobs []*model.Job
			validMsgs := make([]queue.QMsg, 0, len(msgs))
			spans := make([]trace.Span, 0, len(msgs))
			batchId, err := uuid.NewV7()

			if err != nil {
				logger.Log.Error().Err(err)
				continue
			}
			for _, msg := range msgs {
				ctx, span := tracer.Start(msg.Ctx(), "PersistJOBToDB")
				span.AddEvent("batch.context",
					trace.WithAttributes(attribute.String("batchId", batchId.String())),
				)
				id := string(msg.Data())
				j, err := s.GetJob(ctx, id)
				if err != nil {
					logger.Log.Error().Err(err).Str("id", id).Msg("Unable to retrieve job")
					span.RecordError(err)
					span.End()
					continue
				}
				jobs = append(jobs, j)
				validMsgs = append(validMsgs, msg)
				spans = append(spans, span)
			}

			_, bspan := tracer.Start(context.Background(), "BatchJOBsToDB")
			bspan.AddEvent("batch.context",
				trace.WithAttributes(attribute.String("batchId", batchId.String())),
			)

			if err := s.repo.CreateJobs(ctx, jobs); err != nil {
				logger.Log.Error().Err(err).Msg("failed to persist jobs to db.")
				bspan.RecordError(err)
				continue
			}

			for idx, msg := range validMsgs {
				msg.Ack()
				spans[idx].End()
				bspan.AddLink(trace.Link{
					SpanContext: spans[idx].SpanContext(),
				})
			}
			bspan.End()
		}
	}
}

func (s *JobService) PersistCodeToDB(ctx context.Context) {
	tracer := job_tracer.GetTracer()
	sub, err := s.queue.SubscribeEvent(queue.JobCreated, queue.JOB_CODE_CONSUMER)
	if err != nil {
		log.Fatalf("unable to subscribe to Nats events: %v", err)
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
			// should parameterise fetch and timeout numbers.
			msgs, err := sub.Fetch(10, 250*time.Millisecond)
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

					ctx, span := tracer.Start(msg.Ctx(), "PersistCodeToDB")
					defer span.End()

					id := string(msg.Data())
					j, err := s.GetJob(ctx, id)
					if err != nil {
						logger.Log.Error().Err(err).Str("id", id).Msg("Unable to retrieve job")
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

func (s *JobService) validateInputRequest(input model.JobRequest) error {
	if !slices.Contains(EXECUTION_ENGINE, input.ExecutionEngine) {
		return ErrInvalidExecutionEngine
	}
	return nil
}

func (s *JobService) AddIdempotencyKey(ctx context.Context, ik string, j *model.Job) error {
	return s.cache.Put(ctx, util.GetIdempotencyKey(ik), j.ID.String(), s.cache.GetDefaultTTL())
}
