-- Job Processing Platform — database schema
-- This matches the ConfigMap in infra/k8s/postgres/configmap.yaml — keep both in sync.
-- Run locally: psql postgres://jpp:jpp@localhost:5432/jpp -f scripts/db/init.sql

CREATE TABLE IF NOT EXISTS jobs (
    id          TEXT        PRIMARY KEY,
    type        TEXT        NOT NULL,
    status      TEXT        NOT NULL DEFAULT 'pending',
    payload     TEXT,
    result      TEXT,
    error       TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes support common query patterns
CREATE INDEX IF NOT EXISTS idx_jobs_status     ON jobs (status);
CREATE INDEX IF NOT EXISTS idx_jobs_type       ON jobs (type);
CREATE INDEX IF NOT EXISTS idx_jobs_created_at ON jobs (created_at DESC);

-- Useful for monitoring: count jobs by status
-- SELECT status, count(*) FROM jobs GROUP BY status;
