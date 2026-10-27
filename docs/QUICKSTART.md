# Quick start on a disposable Linux host

Orisema Infrastructure is **experimental and not recommended for production**.
This walkthrough creates a real Nginx container, publishes TCP port 8080 on all
host interfaces and downloads an image through Podman. Use a disposable Linux
host, confirm that port 8080 is free and review firewall exposure first.

## Prerequisites

- Linux AMD64 or ARM64;
- a verified Orisema binary installed as described in
  [INSTALLATION.md](INSTALLATION.md);
- Podman available to the current user;
- the repository checkout, which provides the example manifest;
- network access for Podman to pull `docker.io/library/nginx:1.27`.

Rootful and rootless Podman maintain different container stores. Run every
command below as the same user; do not alternate between `podman` and
`sudo podman`.

## 1. Inspect the tools and manifest

```sh
orisema version
podman version
orisema validate examples/nginx.yaml
```

Read `examples/nginx.yaml`. The example declares the container name
`orisema-hello-orisema-web` and maps host port 8080 to port 80.

## 2. Preview without changing Podman

```sh
orisema plan examples/nginx.yaml
```

On a clean host, expect `CREATE`. `plan` is read-only. Stop if the result is
`DRIFT` or `REVIEW`; Orisema will not adopt, replace or reconcile that container.

## 3. Deploy with explicit approval

```sh
orisema deploy examples/nginx.yaml
# Review the plan, then type exactly: DEPLOY
```

Deployment creates only a missing service classified as `CREATE`. It has no
transaction or rollback: if a later service fails, an earlier service may remain.

## 4. Observe

```sh
orisema status examples/nginx.yaml
orisema plan examples/nginx.yaml
curl -I http://127.0.0.1:8080/
orisema logs examples/nginx.yaml web
```

The second plan should report `NOOP`, but this means only that the supported
declared fields match. It is not evidence that the process is running, healthy,
secure or using the expected image digest. `status`, `diagnose` and an external
probe provide different evidence.

## 5. Clean up explicitly

Orisema deliberately has no remove command. Confirm the exact managed labels
before deleting the laboratory container:

```sh
podman inspect --format '{{json .Config.Labels}}' orisema-hello-orisema-web
podman stop orisema-hello-orisema-web
podman rm orisema-hello-orisema-web
```

Expected ownership labels include `dev.orisema.managed=true`,
`dev.orisema.app=hello-orisema` and `dev.orisema.service=web`. Do not remove a
container when those values do not match.

## What this does not validate

This quick start does not establish production readiness, upgrade/rollback,
persistent storage, backup/restore, secrets management, remote operation,
multi-node behavior or Docker compatibility. See [COMPATIBILITY.md](COMPATIBILITY.md)
and [CLI.md](CLI.md) before testing additional commands.
