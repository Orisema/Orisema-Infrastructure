# CLI prototype (Alpha 5 development)

This is an **experimental laboratory CLI**, not production-ready. `validate` works offline; `plan`, `status` and lifecycle commands query local Podman.

## Requirements

Precompiled release binaries do not require Go. Building from source requires
Go 1.23+ and access to the module dependency `gopkg.in/yaml.v3`. Commands that
inspect or change containers require Podman on Linux; `version` and `validate`
do not query Podman.

## Run

```sh
go run ./cmd/orisema version
go run ./cmd/orisema operation diagnose --journal /secure/path/orisema-operations
go run ./cmd/orisema update plan examples/nginx-health.yaml
go run ./cmd/orisema update prepare examples/nginx-health.yaml web --journal /secure/path/orisema-operations
go mod tidy
go test ./...
go run ./cmd/orisema validate examples/nginx.yaml
go run ./cmd/orisema plan examples/nginx.yaml
go run ./cmd/orisema status examples/nginx.yaml
go run ./cmd/orisema diagnose examples/nginx.yaml
go run ./cmd/orisema recover plan examples/nginx.yaml
go run ./cmd/orisema recover init examples/nginx-restart.yaml --history /secure/path/orisema-recovery.jsonl --operator alice
go run ./cmd/orisema recover plan examples/nginx-restart.yaml --history /secure/path/orisema-recovery.jsonl
go run ./cmd/orisema recover prepare examples/nginx-restart.yaml web --history /secure/path/orisema-recovery.jsonl --operator alice --reason "incident reviewed"
go run ./cmd/orisema recover apply examples/nginx-restart.yaml web --history /secure/path/orisema-recovery.jsonl --token <token-from-prepare>
go run ./cmd/orisema recover verify examples/nginx-restart.yaml web --history /secure/path/orisema-recovery.jsonl --token <consumed-token>
go run ./cmd/orisema recover history examples/nginx-restart.yaml --history /secure/path/orisema-recovery.jsonl
go run ./cmd/orisema recover reconcile examples/nginx-restart.yaml web --history /secure/path/orisema-recovery.jsonl --token <unresolved-token> --outcome succeeded --operator alice --reason "evidence reviewed"
go run ./cmd/orisema recover record examples/nginx-restart.yaml web START failed --history /secure/path/orisema-recovery.jsonl
```

See [COMPATIBILITY.md](COMPATIBILITY.md) for a compact command/mutation matrix
and [QUICKSTART.md](QUICKSTART.md) for the reviewed end-to-end laboratory.

The `validate` command checks the versioned manifest structure, service
identifiers, images, port ranges and version-specific bind rules. See the
[`v1alpha2` contract](MANIFEST-V1ALPHA2.md).

`operation diagnose` reads the Alpha 4 operation journal without querying
Podman or changing journal files. It lists only incomplete operations and
always reports automatic resume as unsafe. Missing, corrupt or unsafe journal
state returns an error instead of being treated as empty.

The `status` command reads local Podman containers without changing them (see [PODMAN.md](PODMAN.md)).

The `diagnose` command is also read-only and presents desired configuration,
observed execution, application health, detected failure and recommended action
as separate fields. It does not restart, replace or otherwise modify a container.

The `recover plan` command converts that diagnosis into typed operator guidance
with preconditions, risk and confirmation requirements. It is read-only; a
separate command is required for execution. Optional declarative recovery guardrails
are displayed. An explicit versioned history journal feeds the guardrail
decision, and `recover record` can append an operator-supplied START result.
`recover prepare` revalidates the runtime and atomically reserves an eligible
START for five minutes, preventing concurrent local preparation through the
same journal. `recover apply` consumes one unexpired token after exact textual
confirmation, revalidates again, durably begins the attempt, performs only
`podman start`, and records the command result as `start-command-succeeded` or
`start-command-failed`. It does not call command success “application recovery”
and provides no automatic retry or
rollback. See
[RECOVERY-PLANNING.md](RECOVERY-PLANNING.md).
`recover history` presents a read-only journal view and identifies begun
executions without a result. `recover reconcile` records an explicitly confirmed
operator assertion plus a fresh diagnostic for one such consumed token; it does
not infer outcomes or mutate Podman.
`recover verify` appends a fresh post-command observation such as `healthy`,
`unhealthy` or `running-unverified`. It is read-only with respect to Podman and
does not infer causality.

The `plan` command inspects Podman and compares selected declared fields only. It does not reserve ports or provide full configuration equivalence; see [PLANNING.md](PLANNING.md).

