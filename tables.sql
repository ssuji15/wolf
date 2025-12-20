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

CREATE TABLE job_outbox (
    id UUID PRIMARY KEY,
    status TEXT NOT NULL CHECK(
        status IN ('PENDING', 'IN_PROGRESS', 'PUBLISHED', 'FAILED')
    ),
    locked_at TIMESTAMPTZ,
    retry_count INT NOT NULL DEFAULT 0,
    trace_context TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_job_outbox_pending_retry 
ON job_outbox (created_at) 
WHERE status IN ('PENDING', 'IN_PROGRESS') 
AND retry_count < 3;

CREATE INDEX idx_jobs_execution_engine ON jobs (execution_engine);
CREATE INDEX idx_jobs_status ON jobs (status);

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