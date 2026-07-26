#!/usr/bin/env bash
set -euo pipefail

NAMESPACE="${PRAETOR_VALIDATION_NAMESPACE:-praetor-secrets}"
RELEASE="${PRAETOR_HELM_RELEASE:-praetor}"
API_PORT="${PRAETOR_VALIDATION_API_PORT:-18081}"
API="http://127.0.0.1:$API_PORT/api/v1"
PASSWORD="${PRAETOR_VALIDATION_LDAP_PASSWORD:-praetor123}"
PORT_FORWARD_PID=""
PORT_FORWARD_LOG=""
PHASE="preflight"

die() { echo "error: $*" >&2; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || die "required command '$1' is not installed"; }
for command in curl jq kubectl; do need "$command"; done

cleanup() {
  [[ -z "$PORT_FORWARD_PID" ]] || kill "$PORT_FORWARD_PID" 2>/dev/null || true
  [[ -z "$PORT_FORWARD_LOG" ]] || rm -f "$PORT_FORWARD_LOG"
}
diagnostics() {
  echo "==> Governed LDAP journey failed during phase: $PHASE" >&2
  kubectl get pods -n "$NAMESPACE" -o wide >&2 || true
  kubectl get events -n "$NAMESPACE" --sort-by=.lastTimestamp 2>/dev/null | tail -n 40 >&2 || true
  for workload in "deployment/$RELEASE-api" deployment/praetor-scheduler statefulset/praetor-executor; do
    echo "==> $workload" >&2
    kubectl logs -n "$NAMESPACE" "$workload" --all-containers --tail=80 >&2 || true
  done
}
on_exit() {
  local status=$?
  cleanup
  (( status == 0 )) || diagnostics
  exit "$status"
}
trap on_exit EXIT

PORT_FORWARD_LOG="$(mktemp "${TMPDIR:-/tmp}/praetor-ldap-journey.XXXXXX")"
kubectl port-forward -n "$NAMESPACE" "svc/$RELEASE-api" "$API_PORT:8080" >"$PORT_FORWARD_LOG" 2>&1 &
PORT_FORWARD_PID=$!
for _ in $(seq 1 30); do
  curl -fsS "$API/ping" >/dev/null 2>&1 && break
  kill -0 "$PORT_FORWARD_PID" 2>/dev/null || { cat "$PORT_FORWARD_LOG" >&2; die "API tunnel stopped"; }
  sleep 1
done
curl -fsS "$API/ping" >/dev/null 2>&1 || die "API did not become reachable"

