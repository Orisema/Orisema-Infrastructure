# Manifest contract: `orisema.dev/v1alpha4`

`v1alpha4` is the opt-in Alpha 7 contract for explicitly public configuration
and references to pre-existing external Podman networks. It preserves every
validated `v1alpha3` field, including file-delivered secrets.

The current increment implements schema and validation only. `validate` accepts
a valid manifest, while `plan` returns `REVIEW` and `deploy` makes no changes
when `publicEnvironment` or service network bindings are present. Runtime
delivery will be enabled only after identity resolution and revalidation land.

## Public environment

```yaml
publicEnvironment:
  - name: APP_MODE
    value: laboratory
  - name: HTTP_PORT
    value: "8080"
```

These values are not secrets. They may be visible through Podman inspection,
the process environment, host process arguments and configuration backups.
Plans and validation errors avoid echoing values to reduce accidental exposure,
but that is not a secrecy guarantee. Use `secrets` for credentials and tokens.

Names use the portable environment form `[A-Za-z_][A-Za-z0-9_]{0,127}`. Values
are strings of at most 4096 bytes without NUL, CR or LF. Each service accepts at
most 128 entries and 32 KiB total. Duplicate names are rejected.

## External networks

```yaml
networks:
  - name: private
    source:
      podman:
        name: precreated-private-network

services:
  - name: web
    image: nginx:1.27-alpine
    networks:
      - name: private
```

Application network names are logical references. Podman names identify
external `bridge` networks that the operator creates separately. The first
runtime increment will inspect and revalidate immutable IDs but will never
create, adopt, change or remove those networks.

At most 32 networks may be declared and each service may bind at most 8.
Bindings must reference declarations, duplicates are rejected, every declared
network must be consumed, and every service must bind at least one network when
the application declares networks.

## Compatibility boundary

`v1alpha1`, `v1alpha2` and `v1alpha3` reject every presence of `networks` or
`publicEnvironment`, including `null` and empty sequences. This prevents a
manifest from appearing accepted while silently ignoring Alpha 7 semantics.

See [RFC-0003](RFC-0003-public-config-and-external-networks.md) for the threat
model, runtime design and remaining gates.
