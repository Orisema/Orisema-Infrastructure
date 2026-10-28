#!/usr/bin/env bash
# Copyright 2026 Orisema contributors
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

# Preserve the operator response on a dedicated descriptor so Podman commands
# cannot consume piped confirmation input.
exec 3<&0
exec </dev/null

evidence="${ORISEMA_EVIDENCE_DIR:-$PWD/work/alpha3-update}"
old_image="${ORISEMA_OLD_IMAGE:-docker.io/library/nginx:1.26-alpine}"
new_image="${ORISEMA_NEW_IMAGE:-docker.io/library/nginx:1.27-alpine}"
target=orisema-alpha3-update-web
backup=orisema-alpha3-update-web-rollback
external=orisema-alpha3-external
external_volume=orisema-alpha3-external-data
port=18088

mkdir -p "$evidence"

cleanup() {
  local item
  for item in "$target" "$backup" "$external"; do
    podman rm --force "$item" >/dev/null 2>&1 || true
  done
  podman volume rm "$external_volume" >/dev/null 2>&1 || true
}
trap cleanup EXIT
cleanup

owned() {
  local name=$1
  test "$(podman inspect --format '{{index .Config.Labels "dev.orisema.managed"}}' "$name")" = true
  test "$(podman inspect --format '{{index .Config.Labels "dev.orisema.app"}}' "$name")" = alpha3-update
  test "$(podman inspect --format '{{index .Config.Labels "dev.orisema.service"}}' "$name")" = web
}

run_managed() {
  local image=$1 health=$2
  podman run --detach --name "$target" \
    --label dev.orisema.managed=true \
    --label dev.orisema.app=alpha3-update \
    --label dev.orisema.service=web \
    --security-opt no-new-privileges \
    --publish "127.0.0.1:$port:80/tcp" \
    --health-cmd "[\"CMD-SHELL\",\"$health\"]" \
    --health-interval 30s --health-timeout 5s --health-retries 3 \
    "$image"
}

restore_backup() {
  if podman container exists "$target"; then
    owned "$target"
    podman rm --force "$target" >/dev/null
  fi
  owned "$backup"
  test "$(podman inspect --format '{{.Name}}' "$backup")" = "$backup"
  podman rename "$backup" "$target"
  podman start "$target" >/dev/null
  owned "$target"
}

attempt_update() {
  local expected_id=$1 expected_image=$2 next_image=$3 next_health=$4 label=$5

  # Revalidation is immediately adjacent to the first mutation.
  owned "$target"
  test "$(podman inspect --format '{{.Id}}' "$target")" = "$expected_id"
  test "$(podman inspect --format '{{.Config.Image}}' "$target")" = "$expected_image"
  test "$(podman inspect --format '{{(index (index .HostConfig.PortBindings "80/tcp") 0).HostIP}}' "$target")" = 127.0.0.1
  if podman container exists "$backup"; then
    echo 'rollback slot is already occupied' >&2
    return 1
  fi

  podman stop "$target" >/dev/null
  podman rename "$target" "$backup"
  if ! run_managed "$next_image" "$next_health" > "$evidence/$label-create.txt" 2>&1; then
    restore_backup
    return 1
  fi
  if ! podman healthcheck run "$target" > "$evidence/$label-health.txt" 2>&1; then
    restore_backup
    return 1
  fi
  test "$(podman inspect --format '{{.State.Running}}' "$target")" = true
  test "$(podman inspect --format '{{.State.Healthcheck.Status}}' "$target")" = healthy
}

# A lab-owned sentinel represents resources outside the update target.
podman volume create "$external_volume" > "$evidence/external-volume-create.txt"
podman run --detach --name "$external" --volume "$external_volume:/external" \
  "$old_image" > "$evidence/external-create.txt"
external_id="$(podman inspect --format '{{.Id}}' "$external")"
podman volume inspect "$external_volume" > "$evidence/external-volume-before.json"

run_managed "$old_image" 'nginx -t' > "$evidence/old-create.txt"
old_id="$(podman inspect --format '{{.Id}}' "$target")"
podman healthcheck run "$target" > "$evidence/old-health.txt"
podman inspect "$target" > "$evidence/state-before.json"

printf 'UPDATE web: %s -> %s; preserve %s as rollback slot\n' \
  "$old_image" "$new_image" "$backup" | tee "$evidence/update-plan.txt"

printf 'Type UPDATE web to continue: '
IFS= read -r answer <&3
exec 3<&-
if [ "$answer" != 'UPDATE web' ]; then
  echo 'cancelled; no update mutations made' >&2
  exit 1
fi

# A deliberately unhealthy candidate must restore the same old container.
if attempt_update "$old_id" "$old_image" "$new_image" false failed-update; then
  echo 'unhealthy candidate unexpectedly passed verification' >&2
  exit 1
fi
test "$(podman inspect --format '{{.Id}}' "$target")" = "$old_id"
test "$(podman inspect --format '{{.Config.Image}}' "$target")" = "$old_image"
printf 'automatic rollback restored %s\n' "$old_id" | tee "$evidence/failed-update-rollback.txt"

# A healthy update preserves the old container in the rollback slot.
attempt_update "$old_id" "$old_image" "$new_image" 'nginx -t' successful-update
new_id="$(podman inspect --format '{{.Id}}' "$target")"
test "$new_id" != "$old_id"
test "$(podman inspect --format '{{.Id}}' "$backup")" = "$old_id"
podman inspect "$target" > "$evidence/state-updated.json"

# Explicit application rollback removes only the verified new target and
# restores the exact old container. No data restore is attempted or claimed.
owned "$target"
test "$(podman inspect --format '{{.Id}}' "$target")" = "$new_id"
owned "$backup"
test "$(podman inspect --format '{{.Id}}' "$backup")" = "$old_id"
podman rm --force "$target" >/dev/null
podman rename "$backup" "$target"
podman start "$target" >/dev/null
podman healthcheck run "$target" > "$evidence/rollback-health.txt"
test "$(podman inspect --format '{{.Id}}' "$target")" = "$old_id"
test "$(podman inspect --format '{{.Config.Image}}' "$target")" = "$old_image"
podman inspect "$target" > "$evidence/state-rolled-back.json"

# The external sentinel and its mounted volume must retain identity and state.
test "$(podman inspect --format '{{.Id}}' "$external")" = "$external_id"
test "$(podman inspect --format '{{.State.Running}}' "$external")" = true
test "$(podman inspect --format '{{(index .Mounts 0).Name}}' "$external")" = "$external_volume"
podman volume inspect "$external_volume" > "$evidence/external-volume-after.json"
cmp "$evidence/external-volume-before.json" "$evidence/external-volume-after.json"

printf 'Alpha 3 update and rollback laboratory passed.\n' | tee "$evidence/result.txt"
