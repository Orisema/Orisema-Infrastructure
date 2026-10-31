# Phase 2 operation audit

Status: complete for the documented experimental scope. The
`v0.1.0-alpha.5` workflow published and re-downloaded verified artifacts from
the exact merged commit `b6d8c7b`; an independent download repeated both
checksum checks and the AMD64 embedded version/commit verification.

This audit maps the Phase 2 completion request to durable code, automated tests
and practical laboratories. It does not turn an Alpha into a production claim.

| Requirement | Evidence | Result |
| --- | --- | --- |
| Healthy update ends in `COMPLETED` | operation state machine, `update verify`, Alpha 5 laboratory | Passed |
| Failed health is recoverable and diagnosable | durable `RECOVERY_REQUIRED`, operation diagnosis and failure laboratory | Passed |
| Manual rollback is authorized and revalidated | `update rollback`/`rollback-verify`, exact confirmation and identity checks | Passed |
| Concurrent/interrupted operations fail closed | journal locks, immutable intent, transition tests and race suite | Passed |
| Unmanaged resources are preserved | update/rollback laboratories track unrelated container and volume identity | Passed |
| Backup scope and integrity are explicit | ADR 0003, versioned metadata and per-entry SHA-256 | Passed |
| Corrupt/unsafe backup is rejected | traversal, symlink, special/duplicate/unknown entry and corruption tests | Passed |
| Restore cannot overwrite | absent destination, no-replace publication, confirmation and isolated laboratory | Passed |
| SSH keys and host identity are strict | private registry, external `0600` keys/known-hosts and real `sshd` failures | Passed |
| Remote commands are not arbitrary | internal fixed `true`, inspect and 100-line logs command shapes | Passed |
| Remote ownership and output are bounded | exact labels/name/ID plus 2 MiB inspect and 1 MiB logs limits | Passed |
| Remote operations are audited | locked, fsynced local `0600` `START`/`RESULT` JSONL without stderr/log text | Passed |
| ProxyJump and existing private reachability | separate bastion/target `sshd`, direct route, distinct keys and host keys | Passed |
| Disconnection fails safely | target loss during host check and remote inspection, typed bounded failure | Passed |
| Rootless Podman | isolated deploy, health probe, status, `NOOP` and removal on Podman 4.9.3 | Passed locally |
| Automated quality gates | `go vet`, complete unit/integration suite, full race suite, AMD64/ARM64 packaging | Passed before PR; CI is authoritative |

## Negative and recovery coverage

The suite and laboratories cover authentication failure, unknown and changed
host keys, ProxyJump cycles, command/output injection shapes, timeouts, target
loss, ownership mismatch, oversized or malformed remote output, corrupted
backup, interrupted publication, concurrent operations, stale evidence,
candidate health failure and explicit manual rollback. No negative path enables
automatic destructive recovery.

## Explicitly unclaimed

- Production readiness, high availability or multi-node orchestration.
- Consistent database, volume or persistent-data backup and rollback.
- Remote mutation, automatic NAT traversal, official relay or managed VPN.
- A real external bastion, CGNAT path, WireGuard/Tailscale/Headscale provider or
  real remote Podman host. The transport accepts an operator-provided route;
  those products remain optional and operator-managed.
- Fedora execution, native ARM64 execution, Docker, macOS or Windows. Fedora was
  not available in this work session and is not added to the compatibility
  matrix.
- Signed artifacts, provenance attestations, authenticated operator identity or
  tamper-evident local journals.

## Release verification

The tag workflow tests the immutable source, runs the full race suite, builds
Linux AMD64/ARM64 archives with embedded version/commit metadata, verifies
archive layout and checksums, publishes a prerelease, downloads it again,
checks SHA-256 and executes `orisema version`. The Phase 2 gate is not complete
if any of those steps fails.
