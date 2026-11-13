#!/bin/sh
# Copyright 2026 Orisema contributors
# SPDX-License-Identifier: Apache-2.0
# shellcheck disable=SC2016 # Single-quoted lines generate fixture scripts.

set -eu

repository_root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
lab_root=$(mktemp -d "${TMPDIR:-/tmp}/orisema-installer-lab.XXXXXX")
system_created=false
cleanup() {
  if [ "$system_created" = true ]; then PATH="$fakebin:$PATH" sudo rm -f -- /usr/local/bin/orisema; fi
  rm -rf -- "$lab_root"
}
trap cleanup EXIT HUP INT TERM

case "$(uname -m)" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) echo "installer lab: unsupported architecture" >&2; exit 1 ;;
esac

version=${ORISEMA_LAB_VERSION:-v0.1.0-beta.1}
newer_version=v999.0.0-installer-test
case "$version" in
  v[0-9]*.[0-9]*.[0-9]*-*) ;;
  *) echo "installer lab: invalid ORISEMA_LAB_VERSION" >&2; exit 1 ;;
esac
archive=orisema_linux_${arch}.tar.gz
fixtures=$lab_root/fixtures
fakebin=$lab_root/fakebin
home_dir=$lab_root/home
log=$lab_root/verification.log
mkdir -p "$fixtures" "$fakebin" "$home_dir"

make_candidate() {
  candidate_path=$1
  candidate_mode=$2
  printf '%s\n' '#!/bin/sh' >"$candidate_path"
  if [ "$candidate_mode" = fail_after_first ]; then
    printf '%s\n' \
      'state=${INSTALLER_CANDIDATE_STATE:?}' \
      'count=$(cat "$state" 2>/dev/null || printf 0)' \
      'count=$((count + 1))' \
      'printf "%s\n" "$count" >"$state"' \
      'if [ "$count" -gt 1 ]; then echo "broken candidate"; exit 1; fi' \
      >>"$candidate_path"
  fi
  printf '%s\n' \
    'echo "Orisema Infrastructure"' \
    "echo \"Version: $version\"" \
    'echo "Commit: installer-lab"' \
    "echo \"OS/Arch: linux/$arch\"" \
    >>"$candidate_path"
  chmod 0755 "$candidate_path"
}

publish_fixture() {
  mode=$1
  stage=$lab_root/stage
  rm -rf -- "$stage"
  mkdir -p "$stage"
  make_candidate "$stage/orisema" "$mode"
  tar -czf "$fixtures/$archive" -C "$stage" orisema
  (cd "$fixtures" && sha256sum "$archive" >checksums.txt)
  printf '%s\n' '{"installer_lab":true}' >"$fixtures/checksums.txt.sigstore.json"
}

write_wrapper() {
  wrapper=$1
  real_command=$2
  printf '%s\n' '#!/bin/sh' "echo '$wrapper' >>\"\${INSTALLER_LAB_LOG:?}\"" \
    "exec $real_command \"\$@\"" >"$fakebin/$wrapper"
  chmod 0755 "$fakebin/$wrapper"
}

printf '%s\n' '#!/bin/sh' \
  'echo curl >>"${INSTALLER_LAB_LOG:?}"' \
  'output=' 'url=' \
  'while [ "$#" -gt 0 ]; do' \
  '  case "$1" in' \
  '    --output) output=$2; shift 2 ;;' \
  '    http*) url=$1; shift ;;' \
  '    *) shift ;;' \
  '  esac' \
  'done' \
  'cp "${INSTALLER_FIXTURES:?}/${url##*/}" "$output"' \
  >"$fakebin/curl"
chmod 0755 "$fakebin/curl"

printf '%s\n' '#!/bin/sh' 'echo cosign >>"${INSTALLER_LAB_LOG:?}"' 'exit 0' >"$fakebin/cosign"
printf '%s\n' '#!/bin/sh' 'echo gh >>"${INSTALLER_LAB_LOG:?}"' 'exit 0' >"$fakebin/gh"
chmod 0755 "$fakebin/cosign" "$fakebin/gh"
write_wrapper sha256sum /usr/bin/sha256sum
write_wrapper tar /usr/bin/tar
if [ "${ORISEMA_SYSTEM_LAB:-0}" = 1 ] && [ "$(id -u)" = 0 ]; then
  printf '%s\n' '#!/bin/sh' 'exec "$@"' >"$fakebin/sudo"
  chmod 0755 "$fakebin/sudo"
fi

run_installer() {
  PATH="$fakebin:$PATH" HOME="$home_dir" XDG_BIN_HOME="$home_dir/.local/bin" \
    INSTALLER_FIXTURES="$fixtures" INSTALLER_LAB_LOG="$log" \
    INSTALLER_CANDIDATE_STATE="$lab_root/candidate-state" \
    sh "$repository_root/install.sh" --version "$version" "$@"
}

