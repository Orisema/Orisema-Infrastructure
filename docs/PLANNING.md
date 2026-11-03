# Read-only Podman planning

`orisema plan examples/nginx.yaml` reads `podman ps -a --format json` and inspects existing matching containers using `podman inspect --type container`.

- `CREATE`: expected container name does not exist.
- `REVIEW`: existing container lacks matching Orisema ownership labels, or inspection fails. No adoption.
- `DRIFT`: an owned container differs in any supported declared field, including image, ports, healthcheck, restart policy, resolved secret metadata, public configuration or external-network binding.
- `NOOP`: ownership and every supported declared field match conservatively.

For `v1alpha2`, port comparison includes the loopback `hostIP`; a different
bind address is `DRIFT`. `v1alpha1` retains its legacy all-interface comparison.

**NOOP is intentionally limited:** it does not verify image digest, image
freshness, running state, live health, volumes, undeclared environment entries,
undeclared networks, security settings, firewall or other undeclared runtime
settings. It is not a declaration
of full equivalence or production readiness. `plan` never changes containers.

`deploy` uses the inspected plan: it skips `NOOP`, blocks `DRIFT`/`REVIEW` and creates only `CREATE` services after confirmation. It refuses existing names and never automatically reconciles drift. No rollback or atomic multi-service deployment is available.

Healthcheck comparison covers shell command, interval, timeout and retries. It distinguishes `CMD-SHELL` from `CMD`; it does not compare startup checks or start period. Restart comparison covers the policy name and bounded retry count; it does not imply health recovery or host-reboot guarantees.

Use `status` and `diagnose` for separate operational observations. A stopped or
unhealthy matching container can remain `NOOP`; planning never starts it based
on health. The exact Alpha compatibility boundary is documented in
[COMPATIBILITY.md](COMPATIBILITY.md).
