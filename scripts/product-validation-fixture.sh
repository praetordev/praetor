#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
NAMESPACE="${PRAETOR_VALIDATION_NAMESPACE:-praetor-secrets}"
RELEASE="${PRAETOR_HELM_RELEASE:-praetor}"
CHART="$ROOT/deployments/helm/praetor-v2"
MANIFEST="$ROOT/deployments/product-validation/fixture.yaml"
LABEL="app.kubernetes.io/part-of=praetor-validation-fixture"
CLUSTER="${PRAETOR_K3D_CLUSTER:-praetor-validation}"
MANAGED_HOST_IMAGE="${PRAETOR_VALIDATION_MANAGED_HOST_IMAGE:-praetor-validation-host:validation}"
MANAGED_HOST_SECRET="praetor-validation-managed-host-identity"

usage() { echo "usage: $0 <create|status|cleanup>"; }
die() { echo "error: $*" >&2; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || die "required command '$1' is not installed"; }
for command in curl docker helm jq k3d kubectl ssh-keygen; do need "$command"; done

FIXTURE_PREFIX="Praetor Validation"
API_PORT="${PRAETOR_VALIDATION_API_PORT:-18081}"
API="http://127.0.0.1:$API_PORT/api/v1"
PORT_FORWARD_PID=""
PORT_FORWARD_LOG=""

stop_api_tunnel() {
  [[ -z "$PORT_FORWARD_PID" ]] || kill "$PORT_FORWARD_PID" 2>/dev/null || true
  [[ -z "$PORT_FORWARD_LOG" ]] || rm -f "$PORT_FORWARD_LOG"
  PORT_FORWARD_PID=""; PORT_FORWARD_LOG=""
}

start_api_tunnel() {
  PORT_FORWARD_LOG="$(mktemp "${TMPDIR:-/tmp}/praetor-validation-port-forward.XXXXXX")"
  kubectl port-forward -n "$NAMESPACE" "svc/$RELEASE-api" "$API_PORT:8080" >"$PORT_FORWARD_LOG" 2>&1 &
  PORT_FORWARD_PID=$!
  for _ in $(seq 1 30); do
    curl -fsS "$API/ping" >/dev/null 2>&1 && return
    kill -0 "$PORT_FORWARD_PID" 2>/dev/null || { cat "$PORT_FORWARD_LOG" >&2; die "API tunnel stopped"; }
    sleep 1
  done
  die "API did not become reachable"
}

login() {
  curl -fsS -H 'Content-Type: application/json' \
    -d "$(jq -nc --arg username "$1" --arg password "$2" '{username:$username,password:$password}')" \
    "$API/auth/login" | jq -er .token
}

api_get() { curl -fsS -H "Authorization: Bearer $ADMIN_TOKEN" "$API/$1"; }
api_post() {
  curl -fsS -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' -d "$2" "$API/$1"
}
api_delete() {
  curl -fsS -X DELETE -H "Authorization: Bearer $ADMIN_TOKEN" "$API/$1"
}
grant_team_role() {
  local content_type="$1" object_id="$2" role_name="$3" team_id="$4" role_id
  role_id="$(api_get "role-definitions?content_type=$content_type" | jq -er --arg name "$role_name" '.[] | select(.name == $name) | .id' | head -n1)"
  api_post access "$(jq -nc --arg type "$content_type" --argjson object "$object_id" --argjson role "$role_id" --argjson team "$team_id" '{content_type:$type,object_id:$object,role_definition_id:$role,team_id:$team}')" >/dev/null
}
find_named_id() {
  jq -r --arg name "$2" '(if type == "object" and has("items") then .items else . end)[] | select(.name == $name) | .id' <<<"$(api_get "$1")" | head -n1
}
ensure_named() {
  local path="$1" list_path="$2" name="$3" body="$4" id
  id="$(find_named_id "$list_path" "$name")"
  if [[ -z "$id" ]]; then id="$(api_post "$path" "$body" | jq -er .id)"; fi
  printf '%s' "$id"
}
ensure_policy() {
  local resource_type="$1" resource_id="$2" event="$3" target_id="$4" team_id="${5:-}" policies body id
  policies="$(api_get "notification-policies?resource_type=$resource_type&resource_id=$resource_id")"
  id="$(jq -r --arg event "$event" --argjson target "$target_id" --arg team "$team_id" '
    .[] | select(.event == $event and .notification_template_id == $target)
    | select(($team == "" and (.team_id == null)) or ($team != "" and (.team_id | tostring) == $team))
    | .id' <<<"$policies" | head -n1)"
  if [[ -z "$id" ]]; then
    body="$(jq -nc --arg type "$resource_type" --argjson resource "$resource_id" --arg event "$event" --argjson target "$target_id" --arg team "$team_id" \
      '{resource_type:$type,resource_id:$resource,event:$event,notification_template_id:$target} + if $team == "" then {} else {team_id:($team|tonumber)} end')"
    id="$(api_post notification-policies "$body" | jq -er .id)"
  fi
  printf '%s' "$id"
}

