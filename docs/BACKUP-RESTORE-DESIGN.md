# Backup and restore design

Status: architecture accepted; executable commands are not implemented yet.

## Supported backup set

| Class | First implementation | Restore behavior |
|---|---|---|
| Validated application manifest | Required | New private destination only |
| Application operation journals | Optional | Read-only imported history |
| Container/image runtime state | No | Never inferred or recreated |
| Volumes, bind mounts, databases | No | Never read or written |
| Secrets, SSH keys, credentials | No | Rejected |

The boundary deliberately reflects the current public manifest. A successful
backup does not mean application data was protected.

## Artifact contract

The artifact is a private directory, not a general archive:

```text
backup-<id>/                 mode 0700
  metadata.json             mode 0600
  metadata.sha256           mode 0600
  payload/
    manifest.yaml           mode 0600
    operations.jsonl        mode 0600, optional canonical export
```

Format `orisema.backup/v1alpha1` metadata contains a random backup ID, UTC
timestamp, application, build version, explicit scope and exclusions, plus a
sorted inventory with fixed logical name, media type, byte count and lowercase
SHA-256. JSON is canonical. Unknown versions or entries, duplicates, unexpected
files, non-regular files, symlinks, unsafe permissions, size mismatch or hash
mismatch are fatal.

Initial limits are 1 MiB for the manifest, 32 MiB for journal export, 64 entries
and 64 MiB total. Raising limits is a versioned contract change.

## Backup transaction

1. Validate the manifest and optional same-application journal read-only.
2. Refuse an existing destination and acquire application exclusion.
3. Persist `BACKUP/PREPARING` with source identity and destination.
4. Create private same-filesystem staging and persist every next step.
5. Copy bounded regular files while hashing; reject source changes.
6. Write metadata last, fsync files and staging, revalidate the absent final
   path, rename atomically and fsync the parent.
7. Reopen and verify the published artifact before persisting `COMPLETED`.

Failures keep a diagnostic journal and private unique staging. They never
delete sources or an existing destination.

## Restore transaction

The initial restore accepts only an absent destination and requires exact
confirmation `RESTORE BACKUP <backup-id> <application>`. It fully verifies the
artifact before recording intent or writing payloads. It copies only known
logical entries to fixed destinations in private same-filesystem staging,
validates the manifest again, fsyncs, rechecks absence and publishes atomically.

Imported operation history is never placed into the active operation store and
cannot authorize, resume or replay a mutation.

## Required implementation laboratory

The implementation PR must prove exact backup/restore in isolation; unchanged
source and sentinel files; rejection of corrupted payload and metadata,
truncation, oversize, symlink, FIFO, duplicate/unknown entries, unsafe modes and
versions; one safe concurrency winner; no overwrite on a destination race; and
diagnosable interruption at every persisted boundary.

See [ADR 0003](adr/0003-safe-backup-boundary.md) for the binding decision.
