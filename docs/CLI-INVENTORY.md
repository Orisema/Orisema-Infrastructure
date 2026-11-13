# Verifiable CLI operational inventory

This is the complete public command inventory for the current `main` branch.
It is an operational index, not a production-readiness claim. The canonical
command-path list is printed by `orisema help`; a Go test requires every path to
have exactly one marker in this file.

Unless a row says otherwise, exit `0` means the requested read or bounded
operation completed, exit `1` means validation, safety, runtime, persistence or
confirmation blocked completion, and exit `2` means invalid CLI usage. A
successful diagnostic is never authorization for a later mutation.

Effect labels used below:

- **offline read:** does not contact Podman or change Orisema state;
- **runtime read:** queries local or remote runtime state without changing it;
- **journal/file write:** changes only the explicit path described by the row;
- **runtime mutation:** may change only the identity-scoped Podman resource
  after the documented guard and confirmation.

## General inspection

<!-- command: help -->
### `orisema help`

Syntax: `orisema help` (also `--help` or `-h`). Offline read. Prints usage and
the canonical public command paths. Recovery from failure: use a supported
single help argument. Evidence: `TestHelpAndOperationalInventoryCoverEveryPublicCommand`.

<!-- command: version -->
### `orisema version`

Syntax: `orisema version`. Offline read of embedded version, commit, build,
toolchain and target metadata. It does not prove artifact authenticity; verify
the release first. Evidence: release packaging and quickstart jobs.

<!-- command: validate -->
### `orisema validate`

Syntax: `orisema validate <manifest.yaml>`. Offline read. Validates the bounded
manifest schema and permissions. Correct the manifest on exit `1`; no runtime
or journal cleanup is needed. Evidence: unit tests and every quickstart.

<!-- command: plan -->
### `orisema plan`

Syntax: `orisema plan <manifest.yaml>`. Runtime read. Reports `CREATE`, `NOOP`,
`DRIFT` or `REVIEW` for supported declared fields. Resolve ownership or drift
manually; the command never reserves ports or authorizes deploy. Evidence:
Alpha 1/2 negative labs and quickstarts.

<!-- command: status -->
### `orisema status`

Syntax: `orisema status <manifest.yaml>`. Runtime read of lifecycle, ownership
and health observations. Runtime-unavailable or ambiguous identity exits `1`;
restore read access and repeat. Evidence: healthcheck and quickstart jobs.

<!-- command: diagnose -->
### `orisema diagnose`

Syntax: `orisema diagnose <manifest.yaml>`. Runtime read separating desired
configuration, execution, health, failure and recommendation. Recommendations
never execute. Evidence: diagnostic unit tests and recovery labs.

<!-- command: logs -->
### `orisema logs`

Syntax: `orisema logs <manifest.yaml> <service>`. Runtime read of a fixed last
100 lines after ownership validation; it does not follow logs. Correct service
or ownership/runtime access on failure. Evidence: quickstart jobs.

<!-- command: doctor -->
### `orisema doctor`

Syntax: `orisema doctor <manifest.yaml> --journal <directory>`. Runtime and
journal read only. Emits `orisema-doctor-v2` PASS/WARN/BLOCK checks and stable
cause codes. Follow the inspection-oriented `next` line; do not repair or
delete evidence automatically. Evidence: doctor negative tests and PR #158.

## Journaled local lifecycle

<!-- command: operation init -->
### `orisema operation init`

Syntax: `orisema operation init --journal <absolute-directory>`. Journal write.
Creates a private explicit store or validates an existing safe one; it never
repairs unsafe state. Inspect ownership/mode and choose a safe absent path on
failure. Evidence: operation store tests.

<!-- command: operation diagnose -->
### `orisema operation diagnose`

Syntax: `orisema operation diagnose --journal <absolute-directory>`. Journal
read only. Lists incomplete operations and always rejects automatic resume.
Preserve corrupt/incomplete evidence and investigate runtime identity separately.

<!-- command: deploy -->
### `orisema deploy`

Syntax: `orisema deploy --journal <absolute-directory> <manifest.yaml>`.
Journal write and runtime mutation after exact `DEPLOY` confirmation. Admits
atomically, records each create boundary and verifies returned identities. A
partial failure requires diagnosis; successfully created services are not
rolled back automatically. Evidence: Alpha 10 failure lab and Podman CI.

<!-- command: start -->
### `orisema start`

Syntax: `orisema start --journal <absolute-directory> <manifest.yaml>
<service>`. Journal write and runtime mutation after exact `START` confirmation,
ownership revalidation and conflict admission. Diagnose incomplete evidence on
ambiguous completion; do not retry blindly.

