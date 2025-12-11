
# Resilient Ansible Automation Platform – Vision Document

## 1. Context and Vision

We want to build a next-generation Ansible automation platform that can:

- Run long-lived playbooks and workflows (hours+).
- Survive control-plane database failovers or brief outages without failing jobs.
- Provide rich, per-task/per-host observability similar to (or better than) AWX/AAP.
- Scale horizontally across multiple execution nodes.

In contrast to AWX/AAP’s tighter coupling between Celery workers and the primary Postgres database, this platform is designed around the principle:

> **Execution must not depend on the control-plane database’s real-time availability.**

The database is the system of record for configuration and derived state, but at runtime the true source of truth is the **execution events emitted by workers**.

---

## 2. High-Level Goals and Non-Goals

### 2.1 Goals

1. **Resilient job execution**
   - Database failover / brief outages do not fail running jobs.
   - Jobs may temporarily lose UI updates but execution continues.
   - Final job state always converges to the true outcome once infrastructure is healthy.

2. **Strong eventual consistency**
   - Control-plane state is eventually consistent with what actually happened on the executors.
   - No “job actually succeeded but DB thinks it failed” scenarios.

3. **Crisp separation of concerns**
   - Control plane: templates, configuration, RBAC, job lifecycle, projections.
   - Execution plane: running Ansible, emitting events, pushing logs.
   - Event stream: decoupling layer between execution and persistence.

4. **First-class support for long-running, large-scale jobs**
   - Large inventories and workflows.
   - Efficient logging via object storage.
   - Heartbeats and reconciliation to track health and final state.

5. **Compatibility with familiar Ansible semantics**
   - Per-task, per-host events.
   - Inventory and credential model similar to AWX/AAP.
   - Ability to extend to workflows, inventories, project updates, etc.

### 2.2 Non-Goals (Initial Phase)

- Providing a drop-in UI clone of AWX/AAP.
- Solving multi-tenant SaaS isolation and billing (can be introduced later).
- Guaranteeing execution correctness if *all* backing services (DB, event bus, object storage) are unavailable for long periods.
- Implementing full replay / resume of partially completed playbooks (future work).

---

## 3. Conceptual Architecture

The system is split into two planes plus a backbone:

- **Control Plane (DB-centric):**
  - Postgres (HA)
  - API service
  - Scheduler
  - Event consumer
  - Reconciler
- **Execution Plane (DB-independent):**
  - Execution agents (Ansible runners)
  - Local manifests and event logs
- **Backbone:**
  - Event stream (Kafka/NATS/etc.)
  - Object storage for logs and artifacts

### 3.1 Control Plane

Responsible for:

- Configuration and templates (organizations, inventories, credentials, projects, job templates, workflows).
- RBAC and authentication.
- Launching jobs and creating logical job records.
- Consuming and projecting execution events into the database.
- Reconciliation of stale or lost executions.

If the database is down, the control plane **cannot** launch new jobs or reliably answer queries, but it **must not** impact already-running jobs.

### 3.2 Execution Plane

Responsible for:

- Running Ansible playbooks and workflows.
- Emitting fine-grained events and log chunk references.
- Sending heartbeats indicating liveness.
- Persisting a local manifest and event log as a safety net.

Execution agents should be able to continue as long as they can:

- Read their local manifest.
- Reach target hosts.
- Connect (eventually) to the event stream and object storage.

They **must not** depend on Postgres during job execution.

### 3.3 Event and Log Backbone

- **Event stream** (e.g. Kafka/Redpanda/NATS JetStream):
  - Primary channel for job and task events.
  - Reliable delivery and replay capability.
- **Object storage** (e.g. S3/Minio):
  - Primary storage for stdout and large artifacts.
  - Database holds references (indexes), not the raw logs.

The event stream is the **source of truth for execution**; the database is an eventually consistent projection of it.

---

## 4. Data Model – Concrete Entities

The data model is heavily inspired by AWX but adjusted to enforce control-plane vs execution-plane separation.

