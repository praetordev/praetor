#!/usr/bin/env bash
set -euo pipefail

# Seed the clean product-validation executor with the minimal, deterministic
# runtime required by local-execution journeys. Production packs remain
# immutable release artifacts; this fixture pack deliberately reuses the
# executor image's pinned Python/Ansible and adds the candidate host-runner.

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
NAMESPACE="${PRAETOR_VALIDATION_NAMESPACE:-praetor-secrets}"
RELEASE="${PRAETOR_HELM_RELEASE:-praetor}"
EXECUTOR_ROOT="${PRAETOR_EXECUTOR_ROOT:-$ROOT/../executor}"
PACK_NAME="${PRAETOR_VALIDATION_PACK_NAME:-ansible-runtime}"
WORK="$(mktemp -d "${TMPDIR:-/tmp}/praetor-validation-pack.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT

for command in go kubectl tar; do
  command -v "$command" >/dev/null 2>&1 || {
    echo "error: required command '$command' is not installed" >&2
    exit 1
  }
done

callback="$EXECUTOR_ROOT/deploy/plugins/callback/praetor_checkpoint.py"
[[ -s "$callback" ]] || {
  echo "error: executor checkpoint callback is missing at $callback" >&2
  exit 1
}

executor_pod="$(
  kubectl get pods -n "$NAMESPACE" \
    -l "app.kubernetes.io/component=executor,app.kubernetes.io/instance=$RELEASE" \
    -o jsonpath='{.items[0].metadata.name}'
)"
[[ -n "$executor_pod" ]] || {
  echo "error: validation executor pod is missing" >&2
  exit 1
}

arch="$(
  kubectl exec -n "$NAMESPACE" "$executor_pod" -- uname -m |
    sed -e 's/^aarch64$/arm64/' -e 's/^x86_64$/amd64/'
)"
[[ "$arch" == arm64 || "$arch" == amd64 ]] || {
  echo "error: unsupported validation executor architecture '$arch'" >&2
  exit 1
}

CGO_ENABLED=0 GOOS=linux GOARCH="$arch" \
  go build -o "$WORK/praetor-host-runner" ./cmd/host-runner

pack="/opt/praetor/packs/$PACK_NAME"
archive_root="$WORK/archive"
archive_pack="$archive_root${pack}"
runtime_dir="/tmp/build/runtime"
tarball="$WORK/$PACK_NAME-linux-$arch.tar.gz"
mkdir -p "$archive_pack/bin" "$archive_pack/plugins/callback"
install -m 0755 "$WORK/praetor-host-runner" "$archive_pack/bin/praetor-host-runner"
install -m 0644 "$callback" "$archive_pack/plugins/callback/praetor_checkpoint.py"
printf '%s\n' '#!/bin/sh' 'exec /usr/local/bin/ansible-playbook "$@"' >"$archive_pack/bin/ansible-playbook"
chmod 0755 "$archive_pack/bin/ansible-playbook"
COPYFILE_DISABLE=1 tar -czf "$tarball" -C "$archive_root" "${pack#/}"

kubectl exec -n "$NAMESPACE" "$executor_pod" -- \
  mkdir -p "$pack/bin" "$pack/plugins/callback" "$runtime_dir"
kubectl cp "$WORK/praetor-host-runner" \
  "$NAMESPACE/$executor_pod:$pack/bin/praetor-host-runner"
kubectl cp "$callback" \
  "$NAMESPACE/$executor_pod:$pack/plugins/callback/praetor_checkpoint.py"
kubectl cp "$tarball" \
  "$NAMESPACE/$executor_pod:$runtime_dir/$PACK_NAME-linux-$arch.tar.gz"
kubectl exec -n "$NAMESPACE" "$executor_pod" -- sh -c "
  chmod 0755 '$pack/bin/praetor-host-runner'
  chmod 0644 '$pack/plugins/callback/praetor_checkpoint.py'
  ln -sf /usr/local/bin/ansible-playbook '$pack/bin/ansible-playbook'
  ln -sf /usr/local/bin/python3 '$pack/bin/python3'
  test -x '$pack/bin/praetor-host-runner'
  test -x '$pack/bin/ansible-playbook'
  test -x '$pack/bin/python3'
  test -s '$runtime_dir/$PACK_NAME-linux-$arch.tar.gz'
"

echo "validation execution pack ready for local and remote execution: $PACK_NAME ($arch)"
