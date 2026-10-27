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

## Guardrails for bounded execution

The narrowly scoped `recover apply` path depends on:

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

The CLI displays declared guardrails. Without `--history`, it reports execution
as ineligible because persistent attempt history is not connected. With an
explicit history file, it loads the matching manifest's records and reports the
guardrail decision. Planning itself remains read-only; execution requires a
separate reservation and confirmation.

History uses a versioned JSON Lines journal. The file is limited to 1 MiB,
locked during reads and writes, synchronized after appends, restricted to the
current user and rejected if it is a symbolic link, non-regular file, corrupt,
or has broader read permissions. Records are isolated by manifest, service and
action. A missing or invalid file blocks the plan instead of being interpreted
as an empty history. Initialize it explicitly before first use:

```sh
go run ./cmd/orisema recover init examples/nginx-restart.yaml \
  --history /secure/path/orisema-recovery.jsonl \
  --operator alice
```

Initialization writes an auditable genesis record with manifest, operator and
timestamp. It uses exclusive creation and never truncates an existing path.

The explicit `recover record` command appends an operator-supplied observation:

```sh
go run ./cmd/orisema recover record examples/nginx-restart.yaml web START failed \
  --history /secure/path/orisema-recovery.jsonl
```

The result may be `succeeded` or `failed`. This command does not contact Podman,
perform a recovery attempt, or prove that one occurred. Protect the journal and
record a result only after separately observing the operation. Orisema does not
create parent directories for the supplied path.

## Atomic preparation

`recover prepare` performs a fresh read-only diagnosis and, only when the
recommendation is `START`, atomically evaluates the guardrails and appends a
short-lived reservation to the same journal:

```sh
go run ./cmd/orisema recover prepare examples/nginx-restart.yaml web \
  --history /secure/path/orisema-recovery.jsonl \
  --operator alice \
  --reason "incident reviewed"
```

The reservation contains a random 128-bit token, a five-minute expiry, the
operator and reason, plus the observed container name, configuration, execution
and failure. The journal must already exist (normally through `recover init`)
and have safe permissions. The
decision and append occur under one exclusive file lock, so concurrent local
preparations for the same manifest, service and action cannot both succeed.

Preparation fails closed when Podman inspection is unavailable, the current
diagnosis is not `START`, ownership or supported configuration does not match,
guardrails reject another attempt, history is unsafe, or an unexpired
reservation already exists. Expired reservations no longer block a new prepare.

`recover prepare` never starts a container. File locking coordinates processes
using the same local journal; it is not a distributed lock and the journal is
not tamper-evident.
The operator field is a supplied audit label, not an authenticated identity.
Runtime state may change after preparation, so token consumption revalidates
ownership, supported configuration, stopped state, policy and expiry.

## Explicit START execution

Only a prepared `START` can be consumed:

```sh
go run ./cmd/orisema recover apply examples/nginx-restart.yaml web \
  --history /secure/path/orisema-recovery.jsonl \
  --token <token-from-prepare>
# Type: APPLY START web
```

After confirmation, Orisema performs a fresh diagnosis. Under the journal lock
it verifies the reservation, policy snapshot, expiry, observations and current
guardrails, then appends and synchronizes a begun attempt before calling
`podman start`. The token becomes single-use. A success or failure result is
appended afterward; a crash after the begun record still consumes one attempt.

`recover apply` does not restart a running container, deploy a missing one,
replace drift, handle unhealthy applications, retry automatically or roll back.
The runtime inspection inside the lifecycle operation rechecks ownership again.
Podman and the journal do not share a transaction, so a process crash can leave
a begun attempt with an unknown runtime result; this is intentionally safer than
silently retrying.

## Interrupted attempts

The journal can be inspected without querying or changing Podman:

```sh
go run ./cmd/orisema recover history examples/nginx-restart.yaml \
  --history /secure/path/orisema-recovery.jsonl
```

Reservations and terminal results show only a token prefix. A begun execution
without a result is listed as unresolved and includes its full, already-consumed
token so it can be reconciled.

Orisema never infers that an interrupted attempt succeeded merely because a
container is currently running. After investigating runtime and application
evidence, an operator may explicitly reconcile the consumed token:

```sh
go run ./cmd/orisema recover reconcile examples/nginx-restart.yaml web \
  --history /secure/path/orisema-recovery.jsonl \
  --token <unresolved-token> \
  --outcome succeeded \
  --operator alice \
  --reason "verified service logs and external probe"
# Type: RECONCILE SUCCEEDED web
```

The outcome may be `succeeded` or `failed`. Reconciliation performs a fresh
read-only diagnosis, records the operator-supplied assertion and observed state,
and never changes Podman. It requires exact confirmation, is single-writer under
the journal lock and cannot replace an existing result. The operator field is
still a label rather than an authenticated identity.

## Usage

```sh
go run ./cmd/orisema recover plan examples/nginx-restart.yaml \
  --history /secure/path/orisema-recovery.jsonl
```

Review the typed action and evidence. Execution is limited to the separately
prepared, unexpired and explicitly confirmed `START` flow described above.