### 4.1 Reused / Control-Plane Models

These live in Postgres and are similar to AWX:

- **Organizations & RBAC**
  - `Organization`, `Team`, `User`, `Setting`
- **Inventories**
  - `Inventory`, `Host`, `Group`, `InventorySource`, `InventoryUpdate`
- **Credentials**
  - `CredentialType`, `Credential`, `CredentialInputSource`, `ExecutionEnvironment`
- **Projects & SCM**
  - `Project`, `ProjectUpdate`
- **Job Templates & Workflows**
  - `UnifiedJobTemplate`, `JobTemplate`, `WorkflowJobTemplate`, `WorkflowJobTemplateNode`
- **Execution Infrastructure**
  - `InstanceGroup`, `Instance`, `InstanceLink`
- **Schedules, Notifications, etc.**

These tables are not in the hot execution path and can rely on Postgres as their primary store.

### 4.2 Logical Job: `unified_job`

`unified_job` remains the polymorphic high-level representation of a job, similar to AWX, with concrete derivatives (`job`, `project_update`, `inventory_update`, `workflow_job`, etc.).

Key characteristics:

- Represents **what** is being done (playbook, project update, inventory sync, workflow, etc.).
- Used by the API, UI, and RBAC.
- Stores high-level status (`pending`, `queued`, `running`, `successful`, `failed`, `canceled`, `error`).

`unified_job` does *not* need to be updated on every task event; it is updated by **event consumers and reconcilers**, not by executors directly.

### 4.3 Physical Execution Attempt: `execution_run`

New table, primary identity for the executor:

```sql
CREATE TABLE execution_run (
  id                  UUID PRIMARY KEY,         -- run_id, executor’s primary identity
  unified_job_id      BIGINT NOT NULL REFERENCES unified_job(id),
  attempt_number      INT NOT NULL DEFAULT 1,   -- for retries, future work
  executor_instance_id BIGINT NULL,            -- FK to instance/worker node
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  started_at          TIMESTAMPTZ,
  finished_at         TIMESTAMPTZ,
  state               TEXT NOT NULL DEFAULT 'pending',
                      -- 'pending', 'starting', 'running',
                      -- 'successful', 'failed', 'canceled', 'lost'
  last_heartbeat_at   TIMESTAMPTZ,
  last_event_seq      BIGINT NOT NULL DEFAULT 0
);

CREATE INDEX ON execution_run (unified_job_id);
```

Properties:

- Every job launch creates at least one `execution_run`.
- Executors identify themselves with `execution_run.id` and `unified_job_id`.
- Status is driven by events (`JOB_STARTED`, `JOB_COMPLETED`, heartbeats), not direct executor DB writes.

### 4.4 Job Events: `job_event`

Append-only, written only by the event consumer:

```sql
CREATE TABLE job_event (
  id                  BIGSERIAL PRIMARY KEY,
  unified_job_id      BIGINT NOT NULL REFERENCES unified_job(id),
  execution_run_id    UUID NOT NULL REFERENCES execution_run(id),
  seq                 BIGINT NOT NULL,         -- monotonic per execution_run
  event_type          TEXT NOT NULL,           -- 'JOB_STARTED', 'TASK_OK', 'TASK_FAILED', etc.
  host_id             BIGINT NULL REFERENCES host(id),
  task_name           TEXT NULL,
  play_name           TEXT NULL,
  event_data          JSONB NOT NULL,          -- raw Ansible event payload
  stdout_snippet      TEXT NULL,               -- optional small snippet
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX job_event_run_seq_uniq
  ON job_event (execution_run_id, seq);

CREATE INDEX job_event_job_id_idx ON job_event (unified_job_id);
CREATE INDEX job_event_host_id_idx ON job_event (host_id);
```

- `event_type` captures semantic meaning.
- `event_data` provides raw detail for debugging.
- `execution_run_id + seq` uniqueness guarantees idempotency.

### 4.5 Host Summaries: `job_host_summary`

