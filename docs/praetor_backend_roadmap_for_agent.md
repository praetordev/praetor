
# Praetor Backend Roadmap (for Implementation Agent)

This roadmap turns the **Praetor Backend Implementation Guide** into an ordered, concrete sequence of tasks that an agent can follow.

Each phase has:

- **Goal** – what this phase achieves.
- **Inputs** – documents or components assumed to exist.
- **Deliverables** – what must exist at the end of the phase.
- **Tasks** – step-by-step work items.
- **Acceptance Criteria** – concrete checks for “done”.

---

## Phase 0 – Bootstrap & Ground Truth

### Goal

Set up the working environment and make the core Praetor docs API-accessible so every later phase is grounded in the same truth.

### Inputs

- `vision.md`
- `architecture.md`
- `praetor_rest_api_v1_full.md`
- `praetor_backend_implementation_guide.md`

### Deliverables

- A **backend repo** or monorepo module initialized.
- A **docs** directory containing the above files.
- A simple **“docs index”** file linking all of them.

### Tasks

1. **Create repo / module**
   - Initialize git repository if not existing.
   - Add basic structure:
     - `/docs`
     - `/services/api`
     - `/services/scheduler`
     - `/services/controller`
     - `/services/ingestion`
     - `/services/reconciler`
     - `/services/infra-introspector`
     - `/infra` (deployment manifests, Helm charts, etc.).

2. **Add core docs**
   - Copy the following into `/docs`:
     - `vision.md`
     - `architecture.md`
     - `praetor_rest_api_v1_full.md`
     - `praetor_backend_implementation_guide.md`

3. **Create `docs/INDEX.md`**
   - Briefly summarize each doc.
   - Provide links to each.

### Acceptance Criteria

- Repo builds (even if nothing runs yet).
- `/docs/INDEX.md` exists and links to all core docs.
- Git history shows initial commit with structure + docs.

---

## Phase 1 – Core Domain & Database Schema

### Goal

Define and implement the **core domain model and DB schema**: Jobs, Runs, Events, Host Summaries, Orgs, Users, Projects, Inventories, Credentials, Job Templates, Execution Pools, etc.

### Inputs

- `praetor_rest_api_v1_full.md`
- `praetor_backend_implementation_guide.md` (sections 3 & 10)

### Deliverables

- A **database schema** (migrations) for all core tables.
- A **domain model layer** (ORM/entities) reflecting that schema.
- A **schema diagram** or auto-generated model docs.

### Tasks

1. **Design DB schema**
   - For execution:
     - `jobs`
     - `job_runs`
     - `job_events`
     - `job_host_summaries`
     - `log_chunks`
   - For control plane:
     - `organizations`
     - `users`
     - `teams`
     - `roles`
     - `projects`
     - `inventories`
     - `hosts`
     - `groups`
     - `credentials`
     - `credential_types`
     - `execution_environments`
     - `job_templates`
     - `inventory_sources` (if implemented)
   - For infra:
     - `execution_pools`
     - `control_plane_instances` (if persisted; otherwise treat as derived).

2. **Implement migrations**
   - Use chosen migration tool (e.g. Alembic, Django migrations, etc.).
   - Migrations must be idempotent and reversible where possible.

3. **Implement domain entities / ORM models**
   - Create classes for each table with:
     - Field types.
     - Relationships (FKs, many-to-many).
     - Indexes and unique constraints.

4. **Add test seeds / fixtures**
   - Minimal seed data for development:
     - One organization, one user, minimal roles.
     - A simple project, inventory, host, and job template.

5. **Generate schema docs**
   - Either auto-generate (e.g. ER diagram) or maintain a `docs/schema.md` summarizing key tables.

### Acceptance Criteria

- DB can be created and migrated from scratch.
- Basic queries for key entities (job, job_run, project, inventory) succeed.
- Relationships (FKs) are enforced and consistent.
- Unit tests verify migrations apply cleanly on an empty DB.

