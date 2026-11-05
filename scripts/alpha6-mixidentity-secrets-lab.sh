#!/usr/bin/env bash
set -euo pipefail
umask 077

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
binary=${ORISEMA_BINARY:-"$repo_root/work/orisema"}
evidence_dir=${ORISEMA_EVIDENCE_DIR:-"$repo_root/work/alpha6-mixidentity-secrets"}
image=${MIXIDENTITY_IMAGE:-localhost/mix-identity-web:1.0.0}
secret_name=alpha6-mixidentity-artificial-token
container_name=orisema-alpha6-mixidentity-lab-web
manifest="$evidence_dir/manifest.yaml"
canary="alpha6-mixidentity-canary-${RANDOM}-${RANDOM}-must-not-leak"

mkdir -p "$evidence_dir"
chmod 0700 "$evidence_dir"

cleanup() {
  podman rm --force "$container_name" >/dev/null 2>&1 || true
  podman secret rm "$secret_name" >/dev/null 2>&1 || true
}
trap cleanup EXIT

if podman secret exists "$secret_name" || podman container exists "$container_name"; then
  echo "isolated Alpha 6 MixIdentity resource already exists; refusing to adopt it" >&2
  exit 1
fi
if ! podman image exists "$image"; then
  echo "MixIdentity laboratory image not found: $image" >&2
  exit 1
fi

cat >"$manifest" <<EOF
apiVersion: orisema.dev/v1alpha3
kind: Application
name: alpha6-mixidentity-lab
secrets:
  - name: artificial-token
    source:
      podman:
        name: $secret_name
services:
  - name: web
    image: $image
    secrets:
      - name: artificial-token
        target: artificial-token
EOF

printf '%s' "$canary" | "$binary" secret create "$secret_name" \
  --provider podman --stdin >"$evidence_dir/create.txt"
"$binary" validate "$manifest" >"$evidence_dir/validate.txt"
"$binary" plan "$manifest" >"$evidence_dir/plan-before.txt"
grep -Fq '[CREATE]' "$evidence_dir/plan-before.txt"
journal="$evidence_dir/operations"
"$binary" operation init --journal "$journal" >"$evidence_dir/journal-init.txt"
printf 'DEPLOY\n' | "$binary" deploy --journal "$journal" "$manifest" >"$evidence_dir/deploy.txt"

podman inspect --format '{{.ImageName}}' "$container_name" \
  >"$evidence_dir/image.txt"
grep -Fxq "$image" "$evidence_dir/image.txt"
podman exec "$container_name" stat -c \
  'mode=%a uid=%u gid=%g size=%s' /run/secrets/artificial-token \
  >"$evidence_dir/secret-stat.txt"
grep -Eq '^mode=444 uid=0 gid=0 size=[1-9][0-9]*$' "$evidence_dir/secret-stat.txt"
"$binary" plan "$manifest" >"$evidence_dir/plan-after.txt"
grep -Fq '[NOOP]' "$evidence_dir/plan-after.txt"

if grep -FRq -- "$canary" "$evidence_dir"; then
  echo "MixIdentity secret canary leaked into laboratory evidence" >&2
  exit 1
fi

echo "Alpha 6 MixIdentity artificial-secret laboratory passed."
