#!/usr/bin/env bash
# Copyright 2026 Orisema contributors
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

binary="${ORISEMA_BINARY:-$PWD/work/orisema}"
evidence="${ORISEMA_EVIDENCE_DIR:-$PWD/work/alpha5-update-apply}"
old_image="${ORISEMA_OLD_IMAGE:-docker.io/library/nginx:1.26-alpine}"
new_image="${ORISEMA_NEW_IMAGE:-docker.io/library/nginx:1.27-alpine}"
health_command="${ORISEMA_HEALTH_COMMAND:-nginx -t}"
expected_decision="${ORISEMA_EXPECT_DECISION:-COMPLETE}"
force_candidate_stop="${ORISEMA_FORCE_CANDIDATE_STOP:-false}"
rollback_after_failure="${ORISEMA_ROLLBACK_AFTER_FAILURE:-false}"
target=orisema-alpha5-apply-web
external=orisema-alpha5-apply-external
external_volume=orisema-alpha5-apply-data
port=18089
backup=""
failed=""

mkdir -p "$evidence"
manifest="$evidence/manifest.yaml"
journal="$evidence/journal"

cleanup() {
	podman rm --force --time 0 "$target" >/dev/null 2>&1 || true
	if [ -n "$backup" ]; then
		podman rm --force --time 0 "$backup" >/dev/null 2>&1 || true
	fi
	if [ -n "$failed" ]; then
		podman rm --force --time 0 "$failed" >/dev/null 2>&1 || true
	fi
	podman rm --force --time 0 "$external" >/dev/null 2>&1 || true
  podman volume rm "$external_volume" >/dev/null 2>&1 || true
}
trap cleanup EXIT
cleanup

sed -e "s|NEW_IMAGE|$new_image|" -e "s|HEALTH_COMMAND|$health_command|" > "$manifest" <<'YAML'
apiVersion: orisema.dev/v1alpha2
kind: Application
name: alpha5-apply
services:
  - name: web
    image: NEW_IMAGE
    ports:
      - host: 18089
        container: 80
        hostIP: 127.0.0.1
    healthcheck:
      command: "HEALTH_COMMAND"
      interval: 30s
      timeout: 5s
      retries: 3
YAML

podman volume create "$external_volume" > "$evidence/external-volume-create.txt"
podman run --detach --name "$external" --volume "$external_volume:/external" \
  "$old_image" > "$evidence/external-create.txt"
external_id="$(podman inspect --format '{{.Id}}' "$external")"
podman volume inspect "$external_volume" > "$evidence/external-volume-before.json"

podman run --detach --name "$target" \
  --label dev.orisema.managed=true \
  --label dev.orisema.app=alpha5-apply \
  --label dev.orisema.service=web \
  --security-opt no-new-privileges \
  --publish "127.0.0.1:$port:80/tcp" \
  --health-cmd "[\"CMD-SHELL\",\"$health_command\"]" \
  --health-interval 30s --health-timeout 5s --health-retries 3 \
  "$old_image" > "$evidence/old-create.txt"
old_id="$(podman inspect --format '{{.Id}}' "$target")"
podman inspect "$target" > "$evidence/state-before.json"

"$binary" update prepare "$manifest" web --journal "$journal" | tee "$evidence/prepare.txt"
operation_id="$(awk '/^Update intent persisted:/ {print $4}' "$evidence/prepare.txt")"
test -n "$operation_id"
printf 'AUTHORIZE UPDATE alpha5-apply/web\n' | \
  "$binary" update authorize "$manifest" web --journal "$journal" --operation "$operation_id" | tee "$evidence/authorize.txt"
printf 'APPLY UPDATE alpha5-apply/web\n' | \
  "$binary" update apply "$manifest" web --journal "$journal" --operation "$operation_id" | tee "$evidence/apply.txt"

