# Podman read-only inspection (v0.1.0-dev)

The `status` command reads the **local** Podman container list. It does not start, stop, remove or change anything.

```sh
go run ./cmd/orisema status examples/nginx.yaml
```

Container names are expected to follow `orisema-<manifest-name>-<service-name>`. A container with another name is not treated as managed by Orisema.

The command only reports existence, current state and image reference. It does **not** verify ports, volumes, image digests, security configuration or desired-state equivalence.

`plan` compares the supported declarative fields with local Podman state.
`deploy` creates only services classified as `CREATE`, after explicit
confirmation; it never replaces or updates an existing container.

Run as the same user that owns the rootless Podman containers. Rootful and rootless Podman show different container sets.

No credentials or application environment variables should be committed.
