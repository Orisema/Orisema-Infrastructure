# Beta 1 release audit

Date: 2026-11-13

Target: `v0.1.0-beta.1`

Source commit: `f78cc1d218a3fe3b148263419db1d88d6b2b809c`

Gate issue: #179

Release run: [43435385528](https://github.com/Orisema/Orisema-Infrastructure/actions/runs/43435385528)

## Result

The Beta 1 source and publication gates passed. The tag resolves to the exact
source commit above. The release workflow repeated vet, tests and race checks,
recorded the Beta source gate, built both Linux archives, signed and attested
the distribution, published it, downloaded it again and completed the
post-publication installation checks.

The release remains experimental and is not recommended for production.

## Controlled public-configuration replacement

- The planner admits only drift exclusive to explicitly public configuration
  for one service without secret or network bindings.
- The persisted v3 intent contains names and SHA-256 fingerprints, never
  public values or private locators.
- Authorization and apply use separate exact confirmations bound to the
  application, service and operation ID, with fresh plan and ownership checks.
- Apply journals every runtime boundary, preserves the exact previous
  container stopped, and creates one unverified candidate with the unchanged
  image.
- Verify records one observation and commits only `COMPLETED`,
  `RECOVERY_REQUIRED` or an explicit wait state.
- Manual rollback accepts only intact `RECOVERY_REQUIRED`, revalidates the
  desired fingerprint and both container identities, preserves the failed
  candidate, restores the exact previous ID and requires a separate healthy
  verification before `ROLLED_BACK`.

## Evidence observed before this gate

- PRs #166, #168, #170, #172, #174, #176 and #178 were human-approved and
  merged in the documented implementation order.
- The PR #178 candidate passed all 16 GitHub checks, including CodeQL, tests,
  Ubuntu quickstarts, Fedora installer fixtures and two Podman jobs.
- Local `go test ./...`, `go vet ./...`, `go test -race ./...`, healthy
  candidate and failed-candidate/manual-rollback laboratories passed before
  PR #178.
- Wrong confirmation and changed public-configuration evidence left journal
  hashes and both runtime IDs unchanged.

## Published asset evidence

| Asset | Published size | Verification |
| --- | ---: | --- |
| `orisema_linux_amd64.tar.gz` | 1,541,790 bytes | checksum, attestation, archive shape and native version/commit |
| `orisema_linux_arm64.tar.gz` | 1,410,963 bytes | checksum, attestation, archive shape and cross-build metadata |
| `checksums.txt` | 186 bytes | Sigstore and GitHub attestation |
| `checksums.txt.sigstore.json` | 10,727 bytes | Cosign bundle verified against the published checksum file |

The workflow deliberately changed an archive and checksum metadata and
confirmed their rejection before installation. It installed authenticated
Alpha 10, upgraded to Beta 1, rejected implicit downgrade, accepted explicit
downgrade, upgraded again and uninstalled. ARM64 remains cross-build only; no
native ARM64 execution is claimed.

## Residual limits

The release remains experimental and is not recommended for production.
Secrets, networks, combined image/configuration drift, persistent data,
volumes, cleanup, automatic recovery, multiple services, Docker, remote
mutation and native ARM64 execution remain outside the Beta 1 claim.
