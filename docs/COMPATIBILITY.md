# Alpha compatibility matrix

This document distinguishes an artifact that is produced from an environment
that is actually exercised. Orisema Infrastructure remains **experimental and
is not recommended for production**.

## Distribution and runtime

| Environment | Artifact | Automated execution | Runtime coverage |
| --- | --- | --- | --- |
| Ubuntu 24.04.5 AMD64 | Linux AMD64 | CLI, packaging, quick start and extended Podman laboratory | Podman 4.9.3 observed |
| Ubuntu 22.04.5 AMD64 | Linux AMD64 | CLI and isolated quick start | Podman 3.4.4 observed |
| Linux ARM64 | Linux ARM64 | Cross-build and binary metadata inspection only | Native execution not yet validated |
| Other Linux distributions | Architecture-dependent | Not continuously tested | Not established |
| macOS | None | Not tested | Not supported |
| Windows | None | Not tested | Not supported |

The versions above were observed by the CI quick-start matrix on 2026-10-27.
The matrix uses fresh, independent GitHub-hosted runners and records
`/etc/os-release`, kernel architecture and the exact Podman version as workflow
artifacts. Package versions can change when runner images or Ubuntu repositories
change; the workflow evidence for a commit is authoritative. These observations
are not minimum-version guarantees.

The local development laboratory has also exercised Ubuntu 24.04.5 LTS AMD64,
Linux kernel 6.8 and Podman 4.9.3. This is evidence from one environment, not a
minimum-version guarantee.

## Runtime support

- Podman on Linux is the only supported runtime family.
- Rootful and rootless Podman are separate environments; consistent invocation
  is required.
- Docker CLI/Engine compatibility is not implemented or claimed.
- Healthcheck inspect fields and scheduling can vary between Podman versions;
  conservative parsing may report an observation as unavailable.
- Host firewall, SELinux/AppArmor policy, cgroups configuration and registry
  access remain operator responsibilities.

## Command support

| Command | Reads Podman | May change Podman | Notes |
| --- | --- | --- | --- |
| `version` | No | No | Uses only embedded build and Go runtime information |
| `validate` | No | No | Validates the current manifest contract |
| `plan` | Yes | No | Compares only supported declared fields |
| `update plan` | Yes | No | Image-only candidate guidance; never authorizes execution |
| `update prepare` | Yes | Journal only | Persists one plan-bound intent; never changes Podman |
| `update authorize` | Yes | Journal only | Confirms and revalidates intent; never changes Podman |
| `update apply` | Yes | Yes | Revalidates, preserves the stopped previous container and creates one unverified candidate |
| `update verify` | Yes | Journal only | Records one observation and durably decides only terminal health evidence |
| `status`, `diagnose` | Yes | No | Present distinct runtime and health observations |
| `logs` | Yes | No | Returns the last 100 lines; does not follow |
| `deploy` | Yes | Yes | Creates only `CREATE` services after confirmation |
| `start`, `stop` | Yes | Yes | Require ownership and exact confirmation |
| `recover plan`, `history`, `verify` | Some | No | Journal commands can require local journal access |
| `recover prepare` | Yes | Journal only | Reserves an eligible START; does not start it |
| `recover apply` | Yes | Yes | Consumes a prepared token and performs only START |

Backup/restore executable commands are not available yet. The accepted
[architecture](BACKUP-RESTORE-DESIGN.md) initially covers only validated
configuration and optional read-only operation history. It explicitly excludes
volumes, application data, secrets and credentials.

The public Alpha 5 flow now reaches a durable post-update health decision, but
data rollback remains unavailable. The Alpha 5 manual application rollback
preserves both exact container identities. Its laboratory proves that an
unrelated container and named volume retain their identity. It does not promise
data rollback or restoration.
See [the controlled update laboratory](CONTROLLED-UPDATE-LAB.md).

## Manifest compatibility

- `orisema.dev/v1alpha1` remains readable with its legacy all-interface port
  behavior; it cannot use `kind` or `hostIP`.
- `orisema.dev/v1alpha2` requires `kind: Application` and an explicit loopback
  `hostIP` for every port.
- Environment, secrets, volumes, custom permissions and JSON output are not
  supported and fail closed. See [the Alpha 2 contract](MANIFEST-V1ALPHA2.md).

## Meaning of `NOOP`

`NOOP` confirms only matching ownership labels, configured image reference, TCP
port mappings, declared healthcheck command/timing/retries and restart policy.
It does **not** confirm:

- that the container is running or healthy;
- image digest, freshness or registry identity;
- undeclared or image-inherited settings outside the supported comparison;
- volumes, secrets, environment variables, networks or security context;
- firewall exposure, dependencies or application-level reachability;
- production readiness.

A stopped or unhealthy container can correctly remain `NOOP`. Orisema keeps
declarative comparison, operational diagnosis and recovery evidence separate.
