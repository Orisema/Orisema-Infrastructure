# Alpha 10 release audit

Date: 2026-11-06  
Release: [`v0.1.0-alpha.10`](https://github.com/Orisema/Orisema-Infrastructure/releases/tag/v0.1.0-alpha.10)  
Source commit: `3db74f1b090d31336555e80d0574feb6e484f755`  
Gate issue: #150  
Release run: [42257680866](https://github.com/Orisema/Orisema-Infrastructure/actions/runs/42257680866)

## Result

The Alpha 10 compatibility and distribution gate passed. The release workflow
checked out and tagged the exact merge commit, ran the source and race suites,
recorded the reproducible gate evidence, built and inspected both archives,
signed and attested the distribution, published it, downloaded it again and
completed post-publication verification.

The release remains experimental and is not recommended for production.

## Published asset evidence

| Asset | Published size | Verification |
| --- | ---: | --- |
| `orisema_linux_amd64.tar.gz` | 1,532,539 bytes | checksum, attestation, archive shape and native version/commit |
| `orisema_linux_arm64.tar.gz` | 1,403,415 bytes | checksum, attestation, archive shape and cross-build metadata |
| `checksums.txt` | 186 bytes | Sigstore and GitHub attestation |
| `checksums.txt.sigstore.json` | 10,723 bytes | Cosign bundle used against the published checksum file |

The tag resolves to the exact source commit above. ARM64 remains cross-build
only; neither the archive nor this audit claims native ARM64 execution.

## Compatibility and installation evidence

- Fedora 43 and Fedora 45 container jobs passed user and privileged installer
  lifecycles on the release source. They are distribution CI, not native
  Podman-host evidence.
- Ubuntu 22.04/24.04 quickstarts and the extended Ubuntu 24.04 Podman laboratory
  passed on the same PR commit before merge.
- The earlier Fedora 45 native rootless/SELinux record remains historical and
  is not presented as a fresh Alpha 10 native run.
- The published-release gate installed authenticated Alpha 9, upgraded to
  Alpha 10, rejected an implicit downgrade, accepted an explicit authenticated
  downgrade, upgraded again and uninstalled.
- A deliberately changed archive failed checksum and attestation verification;
  changed checksum metadata failed Sigstore verification. No altered input was
  installed.
- The installer fixture separately proved restoration of the previous
  executable after post-install validation failure.

## Operational readiness evidence

- `deploy`, `start` and `stop` use the unified private journal and atomic
  admission boundary.
- Abrupt termination, full storage, corrupt/insecure journal and concurrent
  admission preserve evidence and fail closed in the Alpha 10 laboratory.
- Every public CLI path has an inventory entry enforced by tests.
- `orisema-doctor-v2` provides stable PASS/WARN/BLOCK causes and next steps
  without authorizing mutation.
- Operator and agent paths share confirmation, ownership, revalidation,
  evidence and human-review requirements.

## Reviews and limits

CodeQL, all repository tests, quickstarts, Fedora installer jobs and Podman
laboratories passed. A maintainer explicitly reviewed and authorized the
release effect in PR #161 before merge.

The Tailscale endpoint for the Fedora VM accepted the known SSH host key but
required interactive reauthentication, so no new native Fedora result was
recorded. Native ARM64, offline Sigstore verification, packages, Docker,
remote mutation and production readiness remain outside this release.

