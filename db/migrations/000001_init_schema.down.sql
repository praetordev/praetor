-- Drop Execution Plane
DROP TABLE IF EXISTS job_output_chunks;
DROP TABLE IF EXISTS job_host_summaries;
DROP TABLE IF EXISTS job_events;
DROP TABLE IF EXISTS execution_runs;
DROP TABLE IF EXISTS unified_jobs;

-- Drop Infrastructure
DROP TABLE IF EXISTS instances;
DROP TABLE IF EXISTS instance_groups;

-- Drop Resources
DROP TABLE IF EXISTS job_templates;
DROP TABLE IF EXISTS execution_environments;
DROP TABLE IF EXISTS credentials;
DROP TABLE IF EXISTS credential_types;
DROP TABLE IF EXISTS groups;
DROP TABLE IF EXISTS hosts;
DROP TABLE IF EXISTS inventories;
DROP TABLE IF EXISTS projects;

-- Drop Control Plane
DROP TABLE IF EXISTS roles;
DROP TABLE IF EXISTS teams;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS organizations;

DROP EXTENSION IF EXISTS "uuid-ossp";
