# Declarative healthcheck validation (experimental)

The local automated suite verifies manifest validation, planner comparison and
actual subprocess argument boundaries using a fake runtime. It does not establish
compatibility with a real Podman release or prove application health.

## Isolated laboratory

Keep `orisema-hello-orisema-web` unchanged. Use only `health-lab` on port 8081.
Before deployment, record the server runtime version and confirm these options
are supported by that version:

```sh
podman version
podman run --help | grep -E 'health-(cmd|interval|timeout|retries)'
go test ./...
go vet ./...
go run ./cmd/orisema validate examples/nginx-health.yaml
go run ./cmd/orisema plan examples/nginx-health.yaml
```

Approve the plan before running deploy. Only proceed if the isolated service is
`CREATE`; if it exists, inspect it and do not remove or replace it implicitly.

```sh
go run ./cmd/orisema deploy examples/nginx-health.yaml
go run ./cmd/orisema status examples/nginx-health.yaml
podman healthcheck run orisema-health-lab-web
podman inspect --type container orisema-health-lab-web
go run ./cmd/orisema plan examples/nginx-health.yaml
ORISEMA_TEST_HEALTH_MANIFEST=../../examples/nginx-health.yaml go test -v ./internal/planner -run TestPodmanHealthcheckIntegrationReadOnly
```

Record the version, command outputs and exit codes. Expect `Config.Healthcheck`
to contain `Test: ["CMD-SHELL", "nginx -t"]`, interval `30000000000`, timeout
`5000000000` (nanoseconds), and retries `3`. The final plan must be `NOOP`.
The opt-in integration test reads real state, then changes only its in-memory
manifest to verify `DRIFT`; it never changes containers.

A successful `nginx -t` verifies Nginx configuration only. Scheduled probes may
require systemd/timers in the server environment. A manual probe succeeding does
not establish that automatic scheduling works: observe subsequent health logs
and timestamps with `podman inspect` over more than one interval.

The local development environment has no Podman, so real inspect output, flags,
scheduling, deployment and probe execution remain unvalidated. Do not describe
this feature as laboratory-validated until the above results are available.

## Next incremental change

After this lab passes, add fixtures captured from supported Podman versions and
tests for the two health status fields (`State.Healthcheck` and `State.Health`).
Define precedence for conflicting fields and represent unavailable health
explicitly, independently of running/stopped state. Keep plan read-only and
preserve ownership checks. Restart policies and new manifest fields should be
separate changes with validation, tests and documentation.
