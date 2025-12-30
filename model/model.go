package model

import (
	"time"

	"github.com/google/uuid"
)

// Job represents a job record stored in the database.
type Job struct {
	ID              uuid.UUID  `db:"id" json:"id"`
	ExecutionEngine string     `db:"execution_engine" json:"executionEngine"`
	CodeHash        string     `db:"code_hash" json:"codeHash"`
	Status          string     `db:"status" json:"status"`
	CreationTime    *time.Time `db:"creation_time" json:"creationTime"`
	StartTime       *time.Time `db:"start_time" json:"startTime,omitempty"`
	EndTime         *time.Time `db:"end_time" json:"endTime,omitempty"`
	RetryCount      int        `db:"retry_count" json:"retryCount"`
	OutputHash      string     `db:"output_hash" json:"outputHash,omitempty"`

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
	Code            []byte   `json:"code"`
	Tags            []string `json:"tags"`
}

type WorkerStatus string

const (
	WorkerIdle     WorkerStatus = "idle"
	WorkerBusy     WorkerStatus = "busy"
	WorkerDead     WorkerStatus = "dead"
	WorkerStarting WorkerStatus = "starting"
)

type WorkerMetadata struct {
	ID         string    `db:"id"`
	Name       string    `db:"name"`
	WorkDir    string    `db:"work_dir"`
	SocketPath string    `db:"socket_path"`
	OutputPath string    `db:"output_path"`
	CreatedAt  time.Time `db:"created_at"`
	UpdatedAt  time.Time `db:"updated_at"`
	Status     string    `db:"status"`
	JobID      string    `db:"job_id"`
}

type CreateOptions struct {
	Name            string
	Image           string
	AppArmorProfile string
	CPUQuota        int64
	MemoryLimit     int64
	Labels          map[string]string
	WorkDir         string
}

type Outbox_Job struct {
	ID          string  `db:"id"`
	TraceParent string  `db:"trace_parent"`
	TraceState  *string `db:"trace_state"`
}