publish_fixture normal
: >"$log"
run_installer --user
"$home_dir/.local/bin/orisema" version | grep -Fq "Version: $version"
test "$(sed -n '4p' "$log")" = cosign
test "$(sed -n '5p' "$log")" = sha256sum
test "$(sed -n '6p' "$log")" = gh
test "$(sed -n '7p' "$log")" = tar

printf '%s\n' '#!/bin/sh' 'echo "Orisema Infrastructure"' \
  "echo \"Version: $newer_version\"" "echo \"OS/Arch: linux/$arch\"" \
  >"$home_dir/.local/bin/orisema"
chmod 0755 "$home_dir/.local/bin/orisema"
if run_installer --user >"$lab_root/downgrade.out" 2>&1; then
  echo "installer lab: downgrade without explicit authorization was accepted" >&2
  exit 1
fi
grep -Fq 'use --allow-downgrade explicitly' "$lab_root/downgrade.out"
run_installer --user --allow-downgrade
"$home_dir/.local/bin/orisema" version | grep -Fq "Version: $version"

cp "$fixtures/$archive" "$lab_root/original-archive"
printf '%s\n' tampered >>"$fixtures/$archive"
rm -rf -- "$home_dir/.local/bin"
: >"$log"
if run_installer --user >"$lab_root/tamper.out" 2>&1; then
  echo "installer lab: tampered archive was accepted" >&2
  exit 1
fi
grep -Fq 'FAILED' "$lab_root/tamper.out"
if grep -q '^tar$' "$log"; then
  echo "installer lab: archive was inspected after checksum failure" >&2
  exit 1
fi
mv "$lab_root/original-archive" "$fixtures/$archive"

mkdir -p "$home_dir/.local/bin"
chmod 0755 "$home_dir/.local/bin"
printf '%s\n' '#!/bin/sh' 'echo "Orisema Infrastructure"' 'echo "Version: v0.1.0-alpha.8"' \
  "echo \"OS/Arch: linux/$arch\"" >"$home_dir/.local/bin/orisema"
chmod 0755 "$home_dir/.local/bin/orisema"
publish_fixture fail_after_first
: >"$lab_root/candidate-state"
: >"$log"
if run_installer --user >"$lab_root/rollback.out" 2>&1; then
  echo "installer lab: invalid post-install candidate was accepted" >&2
  exit 1
fi
"$home_dir/.local/bin/orisema" version | grep -Fq 'Version: v0.1.0-alpha.8'
if ! grep -Fq 'previous executable was restored' "$lab_root/rollback.out"; then
  echo "installer lab: rollback diagnostic was not emitted" >&2
  sed -n '1,20p' "$lab_root/rollback.out" >&2
  exit 1
fi
cp "$home_dir/.local/bin/orisema" "$lab_root/old-orisema"

HOME="$home_dir" XDG_BIN_HOME="$home_dir/.local/bin" \
  sh "$repository_root/uninstall.sh" --user
[ ! -e "$home_dir/.local/bin/orisema" ]

printf '%s\n' untouched >"$lab_root/not-orisema"
ln -s "$lab_root/not-orisema" "$home_dir/.local/bin/orisema"
if HOME="$home_dir" XDG_BIN_HOME="$home_dir/.local/bin" \
  sh "$repository_root/uninstall.sh" --user >"$lab_root/link.out" 2>&1; then
  echo "installer lab: uninstaller accepted a symbolic link" >&2
  exit 1
fi
grep -Fq untouched "$lab_root/not-orisema"
rm -- "$home_dir/.local/bin/orisema"

if [ "${ORISEMA_SYSTEM_LAB:-0}" = 1 ]; then
  [ ! -e /usr/local/bin/orisema ] || {
    echo "installer lab: refusing to replace an existing system executable" >&2
    exit 1
  }
  publish_fixture normal
  : >"$log"
  run_installer --system
  system_created=true
  /usr/local/bin/orisema version | grep -Fq "Version: $version"
  PATH="$fakebin:$PATH" sh "$repository_root/uninstall.sh" --system
  system_created=false
  [ ! -e /usr/local/bin/orisema ]

  PATH="$fakebin:$PATH" sudo install -m 0755 "$lab_root/old-orisema" /usr/local/bin/orisema
  system_created=true
  publish_fixture fail_after_first
  : >"$lab_root/candidate-state"
  : >"$log"
  if run_installer --system >"$lab_root/system-rollback.out" 2>&1; then
    echo "installer lab: invalid privileged upgrade was accepted" >&2
    exit 1
  fi
  /usr/local/bin/orisema version | grep -Fq 'Version: v0.1.0-alpha.8'
  grep -Fq 'previous executable was restored' "$lab_root/system-rollback.out"
  PATH="$fakebin:$PATH" sh "$repository_root/uninstall.sh" --system
  system_created=false
fi

echo "Installer lifecycle lab passed: verified install, downgrade, tamper rejection and rollback"
