# Alpha 5 read-only update planning

The first Alpha 5 slice exposes only conservative guidance:

```sh
orisema update plan manifest.yaml
```

The command reads the manifest and local Podman inspection but never creates a
journal, asks for confirmation, pulls an image or changes a container.

A service is `UPDATE_CANDIDATE` only when its Orisema ownership is exact, every
supported declared field except the configured image already matches, the
current image is known and different, and the manifest declares a healthcheck.
Matching services are `NOOP`. Missing containers, inspection failures,
ownership ambiguity, absent healthchecks and compound drift are `BLOCKED`.

Candidate status is not authorization to update. A later slice must persist the
intent and previous resource evidence, acquire application exclusion, request
explicit confirmation and revalidate immediately before any mutation.