login() {
  local username="$1" password="${2:-$PASSWORD}"
  curl -fsS -H 'Content-Type: application/json' \
    -d "$(jq -nc --arg username "$username" --arg password "$password" '{username:$username,password:$password}')" \
    "$API/auth/login"
}
get() { curl -fsS -H "Authorization: Bearer $1" "$API/$2"; }
request_status() {
  local method="$1" token="$2" path="$3" body="${4:-}" output
  local -a args
  output="$(mktemp "${TMPDIR:-/tmp}/praetor-ldap-response.XXXXXX")"
  args=(-sS -o "$output" -w '%{http_code}' -X "$method"
    -H "Authorization: Bearer $token" -H 'Content-Type: application/json')
  [[ -z "$body" ]] || args+=(-d "$body")
  REQUEST_STATUS="$(curl "${args[@]}" "$API/$path")"
  RESPONSE="$(cat "$output")"
  rm -f "$output"
}
get_status() {
  request_status GET "$1" "$2"
}
post_status() {
  request_status POST "$1" "$2" "${3:-}"
}
delete_status() {
  request_status DELETE "$1" "$2" "${3:-}"
}
items() {
  jq -c 'if type == "object" and has("items") then .items else . end'
}
find_named_id() {
  get "$1" "$2" | items | jq -r --arg name "$3" '.[] | select(.name == $name) | .id' | head -n1
}
wait_job() {
  local token="$1" job_id="$2" expected="$3" jobs state=""
  for _ in $(seq 1 180); do
    jobs="$(get "$token" jobs)"
    state="$(jq -r --argjson id "$job_id" '.[] | select(.id == $id) | .status' <<<"$jobs")"
    [[ "$state" =~ ^(successful|failed|error|canceled)$ ]] && break
    sleep 1
  done
  [[ "$state" == "$expected" ]] || die "job $job_id finished '$state', expected '$expected'"
  jq -c --argjson id "$job_id" '.[] | select(.id == $id)' <<<"$jobs"
}
wait_run_logs() {
  local token="$1" run_id="$2" expected="$3" logs=""
  for _ in $(seq 1 30); do
    logs="$(get "$token" "jobs/runs/$run_id/logs")"
    [[ "$logs" == *"$expected"* ]] && {
      printf '%s' "$logs"
      return
    }
    sleep 1
  done
  die "run $run_id logs did not contain '$expected'"
}
wait_notification() {
  local job_id="$1" event="$2" messages count
  for _ in $(seq 1 60); do
    messages="$(kubectl logs -n "$NAMESPACE" deployment/praetor-validation-notification-sink --since=15m 2>/dev/null |
      jq -Rsc --argjson job "$job_id" --arg event "$event" '[split("\n")[] | fromjson? | select(.job_id == $job and .event == $event)]')"
    count="$(jq 'length' <<<"$messages")"
    (( count > 1 )) && die "workflow notification $event for job $job_id was delivered $count times"
    if (( count == 1 )); then
      jq -c '.[0]' <<<"$messages"
      return
    fi
    sleep 1
  done
  die "workflow notification $event for job $job_id was not delivered"
}
assert_user() {
  local json="$1" username="$2" auditor="$3"
  jq -e --arg username "$username" --argjson auditor "$auditor" \
    '.user.username == $username and .user.is_active == true and .user.is_superuser == false and .user.is_system_auditor == $auditor' \
    <<<"$json" >/dev/null || die "unexpected LDAP identity for $username"
}
assert_mapping() {
  local token="$1" user_id="$2" expected_team="$3"
  get "$token" "users/$user_id/organizations" | jq -e '[.[] | select(.name == "Engineering")] | length == 1' >/dev/null || die "Engineering membership is missing"
  get "$token" "users/$user_id/teams" | jq -e --arg team "$expected_team" '[.[] | select(.name == $team)] | length == 1' >/dev/null || die "$expected_team membership is missing"
}
assert_preview_denied() {
  local token="$1" template_id="$2" body="$3" label="$4"
  post_status "$token" "job-templates/$template_id/launch-preview" "$body"
  [[ "$REQUEST_STATUS" == 403 ]] || die "$label returned $REQUEST_STATUS, expected 403"
  if grep -Fq "Praetor Validation" <<<"$RESPONSE"; then
    die "$label revealed an inaccessible resource name"
  fi
}

PHASE="ldap-login-and-mapping"
operator_json="$(login demo-operator)"; assert_user "$operator_json" demo-operator false
approver_json="$(login mwebb)"; assert_user "$approver_json" mwebb false
outsider_json="$(login fwalsh)"; assert_user "$outsider_json" fwalsh false
auditor_json="$(login demo-auditor)"; assert_user "$auditor_json" demo-auditor true
admin_json="$(login "${PRAETOR_VALIDATION_ADMIN_USERNAME:-admin}" "${PRAETOR_VALIDATION_ADMIN_PASSWORD:-admin}")"

operator_token="$(jq -er .token <<<"$operator_json")"; operator_id="$(jq -er .user.id <<<"$operator_json")"
approver_token="$(jq -er .token <<<"$approver_json")"; approver_id="$(jq -er .user.id <<<"$approver_json")"
outsider_token="$(jq -er .token <<<"$outsider_json")"; outsider_id="$(jq -er .user.id <<<"$outsider_json")"
auditor_token="$(jq -er .token <<<"$auditor_json")"
admin_token="$(jq -er .token <<<"$admin_json")"
assert_mapping "$operator_token" "$operator_id" backend-team
assert_mapping "$approver_token" "$approver_id" backend-team
assert_mapping "$outsider_token" "$outsider_id" frontend-team

