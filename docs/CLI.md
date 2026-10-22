# CLI prototype (v0.1.0-dev)

This is a **proof of concept**, not a deployment engine. `status` reads local Podman state; `validate` and `plan` work offline.

## Requirements

Go 1.23+ and access to the Go module dependency `gopkg.in/yaml.v3`.

## Run

```sh
go mod tidy
go test ./...
go run ./cmd/orisema validate examples/nginx.yaml
go run ./cmd/orisema plan examples/nginx.yaml
go run ./cmd/orisema status examples/nginx.yaml
```

The `validate` command checks the manifest structure, service identifiers, images and port ranges.

The `status` command reads local Podman containers without changing them (see [PODMAN.md](PODMAN.md)).

The `plan` command **only prints the requested services**. It does not connect to Podman, inspect the host, create containers, reserve ports or compare current state. Do not interpret this output as a safe-to-apply plan.

## Scope and limits

- YAML schema: `orisema.dev/v1alpha1`; subject to breaking changes.
- Only service names, images and TCP port mappings are represented.
- No support yet for secrets, storage, networking, security contexts or runtime operations.
- Do not use this prototype to deploy production applications.
