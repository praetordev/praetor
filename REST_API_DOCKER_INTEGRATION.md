# REST API Backend v1 Specification

## 1. Introduction

This document specifies the backend API v1 for the AWX system. It defines the resource schemas, endpoints, and expected behaviors.

## 2. Core Resource Schemas

### 2.1 User (simplified)

```json
{
  "id": 1,
  "username": "admin",
  "email": "admin@example.com",
  "first_name": "Admin",
  "last_name": "User",
  "is_superuser": true,
  "is_system_auditor": false
}
```

### 2.2 Inventory (simplified)

```json
{
  "id": 42,
  "name": "Production Inventory",
  "description": "Hosts for production",
  "organization_id": 1,
  "kind": "inventory"
}
```

### 2.3 Job Template (simplified)

```json
{
  "id": 17,
  "name": "Deploy App",
  "description": "Deploy application to servers",
  "inventory_id": 42,
  "project_id": 9,
  "playbook": "deploy.yml",
  "execution_environment": {
    "id": 5,
    "image": "registry.example.com/ee:latest"
  },
  "credential_ids": [5, 6],
  "extra_vars": {
    "some_var": "value"
  }
}
```

### 2.4 Execution Environment

```json
{
  "id": 5,
  "name": "Default EE",
  "description": "Default execution environment",
  "organization_id": 1,
  "image": "quay.io/ansible/awx-ee:latest",
  "credential_id": 10,
  "pull": "missing",         // always | missing | never
  "managed": true,
  "created_at": "2025-12-01T10:00:00Z",
  "modified_at": "2025-12-08T11:22:00Z"
}
```

### 2.5 Job (logical)

```json
{
  "id": 55,
  "name": "Deploy App to Prod",
  "job_template_id": 17,
  "status": "running",              // pending | queued | running | successful | failed | canceled | error
  "created_at": "2025-12-10T09:00:00Z",
  "started_at": "2025-12-10T09:01:10Z",
  "finished_at": null,
  "timeout_seconds": 7200,
  "cancel_requested": false,
  "inventory_id": 42,
  "project_id": 9,
  "execution_environment_id": 5,
  "summary": {
    "hosts_total": 10,
    "hosts_ok": 8,
    "hosts_failed": 1,
    "hosts_unreachable": 1,
    "hosts_skipped": 0,
    "hosts_changed": 7
  },
  "current_run_id": "c0c3bdc8-08c8-49aa-aea3-51f2d036e9f1"
}
```

### 2.6 Job Run (physical attempt)

```json
{
  "id": "c0c3bdc8-08c8-49aa-aea3-51f2d036e9f1",
  "job_id": 55,
  "attempt_number": 1,
  "state": "running",               // pending | starting | running | successful | failed | canceled | lost
  "created_at": "2025-12-10T09:00:10Z",
  "started_at": "2025-12-10T09:01:10Z",
  "finished_at": null,
  "last_heartbeat_at": "2025-12-10T09:05:05Z",
  "last_event_seq": 42,
  "exit_code": null,
  "error_reason": null               // e.g. executor_lost, canceled
}
```

### 2.7 Job Event

```json
{
  "id": 1001,
  "job_id": 55,
  "job_run_id": "c0c3bdc8-08c8-49aa-aea3-51f2d036e9f1",
  "seq": 42,
  "timestamp": "2025-12-10T09:03:11.123Z",
  "event_type": "TASK_OK",          // TASK_OK | TASK_FAILED | JOB_STARTED | JOB_COMPLETED | ...
  "host": "web01.example.com",
  "task_name": "Install nginx",
  "play_name": "Configure web servers",
  "stdout_snippet": "changed: [web01.example.com]",
  "data": {
    "rc": 0,
    "changed": true
  }
}
```

### 2.8 Host Summary

```json
{
  "job_id": 55,
  "host": "web01.example.com",
  "ok": 10,
  "changed": 2,
  "failed": 0,
  "skipped": 0,
  "unreachable": 0,
  "last_event_at": "2025-12-10T09:05:00Z"
}
```

### 2.9 Log Chunk

```json
{
  "job_run_id": "c0c3bdc8-08c8-49aa-aea3-51f2d036e9f1",
  "seq": 0,
  "storage_key": "jobs/c0c3bdc8-08c8-49aa-aea3-51f2d036e9f1/chunk-0000.log",
  "byte_length": 1024,
  "created_at": "2025-12-10T09:04:00Z"
}
```

## 3. Public API (`/api/v1`) – Jobs & Runs

### 3.1 Create Job

POST `/api/v1/jobs`

Request body example:

```json
{
  "job_template_id": 17,
  "execution_environment_id": 5,    // optional override, defaults from template
  "inventory_id": 42,               // optional override, defaults from template
  "execution_environment": null,    // deprecated: use execution_environment_id
  "extra_vars": {
    "version": "1.2.3",
    "env": "prod"
  },
  "limit": null,
  "job_type": "run",
  "verbosity": 3
}
```

Notes:

- `job_template_id`: Required. The ID of the job template to launch.
- `execution_environment_id`: Optional override, defaults from template.
- `inventory_id`: Optional override, defaults from template.
- If `execution_environment_id` is omitted, the template’s execution environment is used.
- `execution_environment`: Deprecated; use `execution_environment_id` instead.
- `extra_vars`: Extra variables to pass to the job.

## 4. Public API – Other Resources

...

## 5. Public API – Job Templates (Brief)

...

## 6. Public API – Execution Environments

### 6.1 List Execution Environments

**GET `/api/v1/execution-environments`**

Query params:

- `organization_id`
- `name` (search)
- `managed` (true/false)
- `limit`, `offset`

Response:

```json
{
  "items": [ { /* Execution Environment summary */ } ],
  "total": 3,
  "limit": 50,
  "offset": 0
}
```

### 6.2 Get Execution Environment Detail

**GET `/api/v1/execution-environments/{id}`**

Returns full Execution Environment resource (see 2.4).

### 6.3 Create Execution Environment

**POST `/api/v1/execution-environments`**

Body: Execution Environment fields (minus IDs, timestamps).

Response: `201 Created` with Execution Environment object.

### 6.4 Update Execution Environment

**PATCH `/api/v1/execution-environments/{id}`**

Partial update, JSON Merge Patch semantics.

### 6.5 Delete Execution Environment

**DELETE `/api/v1/execution-environments/{id}`**

Deletes a non-managed execution environment. Managed EEs SHOULD return `403` or `409` if delete is not allowed.

## 7. Internal API (`/internal/api/v1`) – Executors

### 7.1 Fetch Execution Manifest

Request:

`GET /internal/api/v1/job-runs/{job_run_id}/manifest`

Response example:

```json
{
  "job_run_id": "c0c3bdc8-08c8-49aa-aea3-51f2d036e9f1",
  "job_id": 55,
  "attempt_number": 1,
  "execution_environment": {
    "id": 5,
    "image": "registry.example.com/ee:latest"
  },
  "inventory": {
    "mode": "inline",          // inline | file | scm
    "content": "[all]\nweb01.example.com\n",
    "path": null
  },
  "playbook": {
    "type": "scm",
    "project_id": 9,
    "playbook_path": "deploy.yml"
  },
  "extra_vars": {
    "some_var": "value"
  },
  "credentials": [
    { "id": 5, "type": "machine", "name": "SSH Key" },
    { "id": 6, "type": "vault", "name": "Vault Password" }
  ],
  "timeout_seconds": 7200,
  "limit": null,
  "verbosity": 3
}
```

## 8. Endpoint Summary Table

...

## 9. Notes for Implementation

...