PHASE="governed-resource-discovery"
team_id="$(get "$operator_token" teams | jq -er '(if type == "object" then .items else . end)[] | select(.name == "backend-team") | .id')"
workflow_id="$(get "$operator_token" workflow-templates | jq -er '(if type == "object" then .items else . end)[] | select(.name == "Praetor Validation LDAP Workflow") | .id')"
inventory_id="$(get "$operator_token" inventories | jq -er '(if type == "object" then .items else . end)[] | select(.name == "Praetor Validation Inventory") | .id')" || die "operator cannot use fixture inventory"
prompt_inventory_id="$(find_named_id "$operator_token" inventories "Praetor Validation Prompt Inventory")"
[[ -n "$prompt_inventory_id" ]] || die "operator cannot discover the authorized prompt inventory"
prompt_host_id="$(find_named_id "$operator_token" "inventories/$prompt_inventory_id/hosts/" "praetor-validation-prompt-host")"
[[ -n "$prompt_host_id" ]] || die "operator cannot discover the authorized prompt host"
excluded_host_id="$(find_named_id "$operator_token" "inventories/$prompt_inventory_id/hosts/" "praetor-validation-excluded-host")"
[[ -n "$excluded_host_id" ]] || die "operator cannot discover the second prompt inventory host"
template_id="$(find_named_id "$operator_token" job-templates/ "Praetor Validation Job")"
[[ -n "$template_id" ]] || die "operator cannot discover the executable job template"
template="$(get "$operator_token" "job-templates/$template_id")"
unified_template_id="$(jq -er .unified_job_template_id <<<"$template")"
prompt_credential_id="$(find_named_id "$operator_token" credentials "Praetor Validation Prompt Credential")"
[[ -n "$prompt_credential_id" ]] || die "operator cannot discover the authorized prompt credential"
default_credential_id="$(find_named_id "$operator_token" credentials "Praetor Validation Default Credential")"
[[ -n "$default_credential_id" ]] || die "operator cannot discover the authorized default credential"
hidden_inventory_id="$(find_named_id "$admin_token" inventories "Praetor Validation Hidden Inventory")"
foreign_inventory_id="$(find_named_id "$admin_token" inventories "Praetor Validation Foreign Inventory")"
hidden_credential_id="$(find_named_id "$admin_token" credentials "Praetor Validation Hidden Credential")"
foreign_credential_id="$(find_named_id "$admin_token" credentials "Praetor Validation Foreign Credential")"
for required_id in "$hidden_inventory_id" "$foreign_inventory_id" "$hidden_credential_id" "$foreign_credential_id"; do
  [[ -n "$required_id" ]] || die "a negative authorization fixture resource is missing"
done

get "$operator_token" "inventories/$inventory_id/hosts/" | jq -e '(if type == "object" then .items else . end)[] | select(.name == "praetor-validation-host" and .enabled == true)' >/dev/null || die "operator cannot access fixture host"
get "$outsider_token" inventories | jq -e --argjson inventory "$inventory_id" '[(if type == "object" then .items else . end)[] | select(.id == $inventory)] | length == 0' >/dev/null || die "another team can list the fixture inventory"
get_status "$outsider_token" "inventories/$inventory_id"; status="$REQUEST_STATUS"
[[ "$status" == 403 ]] || die "another team inventory access returned $status, expected 403"
get_status "$outsider_token" "inventories/$inventory_id/hosts/"; status="$REQUEST_STATUS"
[[ "$status" == 403 ]] || die "another team host access returned $status, expected 403"

configuration="$(get "$operator_token" "job-templates/$template_id/launch-configuration")"
jq -e '
  .prompts == {inventory:true,credential:true,variables:false,limit:true,survey:true}
  and .defaults.inventory_id != null
  and .defaults.credential_id != null
  and .defaults.extra_vars.fixture_source == "saved-default"
  and .defaults.limit == "praetor-validation-host"
  and ([.survey_spec.spec[].variable] | sort) == ["approval_secret","change_ticket","deployment_ring"]
' <<<"$configuration" >/dev/null || die "launch configuration does not describe the governed prompts and saved defaults"
jq -e --argjson default "$inventory_id" --argjson prompted "$prompt_inventory_id" '
  [.inventories[].id] | sort == ([$default,$prompted] | sort)
' <<<"$configuration" >/dev/null || die "launch inventory choices are not restricted to the two authorized inventories"
jq -e --argjson default "$default_credential_id" --argjson prompted "$prompt_credential_id" '
  [.credentials[].id] | sort == ([$default,$prompted] | sort)
' <<<"$configuration" >/dev/null || die "launch credential choices are not restricted to the two authorized machine credentials"
jq -e '
  [.credentials[] | has("inputs") or has("secrets_service_id") or has("secrets_service_version")]
  | all(. == false)
' <<<"$configuration" >/dev/null || die "launch credential choices exposed credential inputs or Secrets Service references"
for forbidden in synthetic-validation-password "Praetor Validation Hidden" "Praetor Validation Foreign"; do
  grep -Fiq "$forbidden" <<<"$configuration" && die "launch configuration exposed forbidden material '$forbidden'"
done
get_status "$outsider_token" "job-templates/$template_id/launch-configuration"
[[ "$REQUEST_STATUS" == 403 ]] || die "another team launch configuration returned $REQUEST_STATUS, expected 403"

