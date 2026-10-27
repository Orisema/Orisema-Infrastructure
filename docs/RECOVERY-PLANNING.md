# Assisted recovery planning (experimental)

`orisema recover plan <manifest.yaml>` converts the read-only operational
diagnosis into typed guidance. It never starts, restarts, updates, replaces or
removes a container.

## Typed actions

- `NONE`: no immediate action identified; continue observation.
- `INVESTIGATE`: collect logs and runtime evidence before any mutation.
- `START`: the matching container is stopped; a future execution path would
  require explicit confirmation.
- `REDEPLOY_REVIEW`: the container is missing or declarative drift exists;
  review the normal plan and protect persistent data.
- `RETRY_RUNTIME`: runtime state is unknown; restore access and repeat the
  read-only inspection.

Each step includes its reason, preconditions, risk and whether a future
mutation would require confirmation. The action is a recommendation, not an
authorization.

## Decision boundaries

- A running and healthy service produces `NONE`.
- A running but `unhealthy` service produces `INVESTIGATE`, never `START` or an
  automatic restart.
- A stopped container produces `START` only after the existing ownership and
  supported declarative fields have matched.
- Drift produces `REDEPLOY_REVIEW`; Orisema does not replace the container.
- Repeated restarts produce `INVESTIGATE` because the cumulative counter does
  not establish a failure rate or a safe retry window.
- Runtime and inspection failures produce `RETRY_RUNTIME`; unknown state is not
  treated as absence.

## Guardrails before execution can exist

There is intentionally no `recover apply` command. A later proposal must first
define and persist:

1. an attempt limit scoped to a service and incident;
2. a minimum interval between attempts;
3. a cooldown after repeated failures;
4. an auditable history with observed result;
5. ownership and drift revalidation immediately before mutation;
6. explicit operator confirmation and a clear cancellation path.

Without durable history, Orisema cannot safely distinguish a new incident from
a restart loop. Restart-policy counters and healthcheck retries are not a
substitute for recovery history.

## Declarative guardrails

A service may declare the limits that a future executor would have to enforce:

```yaml
recovery:
  maxAttempts: 3
  minInterval: 5m
  cooldown: 30m
```

`maxAttempts` must be 1–10, `minInterval` must be 1 second through 24 hours,
and `cooldown` must be at least `minInterval` and at most 168 hours. This
declaration is a constraint, not permission to act.

The recovery package now models attempt records and evaluates a rolling
cooldown window plus the minimum interval. Future timestamps block evaluation
conservatively. Only `START` is eligible for evaluation; other typed actions
remain non-executable.

The CLI displays declared guardrails but reports execution as ineligible because
persistent attempt history is not connected yet. It does not create or update a
history file.

## Usage

```sh
go run ./cmd/orisema recover plan examples/nginx-health.yaml
```

Review the typed action and evidence. Use the existing `start` command only as
a separate, explicitly confirmed operator decision. No command in this planning
flow executes a recommendation.
