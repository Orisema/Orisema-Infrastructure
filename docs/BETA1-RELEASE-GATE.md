# Beta 1 compatibility and release gate

Target: `v0.1.0-beta.1`  
Gate issue: #179  
Parent scope: #164

This gate prepares an experimental prerelease. It does not claim production
readiness, native ARM64 execution, Docker compatibility or rollback of data.

## Source gate

The candidate commit must pass `go vet ./...`, `go test ./...` and
`go test -race ./...`. The ordinary PR workflow must also pass Ubuntu
quickstarts, Fedora installer fixtures, CodeQL and both isolated Podman jobs.
Those Podman jobs run the public-configuration laboratory once to `COMPLETED`
and once through `RECOVERY_REQUIRED` and manual `ROLLED_BACK`.

The negative laboratory proves that wrong confirmation and changed
configuration evidence do not change the journal or runtime. The rollback
restores the exact prior container ID; public values remain absent from the
journal and retained evidence.

## Immutable distribution gate

Closing #179 after human review is the explicit release authorization. The
workflow maps only that issue to `v0.1.0-beta.1`, checks out its exact `main`
commit, refuses a mismatched existing tag, and then:

1. repeats the source, race, archive and installer-lifecycle gates;
2. builds reproducible Linux AMD64 and ARM64 archives;
3. signs checksum metadata with keyless Sigstore and attests every artifact;
4. publishes the prerelease and downloads it again;
5. rejects tampered archives and checksum metadata;
6. upgrades an authenticated Alpha 10 installation to Beta 1, rejects an
   implicit downgrade, accepts an explicit downgrade, upgrades again and
   uninstalls.

Publication results are facts only after the workflow completes. Until then,
the audit may record the source candidate as ready but must not claim that the
tag, release, signatures or post-publication installation passed.

## Deliberate limits

The first Beta increment accepts only drift exclusive to public configuration
on one service without secrets or networks. Drift of secrets, networks, image
combined with configuration, ports, healthcheck, restart policy, volumes or
unknown fields remains blocked. There is no automatic rollback, cleanup,
resume, multi-service transaction or remote mutation.
