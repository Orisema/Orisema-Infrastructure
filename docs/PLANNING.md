# Read-only state-aware plan

`orisema plan examples/nginx.yaml` now queries local Podman (`podman ps -a --format json`) and classifies each requested service:

- `CREATE`: the expected container name does not exist.
- `REVIEW`: the name already exists; no automatic replacement is allowed.

**Important:** A matching name, image tag or running state is not proof of equivalence. This planner does not inspect port bindings, labels, digests, health, volumes or security options. `REVIEW` does not mean `NOOP` or that the container is managed by Orisema.

The existing `deploy` command remains create-only and refuses any existing name. It does not automatically execute this plan. No rollback or multi-service atomicity is implemented.
