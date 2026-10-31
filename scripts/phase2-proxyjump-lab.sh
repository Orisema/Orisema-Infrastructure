#!/usr/bin/env bash
# Copyright 2026 Orisema contributors
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

binary="${ORISEMA_BINARY:-$PWD/work/orisema}"
evidence="${ORISEMA_EVIDENCE_DIR:-$PWD/work/phase2-proxyjump}"
bastion_port="${ORISEMA_BASTION_PORT:-22234}"
target_port="${ORISEMA_TARGET_PORT:-22235}"
bastion_user="orisemajump$$"
target_user="orisematarget$$"
bastion_auth="/tmp/orisema-jump-authorized-$$"
target_auth="/tmp/orisema-target-authorized-$$"
bastion_pid=""
target_pid=""
cleanup() {
  if [ -n "$target_pid" ]; then kill "$target_pid" >/dev/null 2>&1 || true; wait "$target_pid" >/dev/null 2>&1 || true; fi
  if [ -n "$bastion_pid" ]; then kill "$bastion_pid" >/dev/null 2>&1 || true; wait "$bastion_pid" >/dev/null 2>&1 || true; fi
  userdel "$target_user" >/dev/null 2>&1 || true
  userdel "$bastion_user" >/dev/null 2>&1 || true
  unlink "$target_auth" >/dev/null 2>&1 || true
  unlink "$bastion_auth" >/dev/null 2>&1 || true
}
trap cleanup EXIT
useradd --no-create-home --shell /bin/sh "$bastion_user"
useradd --no-create-home --shell /bin/sh "$target_user"
passwd -d "$bastion_user" >/dev/null
passwd -d "$target_user" >/dev/null
mkdir -p "$evidence"
chmod 711 "$evidence"

bastion_client="$evidence/bastion-client"
target_client="$evidence/target-client"
bastion_host="$evidence/bastion-host"
target_host="$evidence/target-host"
rogue_host="$evidence/rogue-host"
bastion_known="$evidence/bastion-known_hosts"
target_known="$evidence/target-known_hosts"
rogue_known="$evidence/rogue-known_hosts"
registry="$evidence/hosts.yaml"
sentinel="$evidence/target-sentinel"
printf 'unchanged\n' > "$sentinel"
for key in "$bastion_client" "$target_client" "$bastion_host" "$target_host" "$rogue_host"; do ssh-keygen -q -t ed25519 -N '' -f "$key"; done
cp "$bastion_client.pub" "$bastion_auth"
cp "$target_client.pub" "$target_auth"
chmod 600 "$bastion_client" "$target_client" "$bastion_auth" "$target_auth"
chown "$bastion_user" "$bastion_auth"
chown "$target_user" "$target_auth"
awk -v host="[127.0.0.1]:$bastion_port" '{print host, $1, $2}' "$bastion_host.pub" > "$bastion_known"
awk -v host="[127.0.0.2]:$target_port" '{print host, $1, $2}' "$target_host.pub" > "$target_known"
awk -v host="[127.0.0.2]:$target_port" '{print host, $1, $2}' "$rogue_host.pub" > "$rogue_known"
chmod 600 "$bastion_known" "$target_known" "$rogue_known"

cat > "$evidence/bastion-sshd_config" <<EOF
Port $bastion_port
ListenAddress 127.0.0.1
HostKey $bastion_host
PidFile $evidence/bastion.pid
AuthorizedKeysFile $bastion_auth
PasswordAuthentication no
KbdInteractiveAuthentication no
PubkeyAuthentication yes
PermitRootLogin no
StrictModes no
AllowUsers $bastion_user
AllowTcpForwarding yes
PermitOpen 127.0.0.2:$target_port
UsePAM no
LogLevel VERBOSE
EOF
cat > "$evidence/target-sshd_config" <<EOF
Port $target_port
ListenAddress 127.0.0.2
HostKey $target_host
PidFile $evidence/target.pid
AuthorizedKeysFile $target_auth
PasswordAuthentication no
KbdInteractiveAuthentication no
PubkeyAuthentication yes
PermitRootLogin no
StrictModes no
AllowUsers $target_user
AllowTcpForwarding no
UsePAM no
LogLevel VERBOSE
EOF
mkdir -p /run/sshd
/usr/sbin/sshd -D -f "$evidence/bastion-sshd_config" -E "$evidence/bastion.log" & bastion_pid=$!
/usr/sbin/sshd -D -f "$evidence/target-sshd_config" -E "$evidence/target.log" & target_pid=$!
for endpoint in "127.0.0.1/$bastion_port" "127.0.0.2/$target_port"; do
  host="${endpoint%/*}"; port="${endpoint#*/}"
  for _ in $(seq 1 50); do if bash -c "exec 3<>/dev/tcp/$host/$port" 2>/dev/null; then break; fi; sleep 0.1; done
