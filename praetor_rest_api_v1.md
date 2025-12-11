
# Praetor REST API Reference (v1)

## Table of Contents
1. [Overview](#overview)
2. [API Standards and Best Practices](#api-standards-and-best-practices)
3. [Authentication](#authentication)
4. [Versioning](#versioning)
5. [API Root and Discovery](#api-root-and-discovery)
6. [Pagination](#pagination)
7. [Filtering, Searching, and Sorting](#filtering-searching-and-sorting)
8. [Response Formats](#response-formats)
9. [Permissions and RBAC](#permissions-and-rbac)
10. [Core Execution Resources](#core-execution-resources)
11. [Control-Plane Resource Endpoints](#control-plane-resource-endpoints)
12. [Special Features](#special-features)
13. [Error Handling](#error-handling)
14. [URL Patterns](#url-patterns)
15. [References](#references)

---

## Overview

The Praetor REST API is a comprehensive RESTful API that provides programmatic access to Praetor's control plane and execution capabilities. The API is designed to:

- Be **Kubernetes-native**.
- Support **long-running Ansible workloads** that are resilient to database and event bus outages.
- Expose a **clean, versioned HTTP surface** for UIs, CLIs, and automation.

The design is inspired by the AWX API but is aligned with Praetor’s execution model (Jobs + Job Runs + Events + Logs).

**Base URL**: `/api/v1/`

**Current Version**: v1 (only supported version)

**Content-Type**: `application/json`

Unless otherwise specified, all examples in this document assume JSON requests and responses.

---

## API Standards and Best Practices

### Core Principles

1. **Paginate Everything**  
   All collection endpoints MUST be paginated.

2. **Performance Target**  
   Typical response time SHOULD be ≤ 250ms under normal load.

3. **Assume Large Data Sets**  
   Design for organizations with thousands of hosts and years of events.

4. **RBAC Filtering**  
   All collections MUST be automatically filtered by the caller’s access permissions.

5. **API Discoverability**  
   The API MUST be discoverable from the root endpoint `/api/`.

6. **RESTful Verbs**  
   Use standard HTTP methods: `GET`, `POST`, `PUT`, `PATCH`, `DELETE`.

7. **Constant-Time Queries**  
   Database query complexity SHOULD NOT grow with result set size.

8. **Limited Query Count**  
   The number of SQL queries for a request SHOULD be bounded and predictable.

### Additional Requirements

- **Indexes**  
  Frequently-used filters (especially those used by the UI) SHOULD be backed by database indexes.

- **UI-Friendly Data**  
  API responses SHOULD provide data at a level that minimizes per-row UI transformations.

- **Serializer Safety**  
  Avoid N+1 query patterns in serializers; prefetch related objects via queryset configuration.

- **Unit Tests**  
  Every route MUST have test coverage (happy path + basic error modes).

---

## Authentication

Praetor supports token-based and session-based authentication.

### Supported Authentication Methods

1. **Session Authentication**
   - Login via `/auth/login/` (implementation-defined).
   - Uses CSRF protection for browser-based clients.
   - Suitable for human-driven UI and interactive API browsers.

2. **HTTP Basic Authentication (Optional)**
   - Can be enabled via configuration.
   - Supports username/password in `Authorization: Basic` header.
   - Requests SHOULD be logged for auditability.

3. **Bearer Token Authentication (Recommended)**
   - `Authorization: Bearer <token>`
   - Token can be a JWT or opaque token issued by Praetor or an external IdP.
   - Tokens MUST carry identity and may carry organization/tenant and role claims.

### Login/Logout Endpoints

Praetor does not prescribe specific login/logout URLs in this spec; typical deployments may expose:

- `POST /auth/login/`  
- `POST /auth/logout/`  

Implementations MAY reuse these patterns.

---

## Versioning

### URL Path Versioning

Praetor uses URL path versioning:

- **API Root**: `/api/`
- **Current Version Root**: `/api/v1/`

Examples:

- `/api/` – version discovery.
- `/api/v1/jobs/` – v1 jobs endpoint.

Future, incompatible versions MUST use a new path prefix (e.g. `/api/v2/`).

---

## API Root and Discovery

### Root Endpoint

**GET** `/api/`

Returns information about available API versions, e.g.:

```json
{
  "description": "Praetor REST API",
  "current_version": "/api/v1/",
  "available_versions": {
    "v1": "/api/v1/"
  }
}
```

### Version 1 Root

**GET** `/api/v1/`

Returns top-level resource endpoints for discovery, for example:

```json
{
  "ping": "/api/v1/ping/",
  "config": "/api/v1/config/",
  "me": "/api/v1/me/",
  "organizations": "/api/v1/organizations/",
  "users": "/api/v1/users/",
  "projects": "/api/v1/projects/",
  "inventories": "/api/v1/inventories/",
  "job_templates": "/api/v1/job-templates/",
  "jobs": "/api/v1/jobs/",
  "job_runs": "/api/v1/runs/",
  "job_events": "/api/v1/job-events/",
  "job_host_summaries": "/api/v1/job-host-summaries/",
  "execution_environments": "/api/v1/execution-environments/",
  "credentials": "/api/v1/credentials/"
}
```

Exact keys MAY evolve; the root is intended for interactive discovery.

---

## Pagination

### Standard Pagination

All list endpoints support pagination with these query parameters:

- `page` – Page number (1-based; default: 1)
- `page_size` – Results per page (default implementation-defined, with a maximum)
- or alternative `limit` / `offset` pairs, depending on the endpoint

Implementations MAY offer both `page/page_size` and `limit/offset`. The recommended default for v1 is `limit/offset` for new endpoints:

- `limit` – Maximum number of items to return (default: 50, max: 500)
- `offset` – Zero-based index into the result set (default: 0)

### Paginated Response Format

Praetor uses a uniform pagination envelope:

```json
{
  "items": [
    // resource objects
  ],
  "total": 99,
  "limit": 50,
  "offset": 0
}
```

Implementations MAY also support a compatibility mode with AWX-style pagination (`count`, `next`, `previous`, `results`) if needed, but `items/total/limit/offset` is the canonical v1 shape.

---

## Filtering, Searching, and Sorting

Praetor adopts AWX-style filtering conventions.

### Sorting

Use `order_by` query parameter:

```text
?order_by=name
?order_by=-name           # Descending
?order_by=name,created_at # Multiple fields, comma-separated
```

### Basic Search

Use `search`:

```text
?search=web
```

This performs case-insensitive search across resource-defined search fields.

Multiple search terms:

- OR logic: `?search=foo&search=bar`
- AND logic: `?search=foo,bar`

### Field Filtering

Field lookups use `field__lookup=value` syntax:

```text
?name__icontains=prod
?created_at__gte=2025-01-01T00:00:00Z
?created_at__lt=2026-01-01T00:00:00Z
```

Common lookups:

- `exact`, `iexact`
- `contains`, `icontains`
- `startswith`, `istartswith`
- `endswith`, `iendswith`
- `gt`, `gte`, `lt`, `lte`
- `isnull` (boolean)
- `in` (comma-separated list)

Related field filtering follows Django-style notation:

```text
?organization__name__icontains=default
```

Implementations SHOULD document per-resource filterable fields in an OpenAPI schema.

---

## Response Formats

### Standard Resource Fields

Most resources include:

- `id` – Primary key (integer or UUID)
- `url` – Canonical URL of the resource
- `created_at` – Creation timestamp
- `modified_at` – Last modification timestamp
- `name` – Human-readable name (where applicable)
- `description` – Optional description
- `summary` or `summary_fields` – Compact representation of key relations

### Content Types

Supported content types:

- `application/json` – Default
- `text/plain` – Used by some log/stream endpoints
- Optional: a human-friendly HTML renderer for development/debugging

Clients SHOULD send `Accept: application/json` unless otherwise required.

---

## Permissions and RBAC

Praetor uses Role-Based Access Control (RBAC).

### Permission Model

Conceptually, permissions operate at several levels:

- **System-level roles** – e.g. `system_admin`, `system_auditor`.
- **Organization-level roles** – e.g. `org_admin`, `org_member`.
- **Resource-level permissions** – e.g. `jobs:read`, `jobs:write`, `templates:read`, `inventories:write`.

All collection endpoints MUST automatically filter based on the calling user’s permissions.

### Error Semantics

- Anonymous or unauthenticated users receive `401 Unauthorized`.
- Authenticated users without sufficient permissions receive `403 Forbidden`.
- Superusers MAY bypass some checks depending on configuration.

The exact mapping between roles and permissions is deployment-specific and out of scope for this document, but the API MUST behave consistently with these semantics.

---

## Core Execution Resources

Praetor’s execution model is based on **Jobs**, **Job Runs**, **Events**, and **Logs**, backed by a resilient execution plane that can tolerate control-plane DB and event bus outages.

### 10.1 Job (Logical Job)

Represents a high-level unit of work (playbook run, workflow, project update, etc.).

**Resource URL**: `/api/v1/jobs/`

#### Job Representation

```json
{
  "id": 123,
  "name": "Deploy web tier",
  "type": "job",                     // job | workflow_job | project_update | inventory_update | system_job | ad_hoc_command
  "job_template_id": 17,
  "organization_id": 1,
  "status": "running",               // pending | queued | running | successful | failed | canceled | error
  "created_at": "2025-12-10T09:00:00Z",
  "started_at": "2025-12-10T09:01:10Z",
  "finished_at": null,
  "timeout_seconds": 7200,
  "cancel_requested": false,
  "inventory_id": 42,
  "project_id": 7,
  "execution_environment_id": 5,
  "labels": ["deploy", "web"],
  "current_run_id": "c0c3bdc8-08c8-49aa-aea3-51f2d036e9f1",
  "summary": {
    "hosts_total": 120,
    "hosts_ok": 115,
    "hosts_failed": 3,
    "hosts_unreachable": 2,
    "hosts_skipped": 0,
    "hosts_changed": 110
  }
}
```

#### Create Job

**POST** `/api/v1/jobs`

Request body:

```json
{
  "job_template_id": 17,
  "execution_environment_id": 5,    // optional override, defaults from template
  "inventory_id": 42,               // optional override, defaults from template
  "extra_vars": {
    "version": "1.2.3",
    "env": "prod"
  },
  "limit": "web*&!web-drain",
  "timeout_seconds": 7200,
  "labels": ["deploy", "web"]
}
```

Notes:

- `extra_vars` MUST be a JSON object.
- If `inventory_id` is omitted, template’s default inventory is used.
- If `execution_environment_id` is omitted, template’s execution environment is used.

Response `201 Created`:

```json
{
  "id": 123,
  "status": "pending",
  "job_template_id": 17,
  "inventory_id": 42,
  "created_at": "2025-12-10T09:00:00Z"
}
```

#### List Jobs

**GET** `/api/v1/jobs`

Common query parameters:

- `status`
- `template_id`
- `inventory_id`
- `project_id`
- `label`
- `created_before`, `created_after`
- `limit`, `offset`

Response:

```json
{
  "items": [
    { "id": 123, "name": "Deploy web tier", "status": "running", "job_template_id": 17, "created_at": "..." },
    { "id": 124, "name": "Sync project", "status": "successful", "job_template_id": 8, "created_at": "..." }
  ],
  "total": 137,
  "limit": 50,
  "offset": 0
}
```

#### Retrieve Job

**GET** `/api/v1/jobs/{job_id}`

Returns the full Job representation.

#### Cancel Job

**POST** `/api/v1/jobs/{job_id}/cancel`

Request body (optional):

```json
{
  "reason": "User requested cancel from UI"
}
```

Response `202 Accepted` (cancel in progress):

```json
{
  "id": 123,
  "status": "running",
  "cancel_requested": true
}
```

If the job is already in a terminal state, server SHOULD return `409 Conflict`.

#### Relaunch Job

**POST** `/api/v1/jobs/{job_id}/relaunch`

Creates a new job based on the given job’s parameters (template, inventory, vars).

Request:

```json
{
  "reuse": "template_and_vars",    // future-proof enum
  "override_extra_vars": {
    "env": "staging"
  }
}
```

Response `201 Created`: a new Job object.

---

### 10.2 Job Run (Physical Execution Attempt)

A Job can have multiple Job Runs (retries, reruns). Job Runs are what executor Pods bind to.

**Resource URL**: `/api/v1/runs/`

#### Job Run Representation

```json
{
  "id": "c0c3bdc8-08c8-49aa-aea3-51f2d036e9f1",
  "job_id": 123,
  "attempt_number": 1,
  "state": "running",               // pending | starting | running | successful | failed | canceled | lost
  "created_at": "2025-12-10T09:00:10Z",
  "started_at": "2025-12-10T09:01:10Z",
  "finished_at": null,
  "last_heartbeat_at": "2025-12-10T09:05:05Z",
  "last_event_seq": 842,
  "exit_code": null,
  "error_reason": null
}
```

#### List Job Runs for a Job

**GET** `/api/v1/jobs/{job_id}/runs`

Response:

```json
{
  "items": [
    {
      "id": "c0c3bdc8-...",
      "job_id": 123,
      "attempt_number": 1,
      "state": "failed",
      "started_at": "2025-12-10T09:01:10Z",
      "finished_at": "2025-12-10T09:10:00Z"
    },
    {
      "id": "bf31c6a1-...",
      "job_id": 123,
      "attempt_number": 2,
      "state": "running",
      "started_at": "2025-12-10T09:11:10Z",
      "finished_at": null
    }
  ],
  "total": 2,
  "limit": 50,
  "offset": 0
}
```

#### Retrieve Job Run

**GET** `/api/v1/runs/{run_id}`

Returns the full Job Run representation.

---

### 10.3 Job Events

Job Events describe play, task, and host-level activity produced during execution.

**Resource URL**: `/api/v1/job-events/`  
**Job-scoped Subresource**: `/api/v1/jobs/{job_id}/events`

#### Job Event Representation

```json
{
  "id": 1001,
  "job_id": 123,
  "run_id": "c0c3bdc8-08c8-49aa-aea3-51f2d036e9f1",
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

#### List Job Events for a Job

**GET** `/api/v1/jobs/{job_id}/events`

Query parameters:

- `from_seq` – Only events with `seq >= from_seq`
- `to_seq` – Only events with `seq <= to_seq`
- `event_type`
- `host`
- `run_id`
- `limit`, `offset`

Response:

```json
{
  "items": [
    {
      "id": 1001,
      "job_id": 123,
      "run_id": "c0c3bdc8-...",
      "seq": 42,
      "timestamp": "2025-12-10T09:03:11.123Z",
      "event_type": "TASK_OK",
      "host": "web01.example.com",
      "task_name": "Install nginx",
      "play_name": "Configure web servers",
      "stdout_snippet": "changed: [web01.example.com]",
      "data": { "rc": 0, "changed": true }
    }
  ],
  "total": 1000,
  "limit": 100,
  "offset": 0
}
```

Implementations MAY provide an additional streaming endpoint (SSE/WebSocket) for live events.

---

### 10.4 Job Host Summaries

Aggregated per-host results for a job.

**Resource URL**: `/api/v1/job-host-summaries/`  
**Job-scoped Subresource**: `/api/v1/jobs/{job_id}/hosts`

#### Host Summary Representation

```json
{
  "job_id": 123,
  "host": "web01.example.com",
  "host_id": 555,
  "ok": 10,
  "changed": 2,
  "failed": 0,
  "skipped": 0,
  "unreachable": 0,
  "last_event_at": "2025-12-10T09:05:00Z"
}
```

#### List Host Summaries for a Job

**GET** `/api/v1/jobs/{job_id}/hosts`

Query parameters:

- `status` – Derived status filter (e.g. `failed`, `unreachable`, `ok`).
- `limit`, `offset`.

---

### 10.5 Logs

Praetor stores logs in object storage, indexed by Job Run and sequence.

#### Aggregated Job Logs

**GET** `/api/v1/jobs/{job_id}/logs`

Query parameters:

- `host` – Optional host filter
- `follow` – If `true`, server MAY stream new log lines until client disconnects

Response (JSON example):

```json
{
  "lines": [
    "PLAY [Configure web servers] *********************************************************",
    "TASK [Install nginx] *****************************************************************",
    "changed: [web01.example.com]"
  ]
}
```

Servers MAY also support `text/plain` streaming for `follow=true`.

#### Log Chunk Index

**GET** `/api/v1/runs/{run_id}/log-chunks`

Response:

```json
{
  "items": [
    {
      "run_id": "c0c3bdc8-08c8-49aa-aea3-51f2d036e9f1",
      "seq": 0,
      "storage_key": "jobs/c0c3bdc8-08c8-49aa-aea3-51f2d036e9f1/chunk-0000.log",
      "byte_length": 1024,
      "created_at": "2025-12-10T09:04:00Z"
    }
  ],
  "total": 20,
  "limit": 50,
  "offset": 0
}
```

---

### 10.6 Internal Execution Manifest (Executors)

This is **not** a public endpoint; it is used by executor Pods inside the cluster.

**GET** `/internal/api/v1/runs/{run_id}/manifest`

Response example:

```json
{
  "run_id": "c0c3bdc8-08c8-49aa-aea3-51f2d036e9f1",
  "job_id": 123,
  "attempt_number": 1,
  "execution_environment": {
    "id": 5,
    "image": "registry.example.com/ee:latest"
  },
  "inventory": {
    "mode": "inline",
    "content": "[web]\nweb01.example.com\nweb02.example.com\n",
    "path": null
  },
  "playbook": {
    "type": "scm",
    "project_id": 7,
    "playbook_path": "playbooks/deploy.yml"
  },
  "extra_vars": {
    "version": "1.2.3",
    "env": "prod"
  },
  "credentials": [
    { "id": 100, "type": "machine", "name": "SSH Key" },
    { "id": 101, "type": "vault", "name": "Vault Password" }
  ],
  "timeout_seconds": 7200,
  "limit": "web*&!web-drain",
  "verbosity": 1
}
```

Executors:

- Fetch this manifest once at start-up.
- Do NOT need DB access; they rely on event bus + object storage for runtime I/O.

---

## Control-Plane Resource Endpoints

Praetor exposes a control-plane surface inspired by AWX. Only a high-level summary is given here; detailed schemas should be defined in the OpenAPI spec.

### Organizations

**Base URL**: `/api/v1/organizations/`

Typical endpoints:

- `GET /api/v1/organizations/` – List organizations
- `POST /api/v1/organizations/` – Create organization
- `GET /api/v1/organizations/{id}/` – Organization detail
- `PATCH /api/v1/organizations/{id}/` – Update organization
- `DELETE /api/v1/organizations/{id}/` – Delete organization (subject to constraints)
- Subresources (examples):
  - `/api/v1/organizations/{id}/users/`
  - `/api/v1/organizations/{id}/projects/`
  - `/api/v1/organizations/{id}/job-templates/`
  - `/api/v1/organizations/{id}/execution-environments/`

### Users and Teams

**Users**: `/api/v1/users/`  
**Teams**: `/api/v1/teams/`

Typical operations: list, create, retrieve, update, delete, list roles, list memberships.

### Projects

**Base URL**: `/api/v1/projects/`

- `GET /api/v1/projects/`
- `POST /api/v1/projects/`
- `GET /api/v1/projects/{id}/`
- `PATCH /api/v1/projects/{id}/`
- `DELETE /api/v1/projects/{id}/`
- `POST /api/v1/projects/{id}/sync/` – Start project sync
- `GET /api/v1/projects/{id}/playbooks/` – Discover playbooks

### Execution Environments

**Base URL**: `/api/v1/execution-environments/`

Execution Environment representation:

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

Typical endpoints:

- `GET /api/v1/execution-environments/`
- `POST /api/v1/execution-environments/`
- `GET /api/v1/execution-environments/{id}/`
- `PATCH /api/v1/execution-environments/{id}/`
- `DELETE /api/v1/execution-environments/{id}/`

### Credentials, Inventories, Hosts, Groups, Job Templates, Workflows, Schedules, Notifications, Labels, Roles, Activity Stream

Praetor SHOULD expose these resource families with endpoints and semantics broadly compatible with AWX, but under `/api/v1/` and with Praetor-specific shapes where necessary.

Implementations MAY:

- Reuse AWX-style URL patterns (e.g. `/api/v1/job-templates/{id}/launch/`).
- Adjust request/response payloads to reflect Praetor’s internal data model.
- Document each resource in the OpenAPI schema rather than fully in this reference.

---

## Special Features

Praetor MAY implement a subset of AWX-style special features:

- Copy resources (e.g. job templates, projects).
- Launch actions (`/launch/` on job templates and workflows).
- Cancel actions (`/cancel/` on jobs, project updates, inventory updates, etc.).
- Relaunch actions (`/relaunch/` on jobs, ad hoc commands, workflows).
- Variable data endpoints (`/variable_data/` on inventories, hosts, groups).
- Schedule preview endpoints for RRULE-based schedules.

Exact coverage is implementation-dependent and SHOULD be reflected in the OpenAPI schema.

---

## Error Handling

### HTTP Status Codes

Praetor adheres to standard REST semantics:

- `200 OK` – Success
- `201 Created` – Resource created
- `202 Accepted` – Action accepted (async)
- `204 No Content` – Success, no body
- `400 Bad Request` – Invalid request payload or parameters
- `401 Unauthorized` – Missing/invalid authentication
- `403 Forbidden` – Permission denied
- `404 Not Found` – Resource not found
- `405 Method Not Allowed` – Unsupported HTTP method
- `409 Conflict` – Resource conflict (e.g. job already finished)
- `415 Unsupported Media Type` – Unsupported `Content-Type`
- `500 Internal Server Error` – Unexpected error

### Error Response Format

Praetor uses a structured error envelope:

```json
{
  "error": {
    "code": "validation_error",
    "message": "extra_vars is not valid JSON",
    "details": {
      "field": "extra_vars",
      "reason": "Expected object"
    }
  }
}
```

Validation errors MAY also be returned using field-based maps when appropriate.

---

## URL Patterns

Praetor follows predictable URL patterns for collections, details, actions, and subresources. Typical patterns:

```text
# Collection
GET    /api/v1/resources/
POST   /api/v1/resources/

# Detail
GET    /api/v1/resources/{id}/
PATCH  /api/v1/resources/{id}/
DELETE /api/v1/resources/{id}/

# Action
POST   /api/v1/resources/{id}/action-name/

# Subresource
GET    /api/v1/resources/{id}/related-resource/
```

Identifier formats:

- Most resources use integer IDs.
- Some (Job Runs, etc.) use UUIDs.

Exact ID formats SHOULD be documented per resource in the OpenAPI schema.

---

## References

- Praetor Architecture and Execution Model (internal doc)
- Praetor Backend API v1 (detailed spec)
- Kubernetes API Reference
- OpenAPI/Swagger documentation generated from Praetor implementation

---

*This document defines the high-level REST contracts for Praetor v1. For full, machine-readable detail, consult the OpenAPI schema exposed by the running control plane instance.*
