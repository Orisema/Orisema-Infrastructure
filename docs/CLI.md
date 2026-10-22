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

## Experimental deployment (create-only)

**Use only on a disposable test host.** Run `validate` and inspect the manifest first.

```sh
go run ./cmd/orisema deploy examples/nginx.yaml
# Type DEPLOY when prompted.
go run ./cmd/orisema status examples/nginx.yaml
curl -I http://127.0.0.1:8080/
```

Deployment uses `podman run --detach` with a managed label and `no-new-privileges`. It refuses an existing exact container name and never uses `--replace`. Published ports bind on all host interfaces by default, so verify the firewall and avoid production servers.

**Known limitations:** No rollback, no idempotent updates, no volume support, no health verification, no preflight check for port conflicts, no image digest enforcement, no transaction across multiple services. A failure may leave a created container behind. Podman itself detects conflicts when creating containers. Do not use this command for sensitive workloads.

To clean up the test manually after confirming its identity:

```sh
podman stop orisema-hello-orisema-web
podman rm orisema-hello-orisema-web
```

## Conservative repeated deployment (experimental)

After the first deployment, rerunning `go run ./cmd/orisema deploy examples/nginx.yaml` should classify the existing owned container as `NOOP` and exit without a confirmation prompt or modifications, **provided its configured image reference and published ports match**.

If any service is `DRIFT` or `REVIEW`, deployment is blocked for the entire manifest before creating any containers. `CREATE` services still require typing `DEPLOY`. A second check for name collisions happens immediately before each creation.

**Limits:** This is idempotent only for the fields currently compared (ownership labels, image reference, TCP port bindings). `NOOP` does not imply that the container is running or healthy, that the image digest matches, or that security options and other settings are equivalent. A multi-service deployment can partially complete if a later create fails. No rollback, automatic reconciliation or destructive update exists.
