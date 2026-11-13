#!/usr/bin/env bash
# Copyright 2026 Orisema contributors
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail
umask 077

root="$(cd -- "$(dirname -- "$0")/.." && pwd)"
version="${VERSION:?VERSION is required}"
commit="${COMMIT:?COMMIT is required}"
evidence="${EVIDENCE_DIR:?EVIDENCE_DIR is required}"

[[ "$version" == v0.1.0-beta.1 ]] || {
  echo "Beta 1 release gate: unexpected VERSION" >&2
  exit 1
}
[[ "$commit" =~ ^[0-9a-f]{40}$ ]] || {
  echo "Beta 1 release gate: COMMIT must be a full lowercase SHA" >&2
  exit 1
}
[[ ! -e "$evidence" ]] || {
  echo "Beta 1 release gate: EVIDENCE_DIR must not already exist" >&2
  exit 1
}
[[ "$(git -C "$root" rev-parse HEAD)" == "$commit" ]] || {
  echo "Beta 1 release gate: checkout does not match COMMIT" >&2
  exit 1
}
[[ -z "$(git -C "$root" status --porcelain --untracked-files=all)" ]] || {
  echo "Beta 1 release gate: checkout must be clean, including untracked files" >&2
  exit 1
}

mkdir -m 0700 -p "$evidence"
dist="$evidence/dist"
private_tmp="$evidence/tmp"
mkdir -m 0700 "$private_tmp"
export TMPDIR="$private_tmp"

{
  printf 'version=%s\ncommit=%s\n' "$version" "$commit"
  printf 'date_utc=%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  printf 'kernel=%s\narchitecture=%s\n' "$(uname -sr)" "$(uname -m)"
  if [[ -r /etc/os-release ]]; then
    sed -n 's/^\(ID\|VERSION_ID\|PRETTY_NAME\)=/os_\1=/p' /etc/os-release
  fi
  go version
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
printf 'beta1_release_gate=passed\n' >"$evidence/result.txt"
