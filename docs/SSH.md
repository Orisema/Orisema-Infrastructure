# Strict SSH transport

Status: foundational transport implemented; remote inspection is delivered
separately and no remote mutation is available.

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

## Validated scope

The automated suite covers schema and permission failures, injection-shaped
values, unknown hosts, proxy cycles, strict argument/config generation, typed
authentication/host-key/connection errors and cancellation. The integration
laboratory runs a real local `sshd`, proves Ed25519 key authentication and
rejects both a wrong client key and an unknown host key.

Real remote networks, VPNs and a real ProxyJump topology are not yet claimed as
validated. WireGuard, Tailscale, Headscale and other routing remain optional and
operator-managed as required by [ADR 0002](adr/0002-independent-connectivity.md).
