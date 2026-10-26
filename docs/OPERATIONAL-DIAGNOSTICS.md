# Operational diagnostics (experimental)

`orisema diagnose` is a read-only view that keeps five concerns separate:

- **configuration:** comparison of the supported manifest fields;
- **execution:** the observed container lifecycle state;
- **health:** the last compatible application health observation;
- **failure:** the condition that currently deserves operator attention;
- **recommended action:** informational guidance, never an automatic action.

The command reuses the conservative planner comparison. Health and execution do
not change `CREATE`, `NOOP`, `DRIFT` or `REVIEW`. Recommendations do not restart,
replace or remove containers.

## Ubuntu Server rootless validation

Run these checks as the unprivileged account that owns the test containers. Do
not use `sudo`; doing so would validate a different, rootful Podman store.

```sh
go version
podman version
podman info --format '{{.Host.Security.Rootless}}'
go test ./...
go vet ./...
go build -o ./orisema ./cmd/orisema
./orisema validate examples/nginx-health.yaml
./orisema plan examples/nginx-health.yaml
./orisema diagnose examples/nginx-health.yaml
```

Expect `podman info` to print `true`. Before deploying, change the example's app
name and host port to values reserved for a disposable test. Continue only if
`plan` reports `CREATE`; never remove or replace a name collision automatically.

```sh
printf 'DEPLOY\n' | ./orisema deploy /path/to/isolated-manifest.yaml
podman healthcheck run orisema-<test-app>-web
./orisema diagnose /path/to/isolated-manifest.yaml
printf 'STOP\n' | ./orisema stop /path/to/isolated-manifest.yaml web
./orisema diagnose /path/to/isolated-manifest.yaml
podman inspect --type container orisema-<test-app>-web
```

After confirming the three Orisema ownership labels and the exact disposable
name, remove the test container manually. Record the OS, Go and Podman versions,
commands, exit codes and diagnostic output.

## Current limitations

- `RestartCount >= 3` is a conservative signal of repeated restarts, not proof
  of a restart loop; no time window or historical rate is available yet.
- Runtime and inspect errors are reported but not retried automatically.
- Recommendations do not evaluate dependencies or root cause.
- No restart policy, recovery history, automatic recovery or rollback exists.
- Rootless compatibility must be validated for each supported Podman version
  and host configuration.