---

## Phase 2 – REST API Skeleton (`/api/v1`)  

### Goal

Expose the **Praetor REST API Reference v1** as stubbed, working endpoints over the core domain models (CRUD and read-only where applicable). No execution yet.

### Inputs

- `praetor_rest_api_v1_full.md`
- Schema + models from Phase 1

### Deliverables

- API service with:
  - Auth + RBAC middleware (even if minimal).
  - Implemented CRUD for core resources.
  - Pagination, filtering, and sorting support.
- A generated **OpenAPI spec** (`praetor_openapi_v1.yaml` or equivalent) matching the implementation.

### Tasks

1. **Set up API service**
   - Choose framework (e.g. FastAPI, Django REST, etc.).
   - Configure routing, error handling, and JSON serialization.

2. **Implement auth & RBAC scaffolding**
   - Token/session authentication.
   - Simple “system_admin” role with full access.
   - Hook RBAC checks into request handling.

3. **Implement core resource endpoints**
   - Collections & details for:
     - `/api/v1/organizations`
     - `/api/v1/users`
     - `/api/v1/teams`
     - `/api/v1/roles` (even read-only for now)
     - `/api/v1/projects`
     - `/api/v1/inventories`
     - `/api/v1/hosts`
     - `/api/v1/groups`
     - `/api/v1/credentials`
     - `/api/v1/credential-types`
     - `/api/v1/execution-environments`
     - `/api/v1/job-templates`
     - `/api/v1/infra/execution-pools`

4. **Implement pagination / filtering / ordering**
   - Support `limit`, `offset`, `order_by`, `search`, and `field__lookup` filters.
   - Ensure consistent response envelope: `{ items, total, limit, offset }`.

5. **Implement basic Job & Run endpoints (no execution)**
   - `/api/v1/jobs`:
     - Create job (validate payload and persist).
     - List and retrieve jobs.
   - `/api/v1/runs`:
     - List and retrieve runs (read-only for now).

6. **Generate / align OpenAPI**
   - Create or update `praetor_openapi_v1.yaml` to match the implemented endpoints.
   - Add CI check to validate that OpenAPI and code stay in sync (where possible).

### Acceptance Criteria

- API service starts and responds on `/api/v1`.
- You can:
  - Create orgs, users, projects, inventories, hosts, job templates via API.
  - Create Jobs (pending execution), and retrieve them.
- Pagination and filtering work on at least one collection (e.g. `projects`, `jobs`).

---

## Phase 3 – Scheduler Service

### Goal

Enable **Job scheduling** into **Job Runs** and connect Jobs ↔ Execution Pools, ready for K8s execution.

### Inputs

- Jobs & Runs schema (Phase 1).
- API service with Job CRUD (Phase 2).
- Execution Pools (schema + endpoints).

### Deliverables

- A **scheduler service** that:
  - Finds pending Jobs.
  - Assigns them to Execution Pools.
  - Creates Job Runs.
- Basic metrics on scheduled vs pending Jobs.

### Tasks

1. **Define scheduling rules**
   - Jobs eligible if:
     - `status = pending`.
     - `current_run_id IS NULL`.
     - Template/inventory/project not disabled.
   - Choose pool selection algorithm (e.g. round robin, label matching).

2. **Implement scheduler loop**
   - Runs on a tick (e.g. every 1–5 seconds) or event-driven.
   - Queries for eligible Jobs.
   - For each Job:
     - Select `execution_pool_id`.
     - Within a DB transaction:
       - Create `job_runs` row (`state = pending`, `attempt_number` incremented).
       - Update `jobs.current_run_id` and `jobs.status` → `waiting` or `running` (per chosen semantics).

3. **Add concurrency safety**
   - Ensure multiple scheduler instances do not double-schedule:
     - Use DB row-level locks or pessimistic locking on Jobs.
     - Or use a “scheduler_owner” column with CAS/update-if-null semantics.

