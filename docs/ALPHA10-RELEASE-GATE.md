# Alpha 10 compatibility and release gate

This gate makes each distribution claim traceable to an environment, exact
commit and kind of evidence. It does not turn the Alpha into production-ready
software.

## Evidence classes

| Class | What it proves | What it does not prove |
| --- | --- | --- |
| Cross-build | A Linux binary/archive was produced and its Go metadata matches the target | Native execution or runtime compatibility on that architecture |
| CI execution | Tests or a laboratory ran on the named disposable image/runner and commit | A minimum distribution or Podman version guarantee |
| Nested laboratory | A userspace/container scenario ran with explicitly recorded privilege and storage boundaries | A native host, boot, SELinux or system integration |
| Native laboratory | The release-shaped binary ran on a real host with recorded kernel/runtime/security mode | Continuous support for every update of that environment |
| Published-release verification | Downloaded assets match authenticated checksums, GitHub attestations, tag and embedded metadata | Offline verification or trust outside GitHub/Sigstore |

`docs/COMPATIBILITY.md` is the claim matrix. ARM64 remains **cross-build only**
until a native ARM64 host executes the candidate and records evidence.

## Reproducible source gate

Run from the exact clean checkout intended for the tag:

```sh
commit="$(git rev-parse HEAD)"
VERSION=v0.1.0-alpha.10 \
COMMIT="$commit" \
EVIDENCE_DIR="$(pwd)/alpha10-release-evidence" \
scripts/alpha10-release-gate.sh
```

The evidence directory must not already exist. The script rejects a checkout
whose HEAD differs from `COMMIT`, records environment/tool versions, runs vet,
tests and the race detector, builds AMD64/ARM64 archives, checks archive shape
and metadata, and exercises install, explicit downgrade, uninstall, tamper
rejection and executable rollback. Its Cosign/GitHub commands are controlled
fixtures: cryptographic verification belongs to the post-publication gate.

## CI and native matrix

- Ubuntu 22.04 and 24.04 quickstarts execute the AMD64 CLI and record runner,
  kernel and Podman evidence.
- Ubuntu 24.04 runs the extended rootful Podman laboratories, including abrupt
  failure, journals, secrets, update/rollback, backup and SSH boundaries.
- Fedora 43 and Fedora 45 containers exercise both user and privileged
  installer lifecycles. These are distribution CI, not native Podman evidence.
- The Fedora 45 AMD64 native rootless record remains the native Fedora runtime
  evidence until a later run is recorded; it is not described as continuous.
- ARM64 archives are cross-built and inspected, never labelled native-tested.

## Post-publication gate

Closing issue #150 after its PR reaches `main` selects
`v0.1.0-alpha.10`. The pinned release workflow must:

1. tag the exact `main` commit and rebuild/test that immutable source;
2. sign `checksums.txt` with Sigstore keyless and attest both archives and the
   checksum metadata;
3. publish and download the complete asset set again;
4. verify signature, checksum, attestations, archive shape, embedded version
   and commit;
5. prove that altered checksum metadata and an altered archive fail closed;
6. install authenticated Alpha 9, upgrade to Alpha 10, reject an implicit
   downgrade, permit an explicit authenticated downgrade, upgrade again and
   uninstall without touching Orisema runtime state.

Any failed check stops the workflow before a successful gate can be claimed.
The workflow URL and release asset list are the final evidence; update the
Alpha 10 audit only after that run succeeds.

## Recovery from release failure

Do not move or recreate the tag on different source. Preserve the failed run
and downloaded evidence, diagnose the failing gate and fix through another PR.
If assets were partially published, do not claim the release as complete; the
workflow may safely replace assets only for the same immutable tag/commit after
the cause is reviewed. The installer verifies before touching its destination
and restores the previous executable when post-install validation fails.

