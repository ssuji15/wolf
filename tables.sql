CREATE TABLE jobs (
    id UUID PRIMARY KEY,

    execution_engine TEXT NOT NULL,
    code_path        TEXT NOT NULL,
    code_hash        TEXT NOT NULL,
    status           TEXT NOT NULL,

    output_path      TEXT,
    creation_time    TIMESTAMPTZ NOT NULL,
    start_time       TIMESTAMPTZ,
    end_time         TIMESTAMPTZ,

    retry_count      INT NOT NULL DEFAULT 0,
    output_hash      TEXT
);

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