4. **Expose basic status**
   - Optional internal endpoint or metrics:
     - Scheduler tick duration.
     - Number of jobs scheduled per tick.
     - Number of pending Jobs per Execution Pool.

### Acceptance Criteria

- New Jobs created in `/api/v1/jobs` transition to having a `job_runs` record with a valid `execution_pool_id`.
- No duplicate `job_runs` for the same logical attempt (no double-schedules).
- Stopping/restarting scheduler does not corrupt state (jobs remain pending or scheduled once).

---

## Phase 4 – Kubernetes Execution Controller

### Goal

Turn **JobRuns** into **Kubernetes pods/jobs** that can run the executor.

### Inputs

- JobRuns created and assigned to pools (Phase 3).
- Execution Pools with K8s config (Phase 2).
- Chosen executor image and baseline manifest format (Phase 5 will refine).

### Deliverables

- A **Kubernetes controller** that:
  - Watches `job_runs` (or a CRD).
  - Creates executor Pods/Jobs with proper labels, env, and scheduling constraints.
  - Tracks Pod lifecycle and updates `job_runs.state` accordingly.

### Tasks

1. **Choose control model**
   - Option A: Controller reads `job_runs` from DB directly.
   - Option B: Introduce a CRD (`AutomationRun`) that mirrors `job_runs` and watch that in-cluster.

2. **Implement controller skeleton**
   - Connect to Kubernetes API.
   - Set up watchers/informers for the chosen objects.
   - Poll DB or CRD for `job_runs.state = pending`.

3. **Create executor Pods/Jobs**
   - For each pending Run:
     - Determine namespace, nodeSelector, tolerations from `execution_pool_id`.
     - Create a K8s Job/Pod with:
       - `env: RUN_ID=` set to `job_runs.id`.
       - Image from Execution Environment (or fixed for now).
       - ServiceAccount / secrets for API access and object storage.

4. **Track Pod status**
   - When Pod moves to:
     - Running → mark `job_runs.state = running` (and `jobs.status` = running if needed).
     - Succeeded → mark `job_runs.state = successful` (exit_code=0).
     - Failed → mark `job_runs.state = failed` (exit_code!=0, error_reason from Pod).

5. **Plumb metrics & logs**
   - Optional: emit metrics for active Pods per pool, success/failure counts, etc.

### Acceptance Criteria

- Creating a Job via API eventually results in a running executor Pod in K8s.
- When Pods exit, `job_runs.state` is updated accordingly.
- No orphaned Pods / Runs for normal flows.

---

## Phase 5 – Executor Image & Manifest Flow

### Goal

Give the executor enough information to run real playbooks and emit logs/events, using the **manifest endpoint**.

### Inputs

- `/internal/api/v1/runs/{run_id}/manifest` spec (Implementation Guide section 10.6).
- Basic executor image that can call HTTP APIs.

### Deliverables

- `/internal/api/v1/runs/{run_id}/manifest` implemented in API.
- Executor image that:
  - Bootstraps with `RUN_ID`.
  - Fetches the manifest.
  - Executes a basic playbook or command.
  - Writes logs to local spool.

### Tasks

1. **Implement manifest endpoint**
   - `GET /internal/api/v1/runs/{run_id}/manifest`:
     - Look up `job_runs`, `jobs`, `job_templates`, `projects`, `inventories`, `credentials`, `execution_environments`.
     - Assemble manifest JSON matching the guide’s example.
   - Add tests for different Job types (regular job, project update, inventory update).

2. **Wire executor Pod to call manifest**
   - On container start:
     - Read `RUN_ID` and `PRAETOR_API_URL` from env.
     - Call manifest endpoint with appropriate auth.
     - Log errors and exit gracefully if manifest cannot be fetched.

