#!/usr/bin/env bash
# Copyright 2026 Orisema contributors
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
binary="${ORISEMA_BINARY:-$root/work/orisema}"
evidence="${ORISEMA_EVIDENCE_DIR:-$root/work/alpha1-negative}"
name_manifest="$root/testdata/alpha1/name-conflict.yaml"
port_manifest="$root/testdata/alpha1/port-conflict.yaml"
cancel_manifest="$root/testdata/alpha1/cancelled.yaml"

name_target=orisema-regression-name-web
port_target=orisema-regression-port-web
port_holder=orisema-regression-port-holder
cancel_target=orisema-regression-cancel-web

mkdir -p "$evidence"

cleanup() {
  local target
  for target in "$name_target" "$port_target" "$port_holder" "$cancel_target"; do
    podman rm --force "$target" >/dev/null 2>&1 || true
  done
}
trap cleanup EXIT
cleanup

test -x "$binary"
podman version > "$evidence/podman-version.txt"
"$binary" version > "$evidence/orisema-version.txt"

# An unmanaged container with the expected name must yield REVIEW. Deploy and
# lifecycle commands must leave its identity and running state unchanged.
podman run --detach --name "$name_target" docker.io/library/nginx:1.27 \
  > "$evidence/name-unmanaged-create.txt"
unmanaged_id="$(podman inspect --format '{{.Id}}' "$name_target")"
"$binary" plan "$name_manifest" | tee "$evidence/name-review.txt"
grep -Fq '[REVIEW]' "$evidence/name-review.txt"
if printf 'DEPLOY\n' | "$binary" deploy "$name_manifest" \
    > "$evidence/name-deploy.txt" 2>&1; then
  echo 'deploy unexpectedly accepted an unmanaged name collision' >&2
  exit 1
fi
grep -Fq 'deployment blocked' "$evidence/name-deploy.txt"
if printf 'STOP\n' | "$binary" stop "$name_manifest" web \
    > "$evidence/name-stop.txt" 2>&1; then
  echo 'stop unexpectedly accepted an unmanaged container' >&2
  exit 1
fi
grep -Fq 'not verified as an Orisema-owned container' "$evidence/name-stop.txt"
test "$(podman inspect --format '{{.Id}}' "$name_target")" = "$unmanaged_id"
test "$(podman inspect --format '{{.State.Running}}' "$name_target")" = true

# Correct ownership labels are not enough: a declared port mismatch is DRIFT,
# and deployment must still leave the container untouched.
podman rm --force "$name_target" >/dev/null
podman run --detach --name "$name_target" \
  --label dev.orisema.managed=true \
  --label dev.orisema.app=regression-name \
  --label dev.orisema.service=web \
  --security-opt no-new-privileges \
  docker.io/library/nginx:1.27 > "$evidence/name-managed-create.txt"
managed_id="$(podman inspect --format '{{.Id}}' "$name_target")"
"$binary" plan "$name_manifest" | tee "$evidence/name-drift.txt"
grep -Fq '[DRIFT]' "$evidence/name-drift.txt"
if printf 'DEPLOY\n' | "$binary" deploy "$name_manifest" \
    > "$evidence/drift-deploy.txt" 2>&1; then
  echo 'deploy unexpectedly accepted drift' >&2
  exit 1
fi
test "$(podman inspect --format '{{.Id}}' "$name_target")" = "$managed_id"

# A separate container holds the requested host port. The failed CREATE must
# preserve the holder and must not leave a target container behind.
podman run --detach --name "$port_holder" --publish 18084:80 \
  docker.io/library/nginx:1.27 > "$evidence/port-holder-create.txt"
holder_id="$(podman inspect --format '{{.Id}}' "$port_holder")"
if printf 'DEPLOY\n' | "$binary" deploy "$port_manifest" \
    > "$evidence/port-deploy.txt" 2>&1; then
  echo 'deploy unexpectedly accepted an occupied host port' >&2
  exit 1
fi
grep -Fq 'deployment stopped' "$evidence/port-deploy.txt"
test "$(podman inspect --format '{{.Id}}' "$port_holder")" = "$holder_id"
test "$(podman inspect --format '{{.State.Running}}' "$port_holder")" = true
if podman container exists "$port_target"; then
  echo 'failed port deployment left a target container behind' >&2
  exit 1
fi

# Refusing confirmation must not create anything.
if printf 'CANCEL\n' | "$binary" deploy "$cancel_manifest" \
    > "$evidence/cancelled-deploy.txt" 2>&1; then
  echo 'deploy unexpectedly accepted a cancelled confirmation' >&2
  exit 1
fi
grep -Fq 'cancelled' "$evidence/cancelled-deploy.txt"
if podman container exists "$cancel_target"; then
  echo 'cancelled deployment created a container' >&2
  exit 1
fi

# Missing runtime must fail closed before confirmation or mutation.
before="$(podman ps --all --no-trunc --format '{{.ID}} {{.Names}}' | sort)"
if PATH=/nonexistent "$binary" plan "$cancel_manifest" \
    > "$evidence/runtime-unavailable.txt" 2>&1; then
  echo 'plan unexpectedly succeeded without Podman on PATH' >&2
  exit 1
fi
grep -Fq 'read Podman containers' "$evidence/runtime-unavailable.txt"
after="$(podman ps --all --no-trunc --format '{{.ID}} {{.Names}}' | sort)"
test "$before" = "$after"

printf 'Alpha 1 negative regressions passed.\n' | tee "$evidence/result.txt"
