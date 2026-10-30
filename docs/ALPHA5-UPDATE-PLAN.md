# Alpha 5 read-only update planning

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
