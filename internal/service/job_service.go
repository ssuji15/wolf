package service

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/ssuji15/wolf/internal/db"
	"github.com/ssuji15/wolf/internal/storage"
	"github.com/ssuji15/wolf/model"
)

type JobService struct {
	repo  *db.JobRepository
	minio storage.Storage
}

func NewJobService(dbClient *db.DB, minio storage.Storage) *JobService {
	return &JobService{
		repo:  db.NewJobRepository(dbClient),
		minio: minio,
	}
}

func (s *JobService) CreateJob(ctx context.Context, input model.JobRequest) (model.JobResponse, error) {

	// ---------- Step 1: Decode Base64 ----------
	decoded, err := base64.StdEncoding.DecodeString(input.CodeBase64)
	if err != nil {
		return model.JobResponse{}, fmt.Errorf("invalid base64 code: %w", err)
	}

	// ---------- Step 2: Compute SHA256 Hash ----------
	hashBytes := sha256.Sum256(decoded)
	codeHash := fmt.Sprintf("%x", hashBytes[:])

	// ---------- Step 3: Upload to MinIO (S3) ----------
	jobID := uuid.New()
	objectPath := fmt.Sprintf("jobs/%s/code.bin", jobID.String())

	if err := s.minio.Upload(ctx, objectPath, decoded); err != nil {
		return model.JobResponse{}, fmt.Errorf("failed to upload code to minio: %w", err)
	}

	// ---------- Step 4: Build Job model ----------
	now := time.Now()

	job := model.Job{
		ID:              jobID,
		ExecutionEngine: input.ExecutionEngine,
		CodePath:        objectPath,
		CodeHash:        codeHash,
		Status:          "Pending",
		OutputPath:      nil, // will be set when execution completes
		CreationTime:    now,
		StartTime:       nil,
		EndTime:         nil,
		RetryCount:      0,
		OutputHash:      nil,
	}

	// ---------- Step 5: Insert into DB ----------
	_, err = s.repo.CreateJob(ctx, job, input.Tags)
	if err != nil {
		return model.JobResponse{}, fmt.Errorf("db insert failed: %w", err)
	}

	return s.getJobResponse(ctx, &job)
}

func (s *JobService) ListJobs(ctx context.Context) ([]model.JobResponse, error) {
	jobs, err := s.repo.ListJobs(ctx)
	if err != nil {
		return []model.JobResponse{}, fmt.Errorf("unable to retrieve from db: %w", err)
	}
	var response []model.JobResponse
	for _, j := range jobs {
		r, err := s.getJobResponse(ctx, j)
		if err != nil {
			return []model.JobResponse{}, fmt.Errorf("unable to retrieve from db: %w", err)
		}
		response = append(response, r)
	}
	return response, nil
}

func (s *JobService) GetJob(ctx context.Context, uuid string) (model.JobResponse, error) {

	// 1. Retrieve Job from DB
	job, err := s.repo.GetByID(ctx, uuid)
	if err != nil {
		return model.JobResponse{}, fmt.Errorf("unable to retrieve from db: %w", err)
	}

	return s.getJobResponse(ctx, job)
}

func (s *JobService) getJobResponse(ctx context.Context, job *model.Job) (model.JobResponse, error) {
	// 2. Retrieve Code from storage
	codeRaw, err := s.minio.Download(ctx, job.CodePath)
	if err != nil {
		return model.JobResponse{}, fmt.Errorf("unable to retrieve code from storage: %w", err)
	}
	codeEncoded := base64.StdEncoding.EncodeToString(codeRaw)

	// 3. Retrieve output from storage if it exist

	outputEncoded := ""
	if job.OutputPath != nil {
		outputRaw, err := s.minio.Download(ctx, *job.OutputPath)
		if err != nil {
			return model.JobResponse{}, fmt.Errorf("unable to retrieve output from storage: %w", err)
		}
		outputEncoded = base64.StdEncoding.EncodeToString(outputRaw)
	}

	response := model.JobResponse{
		ID:              job.ID,
		ExecutionEngine: job.ExecutionEngine,
		CodeBase64:      codeEncoded,
		Status:          job.Status,
		OutputBase64:    outputEncoded,
		CreationTime:    job.CreationTime,
		StartTime:       job.StartTime,
		EndTime:         job.EndTime,
	}
	return response, nil
}
