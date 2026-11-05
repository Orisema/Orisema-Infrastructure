#!/bin/sh
# Copyright 2026 Orisema contributors
# SPDX-License-Identifier: Apache-2.0

set -eu

repository=Orisema/Orisema-Infrastructure
release_base=https://github.com/Orisema/Orisema-Infrastructure/releases/download
issuer=https://token.actions.githubusercontent.com
identity_regexp='^https://github.com/Orisema/Orisema-Infrastructure/.github/workflows/release.yml@refs/(heads/main|tags/v[0-9].*)$'
version=
bin_dir=
reinstall=false

usage() {
  echo "Usage: install.sh --version <vX.Y.Z[-prerelease]> (--user | --bin-dir <absolute-path>) [--reinstall]" >&2
}

fail() {
  echo "orisema installer: $*" >&2
  exit 1
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --version)
      [ "$#" -ge 2 ] || { usage; exit 2; }
      version=$2
      shift 2
      ;;
    --user)
      [ -z "$bin_dir" ] || fail "choose only one installation destination"
      bin_dir=${XDG_BIN_HOME:-${HOME:?HOME is required}/.local/bin}
      shift
      ;;
    --bin-dir)
      [ "$#" -ge 2 ] || { usage; exit 2; }
      [ -z "$bin_dir" ] || fail "choose only one installation destination"
      bin_dir=$2
      shift 2
      ;;
    --reinstall)
      reinstall=true
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      usage
      exit 2
      ;;
  esac
done

if [ -z "$version" ] \
  || ! printf '%s\n' "$version" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$'; then
  fail "--version must be an explicit release tag"
fi
[ -n "$bin_dir" ] || fail "select --user or --bin-dir; privileged installation is not implemented yet"
case "$bin_dir" in
  /*) ;;
  *) fail "installation directory must be an absolute path" ;;
esac

[ "$(uname -s)" = Linux ] || fail "only Linux is supported"
case "$(uname -m)" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) fail "unsupported architecture" ;;
esac

for command_name in curl cosign gh sha256sum tar mktemp install grep stat id mv find; do
  command -v "$command_name" >/dev/null 2>&1 || fail "required command is unavailable: $command_name"
done

archive=orisema_linux_${arch}.tar.gz
temp_dir=$(mktemp -d "${TMPDIR:-/tmp}/orisema-install.XXXXXX")
chmod 0700 "$temp_dir"
cleanup() {
  rm -rf -- "$temp_dir"
}
trap cleanup EXIT HUP INT TERM

download() {
  asset=$1
  curl --proto '=https' --tlsv1.2 --location --fail --show-error --silent \
    --output "$temp_dir/$asset" "$release_base/$version/$asset"
}

download "$archive"
download checksums.txt
download checksums.txt.sigstore.json

cosign verify-blob "$temp_dir/checksums.txt" \
  --bundle "$temp_dir/checksums.txt.sigstore.json" \
  --certificate-oidc-issuer "$issuer" \
  --certificate-identity-regexp "$identity_regexp" >/dev/null

checksum_line=$(grep " $archive\$" "$temp_dir/checksums.txt" || true)
[ -n "$checksum_line" ] || fail "selected archive is absent from authenticated checksums"
(cd "$temp_dir" && printf '%s\n' "$checksum_line" | sha256sum --check --strict)

gh attestation verify "$temp_dir/$archive" --repo "$repository" >/dev/null

archive_entries=$(tar -tzf "$temp_dir/$archive")
[ "$archive_entries" = orisema ] || fail "archive must contain exactly one top-level orisema file"
tar -xzf "$temp_dir/$archive" -C "$temp_dir" -- orisema
[ -f "$temp_dir/orisema" ] && [ ! -L "$temp_dir/orisema" ] || fail "archive did not produce a regular binary"
chmod 0755 "$temp_dir/orisema"

candidate_report=$("$temp_dir/orisema" version) || fail "candidate binary did not execute"
printf '%s\n' "$candidate_report" | grep -Fq "Version: $version" || fail "candidate version does not match requested release"
printf '%s\n' "$candidate_report" | grep -Fq "OS/Arch: linux/$arch" || fail "candidate architecture does not match this host"

if [ -e "$bin_dir" ]; then
  [ -d "$bin_dir" ] && [ ! -L "$bin_dir" ] || fail "installation destination must be a real directory"
else
  install -d -m 0755 "$bin_dir"
fi
[ "$(stat -c %u "$bin_dir")" = "$(id -u)" ] || fail "installation destination must be owned by the current user"
find "$bin_dir" -maxdepth 0 -perm /022 | grep -q . && fail "installation destination must not be writable by group or others"

destination=$bin_dir/orisema
if [ -L "$destination" ] || { [ -e "$destination" ] && [ ! -f "$destination" ]; }; then
  fail "existing destination must be a regular file, not a link or special file"
fi
if [ -f "$destination" ]; then
  current_report=$("$destination" version 2>/dev/null || true)
  if printf '%s\n' "$current_report" | grep -Fq "Version: $version" && [ "$reinstall" != true ]; then
    fail "requested version is already installed; use --reinstall to replace it explicitly"
  fi
fi

staged=$(mktemp "$bin_dir/.orisema.install.XXXXXX")
backup=$bin_dir/.orisema.backup.$$
[ ! -e "$backup" ] || fail "temporary backup path already exists"
install -m 0755 "$temp_dir/orisema" "$staged"

had_previous=false
if [ -f "$destination" ]; then
  mv "$destination" "$backup"
  had_previous=true
fi
if ! mv "$staged" "$destination"; then
  [ "$had_previous" = false ] || mv "$backup" "$destination"
  fail "could not commit the new executable"
fi

installed_report=$("$destination" version 2>/dev/null || true)
if ! printf '%s\n' "$installed_report" | grep -Fq "Version: $version" \
  || ! printf '%s\n' "$installed_report" | grep -Fq "OS/Arch: linux/$arch"; then
  if [ "$had_previous" = true ]; then
    mv "$backup" "$destination"
    fail "post-install validation failed; previous executable was restored"
  else
    rm -f -- "$destination"
    fail "post-install validation failed; invalid executable was removed"
  fi
fi

if [ "$had_previous" = true ]; then
  rm -f -- "$backup"
fi
echo "Orisema $version installed at $destination"
echo "No shell configuration, containers, journals or application data were changed."
