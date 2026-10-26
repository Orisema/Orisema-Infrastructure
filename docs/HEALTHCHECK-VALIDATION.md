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

## Executed isolated CI validation

[GitHub Actions run 40317976621](https://github.com/Orisema/Orisema-Infrastructure/actions/runs/40317976621)
validated commit `50f75df1c1db634c8ac7e755681494c8b2f6b771` on Ubuntu
24.04 with rootful Podman 4.9.3 and Go 1.23.12. Vet and unit tests passed;
creation, manual probe, real inspect comparison, `NOOP` and in-memory `DRIFT`
checks passed. A successful automatic probe was observed after the manual
probe. The run retains inspect JSON and plans as artifacts.

This establishes one runtime configuration. The original Ubuntu server,
rootless execution and other Podman versions still need their own validation.
The original `hello-orisema` laboratory was not accessed or changed.

## Next incremental change implemented

`status` now separates missing health configuration from unavailable health.
It accepts the known `starting`, `healthy` and `unhealthy` observations from
either runtime health field, reports conflicts conservatively, and suppresses
stale health observations for stopped or paused containers. Unit tests cover
synthetic inspect variants. CI also checks real healthy and stopped states.
Planning remains read-only; a stopped matching container remains `NOOP` and is
not restarted automatically. Ownership protections are unchanged.

Next, collect fixtures from additional supported Podman versions, especially
rootless environments, before adding restart policies in a separate change.
