#!/usr/bin/env bash
# Copyright 2026 Orisema contributors
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

binary="${ORISEMA_BINARY:-$PWD/work/orisema}"
evidence="${ORISEMA_EVIDENCE_DIR:-$PWD/work/phase2-ssh-transport}"
port="${ORISEMA_SSH_PORT:-22229}"
target_user="orisemalab$$"
authorized="/tmp/orisema-ssh-authorized-$$"
sshd_pid=""
cleanup() {
  if [ -n "$sshd_pid" ]; then
    kill "$sshd_pid" >/dev/null 2>&1 || true
    wait "$sshd_pid" >/dev/null 2>&1 || true
  fi
  userdel "$target_user" >/dev/null 2>&1 || true
  unlink "$authorized" >/dev/null 2>&1 || true
}
trap cleanup EXIT
useradd --no-create-home --shell /bin/sh "$target_user"
passwd -d "$target_user" >/dev/null
mkdir -p "$evidence"
chmod 711 "$evidence"

client_key="$evidence/client"
wrong_key="$evidence/wrong-client"
host_key="$evidence/host"
known="$evidence/known_hosts"
registry="$evidence/hosts.yaml"
sshd_config="$evidence/sshd_config"

ssh-keygen -q -t ed25519 -N '' -f "$client_key"
ssh-keygen -q -t ed25519 -N '' -f "$wrong_key"
ssh-keygen -q -t ed25519 -N '' -f "$host_key"
cp "$client_key.pub" "$authorized"
chmod 600 "$client_key" "$wrong_key" "$authorized"
chown "$target_user" "$authorized"
awk -v host="[127.0.0.1]:$port" '{print host, $1, $2}' "$host_key.pub" > "$known"
chmod 600 "$known"

cat > "$sshd_config" <<EOF
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
LogLevel VERBOSE
EOF

mkdir -p /run/sshd
chmod 755 /run/sshd
/usr/sbin/sshd -D -f "$sshd_config" -E "$evidence/sshd.log" &
sshd_pid=$!
for _ in $(seq 1 50); do
  if bash -c "exec 3<>/dev/tcp/127.0.0.1/$port" 2>/dev/null; then break; fi
  sleep 0.1
done

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
chmod 600 "$registry"
"$binary" host check "$registry" local-lab | tee "$evidence/success.txt"
grep -Fq 'outcome=OK' "$evidence/success.txt"

cp "$registry" "$evidence/bad-auth.yaml"
sed -i "s|identityFile: $client_key|identityFile: $wrong_key|" "$evidence/bad-auth.yaml"
chmod 600 "$evidence/bad-auth.yaml"
if "$binary" host check "$evidence/bad-auth.yaml" local-lab > "$evidence/auth-failure.txt" 2>&1; then
  echo 'wrong SSH identity unexpectedly succeeded' >&2
  exit 1
fi
grep -Fq 'outcome=AUTHENTICATION_FAILED' "$evidence/auth-failure.txt"

: > "$evidence/unknown_hosts"
chmod 600 "$evidence/unknown_hosts"
cp "$registry" "$evidence/unknown-host.yaml"
sed -i "s|knownHostsFile: $known|knownHostsFile: $evidence/unknown_hosts|" "$evidence/unknown-host.yaml"
chmod 600 "$evidence/unknown-host.yaml"
if "$binary" host check "$evidence/unknown-host.yaml" local-lab > "$evidence/host-key-failure.txt" 2>&1; then
  echo 'unknown SSH host key unexpectedly succeeded' >&2
  exit 1
fi
grep -Fq 'outcome=HOST_KEY_FAILED' "$evidence/host-key-failure.txt"

printf 'Phase 2 strict SSH transport, key authentication and host-key rejection passed.\n' | tee "$evidence/result.txt"