<!-- command: stop -->
### `orisema stop`

Syntax: `orisema stop --journal <absolute-directory> <manifest.yaml>
<service>`. Same guard as `start`, with exact `STOP` confirmation and final
stopped-state verification. There is no implicit restart or rollback.

## Secret metadata and lifecycle

<!-- command: secret create -->
### `orisema secret create`

Syntax: `orisema secret create <name> --provider podman --stdin`. Runtime
mutation: reads a bounded value only from stdin and creates an owned Podman
secret. On failure, inspect metadata; the value is never printed or recoverable
through Orisema. Evidence: Alpha 6 lifecycle lab.

<!-- command: secret list -->
### `orisema secret list`

Syntax: `orisema secret list`. Runtime read of managed secret metadata only.
Values are never returned. Restore Podman read access on failure.

<!-- command: secret inspect -->
### `orisema secret inspect`

Syntax: `orisema secret inspect <name>`. Runtime read of one owned secret's
metadata and consumers; never reads its value.

<!-- command: secret remove -->
### `orisema secret remove`

Syntax: `orisema secret remove <name>`. Runtime mutation after ownership,
consumer and immutable-ID checks plus exact confirmation. A referenced or
ambiguous secret remains untouched.

## Controlled update

All continuation commands below require the same explicit initialized journal
and exact operation ID. Corrupt, unsafe, conflicting or stale evidence blocks
before mutation.

<!-- command: update plan -->
### `orisema update plan`

Syntax: `orisema update plan <manifest.yaml>`. Runtime read. Reports an
image-only candidate only when ownership, supported configuration and health
contract are eligible. For observed drift it reports closed, value-free
difference categories and a deterministic fingerprint. Categories do not
authorize configuration replacement, and secret values and private locators
are excluded. It creates no intent.

<!-- command: update prepare -->
### `orisema update prepare`

Syntax: `orisema update prepare <manifest.yaml> <service> --journal
<directory>`. Runtime read plus journal write. Persists one plan-bound pending
intent without confirmation or Podman mutation. Beta 1 additionally permits a
review-only intent for exclusive `PUBLIC_CONFIG` drift on an otherwise
unconfigured service; it stores names and fingerprints without values.

<!-- command: update authorize -->
### `orisema update authorize`

Syntax: `orisema update authorize <manifest.yaml> <service> --journal
<directory> --operation <id>`. Runtime revalidation plus journal write after
exact authorization confirmation. It still makes no Podman change.
Public-configuration intents require application, service and operation ID in
the confirmation and may advance only to non-executable `PREPARING` after fresh
evidence matches.

<!-- command: update apply -->
### `orisema update apply`

Syntax: `orisema update apply <manifest.yaml> <service> --journal <directory>
--operation <id>`. Journaled runtime mutation after exact confirmation. It
preserves the previous container and creates an unverified candidate. Failures
receive no automatic cleanup, retry or rollback.
For a Beta 1 public-configuration intent, exact confirmation includes the
operation ID and the command may only preserve the previous unit and create the
candidate. It remains `APPLYING` until explicit verify; cleanup is blocked.

<!-- command: update verify -->
### `orisema update verify`

Syntax: `orisema update verify <manifest.yaml> <service> --journal <directory>
--operation <id>`. Runtime read plus journal write. Records one observation and
decides `COMPLETED`, `RECOVERY_REQUIRED` or `WAIT`; it never polls or cleans up.
The same decisions apply to Beta 1 public-configuration candidates after
value-free configuration and exact resource evidence are revalidated.

<!-- command: update rollback -->
### `orisema update rollback`

Syntax: `orisema update rollback <manifest.yaml> <service> --journal
<directory> --operation <id>`. Journaled runtime mutation available only to an
intact `RECOVERY_REQUIRED` update after exact confirmation. It preserves the
failed candidate and restores the exact previous container; application data
is not rolled back. Public-configuration rollback restores that preserved
identity as-is; old public values are absent from the journal and are not
reconstructed from the current manifest.

<!-- command: update rollback-verify -->
### `orisema update rollback-verify`

Syntax: `orisema update rollback-verify <manifest.yaml> <service> --journal
<directory> --operation <id>`. Runtime read plus journal write. Only healthy
evidence commits `ROLLED_BACK`; other outcomes remain intervention-required.

## Configuration backup

<!-- command: backup create -->
### `orisema backup create`

