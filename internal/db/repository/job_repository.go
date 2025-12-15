package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/ssuji15/wolf/internal/db"
	"github.com/ssuji15/wolf/internal/job_tracer"
	"github.com/ssuji15/wolf/model"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

type JobRepository struct {
	db *db.DB
}

func NewJobRepository(db *db.DB) *JobRepository {
	return &JobRepository{db: db}
}

func (r *JobRepository) ListJobs(ctx context.Context) ([]*model.Job, error) {

	tracer := job_tracer.GetTracer()
	ctx, span := tracer.Start(ctx, "Postgres/ListJob")
	defer span.End()

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
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
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
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		jobs = append(jobs, &j)
	}

	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	return jobs, nil
}

func (r *JobRepository) GetByID(ctx context.Context, id string) (*model.Job, error) {

	tracer := job_tracer.GetTracer()
	ctx, span := tracer.Start(ctx, "Postgres/GetJob")
	defer span.End()

	var job model.Job
	query := `SELECT * FROM jobs WHERE id = $1`

	row := r.db.Pool.QueryRow(ctx, query, id)
	err := row.Scan(&job.ID, &job.ExecutionEngine, &job.CodePath,
		&job.CodeHash, &job.Status, &job.OutputPath, &job.CreationTime,
		&job.StartTime, &job.EndTime, &job.RetryCount, &job.OutputHash)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	return &job, nil
}

// Inserts job + tags in a transaction
func (r *JobRepository) CreateJob(ctx context.Context, job model.Job, tags []string) (uuid.UUID, error) {

	tracer := job_tracer.GetTracer()
	ctx, span := tracer.Start(ctx, "Postgres/CreateJob")
	defer span.End()

	span.SetAttributes(attribute.String("id", job.ID.String()))

	tx, err := r.db.Pool.Begin(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
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
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return uuid.Nil, err
	}

	// Insert tags
	for _, t := range tags {
		_, err := tx.Exec(ctx, `
            INSERT INTO tags (jobid, name)
            VALUES ($1, $2)
        `, job.ID, t)

		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return uuid.Nil, err
		}
	}

	// Commit
	if err := tx.Commit(ctx); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return uuid.Nil, err
	}

	return job.ID, nil
}

func (r *JobRepository) UpdateJob(ctx context.Context, job *model.Job) (*model.Job, error) {

	tracer := job_tracer.GetTracer()
	ctx, span := tracer.Start(ctx, "Postgres/UpdateJob")
	defer span.End()

	span.SetAttributes(
		attribute.String("status", job.Status),
	)

	query := `
		UPDATE jobs
		SET
			execution_engine = $2,
			code_path        = $3,
			code_hash        = $4,
			status           = $5,
			output_path      = $6,
			creation_time    = $7,
			start_time       = $8,
			end_time         = $9,
			retry_count      = $10,
			output_hash      = $11
		WHERE id = $1
	`
	_, err := r.db.Pool.Exec(ctx, query, job.ID,
		job.ExecutionEngine,
		job.CodePath,
		job.CodeHash,
		job.Status,
		job.OutputPath,
		job.CreationTime,
		job.StartTime,
		job.EndTime,
		job.RetryCount,
		job.OutputHash)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return &model.Job{}, err
	}
	return job, nil
}
