package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
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

var (
	ErrNotFound = errors.New("job not found")
)

var jb *JobRepository

func NewJobRepository(ctx context.Context) (*JobRepository, error) {
	if jb != nil {
		return jb, nil
	}

	db, err := db.New(ctx)
	if err != nil {
		return nil, err
	}

	jb = &JobRepository{db: db}
	return jb, nil
}

func (r *JobRepository) ListJobs(ctx context.Context, offset string) ([]*model.Job, error) {

	tracer := job_tracer.GetTracer()
	ctx, span := tracer.Start(ctx, "Postgres/ListJob")
	defer span.End()

	var query string
	var args []any
	const limit = 25

	if offset == "" {
		query = `
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
			ORDER BY id DESC
			LIMIT $1`
		args = append(args, limit)
	} else {
		query = `
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
			WHERE id < $1
			ORDER BY id DESC
			LIMIT $2`
		args = append(args, offset, limit)
	}
	rows, err := r.db.Pool.Query(ctx, query, args...)
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
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return &job, nil
}

func (r *JobRepository) UpdateJob(ctx context.Context, job *model.Job) (*model.Job, error) {

	tracer := job_tracer.GetTracer()
	ctx, span := tracer.Start(ctx, "Postgres/UpdateJob")
	defer span.End()

	span.AddEvent("job.context",
		trace.WithAttributes(attribute.String("status", job.Status), attribute.String("id", job.ID.String())),
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

func (r *JobRepository) CreateJobs(ctx context.Context, jobs []*model.Job) error {
	if len(jobs) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(len(jobs))*20*time.Millisecond+time.Second)
	defer cancel()

	tx, err := r.db.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// --- Insert jobs ---
	jobRows := make([][]any, 0, len(jobs))
	for _, j := range jobs {
		jobRows = append(jobRows, []any{
			j.ID,
			j.ExecutionEngine,
			j.CodeHash,
			j.Status,
			j.CreationTime,
			j.StartTime,
			j.EndTime,
			j.RetryCount,
			j.OutputHash,
		})
	}

	_, err = tx.CopyFrom(
		ctx,
		pgx.Identifier{"jobs"},
		[]string{
			"id",
			"execution_engine",
			"code_hash",
			"status",
			"creation_time",
			"start_time",
			"end_time",
			"retry_count",
			"output_hash",
		},
		pgx.CopyFromRows(jobRows),
	)
	if err != nil {
		return err
	}

	// --- Insert tags ---
	tagRows := make([][]any, 0)
	for _, j := range jobs {
		for _, tag := range j.Tags {
			tagRows = append(tagRows, []any{j.ID, tag})
		}
	}

	if len(tagRows) > 0 {
		_, err = tx.CopyFrom(
			ctx,
			pgx.Identifier{"tags"},
			[]string{"jobid", "name"},
			pgx.CopyFromRows(tagRows),
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}