prepare_managed_host() {
  local identity_dir
  if ! kubectl get secret "$MANAGED_HOST_SECRET" -n "$NAMESPACE" >/dev/null 2>&1; then
    identity_dir="$(mktemp -d "${TMPDIR:-/tmp}/praetor-validation-host.XXXXXX")"
    trap 'rm -rf "$identity_dir"' RETURN
    ssh-keygen -q -t ed25519 -N '' -C praetor-validation -f "$identity_dir/id_ed25519"
    ssh-keygen -q -t ed25519 -N '' -C praetor-validation-host -f "$identity_dir/ssh_host_ed25519_key"
    kubectl create secret generic "$MANAGED_HOST_SECRET" -n "$NAMESPACE" \
      --from-file=id_ed25519="$identity_dir/id_ed25519" \
      --from-file=authorized_keys="$identity_dir/id_ed25519.pub" \
      --from-file=ssh_host_ed25519_key="$identity_dir/ssh_host_ed25519_key" >/dev/null
    kubectl label secret "$MANAGED_HOST_SECRET" -n "$NAMESPACE" "$LABEL" --overwrite >/dev/null
    rm -rf "$identity_dir"
    trap - RETURN
  fi

  echo "==> Building disposable managed host"
  docker build -q -f "$ROOT/deployments/product-validation/Dockerfile.managed-host" \
    -t "$MANAGED_HOST_IMAGE" "$ROOT" >/dev/null
  k3d image import --mode direct --cluster "$CLUSTER" "$MANAGED_HOST_IMAGE" >/dev/null
}

forget_managed_host_key() {
  local executor_pod known_hosts="/home/praetor/.ssh/known_hosts"
  executor_pod="$(
    kubectl get pods -n "$NAMESPACE" \
      -l "app.kubernetes.io/component=executor,app.kubernetes.io/instance=$RELEASE" \
      -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true
  )"
  [[ -n "$executor_pod" ]] || return
  kubectl exec -n "$NAMESPACE" "$executor_pod" -- sh -c "
    if [ -f '$known_hosts' ]; then
      ssh-keygen -q -f '$known_hosts' -R 'praetor-validation-managed-host' >/dev/null 2>&1 || true
      ssh-keygen -q -f '$known_hosts' -R '[praetor-validation-managed-host]:22' >/dev/null 2>&1 || true
    fi
  " >/dev/null
}

