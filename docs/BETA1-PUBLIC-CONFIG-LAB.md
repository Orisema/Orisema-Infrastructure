# Beta 1 public configuration candidate laboratory

Status: implementation evidence for issue #173. This laboratory does not
claim verify, completion, rollback or production readiness.

## Boundary

The laboratory changes one explicitly public environment value on one local
Podman service. The image, ports, healthcheck and restart policy remain
unchanged; the service has no secrets, networks or volumes. The previous
container must be preserved stopped under the operation-specific rollback name.

The operation intentionally ends in `APPLYING`. `update verify`, rollback and
cleanup reject this v3 contract. Operators must not remove either container or
edit the journal manually.

## Run

Use an isolated Linux host with rootless Podman and a development binary:

```sh
umask 077
go build -o /tmp/orisema-beta1 ./cmd/orisema
ORISEMA_BINARY=/tmp/orisema-beta1 \
ORISEMA_EVIDENCE_DIR=/tmp/orisema-beta1-evidence \
scripts/beta1-public-config-apply-lab.sh
```

The script owns only containers labeled for application
`beta1-public-config` and removes them on exit. Do not point its evidence path
at an existing directory containing operator data.

## Evidence checked

- wrong authorization and apply confirmations change neither journal nor
  running container;
- the correct confirmations include application, service and operation ID;
- candidate and previous container have distinct IDs;
- the previous container retains the old public value and remains stopped;
- the candidate uses the new value and unchanged image;
- public values appear only in the supplied manifests and live Podman
  configuration, not journal, command output or retained diagnostic evidence;
- verify is rejected and `operation diagnose` reports `APPLYING`.

## Interruption and recovery

Every runtime effect uses the existing update execution journal boundaries:
`stop-previous`, `rename-previous` and `create-candidate`. An interruption
preserves the pending step and all observed IDs. The supported next action in
this increment is inspection only:

```sh
orisema operation diagnose --journal /absolute/journal
```

There is no automatic resume, cleanup or rollback for a Beta 1 public
configuration intent yet. Preserve the journal, candidate and previous
container for the following reviewed recovery increment.
