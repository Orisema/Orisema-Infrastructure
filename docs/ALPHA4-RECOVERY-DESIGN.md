# Alpha 4 reliable recovery design

Status: first safe implementation slice. No update or rollback mutation is
exposed by the CLI.

## Audit of the existing recovery journal

The existing `internal/recovery` JSON Lines history is appropriate for its
narrow `START` flow: it uses restrictive permissions, rejects symbolic links,
locks readers and writers, synchronizes appended records and consumes a
reservation before Podman is called. A begun record without a result is kept as
an unresolved attempt and requires explicit reconciliation.

That schema intentionally accepts only `START`; extending its fields to mean a
multi-step update would weaken its validation and compatibility guarantees.
Alpha 4 therefore preserves it and introduces `internal/operation` as a
runtime-independent operation journal.

## Implemented contract

Each operation is one versioned JSON snapshot containing its ID, application,
services, type, current state, timestamps, resources, recovery evidence,
failure detail and the complete transition event list. Secrets must never be
placed in these fields.

Snapshots are written to a mode `0600` temporary file, synchronized, atomically
renamed and followed by a directory sync. The containing directory must be
private. Separate lock files serialize creation and transitions. Corrupt,
unsafe or unsupported-version snapshots fail closed.

Creation is also serialized across the store. A non-terminal operation reserves
its application name, so a second operation for that application is rejected.
Different applications remain independent, and only a terminal `COMPLETED` or
`ROLLED_BACK` operation releases the application. The read-only listing is
deterministic; one corrupt snapshot blocks both listing and conflict decisions.

Resource evidence is structured as `managed`, `external` or `unknown`.
Managed resources require exact identity plus application/service scope. External
resources require a stable identity and carry no managed scope. Unknown,
missing, duplicate or changed evidence blocks recovery. The pure revalidation
contract compares a fresh observation with the journal without querying or
mutating Podman; runtime collection remains a later integration step.

Allowed states are `PENDING`, `PREPARING`, `APPLYING`, `VERIFYING`,
`COMPLETED`, `FAILED`, `INTERRUPTED`, `RECOVERY_REQUIRED`, `ROLLING_BACK` and
`ROLLED_BACK`. Invalid transitions and timestamp regression are rejected.
Terminal operations cannot transition further.

## Conservative recovery boundary

The read-only assessment never authorizes automatic resume. Every incomplete
operation requires intervention. A `RECOVERY_REQUIRED` operation is only a
rollback candidate when both previous-version and recovery evidence exist; it
still requires fresh ownership/runtime validation and explicit authorization.

This slice does not inspect Podman, execute updates, remove containers or
volumes, resume an operation, or roll anything back. The read-only CLI command
is available as:

```sh
orisema operation diagnose --journal /secure/path/orisema-operations
```

It opens only an existing store, creates no directory or lock, and lists
incomplete operations with their conservative assessment.

## Objective health evaluation

The runtime-independent evaluator accepts a verification start, current time,
bounded policy and ordered observations. Only a running container with a
configured healthcheck observed as `healthy` inside the window succeeds.
Missing healthchecks, stopped containers, unknown results and unavailable
runtime never imply success. Consecutive `unhealthy` observations fail at the
configured threshold; `starting` may remain pending only until the deterministic
timeout. The evaluator does not poll, sleep, retry or mutate anything.

The journal can atomically move an operation from `APPLYING` to `VERIFYING`
while persisting the policy and initial deadline. Supplied observations and each
deterministic evaluation are then written into the same operation snapshot.
Reopening the store preserves the complete sequence. A refresh can record a
timeout without inventing a probe, and a terminal evaluation rejects further
observations. Persisted evaluation data is recomputed during load; mismatch or
corruption fails closed. No result currently triggers an operation transition
or runtime action.
