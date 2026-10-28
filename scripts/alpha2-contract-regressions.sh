#!/usr/bin/env bash
# Copyright 2026 Orisema contributors
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
binary="${ORISEMA_BINARY:-$root/work/orisema}"
evidence="${ORISEMA_EVIDENCE_DIR:-$root/work/alpha2-contract}"
manifest="$root/testdata/alpha2/loopback.yaml"
target=orisema-alpha2-bind-web

mkdir -p "$evidence"
cleanup() { podman rm --force "$target" >/dev/null 2>&1 || true; }
trap cleanup EXIT
cleanup

"$binary" validate "$manifest" | tee "$evidence/validate.txt"
for invalid in public-bind unknown-environment v1alpha1-hostip; do
  if "$binary" validate "$root/testdata/alpha2/$invalid.yaml" \
      > "$evidence/rejected-$invalid.txt" 2>&1; then
    echo "invalid contract fixture unexpectedly accepted: $invalid" >&2
    exit 1
  fi
done
grep -Fq 'explicit loopback IP' "$evidence/rejected-public-bind.txt"
grep -Fq 'field environment not found' "$evidence/rejected-unknown-environment.txt"
grep -Fq 'hostIP is not part of v1alpha1' "$evidence/rejected-v1alpha1-hostip.txt"

"$binary" plan "$manifest" | tee "$evidence/plan-before.txt"
grep -Fq '[CREATE]' "$evidence/plan-before.txt"
printf 'DEPLOY\n' | "$binary" deploy "$manifest" | tee "$evidence/deploy.txt"
host_ip="$(podman inspect --format '{{(index (index .HostConfig.PortBindings "80/tcp") 0).HostIP}}' "$target")"
test "$host_ip" = 127.0.0.1
printf 'HostIP=%s\n' "$host_ip" | tee "$evidence/inspect-bind.txt"
curl --fail --head --retry 5 --retry-delay 1 http://127.0.0.1:18085/ \
  | tee "$evidence/http-head.txt"
"$binary" plan "$manifest" | tee "$evidence/plan-after.txt"
grep -Fq '[NOOP]' "$evidence/plan-after.txt"
printf 'Alpha 2 contract regressions passed.\n' | tee "$evidence/result.txt"
