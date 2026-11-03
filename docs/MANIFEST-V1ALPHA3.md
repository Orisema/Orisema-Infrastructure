# Manifest contract — `orisema.dev/v1alpha3`

Status: **partial Alpha 6 implementation**. Podman references can be resolved
and delivered locally; the `file` provider and secret lifecycle CLI remain
blocked until their separate gates pass.

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

## Current provider behavior

`orisema validate` validates the new static contract without resolving a
provider or reading secret material. `plan` resolves metadata only. A missing,
mismatched or unavailable reference blocks before mutation. For `podman`, the
resolved immutable ID is revalidated after confirmation and is passed to
`podman run --secret`; the mutable locator is not used for creation.

The container receives a read-only file at `/run/secrets/<target>`. Orisema
stores a provider/ID binding in a managed label so later plans can detect drift
without hashing or reading content. Update and rollback remain blocked for
services with secret bindings until those flows preserve the same contract.

The internal provider interface exposes references and allowlisted metadata
only. It has no content/value field and never invokes `podman secret inspect
--showsecret`. File permission checks, descriptor-safe handoff and lifecycle
commands remain future Alpha 6 increments governed by
[RFC-0002](RFC-0002-secure-secrets.md).

The repository includes
[`examples/alpha6-secret-references.yaml`](../examples/alpha6-secret-references.yaml)
for the isolated Podman laboratory. Its reference exists only while the test
creates a random disposable secret; it is not production configuration.

## Migration

Applications without secrets do not need to migrate. To experiment with static
validation, change `apiVersion` from `v1alpha2` to `v1alpha3` and add the
declarations/bindings above. Do not expect deployment until the delivery gate
is implemented and tested.