`update plan` is also read-only. It reports `UPDATE_CANDIDATE` only for exact
managed ownership plus image-only drift and a declared healthcheck. Every other
difference is blocked. It does not create an operation journal or authorize an
update; see [ALPHA5-UPDATE-PLAN.md](ALPHA5-UPDATE-PLAN.md).

`update prepare` repeats that read-only inspection for one declared service and
persists an exact `PENDING` intent in the Alpha 4 journal. It records container
identity, previous/target image and a deterministic plan fingerprint. It does
not ask for execution confirmation, pull an image or change Podman.

## Scope and limits

- YAML schemas: legacy `orisema.dev/v1alpha1` and current
  `orisema.dev/v1alpha2`; migration is documented and never implicit.
- Only service names, images, TCP port mappings and the existing documented
  health/restart/recovery fields are represented.
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

Deployment uses `podman run --detach` with managed labels and
`no-new-privileges`. It skips matching owned containers, blocks DRIFT/REVIEW and
never uses `--replace`. `v1alpha2` requires loopback-only binds; legacy
`v1alpha1` ports retain their all-interface behavior.

**Known limitations:** No rollback, automatic updates, volume support, health
verification, preflight reservation of ports, image digest enforcement or
transaction across multiple services. Podman detects conflicts when creating
containers; a partially created owned container is removed only after its
ownership labels are verified. Do not use this command for sensitive workloads.

Alpha 3 separately specifies and exercises controlled update and application
rollback in a disposable [laboratory](CONTROLLED-UPDATE-LAB.md). That workflow
is deliberately not exposed as a supported CLI command and does not restore
application data.

To clean up the test manually after confirming its identity:

```sh
podman stop orisema-hello-orisema-web
podman rm orisema-hello-orisema-web
```

## Conservative repeated deployment (experimental)

After the first deployment, rerunning `go run ./cmd/orisema deploy examples/nginx.yaml` should classify the existing owned container as `NOOP` and exit without a confirmation prompt or modifications, **provided its configured image reference and published ports match**.

If any service is `DRIFT` or `REVIEW`, deployment is blocked for the entire manifest before creating any containers. `CREATE` services still require typing `DEPLOY`. A second check for name collisions happens immediately before each creation.

**Limits:** This is idempotent only for the fields currently compared (ownership labels, image reference, TCP port bindings, declared healthcheck command and timing/retries, and restart policy). `NOOP` does not imply that the container is running or healthy, that the image digest matches, or that security options and other settings are equivalent. A multi-service deployment can partially complete if a later create fails. No rollback, automatic reconciliation or destructive update exists.

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

`status` now attempts `podman inspect` for existing containers and displays running state, exit code, restart count, start/finish timestamps, ownership and health if available. Health without a configured healthcheck (including `["NONE"]`) is reported as `not configured`. A configured check without a recognized observation is `unavailable`. Both `State.Healthcheck` and `State.Health` are supported; conflicting nonempty values are reported as unavailable. Stopped or paused containers never display a stale `healthy` observation. Health remains the last probe result, not proof of HTTP availability.

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

## Declarative restart policy (experimental)

Services may declare `restart.policy: on-failure` with a required finite
`maxRetries`, or explicitly use `policy: no`. This policy responds only to a
non-zero process exit; it does not respond to an application becoming
`unhealthy`. See [RESTART-POLICIES.md](RESTART-POLICIES.md) for the supported
contract, runtime differences and rootless validation steps.

## Operational diagnosis (experimental)

Use `diagnose` when an operator needs a recommendation without changing the
declarative plan:

```sh
go run ./cmd/orisema diagnose examples/nginx-health.yaml
```

The command distinguishes missing, stopped, healthy, unhealthy and unobservable
containers. Three or more cumulative restarts are reported as repeated restarts
and prompt investigation, but this is not proof of a restart loop: Podman's
current cumulative `RestartCount` does not provide a time window. A missing
healthcheck produces a recommendation to consider declaring one. Inspection or
runtime failures remain explicit and never imply equivalence or container
absence.

Recommendations are informational. No automatic restart, replacement,
recovery, rollback or health-based planning is implemented. In particular,
`NOOP` continues to describe selected declared fields only and can coexist with
a stopped or unhealthy diagnosis.

See [OPERATIONAL-DIAGNOSTICS.md](OPERATIONAL-DIAGNOSTICS.md) for the data model,
limitations and an Ubuntu Server rootless validation checklist.

See [RECOVERY-PLANNING.md](RECOVERY-PLANNING.md) for typed actions and the
guardrails required before any recovery execution can be designed.
