# Manifest contract — `orisema.dev/v1alpha3`

Status: **partial Alpha 6 implementation**. The parser and static validation
accept external secret references, but delivery is deliberately blocked until
the provider backend and its integration tests exist.

## Compatibility

`v1alpha3` preserves the `v1alpha2` application shape and explicit loopback
port rules. It adds only secret declarations and service bindings. `v1alpha1`
and `v1alpha2` continue to reject the `secrets` key, including empty and null
values; there is no hybrid contract.

## Shape

```yaml
apiVersion: orisema.dev/v1alpha3
kind: Application
name: example

secrets:
  - name: database-password
    source:
      podman:
        name: example-database-password
  - name: legacy-api-key
    source:
      file:
        path: /home/operator/.config/orisema/secrets/legacy-api-key

services:
  - name: app
    image: registry.example/app:1.0.0
    secrets:
      - name: database-password
        target: database-password
      - name: legacy-api-key
        target: legacy-api-key
```

Every declaration has exactly one `podman` or `file` source. File paths must be
absolute and lexically clean; this validation does not read the file or prove
its runtime permissions. Every binding references a declaration and maps it to
a unique, identifier-shaped filename. Future delivery is fixed to
`/run/secrets/<target>`.

An application may declare at most 64 secrets and a service may bind at most
64. Inline values, custom target paths, environment projection and unknown
fields are rejected.

## Current fail-closed behavior

`orisema validate` validates the new static contract without resolving a
provider or reading secret material. A plan containing any secret binding is
classified `REVIEW` with the reason that delivery is not implemented. As a
result, `deploy` makes no changes. This is intentional: accepting a declaration
and starting the container without its secret would be an unsafe partial
implementation.

The internal provider interface exposes references and allowlisted metadata
only. It has no content/value field. Podman inspection, file permission checks,
descriptor-safe handoff, mounting and lifecycle commands remain future Alpha 6
increments governed by [RFC-0002](RFC-0002-secure-secrets.md).

The repository includes
[`examples/alpha6-secret-references.yaml`](../examples/alpha6-secret-references.yaml)
for this static-validation and fail-closed planning behavior. It intentionally
references a nonexistent example secret and must not be used as a deployment
claim.

## Migration

Applications without secrets do not need to migrate. To experiment with static
validation, change `apiVersion` from `v1alpha2` to `v1alpha3` and add the
declarations/bindings above. Do not expect deployment until the delivery gate
is implemented and tested.
