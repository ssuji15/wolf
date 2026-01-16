//go:build integration
// +build integration

package repository

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/ssuji15/wolf/internal/db"
	"github.com/ssuji15/wolf/model"
	tdb "github.com/ssuji15/wolf/tests/integration_test/infra/db"
	"github.com/ssuji15/wolf/tests/integration_test/infra/db/repository"
	"github.com/testcontainers/testcontainers-go"
)

var (
	container    testcontainers.Container
	testDB       *db.DB
	pgPool       *pgxpool.Pool
	POSTGRES_URL string
)

func TestMain(m *testing.M) {
	ctx := context.Background()
	container, testDB, POSTGRES_URL = tdb.SetupContainer(ctx)
	pgPool = testDB.Pool
	repository.ApplySchema(ctx, pgPool)
	code := m.Run()
	_ = container.Terminate(ctx)
	os.Exit(code)
}

func TestJobRepository_CreateJobs_And_GetJobByID(t *testing.T) {
	now := time.Now().UTC()
	tests := []struct {
		name string
		jobs []*model.Job
	}{
		{
			name: "single job with tags",
			jobs: []*model.Job{
				{
					ID:              uuid.New(),
					ExecutionEngine: "docker",
					CodeHash:        fixedHash("code"),
					Status:          "PENDING",
					CreationTime:    &now,
					RetryCount:      0,
					OutputHash:      "",
					Tags:            []string{"a", "b"},
				},
			},
		},
		{
			name: "multiple jobs",
			jobs: []*model.Job{
				newJob("PENDING"),
				newJob("DISPATCHED"),
				newJob("COMPLETED"),
			},
		},
		{
			name: "zero jobs",
			jobs: []*model.Job{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repository.TruncateJobsTables(t, pgPool)
			ctx := context.Background()
			repo := NewJobRepository(ctx, testDB)

			err := repo.CreateJobs(ctx, tt.jobs)
			require.NoError(t, err)

			for _, job := range tt.jobs {
				got, err := repo.GetJobByID(context.Background(), job.ID.String())
				require.NoError(t, err)
				require.Equal(t, job.ID, got.ID)
				require.Equal(t, job.Status, got.Status)
				require.Equal(t, job.ExecutionEngine, got.ExecutionEngine)
			}
		})
	}
}

func TestJobRepository_ListJobs(t *testing.T) {
	repository.TruncateJobsTables(t, pgPool)
	ctx := context.Background()
	repo := NewJobRepository(ctx, testDB)

	jobs := []*model.Job{
		newJob("PENDING"),
		newJob("DISPATCHED"),
		newJob("COMPLETED"),
	}

	require.NoError(t, repo.CreateJobs(context.Background(), jobs))

	tests := []struct {
		name   string
		offset string
		want   int
	}{
		{
			name:   "first page",
			offset: "",
			want:   3,
		},
		{
			name:   "pagination",
			offset: jobs[1].ID.String(),
			want:   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			list, err := repo.ListJobs(context.Background(), tt.offset)
			require.NoError(t, err)
			require.Len(t, list, tt.want)
		})
	}
}

func TestJobRepository_UpdateJob(t *testing.T) {
	repository.TruncateJobsTables(t, pgPool)

	ctx := context.Background()
	repo := NewJobRepository(ctx, testDB)

	job := newJob("PENDING")
	require.NoError(t, repo.CreateJobs(context.Background(), []*model.Job{job}))

	job.Status = "COMPLETED"
	job.RetryCount = 3
	job.EndTime = ptrTime(time.Now().UTC())

	updated, err := repo.UpdateJob(context.Background(), job)
	require.NoError(t, err)
	require.Equal(t, "COMPLETED", updated.Status)

	fetched, err := repo.GetJobByID(context.Background(), job.ID.String())
	require.NoError(t, err)
	require.Equal(t, "COMPLETED", fetched.Status)
	require.Equal(t, 3, fetched.RetryCount)
}

func newJob(status string) *model.Job {
	now := time.Now().UTC()
	id, _ := uuid.NewV7()
	return &model.Job{
		ID:              id,
		ExecutionEngine: "docker",
		CodeHash:        fixedHash("code"),
		Status:          status,
		CreationTime:    &now,
		RetryCount:      0,
		Tags:            []string{"x", "y"},
	}
}

func fixedHash(s string) string {
	return fmt.Sprintf("%064s", s)
}

func ptrTime(t time.Time) *time.Time {
	return &t
}
