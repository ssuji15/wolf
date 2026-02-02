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
	Tags            []string   `json:"tags"`
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

type CreateOptions struct {
	Name            string
	Image           string
	AppArmorProfile string
	CPUQuota        int64
	MemoryLimit     int64
	Labels          map[string]string
	WorkDir         string
	Runtime         string
	EnvVars         map[string]string
}