Syntax: `orisema backup create <manifest.yaml> --output <new-directory>
--journal <operations> [--operations <existing-journal>]`. File/journal write.
Creates a private configuration and optional inert-history artifact; excludes
volumes, data, secrets, credentials, images and runtime state. Remove only the
new artifact after a failed, reviewed attempt.

<!-- command: backup verify -->
### `orisema backup verify`

Syntax: `orisema backup verify <artifact>`. Offline read. Checks strict layout,
permissions and SHA-256 integrity, not publisher authenticity. Preserve a
failed artifact for investigation.

<!-- command: backup restore -->
### `orisema backup restore`

Syntax: `orisema backup restore <artifact> --destination <absent-directory>
--journal <operations>`. File/journal write after exact confirmation. Publishes
only to an absent destination; it never resumes runtime operations.

## SSH and remote read-only inspection

<!-- command: host check -->
### `orisema host check`

Syntax: `orisema host check <registry.yaml> <alias>`. Remote read-only transport
probe using fixed `true`, strict host keys, key-only batch auth and timeouts.
Fix registry/key/connectivity evidence; no arbitrary remote command is exposed.

<!-- command: remote status -->
### `orisema remote status`

Syntax: `orisema remote status <registry.yaml> <alias> <manifest.yaml> --audit
<absolute-path>`. Remote runtime read plus local private audit append. Uses
fixed manifest-derived inspect calls. Preserve audit and fix transport or
ownership failures; no remote mutation occurs.

<!-- command: remote logs -->
### `orisema remote logs`

Syntax: `orisema remote logs <registry.yaml> <alias> <manifest.yaml> <service>
--audit <absolute-path>`. Remote runtime read plus audit append. Returns an
ownership-validated, 100-line and 1 MiB-bounded tail.

## Recovery history and bounded START

The recovery history is separate from the operation store. It is private but
not tamper-resistant. Only `recover apply` can mutate Podman, and only by START.

<!-- command: recover operation -->
### `orisema recover operation`

Syntax: `orisema recover operation <manifest.yaml> --journal <directory>
--operation <id>`. Delegates only an intact `RECOVERY_REQUIRED` update to the
same explicitly confirmed rollback path. Unsupported states remain unchanged.

<!-- command: recover init -->
### `orisema recover init`

Syntax: `orisema recover init <manifest.yaml> --history <file> --operator
<name>`. History-file write. Creates a private application-bound history; it
does not query or change Podman.

<!-- command: recover history -->
### `orisema recover history`

Syntax: `orisema recover history <manifest.yaml> --history <file>`. History
read only. Shows bounded records and unresolved executions without inferring an
outcome.

<!-- command: recover plan -->
### `orisema recover plan`

Syntax: `orisema recover plan <manifest.yaml> [--history <file>]`. Runtime and
optional history read. Reports typed guidance and guardrail eligibility; it
never executes the recommendation.

<!-- command: recover prepare -->
### `orisema recover prepare`

Syntax: `orisema recover prepare <manifest.yaml> <service> --history <file>
--operator <name> --reason <text>`. Runtime read plus history write. Reserves
one eligible START token for five minutes; no Podman mutation occurs.

<!-- command: recover apply -->
### `orisema recover apply`

Syntax: `orisema recover apply <manifest.yaml> <service> --history <file>
--token <token>`. History write and runtime START after exact `APPLY START`
confirmation. Records command success/failure but never calls that application
recovery; verify separately. No retry or rollback is available.

<!-- command: recover verify -->
### `orisema recover verify`

Syntax: `orisema recover verify <manifest.yaml> <service> --history <file>
--token <token>`. Runtime read plus history write. Appends a fresh observation
without inferring causality or changing Podman.

<!-- command: recover reconcile -->
### `orisema recover reconcile`

Syntax: `orisema recover reconcile <manifest.yaml> <service> --history <file>
--token <token> --outcome <succeeded|failed> --operator <name> --reason <text>`.
Runtime read plus confirmed history write for an unresolved execution. It
records an operator assertion and never changes Podman.

<!-- command: recover record -->
### `orisema recover record`

Syntax: `orisema recover record <manifest.yaml> <service> START
<succeeded|failed> --history <file>`. History write of operator-supplied legacy
evidence. It does not inspect or mutate Podman and must not be treated as a
verified execution result.

## Verification command

From the repository root:

```sh
go test ./cmd/orisema -run TestHelpAndOperationalInventoryCoverEveryPublicCommand
```

The full CI additionally exercises packaging, Ubuntu quickstarts, real Podman,
failure injection, recovery, SSH transport, backup/restore and secret lifecycle.
