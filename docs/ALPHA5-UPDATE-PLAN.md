# Alpha 5 conservative update flow

The first Alpha 5 slice exposes only conservative guidance:

```sh
orisema update plan manifest.yaml
```

The command reads the manifest and local Podman inspection but never creates a
journal, asks for confirmation, pulls an image or changes a container.

A service is `UPDATE_CANDIDATE` only when its Orisema ownership is exact, every
supported declared field except the configured image already matches, the
current image is known and different, and the manifest declares a healthcheck.
Matching services are `NOOP`. Missing containers, inspection failures,
ownership ambiguity, absent healthchecks and compound drift are `BLOCKED`.

Candidate status is not authorization to update. A later slice must persist the
intent and previous resource evidence, acquire application exclusion, request
explicit confirmation and revalidate immediately before any mutation.

## Persisted intent

An eligible service can now be recorded without executing it:

```sh
orisema update prepare manifest.yaml service --journal /secure/path/operations
```

The command repeats the live read-only plan, then creates one `PENDING` Alpha 4
operation containing the exact container ID, current and target images,
comparison contract, previous-resource evidence and a deterministic SHA-256
plan fingerprint. The existing per-application exclusion prevents concurrent
intents. Reopening the journal recomputes the fingerprint and rejects accidental
or partial alteration.

This is not a cryptographic signature or protection against an attacker who can
rewrite the entire private journal. It does not request execution confirmation,
pull an image or mutate Podman. A future executor must collect fresh evidence
and must not treat the persisted fingerprint as sufficient authorization.

## Explicit authorization

A pending intent can be authorized for preparation during its five-minute
validity window:

```sh
orisema update authorize manifest.yaml service \
  --journal /secure/path/operations --operation update-...
```

The operator must type `AUTHORIZE UPDATE application/service` exactly. The
command then rebuilds the live plan and requires the same container ID,
ownership, current/target images, supported configuration and fingerprint. Only
then does the journal atomically move from `PENDING` to `PREPARING`, preserving
the fresh resource evidence. Wrong confirmation, expiry, concurrency or any
drift leaves the snapshot unchanged.

`PREPARING` means revalidated authorization, not execution. This command still
cannot pull an image, stop, rename, create or remove a container.

## Preserving replacement execution

An authorized intent can execute its first runtime-changing slice:

```sh
orisema update apply manifest.yaml service \
  --journal /secure/path/operations --operation update-...
```

The operator must type `APPLY UPDATE application/service` exactly. The command
rebuilds the plan and requires the same container ID, ownership, image,
configuration and fingerprint while the authorization is still fresh. Before
the first mutation the journal enters `APPLYING`; it also persists each pending
runtime step before stopping the old container, renaming it to the operation's
unique rollback name, and creating the candidate under the declared service
name. Successful step evidence records both container identities and names.

The previous container is stopped and preserved, never removed. Candidate
creation uses the declared managed labels, ports, restart policy and
healthcheck. Any interruption or failure leaves the partial state and pending
step diagnostic in the journal; this slice deliberately performs no automatic
cleanup, retry, resume or rollback.

Candidate creation is not update success. The operation remains `APPLYING` and
no health conclusion is inferred.

## Post-update health decision

Each explicit verification command records one fresh observation:

```sh
orisema update verify manifest.yaml service \
  --journal /secure/path/operations --operation update-...
```

The command revalidates the exact candidate ID, preserved previous ID,
ownership, image intent and stopped state of the previous container before it
atomically starts `VERIFYING`. Its bounded policy derives from the declared
healthcheck interval, timeout and retry count. A `starting`, unknown or
non-terminal unhealthy observation remains `VERIFYING` with decision `WAIT`;
the operator may invoke the command again to record another observation.

A configured `healthy` result is not committed until both containers are
reinspected. It produces the durable state `COMPLETED`. A stopped candidate,
missing healthcheck, timeout or the declared bounded sequence of unhealthy
results produces `RECOVERY_REQUIRED`. Missing resources, changed identity,
ownership drift, image drift, concurrent decisions or journal inconsistency
fail closed.

Verification never polls, sleeps, changes Podman, removes the previous
container, retries the update or executes rollback. `RECOVERY_REQUIRED` is the
only state accepted by the explicit manual rollback.

## Manual application rollback

`update rollback` requires the exact confirmation `ROLLBACK UPDATE
application/service operation-id`. It freshly validates both container IDs,
images and ownership, persists `ROLLING_BACK` and every next mutation before
stopping and renaming the failed candidate, restoring the preserved container
to its original name and starting it. The failed candidate remains stopped
under a unique diagnostic name; nothing is deleted.

`update rollback-verify` is a separate, Podman-read-only step. It revalidates
the restored and failed containers and records one health observation. Only a
configured `healthy` result commits `ROLLED_BACK`; every other supported result
remains `ROLLING_BACK`. Replay, changed identity, ownership or image, occupied
diagnostic slots, incomplete steps and journal inconsistency fail closed.

This is application-container rollback only. It never restores data, volumes,
secrets or external resources and performs no automatic cleanup or resume.
