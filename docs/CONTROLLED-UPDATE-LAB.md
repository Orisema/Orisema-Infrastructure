# Controlled update and application rollback laboratory

Status: **Alpha 3 design and executable laboratory**. This is not a production
update command and is intentionally not exposed through the Orisema CLI.

## Boundary

The laboratory proves a conservative single-service container replacement:

1. inspect and plan without mutation;
2. require exact `UPDATE web` confirmation;
3. revalidate container identity, ownership, image and bind immediately before
   the first mutation;
4. stop and rename the old container into an exact rollback slot;
5. create the candidate under the original name;
6. run a fresh healthcheck and require `healthy`;
7. automatically restore the same old container when candidate verification
   fails;
8. retain the old container after success so an explicit application rollback
   can restore it.

Only application container/image/configuration state is rolled back. The lab
does **not** restore databases, files, volume contents, external APIs or schema
migrations. A healthy old container is not evidence that its data remains
compatible with the newer application.

## State machine

```text
OLD_RUNNING
    | confirm + revalidate
    v
OLD_STOPPED_AS_ROLLBACK -- create candidate --> CANDIDATE_RUNNING
    ^                                            |
    | health failure                             | healthy
    +--------------------------------------------+--> UPDATED_WITH_ROLLBACK_SLOT
                                                       |
                                                       | explicit rollback
                                                       v
                                                   OLD_RUNNING
```

The first mutation is reversible only while the rollback container remains
present. The rollback slot is never overwritten. An occupied slot, ownership
mismatch, identity change or configuration mismatch fails closed.

## Partial failure and interruption

- Failure before the old container is renamed makes no runtime change.
- Failure to create or health-verify the candidate removes only a candidate
  whose exact Orisema ownership labels were verified, then renames and starts
  the old container.
- Interruption after the rename can leave the old container stopped under the
  rollback name. The evidence identifies both exact names and IDs; automatic
  inference after process death is deliberately not claimed.
- Interruption after candidate creation can leave both candidate and rollback
  containers. An operator must inspect IDs, labels, image and health before
  choosing recovery. Re-running must fail while the rollback slot is occupied.

A future CLI implementation requires a durable, fsynced, versioned operation
journal and reconciliation command before it may automate these interrupted
states. The existing recovery journal is START-specific and is not reused.

## External-resource preservation

The laboratory creates a sentinel container with a named volume. It records the
sentinel ID and volume inspection before the update, then proves both remain
unchanged after failed update, successful update and explicit rollback. Update
steps address only two exact container names and never pass `--volumes`.

This evidence does not turn volumes into a supported manifest feature. It only
proves that the laboratory does not remove or rewrite an unrelated volume.

## Run

Use only an isolated Linux host with Podman and free loopback port `18088`:

```sh
printf 'UPDATE web\n' | scripts/alpha3-update-lab.sh
```

The script uses exact `orisema-alpha3-*` names, saves JSON/text evidence under
`work/alpha3-update`, and removes only its own laboratory resources on exit.
The CI runs the same script and preserves that evidence as an artifact.