PHASE="governed-preview-and-negative-rbac"
survey_secret="disposable-survey-secret"
launch_prompts="$(jq -nc \
  --argjson inventory "$prompt_inventory_id" \
  --argjson credential "$prompt_credential_id" \
  --arg limit "praetor-validation-prompt-host" \
  --arg survey_secret "$survey_secret" \
  '{inventory_id:$inventory,credential_id:$credential,limit:$limit,extra_vars:{change_ticket:"CHG-378",deployment_ring:"canary",approval_secret:$survey_secret}}')"
post_status "$operator_token" "job-templates/$template_id/launch-preview" "$launch_prompts"
[[ "$REQUEST_STATUS" == 200 ]] || die "authorized launch preview returned $REQUEST_STATUS: $RESPONSE"
preview="$RESPONSE"
jq -e --argjson inventory "$prompt_inventory_id" --argjson credential "$prompt_credential_id" '
  .inventory.id == $inventory
  and .credential.id == $credential
  and .inventory_host_count == 2
  and (.inventory_host_sample | sort) == ["praetor-validation-excluded-host","praetor-validation-prompt-host"]
  and .extra_vars.fixture_source == "saved-default"
  and .extra_vars.change_ticket == "CHG-378"
  and .extra_vars.deployment_ring == "canary"
  and .limit == "praetor-validation-prompt-host"
  and .limit_applied_at_execution == true
' <<<"$preview" >/dev/null || die "launch preview did not resolve the selected governed inputs"

post_status "$operator_token" "job-templates/$template_id/launch-preview" '{}'
[[ "$REQUEST_STATUS" == 400 ]] || die "missing required survey answers returned $REQUEST_STATUS, expected 400"
invalid_limit_body="$(jq -nc \
  --argjson inventory "$prompt_inventory_id" \
  --argjson credential "$prompt_credential_id" \
  --arg limit $'praetor-validation-prompt-host\nall' \
  '{inventory_id:$inventory,credential_id:$credential,limit:$limit,extra_vars:{change_ticket:"CHG-378",deployment_ring:"canary",approval_secret:"disposable"}}')"
post_status "$operator_token" "job-templates/$template_id/launch-preview" "$invalid_limit_body"
[[ "$REQUEST_STATUS" == 400 ]] || die "scope-expanding control characters in limit returned $REQUEST_STATUS, expected 400"
assert_preview_denied "$operator_token" "$template_id" "$(jq -nc --argjson id "$hidden_inventory_id" '{inventory_id:$id}')" "same-organization inventory without Use"
assert_preview_denied "$operator_token" "$template_id" "$(jq -nc --argjson id "$foreign_inventory_id" '{inventory_id:$id}')" "cross-organization inventory"
assert_preview_denied "$operator_token" "$template_id" "$(jq -nc --argjson id "$hidden_credential_id" '{credential_id:$id}')" "same-organization credential without Use"
assert_preview_denied "$operator_token" "$template_id" "$(jq -nc --argjson id "$foreign_credential_id" '{credential_id:$id}')" "cross-organization credential"

PHASE="stale-authorization-recheck"
inventory_use_role_id="$(get "$admin_token" 'role-definitions?content_type=inventory' | jq -er '.[] | select(.name == "Inventory Use") | .id' | head -n1)"
inventory_access="$(jq -nc --argjson object "$prompt_inventory_id" --argjson role "$inventory_use_role_id" --argjson team "$team_id" '{content_type:"inventory",object_id:$object,role_definition_id:$role,team_id:$team}')"
delete_status "$admin_token" access "$inventory_access"
[[ "$REQUEST_STATUS" == 204 ]] || die "revoking prompt inventory use returned $REQUEST_STATUS: $RESPONSE"
stale_launch="$(jq -c --argjson template "$unified_template_id" '. + {unified_job_template_id:$template,name:"stale-preview-must-not-launch"}' <<<"$launch_prompts")"
post_status "$operator_token" jobs "$stale_launch"
[[ "$REQUEST_STATUS" == 403 ]] || die "launch after inventory-use revocation returned $REQUEST_STATUS, expected 403"
post_status "$admin_token" access "$inventory_access"
[[ "$REQUEST_STATUS" == 204 ]] || die "restoring prompt inventory use returned $REQUEST_STATUS: $RESPONSE"

