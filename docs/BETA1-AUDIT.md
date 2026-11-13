# Beta 1 source audit

Date: 2026-11-13  
Target: `v0.1.0-beta.1`  
Gate issue: #179

## Current result

The Beta 1 implementation gate is ready for human review. This is a source
audit, not a publication result: the tag, signed artifacts and release do not
exist until #179 is explicitly approved and closed and the release workflow
passes.

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

The authoritative release evidence will be the workflow run for the exact tag
commit. No CI result above is represented as a future publication result.

## Residual limits

The release remains experimental and is not recommended for production.
Secrets, networks, combined image/configuration drift, persistent data,
volumes, cleanup, automatic recovery, multiple services, Docker, remote
mutation and native ARM64 execution remain outside the Beta 1 claim.
