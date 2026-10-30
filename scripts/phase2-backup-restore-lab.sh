#!/usr/bin/env bash
# Copyright 2026 Orisema contributors
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

binary="${ORISEMA_BINARY:-$PWD/work/orisema}"
evidence="${ORISEMA_EVIDENCE_DIR:-$PWD/work/phase2-backup-restore}"
mkdir -p "$evidence"
chmod 700 "$evidence"

manifest="$evidence/source.yaml"
artifact="$evidence/artifact"
restored="$evidence/restored"
journal="$evidence/backup-journal"
restore_journal="$evidence/restore-journal"

cat > "$manifest" <<'YAML'
apiVersion: orisema.dev/v1alpha2
kind: Application
name: backup-lab
services:
  - name: web
    image: docker.io/library/nginx:1.27-alpine
YAML
chmod 600 "$manifest"
printf 'unrelated sentinel\n' > "$evidence/sentinel.txt"
cp "$evidence/sentinel.txt" "$evidence/sentinel-before.txt"

"$binary" backup create "$manifest" --output "$artifact" --journal "$journal" | tee "$evidence/create.txt"
"$binary" backup verify "$artifact" | tee "$evidence/verify.txt"
backup_id="$(sed -n 's/.*"backupId": "\([^"]*\)".*/\1/p' "$artifact/metadata.json")"
test -n "$backup_id"
printf 'RESTORE BACKUP %s backup-lab\n' "$backup_id" | \
  "$binary" backup restore "$artifact" --destination "$restored" --journal "$restore_journal" | tee "$evidence/restore.txt"
cmp "$manifest" "$restored/manifest.yaml"
cmp "$evidence/sentinel-before.txt" "$evidence/sentinel.txt"
test "$(stat -c '%a' "$artifact")" = 700
test "$(stat -c '%a' "$artifact/metadata.json")" = 600
test "$(stat -c '%a' "$restored")" = 700

cp -a "$artifact" "$evidence/corrupt-artifact"
printf 'corruption\n' >> "$evidence/corrupt-artifact/payload/manifest.yaml"
if "$binary" backup verify "$evidence/corrupt-artifact" > "$evidence/corrupt-verify.txt" 2>&1; then
  echo 'corrupt backup unexpectedly verified' >&2
  exit 1
fi

mkdir "$evidence/existing"
chmod 700 "$evidence/existing"
printf 'keep\n' > "$evidence/existing/sentinel.txt"
chmod 600 "$evidence/existing/sentinel.txt"
if printf 'RESTORE BACKUP %s backup-lab\n' "$backup_id" | \
  "$binary" backup restore "$artifact" --destination "$evidence/existing" \
    --journal "$evidence/rejected-restore-journal" > "$evidence/overwrite-refusal.txt" 2>&1; then
  echo 'restore unexpectedly overwrote an existing destination' >&2
  exit 1
fi
grep -Fq keep "$evidence/existing/sentinel.txt"
test ! -e "$evidence/rejected-restore-journal"

printf 'Phase 2 configuration backup, corruption rejection and collision-safe restore passed.\n' | tee "$evidence/result.txt"