seed_api_resources() {
  start_api_tunnel
  trap stop_api_tunnel RETURN
  # Login-time mapping creates/refreshes Engineering and backend-team first.
  login demo-operator praetor123 >/dev/null
  ADMIN_TOKEN="$(login "${PRAETOR_VALIDATION_ADMIN_USERNAME:-admin}" "${PRAETOR_VALIDATION_ADMIN_PASSWORD:-admin}")"
  local org_id foreign_org_id inventory_id host_id prompt_inventory_id prompt_host_id
  local hidden_inventory_id foreign_inventory_id project_id template_id workflow_id
  local ldap_workflow_id team_id notification_id machine_type_id default_credential_id
  local prompt_credential_id hidden_credential_id foreign_credential_id credential_private_key credential_secret
  local managed_host_vars
  org_id="$(find_named_id organizations/ Engineering)"; [[ -n "$org_id" ]] || die "LDAP mapping did not create Engineering"
  team_id="$(find_named_id teams/ backend-team)"; [[ -n "$team_id" ]] || die "LDAP mapping did not create backend-team"
  foreign_org_id="$(ensure_named organizations organizations/ "$FIXTURE_PREFIX Foreign Organization" "$(jq -nc --arg name "$FIXTURE_PREFIX Foreign Organization" '{name:$name,description:"Synthetic cross-organization authorization boundary"}')")"
  inventory_id="$(ensure_named inventories inventories/ "$FIXTURE_PREFIX Inventory" "$(jq -nc --argjson org "$org_id" --arg name "$FIXTURE_PREFIX Inventory" '{organization_id:$org,name:$name,kind:"static"}')")"
  managed_host_vars='{"ansible_host":"praetor-validation-managed-host","ansible_port":22,"ansible_user":"praetor","ansible_connection":"ssh"}'
  host_id="$(find_named_id "inventories/$inventory_id/hosts/" "praetor-validation-host")"
  [[ -n "$host_id" ]] || host_id="$(api_post "inventories/$inventory_id/hosts/" "$(jq -nc --arg name "praetor-validation-host" --argjson variables "$managed_host_vars" '{name:$name,description:"Praetor Validation Host",enabled:true,variables:$variables}')" | jq -er .id)"
  api_post "hosts/$host_id/set-runner" '{}' >/dev/null
  prompt_inventory_id="$(ensure_named inventories inventories/ "$FIXTURE_PREFIX Prompt Inventory" "$(jq -nc --argjson org "$org_id" --arg name "$FIXTURE_PREFIX Prompt Inventory" '{organization_id:$org,name:$name,kind:"static"}')")"
  prompt_host_id="$(find_named_id "inventories/$prompt_inventory_id/hosts/" "praetor-validation-prompt-host")"
  [[ -n "$prompt_host_id" ]] || prompt_host_id="$(api_post "inventories/$prompt_inventory_id/hosts/" "$(jq -nc --arg name "praetor-validation-prompt-host" --argjson variables "$managed_host_vars" '{name:$name,description:"Praetor Validation Prompt Host",enabled:true,variables:$variables}')" | jq -er .id)"
  api_post "hosts/$prompt_host_id/set-runner" '{}' >/dev/null
  [[ -n "$(find_named_id "inventories/$prompt_inventory_id/hosts/" "praetor-validation-excluded-host")" ]] ||
    api_post "inventories/$prompt_inventory_id/hosts/" "$(jq -nc --arg name "praetor-validation-excluded-host" '{name:$name,description:"Praetor Validation Excluded Host",enabled:true,variables:{ansible_connection:"local"}}')" >/dev/null
  hidden_inventory_id="$(ensure_named inventories inventories/ "$FIXTURE_PREFIX Hidden Inventory" "$(jq -nc --argjson org "$org_id" --arg name "$FIXTURE_PREFIX Hidden Inventory" '{organization_id:$org,name:$name,kind:"static"}')")"
  [[ -n "$(find_named_id "inventories/$hidden_inventory_id/hosts/" "praetor-validation-hidden-host")" ]] ||
    api_post "inventories/$hidden_inventory_id/hosts/" "$(jq -nc --arg name "praetor-validation-hidden-host" '{name:$name,description:"Praetor Validation Hidden Host",enabled:true,variables:{ansible_connection:"local"}}')" >/dev/null
  foreign_inventory_id="$(ensure_named inventories inventories/ "$FIXTURE_PREFIX Foreign Inventory" "$(jq -nc --argjson org "$foreign_org_id" --arg name "$FIXTURE_PREFIX Foreign Inventory" '{organization_id:$org,name:$name,kind:"static"}')")"
  [[ -n "$(find_named_id "inventories/$foreign_inventory_id/hosts/" "praetor-validation-foreign-host")" ]] ||
    api_post "inventories/$foreign_inventory_id/hosts/" "$(jq -nc --arg name "praetor-validation-foreign-host" '{name:$name,description:"Praetor Validation Foreign Host",enabled:true,variables:{ansible_connection:"local"}}')" >/dev/null
  machine_type_id="$(find_named_id credential-types Machine)"; [[ -n "$machine_type_id" ]] || die "built-in Machine credential type is missing"
  credential_private_key="$(kubectl get secret "$MANAGED_HOST_SECRET" -n "$NAMESPACE" -o go-template='{{index .data "id_ed25519" | base64decode}}')"
  [[ "$credential_private_key" == *"BEGIN OPENSSH PRIVATE KEY"* ]] || die "managed-host private key is unavailable"
  credential_secret="${PRAETOR_VALIDATION_MACHINE_PASSWORD:-synthetic-validation-password}"
  default_credential_id="$(ensure_named credentials credentials "$FIXTURE_PREFIX Default Credential" "$(jq -nc --argjson org "$org_id" --argjson type "$machine_type_id" --arg name "$FIXTURE_PREFIX Default Credential" --arg key "$credential_private_key" '{organization_id:$org,credential_type_id:$type,name:$name,inputs:{username:"praetor",ssh_private_key:$key}}')")"
  prompt_credential_id="$(ensure_named credentials credentials "$FIXTURE_PREFIX Prompt Credential" "$(jq -nc --argjson org "$org_id" --argjson type "$machine_type_id" --arg name "$FIXTURE_PREFIX Prompt Credential" --arg key "$credential_private_key" '{organization_id:$org,credential_type_id:$type,name:$name,inputs:{username:"praetor",ssh_private_key:$key}}')")"
  hidden_credential_id="$(ensure_named credentials credentials "$FIXTURE_PREFIX Hidden Credential" "$(jq -nc --argjson org "$org_id" --argjson type "$machine_type_id" --arg name "$FIXTURE_PREFIX Hidden Credential" --arg password "$credential_secret" '{organization_id:$org,credential_type_id:$type,name:$name,inputs:{username:"validation-hidden",password:$password}}')")"
  foreign_credential_id="$(ensure_named credentials credentials "$FIXTURE_PREFIX Foreign Credential" "$(jq -nc --argjson org "$foreign_org_id" --argjson type "$machine_type_id" --arg name "$FIXTURE_PREFIX Foreign Credential" --arg password "$credential_secret" '{organization_id:$org,credential_type_id:$type,name:$name,inputs:{username:"validation-foreign",password:$password}}')")"
  project_id="$(ensure_named projects projects "$FIXTURE_PREFIX Project" "$(jq -nc --argjson org "$org_id" --arg name "$FIXTURE_PREFIX Project" '{organization_id:$org,name:$name,scm_type:"git",scm_url:"https://github.com/Niftel/praetor.git"}')")"
  template_id="$(ensure_named job-templates job-templates/ "$FIXTURE_PREFIX Job" "$(jq -nc --argjson org "$org_id" --argjson inv "$inventory_id" --argjson credential "$default_credential_id" --argjson project "$project_id" --arg name "$FIXTURE_PREFIX Job" '{organization_id:$org,inventory_id:$inv,credential_id:$credential,project_id:$project,name:$name,playbook:"playbooks/ping.yml",job_type:"run",forks:1,extra_vars:{fixture_source:"saved-default"},limit:"praetor-validation-host",ask_inventory_on_launch:true,ask_credential_on_launch:true,ask_variables_on_launch:false,ask_limit_on_launch:true,survey_enabled:true,survey_spec:{spec:[{variable:"change_ticket",question_name:"Change ticket",type:"text",required:true},{variable:"deployment_ring",question_name:"Deployment ring",type:"multiplechoice",required:true,choices:["canary","production"]},{variable:"approval_secret",question_name:"Approval secret",type:"password",required:true}]}}')")"
  workflow_id="$(ensure_named workflow-templates workflow-templates "$FIXTURE_PREFIX Workflow" "$(jq -nc --argjson org "$org_id" --argjson jt "$template_id" --arg name "$FIXTURE_PREFIX Workflow" '{organization_id:$org,name:$name,nodes:[{node_key:"approval",node_type:"approval",name:"Team approval"},{node_key:"execute",node_type:"job",job_template_id:$jt,name:"Run validation"}],edges:[{parent_key:"approval",child_key:"execute",edge_type:"success"}]}')")"
  ldap_workflow_id="$(ensure_named workflow-templates workflow-templates "$FIXTURE_PREFIX LDAP Workflow" "$(jq -nc --argjson org "$org_id" --argjson jt "$template_id" --arg name "$FIXTURE_PREFIX LDAP Workflow" '{organization_id:$org,name:$name,nodes:[{node_key:"approval",node_type:"approval",name:"Team approval"},{node_key:"execute",node_type:"job",job_template_id:$jt,name:"Run governed validation"}],edges:[{parent_key:"approval",child_key:"execute",edge_type:"success"}]}')")"
  # The backend team is the synthetic operator boundary: members may use the
  # inventory, launch the workflow, and decide its approval gate. The API still
  # forbids the requester from deciding their own request.
  grant_team_role inventory "$inventory_id" "Inventory Use" "$team_id"
  grant_team_role inventory "$prompt_inventory_id" "Inventory Use" "$team_id"
  grant_team_role credential "$default_credential_id" "Credential Use" "$team_id"
  grant_team_role credential "$prompt_credential_id" "Credential Use" "$team_id"
  grant_team_role job_template "$template_id" "Job Template Execute" "$team_id"
  grant_team_role workflow_template "$ldap_workflow_id" "Workflow Template Execute" "$team_id"
  grant_team_role workflow_template "$ldap_workflow_id" "Workflow Template Approve" "$team_id"
  notification_id="$(ensure_named notification-templates "notification-templates?organization_id=$org_id" "$FIXTURE_PREFIX Notifications" "$(jq -nc --argjson org "$org_id" --arg name "$FIXTURE_PREFIX Notifications" '{organization_id:$org,name:$name,notification_type:"webhook",config:{url:"http://praetor-validation-notification-sink:8080/echo"}}')")"
  for event in success error; do ensure_policy job_template "$template_id" "$event" "$notification_id" >/dev/null; done
  for event in started success error; do ensure_policy workflow_template "$workflow_id" "$event" "$notification_id" >/dev/null; done
  for event in approval approved denied timeout; do
    ensure_policy workflow_template "$workflow_id" "$event" "$notification_id" "$team_id" >/dev/null
    ensure_policy workflow_template "$ldap_workflow_id" "$event" "$notification_id" "$team_id" >/dev/null
  done
  ensure_named "organizations/$org_id/service-principals/" "organizations/$org_id/service-principals/" "$FIXTURE_PREFIX API" "$(jq -nc --arg name "$FIXTURE_PREFIX API" '{name:$name,description:"Synthetic delegated validation principal"}')" >/dev/null
  jq -n --argjson organization_id "$org_id" --argjson team_id "$team_id" --argjson inventory_id "$inventory_id" --argjson host_id "$host_id" --argjson prompt_inventory_id "$prompt_inventory_id" --argjson prompt_host_id "$prompt_host_id" --argjson prompt_credential_id "$prompt_credential_id" --argjson project_id "$project_id" --argjson job_template_id "$template_id" --argjson workflow_id "$workflow_id" --argjson ldap_workflow_id "$ldap_workflow_id" '{organization_id:$organization_id,team_id:$team_id,inventory_id:$inventory_id,host_id:$host_id,prompt_inventory_id:$prompt_inventory_id,prompt_host_id:$prompt_host_id,prompt_credential_id:$prompt_credential_id,project_id:$project_id,job_template_id:$job_template_id,workflow_id:$workflow_id,ldap_workflow_id:$ldap_workflow_id}'
  stop_api_tunnel
  trap - RETURN
}

