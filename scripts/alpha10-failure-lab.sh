#!/usr/bin/env bash
# Copyright 2026 Orisema contributors
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
binary="${ORISEMA_BINARY:-$root/work/orisema}"
evidence="${ORISEMA_EVIDENCE_DIR:-$root/work/alpha10-failure-lab}"
manifest="$root/testdata/alpha10/failure-boundary.yaml"
second_manifest="$root/testdata/alpha10/storage-pressure.yaml"
mountpoint="$(mktemp -d /tmp/orisema-alpha10-journal.XXXXXX)"
wrapper="$(mktemp -d /tmp/orisema-alpha10-bin.XXXXXX)"
real_podman="$(command -v podman)"
journal="$mountpoint/operations"
unrelated=orisema-alpha10-unrelated
deploy_pid=""
phase=setup

cleanup() {
  status=$?
  if [ "$status" -ne 0 ]; then
    printf 'phase=%s line=%s status=%s\n' "$phase" "${BASH_LINENO[0]:-unknown}" "$status" \
      > "$evidence/failure.txt" 2>/dev/null || true
  fi
  if [ -n "$deploy_pid" ]; then kill -9 "$deploy_pid" >/dev/null 2>&1 || true; fi
  if [ -f "$wrapper/run.pid" ]; then kill -9 "$(cat "$wrapper/run.pid")" >/dev/null 2>&1 || true; fi
  "$real_podman" rm --force "$unrelated" >/dev/null 2>&1 || true
  umount "$mountpoint" >/dev/null 2>&1 || true
  rm -rf "$mountpoint" "$wrapper"
}
trap cleanup EXIT

test "$(id -u)" -eq 0 || { echo 'Alpha 10 failure lab must run as root for its isolated tmpfs.' >&2; exit 2; }
test -x "$binary"
mkdir -p "$evidence"
mount -t tmpfs -o size=2m,mode=0700 tmpfs "$mountpoint"

cat > "$wrapper/podman" <<EOF
#!/bin/sh
set -eu
if [ "\${1:-}" = run ]; then
  echo "\$\$" > '$wrapper/run.pid'
  : > '$wrapper/run-reached'
  while :; do sleep 1; done
fi
exec '$real_podman' "\$@"
EOF
chmod 755 "$wrapper/podman"

"$binary" version > "$evidence/orisema-version.txt"
"$real_podman" version > "$evidence/podman-version.txt"
"$binary" operation init --journal "$journal" > "$evidence/journal-init.txt"

# A resource outside the manifest is a sentinel: no failure path may touch it.
"$real_podman" run --detach --name "$unrelated" docker.io/library/nginx:1.27 \
  > "$evidence/unrelated-create.txt"
unrelated_id="$("$real_podman" inspect --format '{{.Id}}' "$unrelated")"

# Kill the CLI after it durably records the pending create and enters Podman.
phase=abrupt-deploy
set +e
printf 'DEPLOY\n' | PATH="$wrapper:$PATH" "$binary" deploy --journal "$journal" "$manifest" \
  > "$evidence/abrupt-deploy.txt" 2>&1 &
deploy_pid=$!
set -e
for _ in $(seq 1 200); do
  [ -f "$wrapper/run-reached" ] && break
  sleep 0.05
done
test -f "$wrapper/run-reached"
grep -Rq '"pendingService": "web"' "$journal"
kill -9 "$deploy_pid"
wait "$deploy_pid" >/dev/null 2>&1 || true
deploy_pid=""
kill -9 "$(cat "$wrapper/run.pid")" >/dev/null 2>&1 || true

# A fresh process (the persistence boundary used by reboot recovery) must report
# the incomplete operation and must never infer that it can resume.
phase=fresh-process-diagnosis
"$binary" operation diagnose --journal "$journal" > "$evidence/after-restart-diagnosis.txt"
grep -Fq 'automatic resume safe: false' "$evidence/after-restart-diagnosis.txt"
grep -Fq 'intervention required: true' "$evidence/after-restart-diagnosis.txt"
cp "$journal"/*.json "$evidence/"

# Exhaust the isolated filesystem. Admission of another application must fail
# before a Podman run; the already committed snapshot must remain readable.
phase=storage-pressure
rm -f "$wrapper/run-reached" "$wrapper/run.pid"
dd if=/dev/zero of="$mountpoint/fill" bs=4096 status=none 2>/dev/null || true
if printf 'DEPLOY\n' | PATH="$wrapper:$PATH" "$binary" deploy --journal "$journal" "$second_manifest" \
    > "$evidence/storage-pressure.txt" 2>&1; then
  echo 'deploy unexpectedly succeeded with a full journal filesystem' >&2
  exit 1
fi
test ! -e "$wrapper/run-reached"
grep -Eq 'temporary file|space left|operation journal' "$evidence/storage-pressure.txt"
rm -f "$mountpoint/fill"
"$binary" operation diagnose --journal "$journal" > "$evidence/after-storage-recovery.txt"
grep -Fq 'automatic resume safe: false' "$evidence/after-storage-recovery.txt"

# Corrupt and insecure copies fail closed and are never repaired implicitly.
phase=corrupt-journal
cp -a "$journal" "$mountpoint/corrupt"
printf '{\n' > "$mountpoint/corrupt/broken.json"
chmod 0600 "$mountpoint/corrupt/broken.json"
if "$binary" operation diagnose --journal "$mountpoint/corrupt" \
    > "$evidence/corrupt-journal.txt" 2>&1; then
  echo 'corrupt journal unexpectedly passed diagnosis' >&2
  exit 1
fi
grep -Fq 'decode operation journal' "$evidence/corrupt-journal.txt"

cp -a "$journal" "$mountpoint/insecure"
phase=insecure-journal
chmod 0755 "$mountpoint/insecure"
if "$binary" operation diagnose --journal "$mountpoint/insecure" \
    > "$evidence/insecure-journal.txt" 2>&1; then
  echo 'insecure journal unexpectedly passed diagnosis' >&2
  exit 1
fi
grep -Fq 'permissions' "$evidence/insecure-journal.txt"
test "$(stat -c '%a' "$mountpoint/insecure")" = 755

test "$("$real_podman" inspect --format '{{.Id}}' "$unrelated")" = "$unrelated_id"
test "$("$real_podman" inspect --format '{{.State.Running}}' "$unrelated")" = true
phase=complete
printf '%s\n' \
  'abrupt-boundary=PASS' \
  'fresh-process-diagnosis=PASS' \
  'storage-pressure=PASS' \
  'corrupt-journal=PASS' \
  'insecure-journal=PASS' \
  'unrelated-resource=PASS' \
  > "$evidence/result.txt"
cat "$evidence/result.txt"