backup="orisema-rollback-${operation_id#update-}"
new_id="$(podman inspect --format '{{.Id}}' "$target")"
test "$new_id" != "$old_id"
test "$(podman inspect --format '{{.Config.Image}}' "$target")" = "$new_image"
test "$(podman inspect --format '{{.State.Running}}' "$target")" = true
test "$(podman inspect --format '{{.Id}}' "$backup")" = "$old_id"
test "$(podman inspect --format '{{.Config.Image}}' "$backup")" = "$old_image"
test "$(podman inspect --format '{{.State.Running}}' "$backup")" = false
podman inspect "$target" > "$evidence/candidate.json"
podman inspect "$backup" > "$evidence/preserved-previous.json"
if [ "$expected_decision" = COMPLETE ]; then
  podman healthcheck run "$target" > "$evidence/candidate-health.txt"
  "$binary" update verify "$manifest" web --journal "$journal" \
    --operation "$operation_id" | tee "$evidence/verify.txt"
  expected_state=COMPLETED
else
  test "$expected_decision" = RECOVERY_REQUIRED
  if [ "$force_candidate_stop" = true ]; then
    podman stop --time 0 "$target" > "$evidence/candidate-stop.txt"
    "$binary" update verify "$manifest" web --journal "$journal" \
      --operation "$operation_id" | tee "$evidence/verify.txt"
  else
    for probe in 1 2 3; do
      if podman healthcheck run "$target" >> "$evidence/candidate-health.txt" 2>&1; then
        echo 'unhealthy laboratory probe unexpectedly succeeded' >&2
        exit 1
      fi
    done
    for attempt in 1 2 3; do
      "$binary" update verify "$manifest" web --journal "$journal" \
        --operation "$operation_id" | tee -a "$evidence/verify.txt"
    done
  fi
  expected_state=RECOVERY_REQUIRED
fi
grep -Fq "decision: $expected_decision" "$evidence/verify.txt"
grep -Fq "state=$expected_state" "$evidence/verify.txt"
grep -Fq "\"state\": \"$expected_state\"" "$journal/$operation_id.json"

if [ "$rollback_after_failure" = true ]; then
  test "$expected_state" = RECOVERY_REQUIRED
  failed="orisema-failed-${operation_id#update-}"
  printf 'ROLLBACK UPDATE alpha5-apply/web %s\n' "$operation_id" | \
    "$binary" update rollback "$manifest" web --journal "$journal" \
      --operation "$operation_id" | tee "$evidence/rollback.txt"
  test "$(podman inspect --format '{{.Id}}' "$target")" = "$old_id"
  test "$(podman inspect --format '{{.State.Running}}' "$target")" = true
  test "$(podman inspect --format '{{.Id}}' "$failed")" = "$new_id"
  test "$(podman inspect --format '{{.State.Running}}' "$failed")" = false
  podman healthcheck run "$target" > "$evidence/rollback-health.txt"
  "$binary" update rollback-verify "$manifest" web --journal "$journal" \
    --operation "$operation_id" | tee "$evidence/rollback-verify.txt"
  grep -Fq 'decision=ROLLED_BACK' "$evidence/rollback-verify.txt"
  grep -Fq '"state": "ROLLED_BACK"' "$journal/$operation_id.json"
  expected_state=ROLLED_BACK
fi

test "$(podman inspect --format '{{.Id}}' "$external")" = "$external_id"
test "$(podman inspect --format '{{.State.Running}}' "$external")" = true
podman volume inspect "$external_volume" > "$evidence/external-volume-after.json"
cmp "$evidence/external-volume-before.json" "$evidence/external-volume-after.json"

"$binary" operation diagnose --journal "$journal" | tee "$evidence/diagnosis.txt"
if [ "$expected_state" = COMPLETED ] || [ "$expected_state" = ROLLED_BACK ]; then
  grep -Fq '  none' "$evidence/diagnosis.txt"
else
  grep -Fq 'state=RECOVERY_REQUIRED' "$evidence/diagnosis.txt"
  grep -Fq 'verification decision: RECOVERY_REQUIRED -> RECOVERY_REQUIRED' "$evidence/diagnosis.txt"
fi
printf 'Alpha 5 preserved update and %s health decision passed.\n' \
  "$expected_decision" | tee "$evidence/result.txt"