done

cat > "$registry" <<EOF
apiVersion: orisema.dev/hosts/v1alpha1
kind: HostRegistry
hosts:
  - name: bastion
    address: 127.0.0.1
    port: $bastion_port
    user: $bastion_user
    identityFile: $bastion_client
    knownHostsFile: $bastion_known
    connectTimeout: 3s
    commandTimeout: 5s
  - name: private-target
    address: 127.0.0.2
    port: $target_port
    user: $target_user
    identityFile: $target_client
    knownHostsFile: $target_known
    proxyJump: bastion
    connectTimeout: 3s
    commandTimeout: 5s
  - name: private-route
    address: 127.0.0.2
    port: $target_port
    user: $target_user
    identityFile: $target_client
    knownHostsFile: $target_known
    connectTimeout: 3s
    commandTimeout: 5s
EOF
chmod 600 "$registry"

"$binary" host check "$registry" private-target | tee "$evidence/proxy-success.txt"
grep -Fq 'outcome=OK' "$evidence/proxy-success.txt"
grep -Fq "Accepted publickey for $bastion_user" "$evidence/bastion.log"
grep -Fq "Accepted publickey for $target_user" "$evidence/target.log"
test "$(cat "$sentinel")" = unchanged
"$binary" host check "$registry" private-route | tee "$evidence/private-route-success.txt"
grep -Fq 'outcome=OK' "$evidence/private-route-success.txt"

: > "$evidence/unknown_hosts"
chmod 600 "$evidence/unknown_hosts"
sed "s|knownHostsFile: $bastion_known|knownHostsFile: $evidence/unknown_hosts|" "$registry" > "$evidence/bad-bastion.yaml"
chmod 600 "$evidence/bad-bastion.yaml"
if "$binary" host check "$evidence/bad-bastion.yaml" private-target > "$evidence/bastion-key-failure.txt" 2>&1; then echo 'unknown bastion key unexpectedly succeeded' >&2; exit 1; fi
grep -Fq 'outcome=HOST_KEY_FAILED' "$evidence/bastion-key-failure.txt"

sed "s|knownHostsFile: $target_known|knownHostsFile: $evidence/unknown_hosts|" "$registry" > "$evidence/bad-target.yaml"
chmod 600 "$evidence/bad-target.yaml"
if "$binary" host check "$evidence/bad-target.yaml" private-target > "$evidence/target-key-failure.txt" 2>&1; then echo 'unknown target key unexpectedly succeeded' >&2; exit 1; fi
grep -Fq 'outcome=HOST_KEY_FAILED' "$evidence/target-key-failure.txt"

sed "s|knownHostsFile: $target_known|knownHostsFile: $rogue_known|" "$registry" > "$evidence/changed-target.yaml"
chmod 600 "$evidence/changed-target.yaml"
if "$binary" host check "$evidence/changed-target.yaml" private-target > "$evidence/target-key-changed.txt" 2>&1; then echo 'changed target key unexpectedly succeeded' >&2; exit 1; fi
grep -Fq 'outcome=HOST_KEY_FAILED' "$evidence/target-key-changed.txt"

kill "$target_pid"; wait "$target_pid" || true; target_pid=""
if "$binary" host check "$registry" private-target > "$evidence/target-loss.txt" 2>&1; then echo 'lost target unexpectedly succeeded' >&2; exit 1; fi
grep -Eq 'outcome=(CONNECTION_FAILED|EXECUTION_FAILED)' "$evidence/target-loss.txt"
test "$(cat "$sentinel")" = unchanged
printf 'Phase 2 real ProxyJump, strict keys and target-loss handling passed.\n' | tee "$evidence/result.txt"
