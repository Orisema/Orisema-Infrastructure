#!/usr/bin/env bash
# Copyright 2026 Orisema contributors
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

ORISEMA_BINARY=${ORISEMA_BINARY:-./work/orisema}
EVIDENCE_DIR=${EVIDENCE_DIR:-./work/alpha7-config-network}
PRIMARY_NETWORK=alpha7-config-network-lab-private
ALTERNATE_NETWORK=alpha7-config-network-lab-alternate
CONTAINER=orisema-alpha7-config-network-lab-web
MANIFEST_DIR=$(mktemp -d)
PRIMARY_MANIFEST=$MANIFEST_DIR/primary.yaml
NETWORK_DRIFT_MANIFEST=$MANIFEST_DIR/network-drift.yaml
CONFIG_DRIFT_MANIFEST=$MANIFEST_DIR/config-drift.yaml

cleanup() {
	set +e
	podman rm -f "$CONTAINER" >/dev/null 2>&1
	podman network rm "$PRIMARY_NETWORK" >/dev/null 2>&1
	podman network rm "$ALTERNATE_NETWORK" >/dev/null 2>&1
	rm -rf "$MANIFEST_DIR"
}
trap cleanup EXIT

mkdir -p "$EVIDENCE_DIR"
if podman container exists "$CONTAINER" || podman network exists "$PRIMARY_NETWORK" || podman network exists "$ALTERNATE_NETWORK"; then
	echo "refusing to adopt pre-existing Alpha 7 lab resources" >&2
	exit 1
fi

podman network create "$PRIMARY_NETWORK" >"$EVIDENCE_DIR/primary-network-id.txt"
podman network create "$ALTERNATE_NETWORK" >"$EVIDENCE_DIR/alternate-network-id.txt"

cat >"$PRIMARY_MANIFEST" <<'YAML'
apiVersion: orisema.dev/v1alpha4
kind: Application
name: alpha7-config-network-lab
networks:
  - name: private
    source:
      podman:
        name: alpha7-config-network-lab-private
services:
  - name: web
    image: docker.io/library/nginx:1.27-alpine
    publicEnvironment:
      - name: APP_MODE
        value: laboratory
    networks:
      - name: private
YAML

"$ORISEMA_BINARY" validate "$PRIMARY_MANIFEST" >"$EVIDENCE_DIR/validate.txt"
"$ORISEMA_BINARY" plan "$PRIMARY_MANIFEST" >"$EVIDENCE_DIR/plan-before.txt"
grep -q 'CREATE' "$EVIDENCE_DIR/plan-before.txt"
JOURNAL="$EVIDENCE_DIR/operations"
"$ORISEMA_BINARY" operation init --journal "$JOURNAL" >"$EVIDENCE_DIR/journal-init.txt"
printf 'DEPLOY\n' | "$ORISEMA_BINARY" deploy --journal "$JOURNAL" "$PRIMARY_MANIFEST" >"$EVIDENCE_DIR/deploy.txt"

podman exec "$CONTAINER" sh -c 'test "$APP_MODE" = laboratory'
podman inspect --format '{{range .Config.Env}}{{if eq . "APP_MODE=laboratory"}}present{{end}}{{end}}' "$CONTAINER" >"$EVIDENCE_DIR/public-config-present.txt"
grep -qx 'present' "$EVIDENCE_DIR/public-config-present.txt"
podman inspect --format '{{range $name, $_ := .NetworkSettings.Networks}}{{$name}}{{end}}' "$CONTAINER" >"$EVIDENCE_DIR/attached-network.txt"
grep -qx "$PRIMARY_NETWORK" "$EVIDENCE_DIR/attached-network.txt"

"$ORISEMA_BINARY" plan "$PRIMARY_MANIFEST" >"$EVIDENCE_DIR/plan-after.txt"
grep -q 'NOOP' "$EVIDENCE_DIR/plan-after.txt"

sed "s/$PRIMARY_NETWORK/$ALTERNATE_NETWORK/" "$PRIMARY_MANIFEST" >"$NETWORK_DRIFT_MANIFEST"
"$ORISEMA_BINARY" plan "$NETWORK_DRIFT_MANIFEST" >"$EVIDENCE_DIR/plan-network-drift.txt"
grep -q 'DRIFT' "$EVIDENCE_DIR/plan-network-drift.txt"

sed 's/value: laboratory/value: changed/' "$PRIMARY_MANIFEST" >"$CONFIG_DRIFT_MANIFEST"
"$ORISEMA_BINARY" plan "$CONFIG_DRIFT_MANIFEST" >"$EVIDENCE_DIR/plan-config-drift.txt"
grep -q 'DRIFT' "$EVIDENCE_DIR/plan-config-drift.txt"

if grep -R -E 'laboratory|value: changed' "$EVIDENCE_DIR" >/dev/null; then
	echo "public configuration value leaked into evidence" >&2
	exit 1
fi

podman rm -f "$CONTAINER" >"$EVIDENCE_DIR/container-removed.txt"
podman network exists "$PRIMARY_NETWORK"
podman network exists "$ALTERNATE_NETWORK"
printf 'external networks preserved\n' >"$EVIDENCE_DIR/external-networks-preserved.txt"

echo "Alpha 7 public configuration and external network lab passed"
