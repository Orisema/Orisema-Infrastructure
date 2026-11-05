#!/usr/bin/env bash
# Copyright 2026 Orisema contributors
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail
umask 077

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
binary=${ORISEMA_BINARY:-"$repo_root/work/orisema"}
evidence=${ORISEMA_EVIDENCE_DIR:-"$repo_root/work/alpha8-configured-rollback"}
old_image=${ORISEMA_OLD_IMAGE:-docker.io/library/nginx:1.26-alpine}
new_image=${ORISEMA_NEW_IMAGE:-docker.io/library/nginx:1.27-alpine}
app=alpha8-configured-rollback
target=orisema-alpha8-configured-rollback-web
network=alpha8-configured-rollback-private
source_dir=$(mktemp -d "${HOME}/.orisema-alpha8-configured.XXXXXX")
source_file="$source_dir/token"
old_manifest="$evidence/old.yaml"
manifest="$evidence/manifest.yaml"
journal="$evidence/journal"
backup=
failed=
canary="alpha8-file-canary-${RANDOM}-${RANDOM}-must-not-leak"

mkdir -p "$evidence"
chmod 0700 "$evidence" "$source_dir"

cleanup() {
  podman rm --force --time 0 "$target" >/dev/null 2>&1 || true
  if [ -n "$backup" ]; then
    podman rm --force --time 0 "$backup" >/dev/null 2>&1 || true
  fi
  if [ -n "$failed" ]; then
    podman rm --force --time 0 "$failed" >/dev/null 2>&1 || true
  fi
  podman network rm "$network" >/dev/null 2>&1 || true
  find "$source_dir" -type f -delete 2>/dev/null || true
  rmdir "$source_dir" 2>/dev/null || true
}
trap cleanup EXIT
cleanup
mkdir -p "$source_dir"
chmod 0700 "$source_dir"

printf '%s' "$canary" >"$source_file"
chmod 0600 "$source_file"
podman network create "$network" >"$evidence/network-create.txt"

write_manifest() {
  local image=$1 output=$2
  sed -e "s|LAB_IMAGE|$image|" -e "s|SECRET_FILE|$source_file|" >"$output" <<'YAML'
apiVersion: orisema.dev/v1alpha4
kind: Application
name: alpha8-configured-rollback
secrets:
  - name: token
    source:
      file:
        path: SECRET_FILE
networks:
  - name: private
    source:
      podman:
        name: alpha8-configured-rollback-private
services:
  - name: web
    image: LAB_IMAGE
    publicEnvironment:
      - name: APP_MODE
        value: alpha8-laboratory
    secrets:
      - name: token
        target: token
    networks:
      - name: private
    healthcheck:
      command: nginx -t
      interval: 5s
      timeout: 2s
      retries: 2
YAML
}

write_manifest "$old_image" "$old_manifest"
write_manifest "$new_image" "$manifest"
printf 'DEPLOY\n' | "$binary" deploy "$old_manifest" >"$evidence/deploy.txt"
old_id=$(podman inspect --format '{{.Id}}' "$target")

"$binary" update prepare "$manifest" web --journal "$journal" >"$evidence/prepare.txt"
operation_id=$(awk '/^Update intent persisted:/ {print $4}' "$evidence/prepare.txt")
test -n "$operation_id"
printf 'AUTHORIZE UPDATE %s/web\n' "$app" | \
  "$binary" update authorize "$manifest" web --journal "$journal" \
    --operation "$operation_id" >"$evidence/authorize.txt"
printf 'APPLY UPDATE %s/web\n' "$app" | \
  "$binary" update apply "$manifest" web --journal "$journal" \
    --operation "$operation_id" >"$evidence/apply.txt"

backup="orisema-rollback-${operation_id#update-}"
new_id=$(podman inspect --format '{{.Id}}' "$target")
test "$new_id" != "$old_id"
test "$(podman inspect --format '{{.Id}}' "$backup")" = "$old_id"
test "$(podman inspect --format '{{.State.Running}}' "$backup")" = false
test "$(podman inspect --format '{{range .Config.Env}}{{println .}}{{end}}' "$target" | grep -c '^APP_MODE=alpha8-laboratory$')" = 1
test "$(podman inspect --format '{{range $name, $_ := .NetworkSettings.Networks}}{{println $name}}{{end}}' "$target" | grep -c "^${network}$")" = 1
test "$(podman exec "$target" sha256sum /run/secrets/token | awk '{print $1}')" = \
  "$(sha256sum "$source_file" | awk '{print $1}')"
if podman secret ls --format '{{.Name}}' | grep -Eq '^orisema-transient-'; then
  echo 'transient secret remained after configured candidate creation' >&2
  exit 1
fi

podman stop --time 0 "$target" >"$evidence/candidate-stop.txt"
"$binary" update verify "$manifest" web --journal "$journal" \
  --operation "$operation_id" >"$evidence/verify.txt"
grep -Fq 'decision: RECOVERY_REQUIRED' "$evidence/verify.txt"

failed="orisema-failed-${operation_id#update-}"
printf 'ROLLBACK UPDATE %s/web %s\n' "$app" "$operation_id" | \
  "$binary" recover operation "$manifest" --journal "$journal" \
    --operation "$operation_id" >"$evidence/rollback.txt"
test "$(podman inspect --format '{{.Id}}' "$target")" = "$old_id"
test "$(podman inspect --format '{{.State.Running}}' "$target")" = true
test "$(podman inspect --format '{{.Id}}' "$failed")" = "$new_id"
test "$(podman inspect --format '{{.State.Running}}' "$failed")" = false
podman healthcheck run "$target" >"$evidence/rollback-health.txt"
"$binary" update rollback-verify "$manifest" web --journal "$journal" \
  --operation "$operation_id" >"$evidence/rollback-verify.txt"
grep -Fq 'decision=ROLLED_BACK' "$evidence/rollback-verify.txt"
grep -Fq '"state": "ROLLED_BACK"' "$journal/$operation_id.json"

if grep -FRq -- "$canary" "$evidence"; then
  echo 'file secret canary leaked into Alpha 8 evidence' >&2
  exit 1
fi
printf 'Alpha 8 configured update failure and rollback laboratory passed.\n' | tee "$evidence/result.txt"