PHASE="prompted-execution"
direct_launch="$(jq -c --argjson template "$unified_template_id" '. + {unified_job_template_id:$template,name:"Praetor Validation Governed Prompt Launch"}' <<<"$launch_prompts")"
post_status "$operator_token" jobs "$direct_launch"
[[ "$REQUEST_STATUS" == 201 ]] || die "governed job launch returned $REQUEST_STATUS: $RESPONSE"
direct_job_id="$(jq -er .id <<<"$RESPONSE")"
direct_job="$(wait_job "$operator_token" "$direct_job_id" successful)"
direct_run_id="$(jq -er .current_run_id <<<"$direct_job")"
jq -e --argjson inventory "$prompt_inventory_id" --argjson credential "$prompt_credential_id" '
  .job_args.snapshot_version == 1
  and .job_args.inventory_id == $inventory
  and .job_args.credential_id == $credential
  and .job_args.prompted_inventory == true
  and .job_args.prompted_credential == true
  and .job_args.extra_vars.fixture_source == "saved-default"
  and .job_args.extra_vars.change_ticket == "CHG-378"
  and .job_args.prompted_extra_vars.deployment_ring == "canary"
  and .job_args.limit == "praetor-validation-prompt-host"
  and .job_args.prompted_limit == "praetor-validation-prompt-host"
  and (.job_args.secrets_credential_id | length) > 0
  and .job_args.secrets_credential_version > 0
' <<<"$direct_job" >/dev/null || die "completed job did not retain the immutable resolved and prompted launch inputs"
events="$(get "$operator_token" "jobs/runs/$direct_run_id/events")"
jq -e '
  ([.[] | select(.event_type == "HOST_OK")] | length) > 0
  and ([.[] | select(.event_type == "HOST_FAILED" or .event_type == "HOST_UNREACHABLE")] | length) == 0
' <<<"$events" >/dev/null || die "prompted execution did not emit successful host events"
direct_logs="$(wait_run_logs "$operator_token" "$direct_run_id" "praetor-validation-prompt-host")"
grep -Fq "praetor-validation-excluded-host" <<<"$direct_logs" && die "excluded host appeared in execution output despite the prompted limit"
grep -Fq "$survey_secret" <<<"$direct_logs" && die "execution output leaked a password survey answer"

PHASE="governed-relaunch"
post_status "$operator_token" jobs "$(jq -nc --argjson template "$unified_template_id" --argjson source "$direct_job_id" '{unified_job_template_id:$template,name:"Praetor Validation Governed Relaunch",relaunch_source_job_id:$source}')"
[[ "$REQUEST_STATUS" == 201 ]] || die "governed relaunch returned $REQUEST_STATUS: $RESPONSE"
relaunch_job_id="$(jq -er .id <<<"$RESPONSE")"
[[ "$relaunch_job_id" != "$direct_job_id" ]] || die "relaunch reused the source job identifier"
relaunch_job="$(wait_job "$operator_token" "$relaunch_job_id" successful)"
jq -e --argjson inventory "$prompt_inventory_id" --argjson credential "$prompt_credential_id" '
  .job_args.snapshot_version == 1
  and .job_args.inventory_id == $inventory
  and .job_args.credential_id == $credential
  and .job_args.prompted_inventory == true
  and .job_args.prompted_credential == true
  and .job_args.prompted_extra_vars.change_ticket == "CHG-378"
  and .job_args.prompted_limit == "praetor-validation-prompt-host"
' <<<"$relaunch_job" >/dev/null || die "relaunch did not re-resolve the original prompt answers"

PHASE="assigned-team-approval"
post_status "$operator_token" "workflow-templates/$workflow_id/launch" "$(jq -nc --argjson team "$team_id" '{approval_team_id:$team}')"
status="$REQUEST_STATUS"
[[ "$status" == 201 ]] || die "authorized workflow launch returned $status: $RESPONSE"
workflow_job_id="$(jq -er .workflow_job_id <<<"$RESPONSE")"
approval_notification="$(wait_notification "$workflow_job_id" approval)"
jq -e '.kind == "workflow approval" and .job_name == "Praetor Validation LDAP Workflow"' <<<"$approval_notification" >/dev/null || die "approval notification identity is incorrect"

approval_id=""
for _ in $(seq 1 60); do
  approvals="$(get "$approver_token" workflow-approvals)"
  approval_id="$(jq -r --argjson job "$workflow_job_id" '.[] | select(.workflow_job_id == $job) | .id' <<<"$approvals" | head -n1)"
  [[ -n "$approval_id" ]] && break
  sleep 1