Projection summarizing events per host:

```sql
CREATE TABLE job_host_summary (
  id              BIGSERIAL PRIMARY KEY,
  unified_job_id  BIGINT NOT NULL REFERENCES unified_job(id),
  host_id         BIGINT NOT NULL REFERENCES host(id),
  changed         INT NOT NULL DEFAULT 0,
  failed          INT NOT NULL DEFAULT 0,
  ok              INT NOT NULL DEFAULT 0,
  skipped         INT NOT NULL DEFAULT 0,
  unreachable     INT NOT NULL DEFAULT 0,
  last_event_at   TIMESTAMPTZ
);

CREATE UNIQUE INDEX job_host_summary_job_host_uniq
  ON job_host_summary (unified_job_id, host_id);
```

The event consumer updates this table based on incoming task results.

### 4.6 Log Chunks: `job_output_chunk`

Index of log storage in object storage:

```sql
CREATE TABLE job_output_chunk (
  id                 BIGSERIAL PRIMARY KEY,
  execution_run_id   UUID NOT NULL REFERENCES execution_run(id),
  seq                BIGINT NOT NULL,          -- chunk sequence
  storage_key        TEXT NOT NULL,            -- e.g. 'jobs/<execution_run_id>/chunk-0010.log'
  byte_length        INT NOT NULL,
  created_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX job_output_run_seq_uniq
  ON job_output_chunk (execution_run_id, seq);
```

- Executors write logs to object storage.
- They emit `LOG_CHUNK` events with references.
- The consumer upserts this table to index those artifacts.

---

## 5. Execution Flow (End-to-End)

### 5.1 Job Launch

1. **Client → API**
   - POST `/jobs/` with `job_template_id`, extra vars, etc.

2. **API Service**
   - Reads job template, inventory, credentials, project, and related configuration from Postgres.
   - Creates `unified_job` row with status `pending`.
   - Returns `unified_job.id` to the client.

3. **Scheduler**
   - Polls for `unified_job.status = 'pending'`.
   - For each:
     - Creates `execution_run` row with `state = 'pending'`.
     - Generates a **job manifest**:
       - Resolved project content reference.
       - Rendered inventory (or reference).
       - Extra vars.
       - Credential secrets (already resolved).
       - Execution environment reference, timeouts, etc.
     - Publishes an execution request message to `job-execution-requests`:

       ```json
       {
         "execution_run_id": "uuid-v7",
         "unified_job_id": 123,
         "job_manifest": { ... }
       }
       ```

   - Updates `unified_job.status` → `'queued'` (or `'waiting'`).

### 5.2 Executor Lifecycle

1. **Receive Execution Request**
   - Executor agent subscribes to `job-execution-requests`.
   - On message:
     - Creates a local run directory and writes `manifest.json`.
     - Prepares environment: project checkout, inventory file, credentials, etc.

2. **Start Execution**
   - Spawns `ansible-runner` using the manifest.
   - Immediately emits a `JOB_STARTED` event with `seq=1`.

3. **During Execution**
   - For each Ansible callback event:
     - Increments `seq`.
     - Sends an event message to the `job-events` topic:

       ```json
       {
         "execution_run_id": "uuid",
         "unified_job_id": 123,
         "seq": 42,
         "timestamp": "...",
         "kind": "TASK_OK",
         "host": "web-01",
         "task_name": "Install nginx",
         "play": "Configure web servers",
         "stdout_snippet": "changed: [web-01]",
         "data": { ...full event payload... }
       }
       ```

   - Periodically sends `HEARTBEAT` events:

     ```json
     {
       "execution_run_id": "uuid",
       "unified_job_id": 123,
       "seq": <next>,
       "kind": "HEARTBEAT",
       "node": "instance-7",
       "timestamp": "..."
     }
     ```

   - Buffers stdout and writes log files to object storage. After each chunk upload, sends a `LOG_CHUNK` event with `seq` and `storage_key`.

