package model

import (
	"time"

	"github.com/google/uuid"
)

// Job represents a job record stored in the database.
type Job struct {
	ID              uuid.UUID  `db:"id" json:"id"`
	ExecutionEngine string     `db:"execution_engine" json:"executionEngine"`
	CodePath        string     `db:"code_path" json:"codePath"`
	CodeHash        string     `db:"code_hash" json:"codeHash"`
	Status          string     `db:"status" json:"status"`
	OutputPath      *string    `db:"output_path" json:"outputPath,omitempty"`
	CreationTime    time.Time  `db:"creation_time" json:"creationTime"`
	StartTime       *time.Time `db:"start_time" json:"startTime,omitempty"`
	EndTime         *time.Time `db:"end_time" json:"endTime,omitempty"`
	RetryCount      int        `db:"retry_count" json:"retryCount"`
	OutputHash      *string    `db:"output_hash" json:"outputHash,omitempty"`

	Tags []Tag `json:"tags"`
}

// Tag represents a tag linked to a specific job.
type Tag struct {
	JobID int64  `db:"job_id" json:"jobId"`
	Name  string `db:"name" json:"name"`
}

// JobRequest is the incoming API payload before DB persistence.
type JobRequest struct {
	ExecutionEngine string   `json:"executionEngine"`
	CodeBase64      string   `json:"code"`
	Tags            []string `json:"tags"`
}

type JobResponse struct {
	ID              uuid.UUID  `json:"id"`
	ExecutionEngine string     `json:"execution_engine"`
	CodeBase64      string     `json:"code"`
	Status          string     `json:"status"`
	OutputBase64    string     `json:"output,omitempty"`
	CreationTime    time.Time  `json:"creation_time"`
	StartTime       *time.Time `json:"start_time,omitempty"`
	EndTime         *time.Time `json:"end_time,omitempty"`
}

type CacheJob struct {
	ID              uuid.UUID
	Code            string
	ExecutionEngine string
}
