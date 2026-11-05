#!/usr/bin/env bash
set -euo pipefail
umask 077

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
binary=${ORISEMA_BINARY:-"$repo_root/work/orisema"}
evidence_dir=${ORISEMA_EVIDENCE_DIR:-"$repo_root/work/alpha6-file-secrets"}
container_name=orisema-alpha6-file-secrets-lab-web
source_dir=$(mktemp -d "${HOME}/.orisema-alpha6-file-lab.XXXXXX")
source_file="$source_dir/token"
manifest="$evidence_dir/manifest.yaml"
canary="alpha6-file-canary-${RANDOM}-${RANDOM}-must-not-leak"

mkdir -p "$evidence_dir"
chmod 0700 "$evidence_dir" "$source_dir"

cleanup() {
  podman rm --force "$container_name" >/dev/null 2>&1 || true
  find "$source_dir" -type f -delete 2>/dev/null || true
  rmdir "$source_dir" 2>/dev/null || true
}
trap cleanup EXIT

if podman container exists "$container_name"; then
  echo "isolated Alpha 6 file-provider container already exists" >&2
  exit 1
fi

printf '%s' "$canary" >"$source_file"
chmod 0600 "$source_file"
cat >"$manifest" <<EOF
apiVersion: orisema.dev/v1alpha3
kind: Application
name: alpha6-file-secrets-lab
secrets:
  - name: file-token
    source:
      file:
        path: $source_file
services:
  - name: web
    image: docker.io/library/nginx:1.27-alpine
    secrets:
      - name: file-token
        target: file-token
EOF
chmod 0600 "$manifest"

"$binary" validate "$manifest" >"$evidence_dir/validate.txt"
"$binary" plan "$manifest" >"$evidence_dir/plan-before.txt"
grep -Fq '[CREATE]' "$evidence_dir/plan-before.txt"
journal="$evidence_dir/operations"
"$binary" operation init --journal "$journal" >"$evidence_dir/journal-init.txt"
printf 'DEPLOY\n' | "$binary" deploy --journal "$journal" "$manifest" >"$evidence_dir/deploy.txt"

podman exec "$container_name" stat -c \
  'mode=%a uid=%u gid=%g size=%s' /run/secrets/file-token \
  >"$evidence_dir/secret-stat.txt"
grep -Eq '^mode=444 uid=0 gid=0 size=[1-9][0-9]*$' "$evidence_dir/secret-stat.txt"
podman inspect --format \
  '{{ index .Config.Labels "dev.orisema.secret.file-token" }}' \
  "$container_name" >"$evidence_dir/secret-label.txt"
grep -Eq '^file:[a-f0-9-]+$' "$evidence_dir/secret-label.txt"

if podman secret ls --format '{{.Name}}' | grep -Eq '^orisema-transient-'; then
  echo "transient secret remained after container creation" >&2
  exit 1
fi
podman restart "$container_name" >/dev/null
podman exec "$container_name" stat -c 'size=%s' /run/secrets/file-token \
  >"$evidence_dir/secret-after-restart.txt"
"$binary" plan "$manifest" >"$evidence_dir/plan-after.txt"
grep -Fq '[NOOP]' "$evidence_dir/plan-after.txt"

if grep -FRq -- "$canary" "$evidence_dir"; then
  echo "file secret canary leaked into laboratory evidence" >&2
  exit 1
fi

echo "Alpha 6 file secret provider laboratory passed."