4. **Completion**
   - On completion, sends a final `JOB_COMPLETED` (or `JOB_FAILED`) event with rc and summary.
   - Updates local manifest with final status and exit code.

The executor **never** calls the database directly during the run.

### 5.3 Event Consumption and Projection

**Event Consumer Service** (stateless, horizontally scalable):

- Subscribes to `job-events` topic.
- For each event:
  1. Begins a DB transaction.
  2. Writes to `job_event` or `job_output_chunk` based on `kind`, enforcing `execution_run_id + seq` uniqueness.
  3. Updates projections:
     - `execution_run.last_event_seq`.
     - `execution_run.last_heartbeat_at` for `HEARTBEAT` events.
     - `job_host_summary` counters for host-related task events.
  4. Updates `unified_job` and `execution_run` status:
     - On first `JOB_STARTED`:
       - `execution_run.state = 'running'`
       - `unified_job.status = 'running'`
       - `started_at = timestamp`
     - On `JOB_COMPLETED` / `JOB_FAILED`:
       - Set `execution_run.state` and `unified_job.status` accordingly.
       - Set `finished_at = timestamp`.

- Commits the transaction, then acknowledges the event offset.

If the database is unavailable or write fails:

- Consumer fails to commit the transaction.
- It does not acknowledge the event offset.
- Events accumulate in the stream and will be replayed later when the consumer recovers.

---

## 6. Failure Modes and Desired Behavior

### 6.1 Database Failover During Long-Running Job

**Scenario:**

- Job has been running for 1 hour.
- DB goes offline for 30–60 seconds during failover.

**Behavior:**

- **Executor:** unaffected; continues running and sending events to the event stream.
- **Event Consumer:** DB writes fail, so it stops committing offsets. Events accumulate in the stream.
- **UI/API:** may display stale state; can optionally show a banner like “control plane degraded; updates delayed”.

After DB is healthy again:

- Consumer resumes.
- Replays all buffered events from the stream.
- Database state catches up and reflects the correct state (including final result).

The job **never fails** due to the DB outage.

### 6.2 Event Stream Outage

If the event stream (Kafka/NATS/etc.) is temporarily down:

- Executor detects publish failures.
- It enters **local buffering mode**:
  - Appends events to a local `events.log` file.
- Periodically retries publishing buffered events.
- If the outage is short:
  - All events eventually reach the stream (possibly in bursts).

If the outage is prolonged:

- Depending on policy:
  - Allow job to complete and later re-stream `events.log` to the event bus.
  - Or, in stricter setups, mark job status as “logs incomplete” until re-sync succeeds.

Crucially, the job’s effect on target systems is independent of the event stream, and we do not treat stream outages as automatic job failures.

### 6.3 Executor Crash Mid-Run

If an executor node crashes mid-run:

- Heartbeats stop.
- No further events are emitted for that `execution_run`.

**Reconciler Service** (periodic job):

1. Finds `execution_run` rows where:
   - `state = 'running'`
   - `now() - last_heartbeat_at > heartbeat_timeout`
2. Checks executor instance health (optional, using `Instance` model).
3. If executor is dead / unreachable and there is no final `JOB_COMPLETED` / `JOB_FAILED` event:
   - Marks `execution_run.state = 'lost'`.
   - Sets `unified_job.status = 'error'` (or a specific “executor_lost” state).

Future enhancements may allow:

- Resumption of a lost run from its manifest and event log (best-effort).
- Automatic retry via new `execution_run` rows (with `attempt_number + 1`).

---

## 7. State Machines

### 7.1 `execution_run.state`

States:

- `pending`
- `starting`
- `running`
- `successful`
- `failed`
- `canceled`
- `lost`

Transitions:

- `pending` → `starting`:
  - Scheduler has created the run and published an execution request.
- `starting` → `running`:
  - First `JOB_STARTED` event consumed.
- `running` → `successful`:
  - `JOB_COMPLETED` event with `rc == 0`.
