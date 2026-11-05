#!/usr/bin/env bash
set -euo pipefail
umask 077

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
binary=${ORISEMA_BINARY:-"$repo_root/work/orisema"}
evidence_dir=${ORISEMA_EVIDENCE_DIR:-"$repo_root/work/alpha6-secret-lifecycle"}
secret_name=alpha6-lifecycle-token
external_name=alpha6-external-token
container_name=orisema-alpha6-lifecycle-lab-web
manifest="$evidence_dir/manifest.yaml"
canary="alpha6-lifecycle-canary-${RANDOM}-${RANDOM}-must-not-leak"

mkdir -p "$evidence_dir"
chmod 0700 "$evidence_dir"

cleanup() {
  podman rm --force "$container_name" >/dev/null 2>&1 || true
  podman secret rm "$secret_name" >/dev/null 2>&1 || true
  podman secret rm "$external_name" >/dev/null 2>&1 || true
}
trap cleanup EXIT

if podman secret exists "$secret_name" || podman secret exists "$external_name" || podman container exists "$container_name"; then
  echo "isolated Alpha 6 lifecycle resource already exists" >&2
  exit 1
fi

cat >"$manifest" <<EOF
apiVersion: orisema.dev/v1alpha3
kind: Application
name: alpha6-lifecycle-lab
secrets:
  - name: lifecycle-token
    source:
      podman:
        name: $secret_name
services:
  - name: web
    image: docker.io/library/nginx:1.27-alpine
    secrets:
      - name: lifecycle-token
        target: lifecycle-token
EOF

printf '%s' "$canary" | "$binary" secret create "$secret_name" \
  --provider podman --stdin >"$evidence_dir/create.txt"
secret_id=$(sed -n 's/.* id=\([a-f0-9]\{25,64\}\) provider=.*/\1/p' "$evidence_dir/create.txt")
test -n "$secret_id"

"$binary" secret list >"$evidence_dir/list.txt"
"$binary" secret inspect "$secret_name" >"$evidence_dir/inspect.txt"
grep -Fq "name=$secret_name id=$secret_id" "$evidence_dir/list.txt"
grep -Fq "managed: true" "$evidence_dir/inspect.txt"

journal="$evidence_dir/operations"
"$binary" operation init --journal "$journal" >"$evidence_dir/journal-init.txt"
printf 'DEPLOY\n' | "$binary" deploy --journal "$journal" "$manifest" >"$evidence_dir/deploy.txt"
remove_status=0
"$binary" secret remove "$secret_name" >"$evidence_dir/remove-consumed.out" \
  2>"$evidence_dir/remove-consumed.err" || remove_status=$?
test "$remove_status" -eq 1
grep -Fq "consumed by containers: $container_name" "$evidence_dir/remove-consumed.err"

podman rm --force "$container_name" >/dev/null
printf 'REMOVE SECRET %s %s\n' "$secret_name" "$secret_id" | \
  "$binary" secret remove "$secret_name" >"$evidence_dir/remove.txt"
if podman secret exists "$secret_name"; then
  echo "managed lifecycle secret remained after confirmed removal" >&2
  exit 1
fi

printf 'external' | podman secret create "$external_name" - >/dev/null
external_status=0
"$binary" secret remove "$external_name" >"$evidence_dir/remove-external.out" \
  2>"$evidence_dir/remove-external.err" || external_status=$?
test "$external_status" -eq 1
grep -Fq 'ownership labels do not identify' "$evidence_dir/remove-external.err"
podman secret exists "$external_name"

if grep -FRq -- "$canary" "$evidence_dir"; then
  echo "lifecycle canary leaked into evidence" >&2
  exit 1
fi

echo "Alpha 6 secret lifecycle laboratory passed."
