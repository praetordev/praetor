# Product validation fixture

The fixture adds synthetic LDAP identities, a local notification receiver, and
a disposable SSH managed host to the integrated Praetor and Secrets Service
development namespace. It never deletes the namespace, databases, persistent
volumes, or Secrets Service keys.

```sh
PRAETOR_SECRETS_ROOT=../praetor-secrets ./scripts/bootstrap-product-validation-base.sh
./scripts/product-validation-fixture.sh create
./scripts/product-validation-fixture.sh status
make validation-ldap-operator-journey
./scripts/product-validation-fixture.sh cleanup
```

The bootstrap command is for a clean k3d cluster. It generates ephemeral PKI
and master keys with the Secrets Service's development bootstrap, deploys two
isolated PostgreSQL instances, installs the Secrets Service and audit sink, and
then installs the released Praetor component set. CI runs the complete lifecycle
from a fresh cluster.

Creation is idempotent: ConfigMaps and workloads use stable names and Helm
reuses the installed release values. The fixture generates an ephemeral
Ed25519 identity in a namespace-scoped Secret, builds a managed-host image from
the candidate executor's pinned Python/Ansible runtime, imports it into the
dedicated k3d cluster, and stores the matching private key through Praetor's
Machine credential API. This makes the governed journey exercise Secrets
Service resolution and remote runner bootstrap rather than simulating execution
with `ansible_connection: local`. When cleanup rotates that disposable host
identity, it removes only the fixture hostname from the executor's persistent
`known_hosts`; host-key verification remains enabled. Cleanup selects only resources labelled
`app.kubernetes.io/part-of=praetor-validation-fixture` and disables the
fixture-owned LDAP mount. API resources are deleted in dependency order by
their reserved `Praetor Validation` names; LDAP-mapped identities and all
unrelated platform data remain intact. All identities and passwords are synthetic.

The LDAP operator journey signs in four synthetic identities through the public
API and verifies the complete governed launch boundary. It checks LDAP
organization/team mapping; server-filtered inventory and credential choices;
saved defaults, structured survey answers and host limits; preview resolution;
launch-time reauthorization after a stale preview; immutable execution inputs;
exact-host execution; governed relaunch; and assigned-team approval followed by
execution. The negative matrix covers cross-team, cross-organization, same-org
resources without `Use`, malformed scope-expanding limits, missing survey
answers, and requester self-approval. Auditor-visible evidence must attribute
preview, launch and approval to the correct synthetic actors without containing
the disposable password-survey answer.

Run it twice against the same fixture to verify rerun behavior, then prove
fixture-owned resource cleanup and recreation:

```sh
make validation-ldap-operator-journey
make validation-ldap-operator-journey
./scripts/product-validation-fixture.sh cleanup
! ./scripts/product-validation-fixture.sh status
./scripts/product-validation-fixture.sh create
```

Its final output is sanitized JSON containing only job/workflow IDs, terminal
status, and synthetic actor/team names.
Set `PRAETOR_LDAP_EVIDENCE_FILE` to retain that sanitized JSON for readiness
aggregation.

## Execution recovery lifecycle

The recovery gate uses the same synthetic identities and notification receiver,
but runs a real checkpointed playbook through the scheduler, executor, host
runner, ingestion, consumer, and Secrets Service:

```sh
make validation-execution-recovery
```

It interrupts ingestion, restarts scheduler and consumer, and deletes the
executor pod while the play is paused. The executor's persistent WAL and
checkpoint must resume the original run without repeating its completed side
effect or resolving its credential again. A second run has its WAL deliberately
removed and must become clearly `lost`/`error`; a subsequent relaunch must create
new run IDs while retaining the initiating user and approval-team boundary.
Approval and terminal webhooks, terminal events, activity-stream actors, and
credential resolution counts are asserted exactly once.
Set `PRAETOR_RECOVERY_EVIDENCE_FILE` to retain the sanitized recovery result.

## Credential execution lifecycle

With an architecture-matched Execution Pack in `build/runtime`, the live secrets
gate exercises the deployed API, scheduler, executor, ingestion service, and
Secrets Service rather than mocks:

```sh
make secrets-execution-e2e
```

It plants a random canary as a Machine credential password, verifies that
Praetor stores only the Secrets Service reference and a masking placeholder,
executes the playbook, and proves that the run resolves its credential exactly
once. It then checks cross-team metadata denial, wrong-workload resolution,
completed-run replay, explicit cancellation, expiry, and credential retirement.
Finally, it scans captured API responses, activity and audit data, database
dumps, terminal executor manifests, and workload logs for the canary. Evidence
output contains IDs and terminal status only; it never contains credential
material.
Set `PRAETOR_E2E_EVIDENCE_FILE` to retain that sanitized result for readiness
aggregation.

## Delegated API lifecycle

The delegated API evidence runner requires `TEST_DATABASE_URL` to reference an
isolated, migrated Praetor database. It executes every delegated workflow launch
scope test and fails if any are skipped:

```sh
PRAETOR_DELEGATED_EVIDENCE_FILE=build/readiness-evidence/delegated-api.json \
TEST_DATABASE_URL="$TEST_DATABASE_URL" \
./scripts/validate-delegated-api-e2e.sh
```

## Delegated service-principal staging fixture

The persistent staging fixture automates the pre-release procedure for a
bounded application identity. It reuses the synthetic pilot workflow,
Engineering inventory, managed host, and backend approval team created by
`make staging-pilot-journey-seed`, but owns its principal, credentials, grant,
and Kubernetes Secret independently.

Always inspect the non-secret plan before mutating staging:

```sh
make delegated-fixture-plan
make staging-pilot-journey-seed
make delegated-fixture-setup
make delegated-fixture-validate
make delegated-fixture-cleanup
```

`setup` is resumable and intentionally rotates the single active credential on
every invocation. The one-time plaintext returned by the API is piped directly
into the namespace-scoped `praetor-delegated-staging-fixture` Secret; it is not
printed, committed, or written to an evidence artifact. The grant is restricted
to one named workflow, inventory, enabled host, host-count maximum, extra-
variable key, backend approval team, and bounded expiry.

The complete pre-release rehearsal proves partial-setup recovery, repeated
setup without duplicate active credentials or grants, idempotent launch replay,
assigned-team approval, successful completion, and fixture-owned cleanup:

```sh
make delegated-fixture-rehearse
```

Cleanup revokes every active fixture credential and grant, disables only the
named synthetic principal, and deletes the Secret only when its ownership label
matches. Shared product-validation resources are preserved.
