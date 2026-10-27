#!/usr/bin/env bash
# Copyright 2026 Orisema contributors
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

output_dir="${OUTPUT_DIR:-dist}"
expected_version="${VERSION:-}"
expected_commit="${COMMIT:-}"

if [[ -z "$expected_version" || -z "$expected_commit" ]]; then
  echo "VERSION and COMMIT are required" >&2
  exit 1
fi

(
  cd "$output_dir"
  sha256sum --check checksums.txt
)

for arch in amd64 arm64; do
  archive="$output_dir/orisema_linux_$arch.tar.gz"
  listing="$(tar -tzf "$archive")"
  if [[ "$listing" != "orisema" ]]; then
    echo "$archive must contain exactly the orisema executable" >&2
    exit 1
  fi
  extracted="$output_dir/verified_linux_$arch"
  mkdir -p "$extracted"
  tar -xzf "$archive" -C "$extracted"
  binary="$extracted/orisema"
  test -x "$binary"
  metadata="$(go version -m "$binary")"
  grep -Fq $'build\tGOOS=linux' <<<"$metadata"
  grep -Fq $'build\tGOARCH='"$arch" <<<"$metadata"
done

if [[ "$(go env GOOS)" == "linux" && "$(go env GOARCH)" == "amd64" ]]; then
  report="$($output_dir/verified_linux_amd64/orisema version)"
  grep -Fq "Version: $expected_version" <<<"$report"
  grep -Fq "Commit: $expected_commit" <<<"$report"
  grep -Fq 'OS/Arch: linux/amd64' <<<"$report"
fi
