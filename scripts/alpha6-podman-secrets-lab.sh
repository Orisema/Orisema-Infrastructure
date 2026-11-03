#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
binary=${ORISEMA_BINARY:-"$repo_root/work/orisema"}
evidence_dir=${ORISEMA_EVIDENCE_DIR:-"$repo_root/work/alpha6-podman-secrets"}
manifest="$repo_root/examples/alpha6-secret-references.yaml"
secret_name=alpha6-example-token
container_name=orisema-alpha6-secrets-lab-web
canary="alpha6-canary-${RANDOM}-${RANDOM}-must-not-leak"

mkdir -p "$evidence_dir"
chmod 0700 "$evidence_dir"

cleanup() {
  podman rm --force "$container_name" >/dev/null 2>&1 || true
  podman secret rm "$secret_name" >/dev/null 2>&1 || true
}
trap cleanup EXIT

if podman secret exists "$secret_name" || podman container exists "$container_name"; then
  echo "isolated Alpha 6 resource already exists; refusing to adopt it" >&2
  exit 1
fi

printf '%s' "$canary" | podman secret create \
  --label dev.orisema.lab=alpha6 "$secret_name" - >/dev/null

"$binary" validate "$manifest" >"$evidence_dir/validate.txt"
"$binary" plan "$manifest" >"$evidence_dir/plan-before.txt"
grep -Fq '[CREATE]' "$evidence_dir/plan-before.txt"

printf 'DEPLOY\n' | "$binary" deploy "$manifest" >"$evidence_dir/deploy.txt"
podman exec "$container_name" stat -c \
  'mode=%a uid=%u gid=%g size=%s' /run/secrets/example-token \
  >"$evidence_dir/secret-stat.txt"
grep -Eq '^mode=444 uid=0 gid=0 size=[1-9][0-9]*$' "$evidence_dir/secret-stat.txt"

podman inspect --format \
  '{{ index .Config.Labels "dev.orisema.secret.example-token" }}' \
  "$container_name" >"$evidence_dir/secret-label.txt"
grep -Eq '^podman:[a-f0-9]{25,64}$' "$evidence_dir/secret-label.txt"

"$binary" plan "$manifest" >"$evidence_dir/plan-after.txt"
grep -Fq '[NOOP]' "$evidence_dir/plan-after.txt"
if grep -FRq -- "$canary" "$evidence_dir"; then
  echo "secret canary leaked into laboratory evidence" >&2
  exit 1
fi

echo "Alpha 6 Podman secret delivery laboratory passed."