3. **Run basic task**
   - For now, executor can:
     - Write manifest to disk.
     - Run a simple command/playbook (e.g. `echo "hello from RUN_ID"`).
   - Write stdout/stderr into local spool file(s).

4. **Plumb back status**
   - Ensure executor exit code is captured by K8s controller and reflected in `job_runs.state`.

### Acceptance Criteria

- A Job launched from API leads to:
  - Scheduler assigning a Run.
  - K8s controller starting an executor Pod.
  - Executor calling manifest endpoint and running the configured task.
- Basic logs exist on disk inside Pod (even if not yet pushed to object storage).

---

## Phase 6 – Events, Logs, and Ingestion

### Goal

Implement **Job Events**, **Host Summaries**, and **Logs** end to end so the UI can see live-ish job output and host status.

### Inputs

- JobEvents and JobHostSummaries tables (Phase 1).
- Local spool logging in executor (Phase 5).
- Event bus infrastructure (Kafka, NATS, etc.) or stub for now.

### Deliverables

- Executor sends events/logs to the bus (and spool).
- Ingestion service persists events and summaries to DB.
- `/api/v1/job-events`, `/api/v1/job-host-summaries`, and `/api/v1/jobs/{id}/logs` are implemented.

### Tasks

1. **Define bus contracts**
   - Message formats for:
     - `job-events` topic.
     - `run-state` topic (optional—controller may handle state separately).
   - Include `run_id`, `job_id`, `seq`, `event_type`, timestamp, payload.

2. **Extend executor to emit events & logs**
   - For each Ansible event/log line:
     - Write to local spool (append-only).
     - Publish event/log batch to bus when network is available.

3. **Implement ingestion service**
   - Consume from `job-events`:
     - Insert into `job_events` (idempotent on `run_id + seq`).
     - Update `job_host_summaries` and Jobs summary counts.
   - Optionally consume from `run-state` for run-level state updates.

4. **Implement log chunk indexing**
   - Upload log chunks from executor to object storage with `run_id` + `seq`.
   - Ingestion service writes `log_chunks` rows referencing `storage_key` + `seq`.
   - Implement `/api/v1/runs/{run_id}/log-chunks` and `/api/v1/jobs/{job_id}/logs` using this index.

5. **Front-end-compatible formats**
   - Ensure `/api/v1/job-events` and `/api/v1/jobs/{id}/logs` return data that can drive AWX-style UI views (even if initially simplified).

### Acceptance Criteria

- Running a Job produces:
  - `job_events` rows in DB.
  - `job_host_summaries` rows aggregated per host.
  - Log chunks stored in object storage and indexed in DB.
- `/api/v1/jobs/{id}/events`, `/api/v1/jobs/{id}/hosts`, `/api/v1/jobs/{id}/logs` return meaningful data.

---

## Phase 7 – Resilience & Reconciliation

### Goal

Make the system robust to **DB** and **event bus** outages while Jobs are running, via **spool-based reconciliation**.

### Inputs

- Local spool writing in executor.
- Ingestion service with bus integration.

### Deliverables

- A reconciliation process that can reconstruct DB state from spool/object storage.
- Verified behaviour under simulated outages.

### Tasks

1. **Design reconciliation markers**
   - For each `run_id`, track:
     - Highest `seq` seen in DB.
     - Whether DB considers the run complete.

2. **Implement reconciliation job**
   - Periodically scan runs where:
     - `job_runs.state` is non-terminal OR recently terminal.
     - The spool or object storage contains events/logs beyond DB’s last `seq`.
   - Re-read and replay missing events/logs into DB.

3. **Handle DB downtime scenario**
   - Simulate: DB down while executor runs.
   - Ensure executor still writes spool and optional object storage.
   - After DB returns, reconciliation should:
     - Insert events into `job_events`.
     - Update `job_host_summaries` and `jobs.status` / `job_runs.state`.

