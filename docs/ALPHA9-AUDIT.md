# Alpha 9 release audit

Date: 2026-11-05  
Target: `v0.1.0-alpha.9`  
Gate issue: #124

## Result

The Alpha 9 source gate passed. Operational recovery, signed distribution and
the reversible installation lifecycle agree with RFC-0005 and RFC-0006. The
release remains experimental and is not recommended for production.

## Operational recovery evidence

- `orisema doctor` validates manifest, rootless/rootful Podman boundary,
  operation store, bindings, declared runtime and storage margins without
  changing the runtime or journal.
- Update continuations validate the complete store and reject another pending
  operation, unsafe snapshots and corruption before confirmation.
- `recover operation` accepts only an intact `RECOVERY_REQUIRED` update and
  delegates to the revalidated, explicitly confirmed rollback path.
- Unsupported recovery states fail closed without deleting evidence.
- The configured rollback laboratory now exercises the public recovery entry
  point and proves that the preserved container is restored.

## Distribution and installation evidence

- The release workflow uses pinned actions, GitHub OIDC, Sigstore keyless
  signing and GitHub artifact attestations.
- Published archives, authenticated checksums and attestations are downloaded
  and verified again after publication.
- The release workflow runs the public installer against the published release
  and then runs the explicit uninstaller.
- The installer laboratory proves verification order, tamper rejection,
  explicit downgrade authorization, user/system installation, rollback after
  failed post-install validation, conservative uninstall and link rejection.
- A failed replacement retains either the original executable or its private
  same-filesystem backup; successful commit uses an atomic rename.

## Platform evidence

| Environment | Evidence | Result |
| --- | --- | --- |
| Ubuntu 22.04 | quickstart CI | Passed |
| Ubuntu 24.04 | quickstart, Podman and installer CI | Passed |
| Fedora 45 AMD64 | native VM, Podman 6.1.3, rootless, SELinux Enforcing | Passed |
| Linux ARM64 | cross-build and archive inspection | Passed; native runtime remains untested |

The Fedora gate ran the release-shaped AMD64 binary, manifest validation,
read-only `doctor`, and the installer laboratory in a private temporary
directory. The journal lock metadata remained unchanged across `doctor`.

## Deliberate limits

- Legacy `deploy`, `start` and `stop` still lack an explicit journal argument;
  the common mutation guard covers update continuations that carry a journal.
- Recovery does not infer runtime success or resume ambiguous steps.
- Offline Sigstore verification is not provided.
- `.deb` and `.rpm` were evaluated but deferred: adding package repositories
  and another signing/update trust path would broaden the Alpha 9 security
  contract without improving the verified tarball flow.
- Native ARM64 execution, database/volume rollback and distributed recovery
  remain future work.
