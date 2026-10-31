# Installation and release verification

Orisema Infrastructure is **experimental software and is not recommended for
production environments**.

Release archives let Linux users run Orisema without installing Go. The latest
experimental release, [`v0.1.0-alpha.5`](https://github.com/Orisema/Orisema-Infrastructure/releases/tag/v0.1.0-alpha.5),
was published on 2026-10-31 and remains unsuitable for production.

## Supported distribution targets

| Platform | Binary produced | Execution validation |
| --- | --- | --- |
| Linux AMD64 | Yes | Unit tests and the release binary run in CI; Podman integration runs on Ubuntu 24.04 AMD64 |
| Linux ARM64 | Yes | Cross-compiled and inspected as a Linux ARM64 Go binary; native execution still requires an ARM64 test host |
| macOS | No | Not supported in this release |
| Windows | No | Not supported in this release |

Podman on Linux is the only runtime currently supported. Docker is not
officially supported. A distributed binary does not imply that every Linux
distribution, kernel, Podman version or ARM64 machine has been tested.

## Download and verify

Select the release version and your architecture (`amd64` or `arm64`):

```sh
version=v0.1.0-alpha.5
arch=amd64
curl -fLO "https://github.com/Orisema/Orisema-Infrastructure/releases/download/${version}/orisema_linux_${arch}.tar.gz"
curl -fLO "https://github.com/Orisema/Orisema-Infrastructure/releases/download/${version}/checksums.txt"
grep " orisema_linux_${arch}.tar.gz$" checksums.txt | sha256sum --check
```

Do not install the archive if checksum verification fails. SHA-256 detects a
damaged or mismatched download, but this alpha pipeline does not yet sign
artifacts or provide provenance attestations.

Extract and inspect the binary before installing it:

```sh
tar -xzf "orisema_linux_${arch}.tar.gz"
./orisema version
sudo install -m 0755 orisema /usr/local/bin/orisema
orisema version
```

The version report includes the release tag, source commit, reproducible build
timestamp, Go toolchain and target OS/architecture. An ordinary local build
reports `Version: development`, `Commit: unknown` and `Built: unknown`.

## Runtime requirement

Install and configure Podman separately, then run Orisema as the same user that
owns the intended containers. Rootful and rootless Podman have separate views
of containers. Read [Podman integration](PODMAN.md) and the CLI limitations in
[CLI.md](CLI.md) before testing deployment commands.

## Uninstall

Removing the executable does not remove containers or other Podman resources:

```sh
sudo rm /usr/local/bin/orisema
```

Inspect and remove runtime resources separately and explicitly. Orisema does
not perform automatic cleanup, replacement or rollback.
