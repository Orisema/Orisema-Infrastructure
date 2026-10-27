# Read-only Podman planning

`orisema plan examples/nginx.yaml` reads `podman ps -a --format json` and inspects existing matching containers using `podman inspect --type container`.

- `CREATE`: expected container name does not exist.
- `REVIEW`: existing container lacks matching Orisema ownership labels, or inspection fails. No adoption.
- `DRIFT`: owned container has a different configured image reference, published port bindings, declared healthcheck or restart policy.
- `NOOP`: ownership labels, configured image reference, declared TCP port bindings, healthcheck configuration and restart policy match.

**NOOP is intentionally limited:** it does not verify image digest, image
freshness, running state, live health, volumes, environment variables, networks,
security settings, firewall or other undeclared runtime settings. It compares
only the supported restart-policy fields described below. It is not a declaration
of full equivalence or production readiness. `plan` never changes containers.

`deploy` uses the inspected plan: it skips `NOOP`, blocks `DRIFT`/`REVIEW` and creates only `CREATE` services after confirmation. It refuses existing names and never automatically reconciles drift. No rollback or atomic multi-service deployment is available.

Healthcheck comparison covers shell command, interval, timeout and retries. It distinguishes `CMD-SHELL` from `CMD`; it does not compare startup checks or start period. Restart comparison covers the policy name and bounded retry count; it does not imply health recovery or host-reboot guarantees.

Use `status` and `diagnose` for separate operational observations. A stopped or
unhealthy matching container can remain `NOOP`; planning never starts it based
on health. The exact Alpha compatibility boundary is documented in
[COMPATIBILITY.md](COMPATIBILITY.md).
