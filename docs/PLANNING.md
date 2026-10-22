# Read-only Podman planning

`orisema plan examples/nginx.yaml` reads `podman ps -a --format json` and inspects existing matching containers using `podman inspect --type container`.

- `CREATE`: expected container name does not exist.
- `REVIEW`: existing container lacks matching Orisema ownership labels, or inspection fails. No adoption.
- `DRIFT`: owned container has a different configured image reference or published port bindings.
- `NOOP`: ownership labels, configured image reference and declared TCP port bindings match.

**NOOP is intentionally limited:** it does not verify image digest, image freshness, running state, health, volumes, restart policy, security settings, firewall, or other runtime settings. It is not a declaration of full equivalence or production readiness. `plan` never changes containers.

`deploy` remains create-only and refuses existing names; it does not execute this plan or automatically reconcile drift. No rollback or atomic multi-service deployment is available.
