package db

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/ssuji15/wolf/model"
)

type JobRepository struct {
	db *DB
}

func NewJobRepository(db *DB) *JobRepository {
	return &JobRepository{db: db}
}

func (r *JobRepository) ListJobs(ctx context.Context) ([]*model.Job, error) {
	query := `
		SELECT 
			id,
			execution_engine,
			code_path,
			code_hash,
			status,
			output_path,
			creation_time,
			start_time,
			end_time,
			retry_count,
			output_hash
		FROM jobs
		ORDER BY creation_time DESC`
	rows, err := r.db.Pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []*model.Job
	for rows.Next() {
		var j model.Job
		err := rows.Scan(
			&j.ID,
			&j.ExecutionEngine,
			&j.CodePath,
			&j.CodeHash,
			&j.Status,
			&j.OutputPath,
			&j.CreationTime,
			&j.StartTime,
			&j.EndTime,
			&j.RetryCount,
			&j.OutputHash,
		)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, &j)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return jobs, nil
}

func (r *JobRepository) GetByID(ctx context.Context, id string) (*model.Job, error) {
	var job model.Job
	query := `SELECT * FROM jobs WHERE id = $1`

	row := r.db.Pool.QueryRow(ctx, query, id)
	err := row.Scan(&job.ID, &job.ExecutionEngine, &job.CodePath,
		&job.CodeHash, &job.Status, &job.OutputPath, &job.CreationTime,
		&job.StartTime, &job.EndTime, &job.RetryCount, &job.OutputHash)

	if err != nil {
		return nil, fmt.Errorf("failed to get job by id %s: %w", id, err)
	}

	return &job, nil
}

// Inserts job + tags in a transaction
func (r *JobRepository) CreateJob(ctx context.Context, job model.Job, tags []string) (uuid.UUID, error) {
	tx, err := r.db.Pool.Begin(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	defer tx.Rollback(ctx)

	// Insert job
	_, err = tx.Exec(ctx, `
        INSERT INTO jobs (
            id,
            execution_engine,
            code_path,
            code_hash,
            status,
            output_path,
            creation_time,
            start_time,
            end_time,
            retry_count,
            output_hash
        )
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
    `,
		job.ID,
		job.ExecutionEngine,
		job.CodePath,
		job.CodeHash,
		job.Status,
		job.OutputPath,
		job.CreationTime,
		job.StartTime,
		job.EndTime,
		job.RetryCount,
		job.OutputHash,
	)
	if err != nil {
		return uuid.Nil, err
	}

	// Insert tags
	for _, t := range tags {
		_, err := tx.Exec(ctx, `
            INSERT INTO tags (jobid, name)
            VALUES ($1, $2)
        `, job.ID, t)

		if err != nil {
			return uuid.Nil, err
		}
	}

	// Commit
	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, err
	}

	return job.ID, nil
}