4. **Handle bus downtime scenario**
   - Simulate: bus down while executor runs.
   - Events/logs stay in spool and are uploaded once bus is restored or via direct spool-based ingestion.

### Acceptance Criteria

- A Job that runs while DB or bus is down still ends up with correct:
  - `job_events`
  - `job_host_summaries`
  - `jobs.status` and `job_runs.state`
  - `logs` in object storage and visible via API.
- No duplicate or out-of-order events in DB (idempotent replay works).

---

## Phase 8 – Infra & Control Plane Management

### Goal

Expose AWX-like **Instance Groups / Instances** UX:

- Execution Pools → Execution Instances.
- Control Plane Instances with enable/disable and CPU/RAM tuning.

### Inputs

- Execution Pools implemented (Phase 2).
- K8s controller and scheduler (Phases 3–4).

### Deliverables

- `/api/v1/infra/instances` endpoints powered by cluster state.
- `/api/v1/infra/control-plane-instances` endpoints wired to cluster and/or config.
- Control-plane-enable/disable and capacity fields enforced in scheduler/controller logic.

### Tasks

1. **Execution Instances discovery**
   - Implement infra introspection service:
     - Periodically list Pods with role `executor`.
     - Group by Execution Pool.
     - Expose `/api/v1/infra/instances` and `/api/v1/infra/execution-pools/{id}/instances` using this view.

2. **Control Plane Instances model**
   - Decide source of truth:
     - Static config in DB vs derived from Pods/Deployments.
   - Implement CRUD or read-only view:
     - `/api/v1/infra/control-plane-instances`
     - `/api/v1/infra/control-plane-instances/{id}`

3. **Enable/disable and capacity controls**
   - Implement `PATCH /api/v1/infra/control-plane-instances/{id}` to change:
     - `enabled`
     - `cpu_cores`
     - `memory_mb`
   - Update scheduler/controller to respect `enabled` and capacity fields.

### Acceptance Criteria

- The API returns the same logical information as AWX’s Instances/Instance Groups pages.
- Toggling `enabled` or adjusting CPU/RAM on control plane instances affects scheduling/routing decisions as designed.

---

## Phase 9 – Advanced Features & UI Parity Polish

### Goal

Bring the rest of AWX-equivalent features online and refine everything needed for full UI parity.

### Inputs

- All previous phases implemented and stable.

### Deliverables

- Workflows, approvals, schedules, notifications, activity stream, bulk operations.
- Performance and scalability adjustments.

### Tasks

1. **Workflows & approvals**
   - Implement Workflow Job Templates and Workflow Jobs.
   - Implement approvals and `/api/v1/workflow-approvals` approve/deny actions.

2. **Schedules**
   - Implement `/api/v1/schedules` and `schedules/preview`.
   - Integrate scheduling engine for RRULE-driven Jobs.

3. **Notifications**
   - Implement notification templates and notification dispatchers.
   - Wire event triggers to notifications (job failure, success, etc.).

4. **Activity stream**
   - Log changes to key resources and expose via `/api/v1/activity-stream`.

5. **Bulk operations**
   - Implement `/api/v1/bulk/...` endpoints for host creation, deletion, and bulk job launch.

6. **Performance and scaling**
   - Optimize queries and indexing.
   - Add caching where necessary (e.g. for inventories, credential types).

### Acceptance Criteria

- For each AWX UI area, there is a functionally equivalent Praetor view powered by `/api/v1`.
- Performance targets are met (e.g. jobs/event lists behave reasonably at expected scale).

---

## Summary

If an agent follows this roadmap **in order**, and each phase meets its acceptance criteria, the result will be a fully implemented Praetor backend that:

- Conforms to the **Praetor REST API Reference v1**.
- Implements the **Job / Run / Event / Log** execution model.
- Integrates with Kubernetes, object storage, and an event bus.
- Is resilient to DB and bus outages during long-running jobs.
- Provides UI-level feature parity with AWX for all major workflows.
