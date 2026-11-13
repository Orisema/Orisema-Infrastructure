# CLI prototype (Alpha 10 development)

This is an **experimental laboratory CLI**, not production-ready. `validate` works offline; `plan`, `status` and lifecycle commands query local Podman.

The Alpha 7 `v1alpha4` schema accepts bounded `publicEnvironment` and external
network references. `plan` resolves external network metadata and detects drift
without printing public values. CREATE deploys environment entries and attaches
pre-existing `bridge` networks by revalidated ID. Configured update, verify and
manual application rollback revalidate these bindings; network lifecycle
remains external and fail-closed.

## Requirements

Precompiled release binaries do not require Go. Building from source requires
Go 1.23+ and access to the module dependency `gopkg.in/yaml.v3`. Commands that
inspect or change containers require Podman on Linux; `version` and `validate`
do not query Podman.

## Run

```sh
go run ./cmd/orisema version
go run ./cmd/orisema doctor examples/nginx.yaml --journal /secure/path/orisema-operations
go run ./cmd/orisema operation init --journal /secure/path/orisema-operations
go run ./cmd/orisema operation diagnose --journal /secure/path/orisema-operations
go run ./cmd/orisema update plan examples/nginx-health.yaml
go run ./cmd/orisema update prepare examples/nginx-health.yaml web --journal /secure/path/orisema-operations
go run ./cmd/orisema update authorize examples/nginx-health.yaml web --journal /secure/path/orisema-operations --operation <id>
go run ./cmd/orisema update apply examples/nginx-health.yaml web --journal /secure/path/orisema-operations --operation <id>
go run ./cmd/orisema update verify examples/nginx-health.yaml web --journal /secure/path/orisema-operations --operation <id>
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

See [CLI-INVENTORY.md](CLI-INVENTORY.md) for every public syntax, effect, exit
class and recovery boundary, [COMPATIBILITY.md](COMPATIBILITY.md) for a compact
command/mutation matrix and [QUICKSTART.md](QUICKSTART.md) for the reviewed
end-to-end laboratory.

The `validate` command checks the versioned manifest structure, service
identifiers, images, port ranges and version-specific bind rules. See the
[`v1alpha2` contract](MANIFEST-V1ALPHA2.md).

## Secret lifecycle (Alpha 6)

`orisema secret create NAME --provider podman --stdin` creates a managed
Podman secret from a bounded pipe or redirected file. `secret list` and
`secret inspect NAME` expose metadata only. `secret remove NAME` requires exact
ownership, no container consumers and a confirmation containing both name and
immutable ID. Secret values cannot be shown or exported by the CLI. See the
[`v1alpha3` contract](MANIFEST-V1ALPHA3.md).

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
difference is blocked. Drift output includes only closed difference categories
and a deterministic fingerprint of value-free metadata; it never includes
secret values or private file locators. The new categories are evidence for
review and do not make configuration drift eligible. The command does not
create an operation journal or authorize an update; see
[ALPHA5-UPDATE-PLAN.md](ALPHA5-UPDATE-PLAN.md).

`update prepare` repeats that read-only inspection for one declared service and
persists an exact `PENDING` intent in the Alpha 4 journal. It records container
identity, previous/target image and a deterministic plan fingerprint. It does
not ask for execution confirmation, pull an image or change Podman.

For Beta 1, `update prepare` also accepts drift exclusively classified as
`PUBLIC_CONFIG` when the image is unchanged, a healthcheck is declared and the
service has no secret or network bindings. It persists names and fingerprints,
never public values. This intent is review-only: `update authorize` rejects it
unless the operator supplies the exact confirmation containing application,
service and operation ID and fresh evidence matches. Successful authorization
advances only to `PREPARING`; `update apply` rejects it before confirmation.
Mixed or unknown drift remains blocked.

`update authorize` requires exact textual confirmation and repeats the complete
read-only plan within five minutes of preparation. Matching fresh evidence
atomically advances the journal to `PREPARING`; failure leaves it untouched.
For a public-configuration intent, the phrase is `AUTHORIZE PUBLIC CONFIG
application/service operation-id`; image, configuration fingerprint, container
identity and ownership must still match. The command performs no runtime
mutation, and the resulting public-configuration intent is not executable in
this increment.

`update apply` requires a fresh authorized `PREPARING` intent and the exact
confirmation `APPLY UPDATE application/service`. It revalidates the complete
plan, persists `APPLYING` and each pending mutation, stops and renames the old
container to a unique rollback name, then creates the declared candidate. The
old container is preserved and failures receive no automatic cleanup, retry or
rollback. A created candidate remains unverified; this command does not claim
health or completion.

For an authorized Beta 1 public-configuration intent, the confirmation is
`APPLY PUBLIC CONFIG application/service operation-id`. The command revalidates
the value-free plan, persists every next effect, stops and preserves the exact
previous container, then creates a candidate with the new public configuration
and unchanged image. It ends in `APPLYING`; the separate verify command records
the durable decision. Rollback and cleanup remain unavailable for this
contract, so the preserved unit must not be removed manually.

`update verify` is read-only with respect to Podman and records one explicit
post-update observation per invocation. It freshly revalidates the candidate
and preserved previous container, then persists `VERIFYING`. Healthy evidence
commits `COMPLETED`; terminal non-success commits `RECOVERY_REQUIRED`; an
inconclusive observation reports `WAIT`. It performs no polling, retry, cleanup
or rollback. For configured services it resolves secret and external-network
identities again and compares the complete public configuration and binding
fingerprint; altered identities, configuration or ownership block the decision.

`update rollback` accepts only a durable `RECOVERY_REQUIRED` update and requires
the exact confirmation `ROLLBACK UPDATE application/service operation-id`. It
revalidates the two exact containers, preserves the failed candidate under a
diagnostic name and restores and starts the stopped previous container. Each
mutation boundary is persisted first. Configured bindings are freshly
revalidated, but the preserved previous container is restored as-is rather
than reconstructed. It does not restore data, mutate external resources or
remove any container.

`update rollback-verify` reinspects both exact containers and records the
restored container's configured health. Only `healthy` commits `ROLLED_BACK`;
other results remain `ROLLING_BACK` for explicit operator intervention.

## Scope and limits

- YAML schemas: `orisema.dev/v1alpha1` through opt-in `v1alpha4`; migration is
  documented and never implicit.
- Supported fields are limited to those documented in the versioned manifest
  contracts, including file-delivered secrets, explicitly public environment
  and pre-existing external Podman `bridge` networks.
- No support yet for persistent storage, managed network lifecycle, DNS aliases,
  resource limits or full security-context management.
- Do not use this prototype to deploy production applications.

## Configuration backup and restore

`backup create <manifest> --output <new-directory> --journal <operations>`
validates and snapshots the exact manifest into a private, versioned artifact.
`--operations <existing-journal>` optionally adds canonical, read-only history
for the same application. It never reads volumes, databases, secrets,
credentials, images or container layers.

`backup verify <artifact>` validates strict layout and permissions, canonical
metadata, bounded sizes and SHA-256 for every payload. It rejects symlinks,
special files, unknown or duplicate entries, unsupported versions and corrupt
content. Hashes establish integrity, not authenticity.

`backup restore <artifact> --destination <absent-directory> --journal
<operations>` verifies before prompting for the exact backup/application-bound
confirmation. It publishes fixed filenames atomically without replacement and
verifies them again. Imported journals are inert history and cannot resume an
operation. Existing destinations are always rejected.

## SSH connectivity

`host check <registry.yaml> <alias>` validates a private local host registry and
performs a fixed, non-mutating SSH `true` probe. It requires external key files,
strict known-hosts verification, key-only batch authentication and bounded
timeouts. Results are typed and SSH stderr is not exposed. See [SSH.md](SSH.md).

`remote status <registry.yaml> <alias> <manifest.yaml> --audit <absolute-path>`
performs bounded `podman inspect` calls for manifest-derived container names.
`remote logs <registry.yaml> <alias> <manifest.yaml> <service> --audit
<absolute-path>` revalidates exact ownership and returns the fixed 100-line
tail. Both require durable private local audit records and fail closed if
audit, transport, decoding, size, identity or ownership checks fail. There is
no arbitrary remote command facility and no remote mutation.

## Experimental deployment

**Use only on a disposable test host.** Run `validate` and inspect the manifest first.

```sh
go run ./cmd/orisema operation init --journal /secure/path/orisema-operations
go run ./cmd/orisema deploy --journal /secure/path/orisema-operations examples/nginx.yaml
# Type DEPLOY when prompted.
go run ./cmd/orisema status examples/nginx.yaml
curl -I http://127.0.0.1:8080/
```

Deployment uses `podman run --detach` with managed labels and
`no-new-privileges`. It skips matching owned containers, blocks DRIFT/REVIEW and
never uses `--replace`. `v1alpha2` requires loopback-only binds; legacy
`v1alpha1` ports retain their all-interface behavior.

**Known deploy limitations:** No automatic rollback, volume support,
application-readiness verification, preflight reservation of ports, image
digest enforcement or transaction across multiple services. Podman detects
conflicts when creating containers; previously completed services are never
rolled back automatically after a later failure. Controlled update and
container rollback are separate public commands and do not restore application
data. Do not use this command for sensitive workloads.

To clean up the test manually after confirming its identity:

```sh
podman stop orisema-hello-orisema-web
podman rm orisema-hello-orisema-web
```

## Conservative repeated deployment (experimental)

After the first deployment, rerunning `go run ./cmd/orisema deploy --journal /secure/path/orisema-operations examples/nginx.yaml` should classify the existing owned container as `NOOP` and exit without a confirmation prompt or modifications, **provided its configured image reference and published ports match**.

If any service is `DRIFT` or `REVIEW`, deployment is blocked for the entire manifest before creating any containers. `CREATE` services still require typing `DEPLOY`. A second check for name collisions happens immediately before each creation.

**Limits:** This is idempotent only for the fields currently compared (ownership labels, image reference, TCP port bindings, declared healthcheck command and timing/retries, and restart policy). `NOOP` does not imply that the container is running or healthy, that the image digest matches, or that security options and other settings are equivalent. A multi-service deployment can partially complete if a later create fails. No rollback, automatic reconciliation or destructive update exists.

## Experimental service lifecycle

Commands require the service to be declared in the provided manifest and the existing container to have matching Orisema ownership labels (`dev.orisema.managed`, `dev.orisema.app`, `dev.orisema.service`).

```sh
go run ./cmd/orisema logs examples/nginx.yaml web
go run ./cmd/orisema stop --journal /secure/path/orisema-operations examples/nginx.yaml web
# Type STOP to confirm
go run ./cmd/orisema start --journal /secure/path/orisema-operations examples/nginx.yaml web
# Type START to confirm
```

`logs` returns only the last 100 lines and does not follow. `start` and `stop`
require an initialized absolute journal path and interactive confirmation. They
admit a durable operation atomically, revalidate the exact container identity,
persist `APPLYING` before issuing Podman commands and reach `COMPLETED` only
after inspecting the requested final state. An error or ambiguous result leaves
blocking evidence for diagnosis. They never remove containers and do not check
desired image/ports, application health or service dependencies. **Do not run
stop on critical workloads.** The earlier `deploy` command still treats a
stopped matching container as `NOOP` and will not automatically restart it.

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
go run ./cmd/orisema deploy --journal /secure/path/orisema-operations examples/nginx-health.yaml
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

# Doctor

Initialize an operation store deliberately before using journaled commands:

```bash
orisema operation init --journal /secure/path/orisema-operations
```

The path must be absolute. Initialization creates a private store when it is
absent and is idempotent only for an already safe store. It does not repair
permissions, replace a missing lock, load a manifest or contact Podman.

`orisema doctor <manifest.yaml> --journal <directory>` performs pre-operational
checks without changing Podman or the journal. Its human-readable output uses
contract `orisema-doctor-v2`: every line has a stable check ID, status and
machine-searchable `code`. It
validates the manifest, reads the current Podman rootless/rootful boundary,
validates every operation snapshot, reports incomplete operations scoped to
the application, resolves metadata-only secret and network identities and
compares declared containers through the conservative planner and measures free
space for both the journal and Podman graph root.

Checks are reported as `PASS`, `WARN` or `BLOCK`. A missing container is a
`WARN`, while drift, ownership ambiguity and unavailable bindings are `BLOCK`.
The final result is `PASS`, `WARN` or `BLOCK`; both `PASS` and `WARN` exit `0`,
while neither authorizes a mutation. The stable blocking codes are:

| Area | Codes |
| --- | --- |
| Manifest | `MANIFEST_INVALID` |
| Podman/runtime | `RUNTIME_UNAVAILABLE`, `RUNTIME_INVENTORY_UNAVAILABLE`, `RUNTIME_DRIFT_OR_OWNERSHIP` |
| Journal | `JOURNAL_UNAVAILABLE`, `JOURNAL_UNSAFE`, `JOURNAL_CORRUPT`, `OPERATION_INCOMPLETE` |
| Storage | `JOURNAL_STORAGE_UNAVAILABLE`, `JOURNAL_SPACE_LOW`, `RUNTIME_STORAGE_UNAVAILABLE`, `RUNTIME_SPACE_LOW` |
| Bindings | `SECRET_BINDING_UNAVAILABLE`, `NETWORK_BINDING_UNAVAILABLE` |

Exit code `0` means no blocking check was found, `1` means diagnosis completed
with a block or could not be completed, and `2` means invalid CLI usage. A
`PASS` is not authorization to mutate. A missing, unsafe or corrupt journal
fails closed and must be preserved for investigation.

Secret values, locators and raw manifest parsing errors are not printed. The
next step for a block is inspection or a separately reviewed operator action;
it never invokes cleanup, permission repair, deploy or recovery. Independent
checks continue where safe, so an unavailable runtime does not hide an unsafe
journal. The command does not forecast
volume growth or enforce a mutation guard on legacy commands without a journal.
Both storage checks require at least 256 MiB available; this is an operational
safety margin, not a capacity forecast for images or application data. See
[RFC-0005](RFC-0005-operational-recovery-and-doctor.md).

Update continuations that already require `--journal` validate the complete
operation store before confirmation. Only the exact incomplete operation for
the manifest application may continue; another pending ID, an unsafe snapshot
or corruption blocks before runtime mutation. `update prepare` retains its
atomic per-application exclusion when it creates the intent.

`orisema recover operation <manifest.yaml> --journal <directory> --operation
<id>` is the Alpha 9 recovery entry point. It fails closed for every unsupported
state and delegates only an intact `RECOVERY_REQUIRED` update to the same
revalidated, explicitly confirmed rollback implementation used by `update
rollback`. It does not infer success, retry a runtime step or delete evidence.

`deploy`, `start` and `stop` require an explicit initialized operation journal
and are covered by the Alpha 10 mutation guard. Their old syntax is rejected
before runtime inspection.
