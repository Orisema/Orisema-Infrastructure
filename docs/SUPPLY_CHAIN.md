# Third-party components and supply-chain transparency

The Orisema CLI does not require a community-operated service. Its Go dependencies are recorded in go.mod/go.sum; Podman on Linux is an external runtime prerequisite.

## SBOM

The supply-chain workflow generates a CycloneDX JSON SBOM on pushes and pull requests, and uploads it as a CI artifact. It inventories the Go module dependency graph, not the host operating system, Podman packages, container images, or every binary runtime component.

For a local inventory:

```sh
mkdir -p dist
go run github.com/CycloneDX/cyclonedx-gomod/cmd/cyclonedx-gomod@v1.9.0 mod -json -output dist/orisema-sbom.cdx.json .
```

The generator is pinned independently and is not a runtime dependency of the CLI. Review its upstream license and updates before changing the pinned version.

## Vulnerability checks

CI runs govulncheck as an **advisory** check, without blocking releases. Findings require triage for reachability, affected versions and remediation; no result is a security guarantee. The scanning tool currently uses the latest release, so its results can change between runs.

## Release policy

Publishing the SBOM as a GitHub Release asset requires release-workflow integration and validation. Until then, the SBOM is a CI artifact, not a release asset. Future work should attach the generated SBOM and its checksum to each tagged release, verify the artifact and document provenance.
