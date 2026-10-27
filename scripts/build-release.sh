#!/usr/bin/env bash
# Copyright 2026 Orisema contributors
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

version="${VERSION:-}"
commit="${COMMIT:-}"
output_dir="${OUTPUT_DIR:-dist}"

if [[ ! "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]; then
  echo "VERSION must be a tag such as v0.1.0-alpha.1" >&2
  exit 1
fi
if [[ ! "$commit" =~ ^[0-9a-f]{7,40}$ ]]; then
  echo "COMMIT must be a 7-40 character lowercase hexadecimal revision" >&2
  exit 1
fi
if [[ -e "$output_dir" ]]; then
  echo "OUTPUT_DIR already exists: $output_dir" >&2
  exit 1
fi

source_date_epoch="${SOURCE_DATE_EPOCH:-$(git show -s --format=%ct "$commit")}" 
if [[ ! "$source_date_epoch" =~ ^[0-9]+$ ]]; then
  echo "SOURCE_DATE_EPOCH must be an integer Unix timestamp" >&2
  exit 1
fi
built="$(date -u -d "@$source_date_epoch" +%Y-%m-%dT%H:%M:%SZ)"

mkdir -p "$output_dir"
for arch in amd64 arm64; do
  staging="$output_dir/orisema_linux_$arch"
  mkdir -p "$staging"
  CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build \
    -trimpath \
    -ldflags "-s -w -buildid= -X github.com/Orisema/Orisema-Infrastructure/internal/buildinfo.version=$version -X github.com/Orisema/Orisema-Infrastructure/internal/buildinfo.commit=$commit -X github.com/Orisema/Orisema-Infrastructure/internal/buildinfo.built=$built" \
    -o "$staging/orisema" ./cmd/orisema
  chmod 0755 "$staging/orisema"
  tar --sort=name --mtime="@$source_date_epoch" --owner=0 --group=0 --numeric-owner \
    -C "$staging" -cf - orisema | gzip -n > "$output_dir/orisema_linux_$arch.tar.gz"
done

(
  cd "$output_dir"
  sha256sum orisema_linux_amd64.tar.gz orisema_linux_arm64.tar.gz > checksums.txt
)
