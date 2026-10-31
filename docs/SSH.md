# Strict SSH transport

Status: strict transport and audited read-only remote inspection implemented;
no remote mutation is available.

The Orisema uses the system OpenSSH client with a generated private config. It
does not implement a VPN, host-key enrollment, password store or arbitrary
remote shell. Network reachability must already exist, directly or through a
registered bastion.

## Host registry

The registry must be a regular, non-symlink file with mode `0600`:

```yaml
apiVersion: orisema.dev/hosts/v1alpha1
kind: HostRegistry
hosts:
  - name: bastion
    address: bastion.example.net
    port: 22
    user: deploy
    identityFile: /home/operator/.ssh/orisema_bastion
    knownHostsFile: /home/operator/.ssh/orisema_known_hosts
  - name: private-web
    address: 10.20.0.15
    port: 22
    user: deploy
    identityFile: /home/operator/.ssh/orisema_private
    knownHostsFile: /home/operator/.ssh/orisema_known_hosts
    proxyJump: bastion
    connectTimeout: 10s
    commandTimeout: 30s
```

Identity and known-hosts paths must be absolute, regular, non-symlink files with
mode `0600`. The registry stores paths only, never private-key material or
passwords. Host aliases, users and addresses reject whitespace, option-like
prefixes, shell syntax and control characters. Proxy chains are limited to
three hops and cycles are rejected.

Populate `knownHostsFile` through an independently authenticated channel. The
Orisema always enables `StrictHostKeyChecking=yes`; it never uses `accept-new`,
disables checking or edits the file. A missing, unknown or changed key fails
closed.

## Connectivity check

```sh
orisema host check /private/hosts.yaml private-web
```

The check executes only the fixed remote command `true`, using key-only batch
authentication, bounded connect/command timeouts and process-group cancellation.
It reports a typed outcome without returning SSH stderr, which may contain
sensitive infrastructure details. It does not inspect or mutate the host.

Generated OpenSSH config files live in a private temporary directory and are
removed after the command. User-supplied SSH options and commands are not part
of the public API.

## Read-only runtime inspection

```sh
orisema remote status /private/hosts.yaml private-web app.yaml \
  --audit /private/orisema-remote-audit.jsonl
orisema remote logs /private/hosts.yaml private-web app.yaml web \
  --audit /private/orisema-remote-audit.jsonl
```

`remote status` runs only `podman inspect --type container` for exact container
names derived from the validated local manifest. `remote logs` first repeats
that inspection and verifies the three Orisema ownership labels, then runs only
`podman logs --tail 100`. There is no operator-supplied command, container name,
SSH option or log tail.

Inspect output is limited to 2 MiB and must decode to exactly one object with a
nonempty ID, the expected name and exact ownership. Log output is limited to 1
MiB. Timeouts, malformed output, extra inspect objects, missing containers,
ownership mismatch and excess output fail closed. Neither operation mutates
remote state.

The required absolute audit path is local. Its parent may not be group/world
writable; the regular audit file is created as `0600`, opened without following
symlinks, locked, appended and synchronized. Each operation writes correlated
`START` and `RESULT` JSON records. Records include the host alias, action,
application/service, typed outcome and duration, but never keys, SSH stderr or
container log content. If `START` cannot be recorded, no SSH query occurs.

## Validated scope

The automated suite covers schema and permission failures, injection-shaped
values, unknown hosts, proxy cycles, strict argument/config generation, typed
authentication/host-key/connection errors and cancellation. The integration
laboratory runs a real local `sshd`, proves Ed25519 key authentication and
rejects both a wrong client key and an unknown host key.

A second real-`sshd` laboratory exercises status and log requests end to end
against a deliberately constrained Podman fixture, verifies ownership and
health reporting, checks the 100-line log command, preserves an unrelated
sentinel and proves that audit records contain no log text. Unit tests cover
invalid ownership, injection-shaped identities and oversized output.

A real local ProxyJump laboratory uses separate bastion and target `sshd`
processes, users, client keys, host keys and known-hosts files. It verifies both
hops, rejects unknown keys for either hop, observes bounded failure after target
loss and preserves unrelated target evidence. The same target is also reached
directly through an already available private loopback route, demonstrating
that the registry has no dependency on how reachability was established.

This laboratory does not claim validation over real CGNAT, an external bastion
or a specific VPN product. WireGuard, Tailscale, Headscale and other routing
remain optional and operator-managed as required by
[ADR 0002](adr/0002-independent-connectivity.md).
