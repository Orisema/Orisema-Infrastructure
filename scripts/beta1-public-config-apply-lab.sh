#!/usr/bin/env bash
# Copyright 2026 Orisema contributors
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail
umask 077

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
binary=${ORISEMA_BINARY:-"$repo_root/work/orisema"}
evidence=${ORISEMA_EVIDENCE_DIR:-"$repo_root/work/beta1-public-config-apply"}
image=${ORISEMA_IMAGE:-docker.io/library/nginx:1.27-alpine}
health_command=${ORISEMA_HEALTH_COMMAND:-nginx -t}
expected_decision=${ORISEMA_EXPECT_DECISION:-COMPLETE}
app=beta1-public-config
target=orisema-beta1-public-config-web
old_value="beta1-old-${RANDOM}-${RANDOM}"
new_value="beta1-new-${RANDOM}-${RANDOM}"
old_manifest="$evidence/old.yaml"
manifest="$evidence/manifest.yaml"
journal="$evidence/journal"
backup=

mkdir -p "$evidence"
chmod 0700 "$evidence"

cleanup() {
  podman rm --force --time 0 "$target" >/dev/null 2>&1 || true
  if [ -n "$backup" ]; then
    podman rm --force --time 0 "$backup" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT
cleanup

write_manifest() {
  local value=$1 output=$2
  sed -e "s|LAB_IMAGE|$image|" -e "s|LAB_VALUE|$value|" -e "s|HEALTH_COMMAND|$health_command|" >"$output" <<'YAML'
apiVersion: orisema.dev/v1alpha4
kind: Application
name: beta1-public-config
services:
  - name: web
    image: LAB_IMAGE
    publicEnvironment:
      - name: APP_MODE
        value: LAB_VALUE
    healthcheck:
      command: "HEALTH_COMMAND"
      interval: 5s
      timeout: 2s
      retries: 2
YAML
}

write_manifest "$old_value" "$old_manifest"
write_manifest "$new_value" "$manifest"
"$binary" operation init --journal "$journal" >"$evidence/journal-init.txt"
printf 'DEPLOY\n' | "$binary" deploy --journal "$journal" "$old_manifest" >"$evidence/deploy.txt"
old_id=$(podman inspect --format '{{.Id}}' "$target")

"$binary" update plan "$manifest" >"$evidence/plan.txt"
grep -Fq '[PUBLIC_CONFIG_PREPARE]' "$evidence/plan.txt"
"$binary" update prepare "$manifest" web --journal "$journal" >"$evidence/prepare.txt"
operation_id=$(awk '/^Update intent persisted:/ {print $4}' "$evidence/prepare.txt")
test -n "$operation_id"

journal_before=$(sha256sum "$journal/$operation_id.json" | awk '{print $1}')
status=0
printf 'AUTHORIZE PUBLIC CONFIG wrong/value wrong-id\n' | \
  "$binary" update authorize "$manifest" web --journal "$journal" \
    --operation "$operation_id" >"$evidence/authorize-wrong.out" 2>"$evidence/authorize-wrong.err" || status=$?
test "$status" -ne 0
test "$(podman inspect --format '{{.Id}}' "$target")" = "$old_id"
test "$(podman inspect --format '{{.State.Running}}' "$target")" = true
test "$(sha256sum "$journal/$operation_id.json" | awk '{print $1}')" = "$journal_before"

printf 'AUTHORIZE PUBLIC CONFIG %s/web %s\n' "$app" "$operation_id" | \
  "$binary" update authorize "$manifest" web --journal "$journal" \
    --operation "$operation_id" >"$evidence/authorize.txt"
journal_before=$(sha256sum "$journal/$operation_id.json" | awk '{print $1}')
status=0
printf 'APPLY PUBLIC CONFIG wrong/value wrong-id\n' | \
  "$binary" update apply "$manifest" web --journal "$journal" \
    --operation "$operation_id" >"$evidence/apply-wrong.out" 2>"$evidence/apply-wrong.err" || status=$?
test "$status" -ne 0
test "$(podman inspect --format '{{.Id}}' "$target")" = "$old_id"
test "$(podman inspect --format '{{.State.Running}}' "$target")" = true
test "$(sha256sum "$journal/$operation_id.json" | awk '{print $1}')" = "$journal_before"

printf 'APPLY PUBLIC CONFIG %s/web %s\n' "$app" "$operation_id" | \
  "$binary" update apply "$manifest" web --journal "$journal" \
    --operation "$operation_id" >"$evidence/apply.txt"

backup="orisema-rollback-${operation_id#update-}"
new_id=$(podman inspect --format '{{.Id}}' "$target")
test "$new_id" != "$old_id"
test "$(podman inspect --format '{{.Id}}' "$backup")" = "$old_id"
test "$(podman inspect --format '{{.State.Running}}' "$backup")" = false
test "$(podman inspect --format '{{range .Config.Env}}{{println .}}{{end}}' "$target" | grep -c "^APP_MODE=${new_value}$")" = 1
test "$(podman inspect --format '{{range .Config.Env}}{{println .}}{{end}}' "$backup" | grep -c "^APP_MODE=${old_value}$")" = 1
podman inspect --format '{{json .State}}' "$target" >"$evidence/candidate.json"
podman inspect --format '{{json .State}}' "$backup" >"$evidence/preserved-previous.json"

if [ "$expected_decision" = COMPLETE ]; then
  podman healthcheck run "$target" >"$evidence/candidate-health.txt"
  "$binary" update verify "$manifest" web --journal "$journal" \
    --operation "$operation_id" >"$evidence/verify.out"
  grep -Fq 'decision: COMPLETE' "$evidence/verify.out"
  grep -Fq 'state=COMPLETED' "$evidence/verify.out"
  expected_state=COMPLETED
else
  test "$expected_decision" = RECOVERY_REQUIRED
  podman stop --time 0 "$target" >"$evidence/candidate-stop.txt"
  "$binary" update verify "$manifest" web --journal "$journal" \
    --operation "$operation_id" >"$evidence/verify.out"
  grep -Fq 'decision: RECOVERY_REQUIRED' "$evidence/verify.out"
  grep -Fq 'state=RECOVERY_REQUIRED' "$evidence/verify.out"
  expected_state=RECOVERY_REQUIRED
fi
"$binary" operation diagnose --journal "$journal" >"$evidence/diagnosis.txt"
if [ "$expected_state" = COMPLETED ]; then
  grep -Fq '  none' "$evidence/diagnosis.txt"
else
  grep -Fq 'state=RECOVERY_REQUIRED' "$evidence/diagnosis.txt"
  journal_before=$(sha256sum "$journal/$operation_id.json" | awk '{print $1}')
  candidate_before=$(podman inspect --format '{{.Id}}' "$target")
  previous_before=$(podman inspect --format '{{.Id}}' "$backup")
  status=0
  printf 'ROLLBACK UPDATE wrong/value wrong-id\n' | \
    "$binary" update rollback "$manifest" web --journal "$journal" \
      --operation "$operation_id" >"$evidence/rollback-wrong.out" 2>"$evidence/rollback-wrong.err" || status=$?
  test "$status" -ne 0
  grep -Fq 'cancelled; journal and runtime unchanged' "$evidence/rollback-wrong.err"
  test "$(sha256sum "$journal/$operation_id.json" | awk '{print $1}')" = "$journal_before"
  test "$(podman inspect --format '{{.Id}}' "$target")" = "$candidate_before"
  test "$(podman inspect --format '{{.Id}}' "$backup")" = "$previous_before"

  write_manifest "beta1-tampered-${RANDOM}-${RANDOM}" "$manifest"
  status=0
  printf 'ROLLBACK UPDATE %s/web %s\n' "$app" "$operation_id" | \
    "$binary" update rollback "$manifest" web --journal "$journal" \
      --operation "$operation_id" >"$evidence/rollback-drift.out" 2>"$evidence/rollback-drift.err" || status=$?
  test "$status" -ne 0
  grep -Fq 'rollback blocked:' "$evidence/rollback-drift.err"
  test "$(sha256sum "$journal/$operation_id.json" | awk '{print $1}')" = "$journal_before"
  test "$(podman inspect --format '{{.Id}}' "$target")" = "$candidate_before"
  test "$(podman inspect --format '{{.Id}}' "$backup")" = "$previous_before"
  write_manifest "$new_value" "$manifest"

  printf 'ROLLBACK UPDATE %s/web %s\n' "$app" "$operation_id" | \
    "$binary" update rollback "$manifest" web --journal "$journal" \
      --operation "$operation_id" >"$evidence/rollback.out"
  failed="orisema-failed-${operation_id#update-}"
  test "$(podman inspect --format '{{.Id}}' "$target")" = "$old_id"
  test "$(podman inspect --format '{{.State.Running}}' "$target")" = true
  test "$(podman inspect --format '{{.Id}}' "$failed")" = "$new_id"
  test "$(podman inspect --format '{{.State.Running}}' "$failed")" = false
  test "$(podman inspect --format '{{range .Config.Env}}{{println .}}{{end}}' "$target" | grep -c "^APP_MODE=${old_value}$")" = 1
  podman healthcheck run "$target" >"$evidence/rollback-health.txt"
  "$binary" update rollback-verify "$manifest" web --journal "$journal" \
    --operation "$operation_id" >"$evidence/rollback-verify.out"
  grep -Fq 'state=ROLLED_BACK decision=ROLLED_BACK' "$evidence/rollback-verify.out"
  backup=$failed
fi

if grep -FRq --exclude='old.yaml' --exclude='manifest.yaml' -- "$old_value" "$evidence" || \
   grep -FRq --exclude='old.yaml' --exclude='manifest.yaml' -- "$new_value" "$evidence"; then
  echo 'public configuration canary leaked into Beta 1 evidence' >&2
  exit 1
fi
if grep -Fq -- "$old_value" "$journal/$operation_id.json" || grep -Fq -- "$new_value" "$journal/$operation_id.json"; then
  echo 'public configuration canary leaked into Beta 1 journal' >&2
  exit 1
fi

printf 'Beta 1 public configuration %s laboratory passed.\n' "$expected_decision" | tee "$evidence/result.txt"
