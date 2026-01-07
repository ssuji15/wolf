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
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/ssuji15/wolf/internal/db"
	"github.com/ssuji15/wolf/model"
)

var (
	testDB       *db.DB
	pgPool       *pgxpool.Pool
	POSTGRES_URL string
)

func TestMain(m *testing.M) {
	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        "postgres:15",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     "test",
			"POSTGRES_PASSWORD": "test",
			"POSTGRES_DB":       "testdb",
		},
		WaitingFor: wait.ForListeningPort("5432/tcp"),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		panic(err)
	}

	host, _ := container.Host(ctx)
	port, _ := container.MappedPort(ctx, "5432")

	POSTGRES_URL = fmt.Sprintf(
		"postgres://test:test@%s:%s/testdb?sslmode=disable",
		host,
		port.Port(),
	)

	os.Setenv("POSTGRES_URL", POSTGRES_URL)

	db, err := db.New(ctx)
	if err != nil {
		panic(err)
	}
	pgPool = db.Pool
	applySchema(ctx, pgPool)

	code := m.Run()
	_ = container.Terminate(ctx)
	os.Exit(code)
}

func applySchema(ctx context.Context, pool *pgxpool.Pool) error {
	schema := `
CREATE TABLE jobs (
    id UUID PRIMARY KEY,
    execution_engine TEXT NOT NULL,
    code_hash        CHAR(64) NOT NULL,
    status           TEXT NOT NULL CHECK(
        status IN ('PENDING', 'DISPATCHED', 'COMPLETED', 'FAILED')
    ),
    creation_time    TIMESTAMPTZ NOT NULL,
    start_time       TIMESTAMPTZ,
    end_time         TIMESTAMPTZ,
    retry_count      INT NOT NULL DEFAULT 0,
    output_hash      CHAR(64)
);

CREATE TABLE tags (
    jobid UUID NOT NULL,
    name  TEXT NOT NULL,

    CONSTRAINT fk_tags_jobid
        FOREIGN KEY (jobid)
        REFERENCES jobs(id)
        ON DELETE CASCADE,

    CONSTRAINT tags_jobid_name_unique
        UNIQUE (jobid, name)
);

CREATE INDEX idx_tags_name ON tags (name);
`
	_, err := pool.Exec(ctx, schema)
	return err
}

func truncateTables(t *testing.T) {
	t.Helper()
	_, err := pgPool.Exec(context.Background(), `
		TRUNCATE TABLE tags RESTART IDENTITY CASCADE;
		TRUNCATE TABLE jobs RESTART IDENTITY CASCADE;
	`)
	require.NoError(t, err)
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
			truncateTables(t)
			ctx := context.Background()
			repo, err := NewJobRepository(ctx)
			require.NoError(t, err)

			err = repo.CreateJobs(ctx, tt.jobs)
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
	truncateTables(t)
	ctx := context.Background()
	repo, err := NewJobRepository(ctx)
	require.NoError(t, err)

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
	truncateTables(t)

	ctx := context.Background()
	repo, err := NewJobRepository(ctx)
	require.NoError(t, err)

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
