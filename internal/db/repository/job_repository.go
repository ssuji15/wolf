package repository

import (
	"context"

	"github.com/ssuji15/wolf/internal/db"
	"github.com/ssuji15/wolf/internal/job_tracer"
	"github.com/ssuji15/wolf/internal/util"
	"github.com/ssuji15/wolf/model"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
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
			code_hash,
			status,
			creation_time,
			start_time,
			end_time,
			retry_count,
			output_hash
		FROM jobs
		ORDER BY creation_time DESC`
	rows, err := r.db.Pool.Query(ctx, query)
	if err != nil {
		util.RecordSpanError(span, err)
		return nil, err
	}
	defer rows.Close()

	var jobs []*model.Job
	for rows.Next() {
		var j model.Job
		err := rows.Scan(
			&j.ID,
			&j.ExecutionEngine,
			&j.CodeHash,
			&j.Status,
			&j.CreationTime,
			&j.StartTime,
			&j.EndTime,
			&j.RetryCount,
			&j.OutputHash,
		)
		if err != nil {
			util.RecordSpanError(span, err)
			return nil, err
		}
		jobs = append(jobs, &j)
	}

	if err := rows.Err(); err != nil {
		util.RecordSpanError(span, err)
		return nil, err
	}

	return jobs, nil
}

func (r *JobRepository) GetJobByID(ctx context.Context, id string) (*model.Job, error) {

	tracer := job_tracer.GetTracer()
	ctx, span := tracer.Start(ctx, "Postgres/GetJob")
	defer span.End()

	var job model.Job
	query := `SELECT * FROM jobs WHERE id = $1`

	row := r.db.Pool.QueryRow(ctx, query, id)
	err := row.Scan(&job.ID, &job.ExecutionEngine,
		&job.CodeHash, &job.Status, &job.CreationTime,
		&job.StartTime, &job.EndTime, &job.RetryCount, &job.OutputHash)

	if err != nil {
		util.RecordSpanError(span, err)
		return nil, err
	}

	return &job, nil
}

// Inserts job + tags in a transaction
func (r *JobRepository) CreateJob(ctx context.Context, job model.Job, tags []string) error {

	tracer := job_tracer.GetTracer()
	ctx, span := tracer.Start(ctx, "Postgres/CreateJob")
	defer span.End()

	span.AddEvent("job.context",
		trace.WithAttributes(attribute.String("job_id", job.ID.String())),
	)

	tx, err := r.db.Pool.Begin(ctx)
	if err != nil {
		util.RecordSpanError(span, err)
		return err
	}
	defer tx.Rollback(ctx)

	// Insert job
	_, err = tx.Exec(ctx, `
        INSERT INTO jobs (
            id,
            execution_engine,
            code_hash,
            status,
            creation_time,
            start_time,
            end_time,
            retry_count,
            output_hash
        )
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
    `,
		job.ID,
		job.ExecutionEngine,
		job.CodeHash,
		job.Status,
		job.CreationTime,
		job.StartTime,
		job.EndTime,
		job.RetryCount,
		job.OutputHash,
	)
	if err != nil {
		util.RecordSpanError(span, err)
		return err
	}

	// Insert tags
	for _, t := range tags {
		_, err := tx.Exec(ctx, `
            INSERT INTO tags (jobid, name)
            VALUES ($1, $2)
        `, job.ID, t)

		if err != nil {
			util.RecordSpanError(span, err)
			return err
		}
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO job_outbox (id, status)
		VALUES($1, $2)
	`, job.ID, "PENDING")

	if err != nil {
		util.RecordSpanError(span, err)
		return err
	}

	// Commit
	if err := tx.Commit(ctx); err != nil {
		util.RecordSpanError(span, err)
		return err
	}

	return nil
}

func (r *JobRepository) UpdateJob(ctx context.Context, job *model.Job) (*model.Job, error) {

	tracer := job_tracer.GetTracer()
	ctx, span := tracer.Start(ctx, "Postgres/UpdateJob")
	defer span.End()

	span.AddEvent("job.context",
		trace.WithAttributes(attribute.String("status", job.Status)),
	)

	query := `
		UPDATE jobs
		SET
			execution_engine = $2,
			code_hash        = $3,
			status           = $4,
			creation_time    = $5,
			start_time       = $6,
			end_time         = $7,
			retry_count      = $8,
			output_hash      = $9
		WHERE id = $1
	`
	_, err := r.db.Pool.Exec(ctx, query, job.ID,
		job.ExecutionEngine,
		job.CodeHash,
		job.Status,
		job.CreationTime,
		job.StartTime,
		job.EndTime,
		job.RetryCount,
		job.OutputHash)
	if err != nil {
		util.RecordSpanError(span, err)
		return &model.Job{}, err
	}
	return job, nil
}

func (r *JobRepository) OutboxJobPublished(ctx context.Context, id string) error {
	query := `
		UPDATE job_outbox
		SET
			status = 'PUBLISHED',
			locked_at = NULL
		WHERE id = $1
	`
	_, err := r.db.Pool.Exec(ctx, query, id)
	return err
}

func (r *JobRepository) OutboxJobFailed(ctx context.Context, id string) error {
	query := `
		UPDATE job_outbox
		SET
			retry_count = retry_count + 1,
			locked_at = NULL,
			status = CASE
				WHEN retry_count + 1 >= 3 THEN 'FAILED'
				ELSE 'PENDING'
			END
		WHERE id = $1
	`
	_, err := r.db.Pool.Exec(ctx, query, id)
	return err
}

func (r *JobRepository) ClaimOutboxJobs(ctx context.Context) ([]string, error) {
	query := `
		UPDATE job_outbox
		SET status = 'IN_PROGRESS',
			locked_at = now(),
			retry_count = CASE 
				WHEN status = 'IN_PROGRESS' THEN retry_count + 1
				ELSE retry_count
			END
		WHERE id IN (
			SELECT id
			FROM job_outbox
			WHERE 
			(
				status = 'PENDING' 
				OR
				(
					status = 'IN_PROGRESS'
					AND locked_at < now() - interval '5 seconds'
				)
			)
			AND retry_count < 3
			ORDER BY created_at
			LIMIT 25
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id;
	`
	rows, err := r.db.Pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		err := rows.Scan(
			&id,
		)
		if err != nil {
			break
		}
		ids = append(ids, id)
	}

	return ids, nil
}
