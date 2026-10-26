# Declarative restart policies (experimental)

Restart policies respond to the main container process exiting. They are not
application health recovery: an `unhealthy` process that remains running does
not trigger `on-failure`.

## Initial manifest contract

```yaml
restart:
  policy: on-failure
  maxRetries: 3
```

The first increment deliberately supports only:

- `no`, with no `maxRetries` field;
- `on-failure`, with `maxRetries` required from 1 through 100.

Omitting `restart` is equivalent to the runtime default `no`. Unbounded
`on-failure`, `always`, `unless-stopped` and Podman's `never` alias are rejected.
This smaller portable contract prevents an accidental infinite failure loop and
does not promise host-reboot behavior.

For new containers, Orisema passes `--restart no` or
`--restart on-failure:<maxRetries>` to Podman. Existing owned containers are
compared using `HostConfig.RestartPolicy.Name` and `MaximumRetryCount`. A
difference is `DRIFT`; Orisema does not update or replace the container.

## Runtime differences and limits

Podman and Docker both support `no`, bounded `on-failure`, `always` and
`unless-stopped`, but reboot and manual-stop behavior varies by runtime and
version. Podman documents integration with `podman-restart.service` and advises
using systemd restart behavior for containers managed as systemd units. Docker
documents daemon-specific behavior and a ten-second successful-start window.

Do not combine this field with Podman's `--health-on-failure=restart`. Orisema
does not set that option. Health recovery, systemd integration and support for
additional policies require separate architecture decisions and tests.

Podman 4.9.3 does not expose `--restart` through `podman update`. Orisema does
not depend on runtime update support: a mismatch remains `DRIFT` and requires
manual review rather than an in-place mutation.

Podman 4.9.3 inspection has returned the default disabled policy as both
`{"Name":"no","MaximumRetryCount":0}` in rootless mode and an empty `Name`
with zero retries in rootful mode. Orisema normalizes only these disabled forms;
active policies and retry counts remain strict comparisons.

References:

- [Podman run restart policy](https://docs.podman.io/en/latest/markdown/podman-run.1.html#restartpolicy)
- [Docker restart policies](https://docs.docker.com/engine/containers/start-containers-automatically/)

## Rootless validation

Use an unprivileged account and a disposable app name:

```sh
podman version
podman info --format '{{.Host.Security.Rootless}}'
go test ./...
go vet ./...
go run ./cmd/orisema validate examples/nginx-restart.yaml
go run ./cmd/orisema plan examples/nginx-restart.yaml
go run ./cmd/orisema deploy examples/nginx-restart.yaml
# Type DEPLOY only when the plan is CREATE.
podman inspect --type container orisema-restart-lab-web \
  --format '{{json .HostConfig.RestartPolicy}}'
go run ./cmd/orisema plan examples/nginx-restart.yaml
```

Expect `{"Name":"on-failure","MaximumRetryCount":3}` and a final `NOOP`.
Before cleanup, verify the exact disposable name and the three Orisema ownership
labels. Do not reuse or remove an existing container.

This workflow was executed successfully with Podman 4.9.3 rootless. A second
isolated container created with `on-failure:5` produced `DRIFT` with reason
`restart policy differs`. This evidence does not establish compatibility with
other Podman or Docker versions.