cluster_ready() {
  docker info >/dev/null 2>&1 || die "Docker daemon is unavailable"
  kubectl get --raw=/readyz >/dev/null 2>&1 || die "Kubernetes API is unavailable; run make local-cluster-start"
}

status_fixture() {
  cluster_ready
  local failed=0 target
  for target in "deployment/$RELEASE-api" deployment/praetor-secrets deployment/praetor-audit-sink deployment/praetor-validation-ldap deployment/praetor-validation-managed-host deployment/praetor-validation-notification-sink deployment/praetor-validation-inventory-provider; do
    if ! kubectl get "$target" -n "$NAMESPACE" >/dev/null 2>&1; then
      echo "unhealthy: $target is missing" >&2; failed=1
    elif ! kubectl wait --for=condition=available "$target" -n "$NAMESPACE" --timeout=1s >/dev/null 2>&1; then
      echo "unhealthy: $target is not available" >&2; failed=1
    else
      echo "healthy: $target"
    fi
  done
  (( failed == 0 )) || return 1
}

create_fixture() {
  cluster_ready
  kubectl get namespace "$NAMESPACE" >/dev/null 2>&1 || die "namespace '$NAMESPACE' is missing; deploy the integrated stack first"
  helm status "$RELEASE" -n "$NAMESPACE" >/dev/null 2>&1 || die "Helm release '$RELEASE' is missing from '$NAMESPACE'"
  prepare_managed_host
  kubectl -n "$NAMESPACE" create configmap praetor-validation-ldap-bootstrap --from-file=bootstrap.ldif="$ROOT/deployments/ldap/bootstrap.ldif" --dry-run=client -o yaml | kubectl apply -f - >/dev/null
  kubectl -n "$NAMESPACE" create configmap praetor-validation-ldap-config --from-file=ldap.yaml="$ROOT/deployments/ldap/ldap-config.yaml" --dry-run=client -o yaml | kubectl apply -f - >/dev/null
  kubectl apply -n "$NAMESPACE" -f "$MANIFEST" >/dev/null
  kubectl set image deployment/praetor-validation-managed-host -n "$NAMESPACE" "host=$MANAGED_HOST_IMAGE" >/dev/null
  kubectl rollout restart deployment/praetor-validation-managed-host -n "$NAMESPACE" >/dev/null
  kubectl rollout status deployment/praetor-validation-ldap -n "$NAMESPACE" --timeout=180s
  kubectl rollout status deployment/praetor-validation-notification-sink -n "$NAMESPACE" --timeout=180s
  kubectl rollout status deployment/praetor-validation-inventory-provider -n "$NAMESPACE" --timeout=180s
  kubectl rollout status deployment/praetor-validation-managed-host -n "$NAMESPACE" --timeout=180s
  helm upgrade "$RELEASE" "$CHART" -n "$NAMESPACE" --reuse-values --set ldap.enabled=true --set ldap.existingConfigMap=praetor-validation-ldap-config --set secrets.ldapBindPassword=admin --wait --timeout 5m >/dev/null
  kubectl rollout status "deployment/$RELEASE-api" -n "$NAMESPACE" --timeout=180s
  PRAETOR_EXECUTOR_ROOT="${PRAETOR_EXECUTOR_ROOT:-$ROOT/../executor}" \
    "$ROOT/scripts/stage-validation-execution-pack.sh"
  status_fixture
  echo "==> Seeding API resources"
  seed_api_resources
}

