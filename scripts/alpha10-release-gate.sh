#!/usr/bin/env bash
# Copyright 2026 Orisema contributors
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

root="$(cd -- "$(dirname -- "$0")/.." && pwd)"
version="${VERSION:?VERSION is required}"
commit="${COMMIT:?COMMIT is required}"
evidence="${EVIDENCE_DIR:?EVIDENCE_DIR is required}"

[[ "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]] || {
  echo "release gate: invalid VERSION" >&2
  exit 1
}
[[ "$commit" =~ ^[0-9a-f]{40}$ ]] || {
  echo "release gate: COMMIT must be a full lowercase SHA" >&2
  exit 1
}
[[ ! -e "$evidence" ]] || {
  echo "release gate: EVIDENCE_DIR must not already exist" >&2
  exit 1
}
[[ "$(git -C "$root" rev-parse HEAD)" == "$commit" ]] || {
  echo "release gate: checkout does not match COMMIT" >&2
  exit 1
}
[[ -z "$(git -C "$root" status --porcelain --untracked-files=all)" ]] || {
  echo "release gate: checkout must be clean, including untracked files" >&2
  exit 1
}

mkdir -m 0700 -p "$evidence"
dist="$evidence/dist"

{
  printf 'version=%s\ncommit=%s\n' "$version" "$commit"
  printf 'date_utc=%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  printf 'kernel=%s\narchitecture=%s\n' "$(uname -sr)" "$(uname -m)"
  if [[ -r /etc/os-release ]]; then
    sed -n 's/^\(ID\|VERSION_ID\|PRETTY_NAME\)=/os_\1=/p' /etc/os-release
  fi
  go version
  if command -v podman >/dev/null 2>&1; then
    podman version
    podman info --format 'podman_rootless={{.Host.Security.Rootless}}'
  else
    echo 'podman=unavailable (distribution-only gate)'
  fi
} >"$evidence/environment.txt"

cd "$root"
go mod download
go vet ./... 2>&1 | tee "$evidence/vet.txt"
go test ./... 2>&1 | tee "$evidence/tests.txt"
go test -race ./... 2>&1 | tee "$evidence/race.txt"

VERSION="$version" COMMIT="$commit" OUTPUT_DIR="$dist" scripts/build-release.sh
VERSION="$version" COMMIT="$commit" OUTPUT_DIR="$dist" scripts/verify-release.sh \
  2>&1 | tee "$evidence/archive-verification.txt"
ORISEMA_LAB_VERSION="$version" scripts/alpha9-installer-lab.sh \
  2>&1 | tee "$evidence/installer-lifecycle.txt"

(cd "$dist" && sha256sum --check checksums.txt) >"$evidence/checksum-recheck.txt"
find "$dist" -maxdepth 1 -type f -printf '%f %s bytes\n' | sort \
  >"$evidence/artifacts.txt"
printf 'alpha10_release_gate=passed\n' >"$evidence/result.txt"

