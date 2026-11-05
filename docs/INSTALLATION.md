# Installation and release verification

Orisema Infrastructure is **experimental software and is not recommended for
production environments**.

Release archives let Linux users run Orisema without installing Go. The latest
experimental release, [`v0.1.0-alpha.9`](https://github.com/Orisema/Orisema-Infrastructure/releases/tag/v0.1.0-alpha.9),
was published on 2026-11-05 and remains unsuitable for production.

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
version=v0.1.0-alpha.9
arch=amd64
curl -fLO "https://github.com/Orisema/Orisema-Infrastructure/releases/download/${version}/orisema_linux_${arch}.tar.gz"
curl -fLO "https://github.com/Orisema/Orisema-Infrastructure/releases/download/${version}/checksums.txt"
grep " orisema_linux_${arch}.tar.gz$" checksums.txt | sha256sum --check
```

Do not install the archive if checksum verification fails. Alpha 8 and older
have SHA-256 only. Beginning with Alpha 9, the release workflow is designed to
publish `checksums.txt.sigstore.json`, authenticate the checksum metadata with
Sigstore and create GitHub artifact attestations. The Alpha 9 instructions will
require those checks rather than silently falling back to an unsigned checksum.

The Alpha 9 manual verification contract is:

```sh
curl -fLO "https://github.com/Orisema/Orisema-Infrastructure/releases/download/${version}/checksums.txt.sigstore.json"
cosign verify-blob checksums.txt \
  --bundle checksums.txt.sigstore.json \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --certificate-identity-regexp '^https://github.com/Orisema/Orisema-Infrastructure/.github/workflows/release.yml@refs/(heads/main|tags/v[0-9].*)$'
grep " orisema_linux_${arch}.tar.gz$" checksums.txt | sha256sum --check
gh attestation verify "orisema_linux_${arch}.tar.gz" \
  --repo Orisema/Orisema-Infrastructure
```

This verification is online and requires current Cosign and GitHub CLI
versions. It is not a guarantee attached retroactively to Alpha 8.

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

## Alpha 9 installer

The versioned installer automates the same fail-closed checks. Download it from
the release tag so the reviewed script and requested binary version remain
aligned:

```sh
version=v0.1.0-alpha.9
curl -fSLO "https://raw.githubusercontent.com/Orisema/Orisema-Infrastructure/${version}/install.sh"
less install.sh
sh install.sh --version "$version" --user
```

The initial installer requires `curl`, Cosign, GitHub CLI, `sha256sum` and
standard Linux archive utilities. It verifies the signed checksum metadata,
the selected archive checksum and its GitHub attestation before extraction.
It does not edit shell configuration, start containers or touch Orisema state.

Use `--bin-dir /absolute/user-owned/path` for another unprivileged destination.
Reinstalling the same version requires `--reinstall`. If post-install version
or architecture validation fails during an upgrade, the previous executable is
restored. For a system installation, use `--system`; verification remains
unprivileged and `sudo` is restricted to the final mutation in `/usr/local/bin`:

```sh
sh install.sh --version "$version" --system
```

## Runtime requirement

Install and configure Podman separately, then run Orisema as the same user that
owns the intended containers. Rootful and rootless Podman have separate views
of containers. Read [Podman integration](PODMAN.md) and the CLI limitations in
[CLI.md](CLI.md) before testing deployment commands.

## Uninstall

Download and inspect `uninstall.sh` from the same release tag as the installer.
Then select the installation destination explicitly:

```sh
curl -fSLO "https://raw.githubusercontent.com/Orisema/Orisema-Infrastructure/${version}/uninstall.sh"
less uninstall.sh
sh uninstall.sh --user
sh uninstall.sh --system
```

The uninstaller refuses links, special files and executables that do not report
the Orisema identity and version. Removing the executable does not remove
containers, journals, manifests or application data. Inspect and remove runtime
resources separately and explicitly.