- `running` → `failed`:
  - `JOB_COMPLETED` with `rc != 0` or explicit `JOB_FAILED` event.
- `running` → `canceled`:
  - User cancels, and cancel event is processed.
- `running` → `lost`:
  - Heartbeat timeout with no final completion event, and executor is deemed dead.

### 7.2 `unified_job.status`

High-level status visible to users:

- `pending`
- `queued`
- `running`
- `successful`
- `failed`
- `canceled`
- `error` (e.g. `execution_lost`, scheduler issues)

Transitions driven by:

- Scheduler actions (e.g. `pending` → `queued`).
- Event consumer updates (e.g. `queued` → `running`, `running` → final state).
- Reconciler corrections (e.g. `running` → `error` on lost executors).

---

## 8. Component Responsibilities

### 8.1 API Service

- REST/gRPC endpoints for:
  - Auth, RBAC.
  - CRUD for organizations, inventories, credentials, projects, job templates, workflows.
  - Job launch and cancel.
  - Job/event/log queries (from DB and log service).
- Talks only to Postgres (and log service).

### 8.2 Scheduler

- Periodically scans for `unified_job.status = 'pending'`.
- Creates `execution_run` rows, publishes execution requests.
- Updates `unified_job.status` to `queued`.
- Enforces placement policies (instance groups, capacity, etc.).

### 8.3 Executor Agent

- Runs on worker nodes (`Instance`).
- Subscribes to `job-execution-requests`.
- Handles manifest preparation, environment setup, and Ansible execution.
- Emits events and log chunk references to the event stream.
- Sends heartbeats.
- Maintains local manifest and event log for resiliency.

### 8.4 Event Consumer

- Subscribes to `job-events` topic.
- Writes `job_event`, `job_output_chunk`, and projections (`job_host_summary`, `execution_run`, `unified_job` fields).
- Fully idempotent using `(execution_run_id, seq)` constraints.
- Suspends processing when DB is unavailable; resumes automatically.

### 8.5 Reconciler

- Periodic job/process.
- Detects stale/lost executions via heartbeats and state.
- Corrects `execution_run.state` and `unified_job.status`.
- Optionally initiates retries or follow-up actions.

### 8.6 Log Service (Optional Separate Service)

- Streams logs to the UI by reading `job_output_chunk` and object storage.
- Handles pagination, filtering by host/task, etc.

---

## 9. Why This Is Less Brittle than AWX/AAP

1. **Execution decoupled from DB**
   - Executors do not write to Postgres directly.
   - DB outages affect only the control plane and projections, not the execution of playbooks.

2. **Event stream as the source of truth**
   - Execution events are stored in a durable stream.
   - Database is an eventually consistent consumer of that stream.

3. **Idempotent, append-only event model**
   - Replays after failover are safe.
   - No “last write wins” race conditions for job state.

4. **Logs in object storage**
   - Database is not overloaded by per-line log writes.
   - Log delivery to the UI is flexible and scalable.

5. **Explicit liveness and reconciliation**
   - Heartbeats + reconciler avoid the “job just disappeared” problem.
   - Lost execution is an explicit modeled state.

6. **Future-friendly**
   - `execution_run` model allows retries, multiple attempts, and potentially resuming runs.
   - The event model can be extended to workflows, inventory updates, project syncs, etc.

---

## 10. Next Steps

Short-term next actions:

1. **Define event schemas** (JSON or protobuf) for execution and log events.
2. **Implement a minimal executor** wrapping `ansible-runner` that:
   - Reads a manifest.
   - Emits events and heartbeats to a local Kafka/NATS cluster.
   - Writes logs to MinIO.
3. **Implement event consumer MVP**:
   - Writes to `execution_run`, `unified_job`, `job_event`, and `job_output_chunk` tables.
4. **Produce a basic API/UI** to visualize:
   - Job status.
   - Per-host summaries.
   - Log streaming.

From there, we can harden, benchmark, and extend the platform to workflows and other job types while preserving the core resilience principles defined here.
