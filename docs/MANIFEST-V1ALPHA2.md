# Manifest contract — `orisema.dev/v1alpha2`

Status: **accepted for Alpha 2 on 2026-10-28**. This is still an experimental
contract and is not a production-readiness claim.

## Decision

Alpha 2 introduces a new API version instead of changing the meaning of
`v1alpha1` in place. Both versions are readable:

| Contract | Status | Port bind behavior |
| --- | --- | --- |
| `orisema.dev/v1alpha1` | legacy compatibility | no `kind` or `hostIP`; Podman binds published ports on all interfaces |
| `orisema.dev/v1alpha2` | current Alpha contract | requires `kind: Application`; every published port requires an explicit loopback `hostIP` |

Unknown YAML fields fail validation in both versions. A field added to the Go
data model is not automatically valid in an older API version: version-specific
validation preserves the old contract.

## `v1alpha2` shape

```yaml
apiVersion: orisema.dev/v1alpha2
kind: Application
name: hello-orisema
services:
  - name: web
    image: docker.io/library/nginx:1.27
    ports:
      - host: 8080
        container: 80
        hostIP: 127.0.0.1
```

`hostIP` accepts only IP literals for loopback addresses (`127.0.0.0/8` or
`::1`). Hostnames, an omitted value, `0.0.0.0`, `::` and non-loopback addresses
are rejected. IPv6 is passed to Podman with bracketed publish syntax. Planning
compares the declared and observed IP addresses semantically and reports
`DRIFT` if they differ.

A host port cannot be reused anywhere in one manifest, and a container port
cannot be published more than once within the Alpha 2 contract. This keeps the
planner's one-to-one comparison unambiguous.

This restriction intentionally makes network exposure a future design decision
instead of an accidental default. Proxying or exposing the loopback service is
outside this contract.

## Migration from `v1alpha1`

Migration is explicit and manual:

1. Change `apiVersion` to `orisema.dev/v1alpha2`.
2. Add `kind: Application`.
3. Add `hostIP: 127.0.0.1` (or `::1`) to every published port.
4. Run `orisema validate`, then inspect `orisema plan` before deployment.

Existing `v1alpha1` manifests remain readable. Adding `kind` or `hostIP` while
retaining `v1alpha1` is rejected; there is no hybrid contract. Existing
containers created with an all-interface bind will become `DRIFT` when planned
against a migrated loopback manifest. Alpha 2 does not replace them
automatically.

## Deliberately unsupported configuration

The following service fields remain unknown and are rejected:

- environment variables and environment files;
- secrets or credential references;
- bind mounts, named volumes and storage migration;
- custom users, capabilities, privileged mode and other security context;
- custom networks and resource limits.

Raw secrets will not be accepted inline. A future secrets design must define
the provider, lifetime, file permissions, redaction, rotation and behavior when
the provider is unavailable. Volumes require ownership, backup and rollback
semantics first. Permission fields require a threat model and runtime parity.

The executor continues to pass `no-new-privileges` for created containers, but
that single option is not a complete permissions model.

## CLI output and exit codes

Alpha 2 keeps human-readable output only. `--output json` is rejected as usage,
because the project does not yet have stable typed result and error envelopes.
Adding ad-hoc JSON now would create an undocumented compatibility contract and
risk serializing runtime errors or sensitive values.

The stable exit-code classes are:

| Code | Meaning |
| --- | --- |
| `0` | requested operation completed successfully, including read-only validation or a genuine no-op |
| `1` | valid command failed, was blocked/cancelled, or found invalid input/runtime state |
| `2` | command-line usage is invalid or the command is unsupported |

Human-readable wording is not a machine interface. Errors go to stderr where
the current CLI can distinguish them; successful reports go to stdout. Output
must not include secret values, and the accepted manifest has no secret-bearing
fields.

## Compatibility rule for future changes

A new field or changed meaning requires one of:

- a backward-compatible optional field whose omission has the documented old
  behavior; or
- a new `apiVersion`, migration instructions and tests proving old manifests
  retain their prior validation/runtime semantics.

No feature may silently widen a network bind, enable privilege, expose a
credential or mutate persistent data.