done
[[ -n "$approval_id" ]] || die "assigned team did not receive the approval"
jq -e --argjson job "$workflow_job_id" --argjson team "$team_id" '.[] | select(.workflow_job_id == $job and .approval_team_id == $team and .requested_by == "demo-operator")' <<<"$approvals" >/dev/null || die "approval attribution is incorrect"
[[ "$(get "$operator_token" workflow-approvals | jq 'length')" == 0 ]] || die "requester can see their own approval"
[[ "$(get "$outsider_token" workflow-approvals | jq 'length')" == 0 ]] || die "another team can see the approval"

post_status "$outsider_token" "workflow-job-nodes/$approval_id/approve"; status="$REQUEST_STATUS"
[[ "$status" == 403 ]] || die "another team approval returned $status, expected 403"
post_status "$operator_token" "workflow-job-nodes/$approval_id/approve"; status="$REQUEST_STATUS"
[[ "$status" == 403 ]] || die "requester self-approval returned $status, expected 403"
post_status "$approver_token" "workflow-job-nodes/$approval_id/approve"; status="$REQUEST_STATUS"
[[ "$status" == 204 ]] || die "assigned-team approval returned $status: $RESPONSE"
approved_notification="$(wait_notification "$workflow_job_id" approved)"
jq -e '.kind == "workflow approval" and .job_name == "Praetor Validation LDAP Workflow"' <<<"$approved_notification" >/dev/null || die "approved notification identity is incorrect"

PHASE="approved-workflow-execution"
terminal=""
for _ in $(seq 1 180); do
  run="$(get "$operator_token" "workflow-jobs/$workflow_job_id")"
  terminal="$(jq -r .status <<<"$run")"
  [[ "$terminal" =~ ^(successful|failed|error|canceled)$ ]] && break
  sleep 1
done
[[ "$terminal" == successful ]] || die "workflow finished with status '$terminal'"

PHASE="audit-and-redaction"
audit="$(get "$auditor_token" 'activity-stream?limit=500')"
jq -e --arg path "/api/v1/workflow-templates/$workflow_id/launch" '.[] | select(.username == "demo-operator" and .method == "POST" and .path == $path and .status_code == 201)' <<<"$audit" >/dev/null || die "launch actor is missing from audit evidence"
jq -e --arg path "/api/v1/workflow-job-nodes/$approval_id/approve" '.[] | select(.username == "mwebb" and .method == "POST" and .path == $path and .status_code == 204)' <<<"$audit" >/dev/null || die "approval actor is missing from audit evidence"
jq -e --arg path "/api/v1/job-templates/$template_id/launch-preview" '.[] | select(.username == "demo-operator" and .method == "POST" and .path == $path and .status_code == 200)' <<<"$audit" >/dev/null || die "preview actor is missing from audit evidence"
jq -e '.[] | select(.username == "demo-operator" and .method == "POST" and .path == "/api/v1/jobs" and .status_code == 201)' <<<"$audit" >/dev/null || die "job launch actor is missing from audit evidence"
grep -Fq "$survey_secret" <<<"$audit" && die "auditor-visible activity leaked a password survey answer"

EVIDENCE="$(jq -n --argjson workflow_job_id "$workflow_job_id" --argjson direct_job_id "$direct_job_id" --argjson relaunch_job_id "$relaunch_job_id" --arg status "$terminal" --arg requester demo-operator --arg approver mwebb --arg approval_team backend-team \
  '{schema_version:2,journey:"governed-ldap-operator",result:"pass",workflow_job_id:$workflow_job_id,direct_job_id:$direct_job_id,relaunch_job_id:$relaunch_job_id,status:$status,requester:$requester,approver:$approver,approval_team:$approval_team,checks:["authorized-prompt-choices-only","saved-defaults-and-survey","server-resolved-preview","stale-authorization-denied","cross-organization-denied","cross-team-denied","inaccessible-credential-denied","invalid-limit-denied","immutable-execution-inputs","exact-host-limit","governed-relaunch","team-scoped-approval","requester-self-approval-denied","cross-team-approval-denied","approval-notification-exact-once","approved-notification-exact-once","notification-resource-identity","audit-attribution","audit-secret-redaction"]}')"
if [[ -n "${PRAETOR_LDAP_EVIDENCE_FILE:-}" ]]; then
  umask 077
  printf '%s\n' "$EVIDENCE" >"$PRAETOR_LDAP_EVIDENCE_FILE"
fi
printf '%s\n' "$EVIDENCE"
