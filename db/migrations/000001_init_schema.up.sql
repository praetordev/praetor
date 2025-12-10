-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- =================================================================================
-- CONTROL PLANE & RBAC
-- =================================================================================

CREATE TABLE IF NOT EXISTS organizations (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    modified_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS users (
    id BIGSERIAL PRIMARY KEY,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL, -- Simplified for now
    first_name TEXT,
    last_name TEXT,
    email TEXT,
    is_superuser BOOLEAN NOT NULL DEFAULT FALSE,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    modified_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS teams (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    modified_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, name)
);

CREATE TABLE IF NOT EXISTS roles (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT,
    permissions JSONB NOT NULL DEFAULT '[]'::jsonb
);

-- =================================================================================
-- RESOURCES (Projects, Inventories, Credentials, Templates)
-- =================================================================================

CREATE TABLE IF NOT EXISTS projects (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT,
    scm_type TEXT NOT NULL, -- git, svn, archive
    scm_url TEXT NOT NULL,
    scm_branch TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    modified_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, name)
);

CREATE TABLE IF NOT EXISTS inventories (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT,
    kind TEXT NOT NULL DEFAULT 'smart', -- smart, constructed ?
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    modified_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, name)
);

CREATE TABLE IF NOT EXISTS hosts (
    id BIGSERIAL PRIMARY KEY,
    inventory_id BIGINT NOT NULL REFERENCES inventories(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT,
    variables JSONB DEFAULT '{}'::jsonb,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    modified_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (inventory_id, name)
);

CREATE TABLE IF NOT EXISTS groups (
    id BIGSERIAL PRIMARY KEY,
    inventory_id BIGINT NOT NULL REFERENCES inventories(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT,
    variables JSONB DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    modified_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (inventory_id, name)
);

CREATE TABLE IF NOT EXISTS credential_types (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT,
    inputs JSONB NOT NULL DEFAULT '{}'::jsonb,
    injectors JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    modified_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS credentials (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    credential_type_id BIGINT NOT NULL REFERENCES credential_types(id),
    name TEXT NOT NULL,
    description TEXT,
    inputs JSONB NOT NULL DEFAULT '{}'::jsonb, -- encrypted?
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    modified_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, name)
);

CREATE TABLE IF NOT EXISTS execution_environments (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT REFERENCES organizations(id) ON DELETE CASCADE, -- Nullable for global EEs
    name TEXT NOT NULL,
    image TEXT NOT NULL,
    description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    modified_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, name)
);

CREATE TABLE IF NOT EXISTS job_templates (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT,
    inventory_id BIGINT REFERENCES inventories(id) ON DELETE SET NULL,
    project_id BIGINT REFERENCES projects(id) ON DELETE SET NULL,
    playbook TEXT NOT NULL,
    execution_environment_id BIGINT REFERENCES execution_environments(id) ON DELETE SET NULL,
    forks INT DEFAULT 0,
    job_type TEXT NOT NULL DEFAULT 'run', -- run, check
    verbosity INT DEFAULT 0,
    extra_vars JSONB DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    modified_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, name)
);

-- =================================================================================
-- INFRASTRUCTURE
-- =================================================================================

CREATE TABLE IF NOT EXISTS instance_groups (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    modified_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS instances (
    id BIGSERIAL PRIMARY KEY,
    hostname TEXT NOT NULL UNIQUE,
    version TEXT,
    capacity INT DEFAULT 100,
    enabled BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    modified_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- =================================================================================
-- EXECUTION PLANE
-- =================================================================================

CREATE TABLE IF NOT EXISTS unified_jobs (
    id BIGSERIAL PRIMARY KEY,
    unified_job_template_id BIGINT, -- Polymorphic FK simplified for now
    name TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending', -- pending, queued, running, successful, failed, canceled, error
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    cancel_requested BOOLEAN NOT NULL DEFAULT FALSE,
    job_args JSONB DEFAULT '{}'::jsonb -- Stores launch-time args
);

CREATE TABLE IF NOT EXISTS execution_runs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    unified_job_id BIGINT NOT NULL REFERENCES unified_jobs(id) ON DELETE CASCADE,
    attempt_number INT NOT NULL DEFAULT 1,
    executor_instance_id BIGINT REFERENCES instances(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    state TEXT NOT NULL DEFAULT 'pending', -- pending, starting, running, successful, failed, canceled, lost
    last_heartbeat_at TIMESTAMPTZ,
    last_event_seq BIGINT NOT NULL DEFAULT 0,
    persisted_event_seq BIGINT NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_execution_runs_job_id ON execution_runs (unified_job_id);

CREATE TABLE IF NOT EXISTS job_events (
    id BIGSERIAL PRIMARY KEY,
    unified_job_id BIGINT NOT NULL REFERENCES unified_jobs(id) ON DELETE CASCADE,
    execution_run_id UUID NOT NULL REFERENCES execution_runs(id) ON DELETE CASCADE,
    seq BIGINT NOT NULL,
    event_type TEXT NOT NULL,
    host_id BIGINT REFERENCES hosts(id) ON DELETE SET NULL,
    task_name TEXT,
    play_name TEXT,
    event_data JSONB NOT NULL DEFAULT '{}'::jsonb,
    stdout_snippet TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (execution_run_id, seq)
);
CREATE INDEX IF NOT EXISTS idx_job_events_job_id ON job_events (unified_job_id);
CREATE INDEX IF NOT EXISTS idx_job_events_host_id ON job_events (host_id);

CREATE TABLE IF NOT EXISTS job_host_summaries (
    id BIGSERIAL PRIMARY KEY,
    unified_job_id BIGINT NOT NULL REFERENCES unified_jobs(id) ON DELETE CASCADE,
    host_id BIGINT NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    changed INT NOT NULL DEFAULT 0,
    failed INT NOT NULL DEFAULT 0,
    ok INT NOT NULL DEFAULT 0,
    skipped INT NOT NULL DEFAULT 0,
    unreachable INT NOT NULL DEFAULT 0,
    last_event_at TIMESTAMPTZ,
    UNIQUE (unified_job_id, host_id)
);

CREATE TABLE IF NOT EXISTS job_output_chunks (
    id BIGSERIAL PRIMARY KEY,
    execution_run_id UUID NOT NULL REFERENCES execution_runs(id) ON DELETE CASCADE,
    seq BIGINT NOT NULL,
    storage_key TEXT NOT NULL,
    byte_length INT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (execution_run_id, seq)
);
