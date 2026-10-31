#!/usr/bin/env bash
# Copyright 2026 Orisema contributors
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

binary="${ORISEMA_BINARY:-$PWD/work/orisema}"
evidence="${ORISEMA_EVIDENCE_DIR:-$PWD/work/phase2-remote-inspection}"
port="${ORISEMA_SSH_PORT:-22231}"
target_user="orisemaremote$$"
authorized="/tmp/orisema-remote-authorized-$$"
fake_bin="/tmp/orisema-remote-bin-$$"
sshd_pid=""
cleanup() {
  if [ -n "$sshd_pid" ]; then kill "$sshd_pid" >/dev/null 2>&1 || true; wait "$sshd_pid" >/dev/null 2>&1 || true; fi
  userdel "$target_user" >/dev/null 2>&1 || true
  unlink "$authorized" >/dev/null 2>&1 || true
  unlink "$fake_bin/podman" >/dev/null 2>&1 || true
  rmdir "$fake_bin" >/dev/null 2>&1 || true
}
trap cleanup EXIT
useradd --no-create-home --shell /bin/sh "$target_user"
passwd -d "$target_user" >/dev/null
mkdir -p "$evidence" "$fake_bin"
chmod 711 "$evidence"
chmod 755 "$fake_bin"

client_key="$evidence/client"
host_key="$evidence/host"
known="$evidence/known_hosts"
registry="$evidence/hosts.yaml"
manifest="$evidence/manifest.yaml"
audit="$evidence/audit.jsonl"
sentinel="$evidence/remote-sentinel"
printf 'unchanged\n' > "$sentinel"

ssh-keygen -q -t ed25519 -N '' -f "$client_key"
ssh-keygen -q -t ed25519 -N '' -f "$host_key"
cp "$client_key.pub" "$authorized"
chmod 600 "$client_key" "$authorized"
chown "$target_user" "$authorized"
awk -v host="[127.0.0.1]:$port" '{print host, $1, $2}' "$host_key.pub" > "$known"
chmod 600 "$known"

cat > "$fake_bin/podman" <<'EOF'
#!/bin/sh
set -eu
case "$*" in
  'inspect --type container orisema-remote-lab-web')
    printf '%s\n' '[{"Id":"remote-container-1","Name":"orisema-remote-lab-web","State":{"Status":"running","Running":true,"ExitCode":0,"Healthcheck":{"Status":"healthy"}},"RestartCount":0,"Config":{"Image":"nginx:stable","Labels":{"dev.orisema.managed":"true","dev.orisema.app":"remote-lab","dev.orisema.service":"web"},"Healthcheck":{"Test":["CMD-SHELL","true"]}}}]'
    ;;
  'logs --tail 100 orisema-remote-lab-web')
    printf 'remote bounded log line one\nremote bounded log line two\n'
    ;;
  *)
    printf 'non-allowlisted runtime invocation\n' >&2
    exit 91
    ;;
esac
EOF
chmod 755 "$fake_bin/podman"

cat > "$evidence/sshd_config" <<EOF
Port $port
ListenAddress 127.0.0.1
HostKey $host_key
PidFile $evidence/sshd.pid
AuthorizedKeysFile $authorized
PasswordAuthentication no
KbdInteractiveAuthentication no
PubkeyAuthentication yes
PermitRootLogin no
StrictModes no
AllowUsers $target_user
UsePAM no
SetEnv PATH=$fake_bin:/usr/bin:/bin
LogLevel VERBOSE
EOF
mkdir -p /run/sshd
/usr/sbin/sshd -D -f "$evidence/sshd_config" -E "$evidence/sshd.log" &
sshd_pid=$!
for _ in $(seq 1 50); do if bash -c "exec 3<>/dev/tcp/127.0.0.1/$port" 2>/dev/null; then break; fi; sleep 0.1; done

cat > "$registry" <<EOF
apiVersion: orisema.dev/hosts/v1alpha1
kind: HostRegistry
hosts:
  - name: local-lab
    address: 127.0.0.1
    port: $port
    user: $target_user
    identityFile: $client_key
    knownHostsFile: $known
    connectTimeout: 3s
    commandTimeout: 5s
EOF
cat > "$manifest" <<'EOF'
apiVersion: orisema.dev/v1alpha2
kind: Application
name: remote-lab
services:
  - name: web
    image: nginx:stable
EOF
chmod 600 "$registry"

"$binary" remote status "$registry" local-lab "$manifest" --audit "$audit" | tee "$evidence/status.txt"
grep -Fq 'ownership: verified' "$evidence/status.txt"
grep -Fq 'health: healthy' "$evidence/status.txt"
"$binary" remote logs "$registry" local-lab "$manifest" web --audit "$audit" | tee "$evidence/logs.txt"
grep -Fq 'remote bounded log line two' "$evidence/logs.txt"
test "$(cat "$sentinel")" = unchanged
test "$(stat -c '%a' "$audit")" = 600
test "$(grep -c '"phase":"START"' "$audit")" -eq 2
test "$(grep -c '"phase":"RESULT"' "$audit")" -eq 2
if grep -Fq 'remote bounded log' "$audit"; then echo 'log content leaked into audit' >&2; exit 1; fi
printf 'Phase 2 audited remote read-only inspection passed.\n' | tee "$evidence/result.txt"