cleanup_fixture() {
  cluster_ready
  local db_pod
  start_api_tunnel
  ADMIN_TOKEN="$(login "${PRAETOR_VALIDATION_ADMIN_USERNAME:-admin}" "${PRAETOR_VALIDATION_ADMIN_PASSWORD:-admin}")"
  for credential_name in \
    "$FIXTURE_PREFIX Default Credential" \
    "$FIXTURE_PREFIX Prompt Credential" \
    "$FIXTURE_PREFIX Hidden Credential" \
    "$FIXTURE_PREFIX Foreign Credential"; do
    credential_id="$(find_named_id credentials "$credential_name")"
    [[ -z "$credential_id" ]] || api_delete "credentials/$credential_id" >/dev/null
  done
  stop_api_tunnel
  db_pod="$(kubectl get pods -n "$NAMESPACE" -l "app.kubernetes.io/instance=$RELEASE,app.kubernetes.io/component=postgresql" -o jsonpath='{.items[0].metadata.name}')"
  [[ -n "$db_pod" ]] || die "Praetor database pod is missing"
  kubectl exec -i -n "$NAMESPACE" "$db_pod" -- psql -v ON_ERROR_STOP=1 -U postgres -d praetor >/dev/null <<'SQL'
BEGIN;
DELETE FROM schedules WHERE name = 'Dynamic Inventory E2E Schedule';
DELETE FROM inventory_sources WHERE name = 'Dynamic Inventory E2E Source';
DELETE FROM credentials WHERE name = 'Dynamic Inventory E2E Credential';
DELETE FROM inventories WHERE name = 'Dynamic Inventory E2E Inventory';
DELETE FROM workflow_templates WHERE name = 'Praetor Validation Workflow';
DELETE FROM workflow_templates WHERE name = 'Praetor Validation LDAP Workflow';
DELETE FROM service_principals WHERE name = 'Praetor Validation API';
DELETE FROM notification_templates WHERE name = 'Praetor Validation Notifications';
DELETE FROM job_templates WHERE name = 'Praetor Validation Job';
DELETE FROM hosts WHERE name IN ('Praetor Validation Host','praetor-validation-host');
DELETE FROM hosts WHERE name IN ('Praetor Validation Prompt Host','Praetor Validation Excluded Host','Praetor Validation Hidden Host','Praetor Validation Foreign Host','praetor-validation-prompt-host','praetor-validation-excluded-host','praetor-validation-hidden-host','praetor-validation-foreign-host');
DELETE FROM inventories WHERE name IN ('Praetor Validation Prompt Inventory','Praetor Validation Hidden Inventory','Praetor Validation Foreign Inventory');
DELETE FROM inventories WHERE name = 'Praetor Validation Inventory';
DELETE FROM projects WHERE name = 'Praetor Validation Project';
DELETE FROM organizations WHERE name = 'Praetor Validation Foreign Organization';
COMMIT;
SQL
  kubectl delete all -n "$NAMESPACE" -l "$LABEL" --ignore-not-found >/dev/null
  # The fixture rotates its disposable SSH host identity on recreation. Remove
  # only that hostname from the executor's persistent trust store before the
  # old key disappears; production host-key verification remains enabled.
  forget_managed_host_key
  kubectl delete secret -n "$NAMESPACE" "$MANAGED_HOST_SECRET" --ignore-not-found >/dev/null
  kubectl delete configmap -n "$NAMESPACE" praetor-validation-ldap-bootstrap praetor-validation-ldap-config praetor-validation-inventory-provider praetor-validation-notification-sink --ignore-not-found >/dev/null
  if helm status "$RELEASE" -n "$NAMESPACE" >/dev/null 2>&1; then
    helm upgrade "$RELEASE" "$CHART" -n "$NAMESPACE" --reuse-values --set ldap.enabled=false --set ldap.existingConfigMap= --wait --timeout 5m >/dev/null
  fi
  echo "validation fixture removed; persistent platform data and secrets were preserved"
}

case "${1:-}" in
  create) create_fixture ;;
  status) status_fixture ;;
  cleanup) cleanup_fixture ;;
  *) usage >&2; exit 2 ;;
esac
