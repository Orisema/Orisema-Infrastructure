#!/bin/sh
# Copyright 2026 Orisema contributors
# SPDX-License-Identifier: Apache-2.0
# shellcheck disable=SC2016 # Single-quoted lines generate fixture scripts.

set -eu

repository_root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
lab_root=$(mktemp -d "${TMPDIR:-/tmp}/orisema-installer-lab.XXXXXX")
cleanup() { rm -rf -- "$lab_root"; }
trap cleanup EXIT HUP INT TERM

case "$(uname -m)" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) echo "installer lab: unsupported architecture" >&2; exit 1 ;;
esac

version=v0.1.0-alpha.9
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

run_installer() {
  PATH="$fakebin:$PATH" HOME="$home_dir" XDG_BIN_HOME="$home_dir/.local/bin" \
    INSTALLER_FIXTURES="$fixtures" INSTALLER_LAB_LOG="$log" \
    INSTALLER_CANDIDATE_STATE="$lab_root/candidate-state" \
    sh "$repository_root/install.sh" --version "$version" --user
}

publish_fixture normal
: >"$log"
run_installer
"$home_dir/.local/bin/orisema" version | grep -Fq "Version: $version"
test "$(sed -n '4p' "$log")" = cosign
test "$(sed -n '5p' "$log")" = sha256sum
test "$(sed -n '6p' "$log")" = gh
test "$(sed -n '7p' "$log")" = tar

cp "$fixtures/$archive" "$lab_root/original-archive"
printf '%s\n' tampered >>"$fixtures/$archive"
rm -rf -- "$home_dir/.local/bin"
: >"$log"
if run_installer >"$lab_root/tamper.out" 2>&1; then
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
printf '%s\n' '#!/bin/sh' 'echo "Version: v0.1.0-alpha.8"' \
  "echo \"OS/Arch: linux/$arch\"" >"$home_dir/.local/bin/orisema"
chmod 0755 "$home_dir/.local/bin/orisema"
publish_fixture fail_after_first
: >"$lab_root/candidate-state"
: >"$log"
if run_installer >"$lab_root/rollback.out" 2>&1; then
  echo "installer lab: invalid post-install candidate was accepted" >&2
  exit 1
fi
"$home_dir/.local/bin/orisema" version | grep -Fq 'Version: v0.1.0-alpha.8'
if ! grep -Fq 'previous executable was restored' "$lab_root/rollback.out"; then
  echo "installer lab: rollback diagnostic was not emitted" >&2
  sed -n '1,20p' "$lab_root/rollback.out" >&2
  exit 1
fi

echo "Alpha 9 installer lab passed: verified install, tamper rejection and rollback"
