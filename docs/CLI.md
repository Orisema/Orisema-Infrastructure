# CLI prototype (v0.1.0-dev)

This is an **experimental laboratory CLI**, not production-ready. `validate` works offline; `plan`, `status` and lifecycle commands query local Podman.

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

The `plan` command inspects Podman and compares selected declared fields only. It does not reserve ports or provide full configuration equivalence; see [PLANNING.md](PLANNING.md).

## Scope and limits

- YAML schema: `orisema.dev/v1alpha1`; subject to breaking changes.
- Only service names, images and TCP port mappings are represented.
- No support yet for secrets, persistent storage, custom networks, or full security context management.
- Do not use this prototype to deploy production applications.

## Experimental deployment

**Use only on a disposable test host.** Run `validate` and inspect the manifest first.

```sh
go run ./cmd/orisema deploy examples/nginx.yaml
# Type DEPLOY when prompted.
go run ./cmd/orisema status examples/nginx.yaml
curl -I http://127.0.0.1:8080/
```

Deployment uses `podman run --detach` with a managed label and `no-new-privileges`. It skips matching owned containers, blocks DRIFT/REVIEW and never uses `--replace`. Published ports bind on all host interfaces by default, so verify the firewall and avoid production servers.

**Known limitations:** No rollback, no automatic updates, no volume support, no health verification, no preflight check for port conflicts, no image digest enforcement, no transaction across multiple services. A failure may leave a created container behind. Podman itself detects conflicts when creating containers. Do not use this command for sensitive workloads.

To clean up the test manually after confirming its identity:

```sh
podman stop orisema-hello-orisema-web
podman rm orisema-hello-orisema-web
```

## Conservative repeated deployment (experimental)

After the first deployment, rerunning `go run ./cmd/orisema deploy examples/nginx.yaml` should classify the existing owned container as `NOOP` and exit without a confirmation prompt or modifications, **provided its configured image reference and published ports match**.

If any service is `DRIFT` or `REVIEW`, deployment is blocked for the entire manifest before creating any containers. `CREATE` services still require typing `DEPLOY`. A second check for name collisions happens immediately before each creation.

**Limits:** This is idempotent only for the fields currently compared (ownership labels, image reference, TCP port bindings, declared healthcheck command and timing/retries). `NOOP` does not imply that the container is running or healthy, that the image digest matches, or that security options and other settings are equivalent. A multi-service deployment can partially complete if a later create fails. No rollback, automatic reconciliation or destructive update exists.

## Experimental service lifecycle

Commands require the service to be declared in the provided manifest and the existing container to have matching Orisema ownership labels (`dev.orisema.managed`, `dev.orisema.app`, `dev.orisema.service`).

```sh
go run ./cmd/orisema logs examples/nginx.yaml web
go run ./cmd/orisema stop examples/nginx.yaml web
# Type STOP to confirm
go run ./cmd/orisema start examples/nginx.yaml web
# Type START to confirm
```

`logs` returns only the last 100 lines and does not follow. `start` and `stop` require interactive confirmation, inspect ownership before issuing Podman commands, and never remove containers. They do not check desired image/ports, application health, or service dependencies. **Do not run stop on critical workloads.** The earlier `deploy` command still treats a stopped matching container as `NOOP` and will not automatically restart it.

## Detailed status and integration smoke test

`status` now attempts `podman inspect` for existing containers and displays running state, exit code, restart count, start/finish timestamps, ownership and health if available. Health without an explicit healthcheck is reported as unavailable, **not healthy**. Some Podman versions may expose different health fields.

The opt-in integration test only reads an existing test container; it does not create, start, stop or remove anything:

```sh
ORISEMA_TEST_CONTAINER=orisema-hello-orisema-web go test -v ./internal/runtime
go run ./cmd/orisema status examples/nginx.yaml
```

This integration test is skipped in CI unless `ORISEMA_TEST_CONTAINER` is set.

## Declarative healthchecks (experimental)

An optional `healthcheck` section is supported for each service:

```yaml
healthcheck:
  command: "nginx -t"
  interval: 30s
  timeout: 5s
  retries: 3
```

The command runs **inside the container** using Podman's shell healthcheck; ensure the image contains the required executable. Durations use Go duration syntax and must be at least one second. `retries` must be 1–100. The healthcheck definition is passed at creation time. For an existing container, a different or missing healthcheck results in `DRIFT`, and deployment stops without replacing it.

The separate `examples/nginx-health.yaml` app uses port 8081 and does not modify the existing `hello-orisema` container:

```sh
go run ./cmd/orisema validate examples/nginx-health.yaml
go run ./cmd/orisema plan examples/nginx-health.yaml
go run ./cmd/orisema deploy examples/nginx-health.yaml
# Type DEPLOY to create it.
sleep 35
go run ./cmd/orisema status examples/nginx-health.yaml
podman healthcheck run orisema-health-lab-web
```

The sample `nginx -t` check verifies Nginx configuration, **not HTTP reachability**. Health status may be `starting` or `unknown` before the first probe. Do not assume healthcheck support or inspect output are identical across all Podman versions. Existing containers are never silently updated.


The command is encoded as an explicit JSON `CMD-SHELL` test when passed to
Podman. Exec-form `CMD` tests are not equivalent to shell-form commands.
Durations are compared in nanoseconds, so `30s` and `0.5m` are equivalent.
An inspect test of `["NONE"]` means disabled: it matches an omitted manifest
healthcheck, but never a declared one. An active image-inherited healthcheck
without a manifest declaration is reported as `DRIFT`. Omission does not disable
an image's healthcheck during creation. Empty or unknown test forms fail the
comparison conservatively. Start period, startup checks and other undeclared
healthcheck settings are not compared; `NOOP` remains limited.

See [the healthcheck validation checklist](HEALTHCHECK-VALIDATION.md) for the
remaining real-Podman checks and the next incremental runtime work.
