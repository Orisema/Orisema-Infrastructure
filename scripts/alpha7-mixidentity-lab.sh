#!/usr/bin/env bash
# Copyright 2026 Orisema contributors
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail
umask 077

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
binary=${ORISEMA_BINARY:-"$repo_root/work/orisema"}
evidence_dir=${ORISEMA_EVIDENCE_DIR:-"$repo_root/work/alpha7-mixidentity"}
app_image=${MIXIDENTITY_APP_IMAGE:-localhost/mix-identity_identity-app:latest}
web_image=${MIXIDENTITY_WEB_IMAGE:-localhost/mix-identity-web:1.0.0}
web_old_image=localhost/orisema-alpha8-mixidentity-web:previous
keycloak_image=${MIXIDENTITY_KEYCLOAK_IMAGE:-localhost/mix-identity-keycloak:26.7.5}
postgres_image=${MIXIDENTITY_POSTGRES_IMAGE:-docker.io/library/postgres:17.6-alpine3.22}

network=alpha7-mixidentity-lab-private
identity_db=alpha7-mixidentity-lab-identity-db
keycloak_db=alpha7-mixidentity-lab-keycloak-db
keycloak=alpha7-mixidentity-lab-keycloak
managed_app=orisema-alpha7-mixidentity-lab-identity-app
managed_web=orisema-alpha7-mixidentity-lab-identity-web
manifest_dir=$(mktemp -d)
app_manifest="$manifest_dir/app.yaml"
web_manifest="$manifest_dir/web.yaml"
web_old_manifest="$manifest_dir/web-old.yaml"
combined_manifest="$manifest_dir/combined.yaml"

identity_password="alpha7-identity-${RANDOM}-${RANDOM}-artificial"
keycloak_password="alpha7-keycloak-${RANDOM}-${RANDOM}-artificial"
admin_password="alpha7-admin-${RANDOM}-${RANDOM}-artificial"
app_key="base64:YWxwaGE3LW1peGlkZW50aXR5LWFydGlmaWNpYWwtMTI="
client_secret="alpha7-client-${RANDOM}-${RANDOM}-artificial"
rollback_name=
failed_name=

cleanup() {
	set +e
	for container in "$managed_web" "$managed_app" "$keycloak" "$keycloak_db" "$identity_db"; do
		podman rm --force "$container" >/dev/null 2>&1 || true
	done
	if [ -n "$rollback_name" ]; then
		podman rm --force "$rollback_name" >/dev/null 2>&1 || true
	fi
	if [ -n "$failed_name" ]; then
		podman rm --force "$failed_name" >/dev/null 2>&1 || true
	fi
	podman network rm "$network" >/dev/null 2>&1
	podman image rm "$web_old_image" >/dev/null 2>&1 || true
	find "$manifest_dir" -mindepth 1 -delete 2>/dev/null || true
	rmdir "$manifest_dir" 2>/dev/null || true
}
trap cleanup EXIT

mkdir -p "$evidence_dir"
chmod 0700 "$evidence_dir"

for resource in "$managed_web" "$managed_app" "$keycloak" "$keycloak_db" "$identity_db"; do
	if podman container exists "$resource"; then
		echo "refusing to adopt pre-existing Alpha 7 MixIdentity container: $resource" >&2
		exit 1
	fi
done
if podman network exists "$network"; then
	echo "refusing to adopt pre-existing Alpha 7 MixIdentity network: $network" >&2
	exit 1
fi

for image in "$app_image" "$web_image" "$keycloak_image" "$postgres_image"; do
	if ! podman image exists "$image"; then
		printf 'skipped: operator-owned MixIdentity image unavailable\n' >"$evidence_dir/skipped.txt"
		echo "Alpha 7 MixIdentity laboratory skipped: image unavailable: $image"
		exit 0
	fi
done
podman tag "$web_image" "$web_old_image"

wait_healthy() {
	local container=$1
	local attempts=$2
	for ((attempt = 1; attempt <= attempts; attempt++)); do
		if podman healthcheck run "$container" >/dev/null 2>&1; then
			return 0
		fi
		sleep 2
	done
	echo "container did not become healthy: $container" >&2
	podman logs --tail 50 "$container" >&2 || true
	return 1
}

podman network create "$network" >"$evidence_dir/network-created.txt"
podman run --detach --name "$identity_db" --network "$network" --network-alias identity-db \
	--env POSTGRES_DB=mix_identity --env POSTGRES_USER=mix_identity --env "POSTGRES_PASSWORD=$identity_password" \
	--health-cmd 'pg_isready -U mix_identity -d mix_identity' --health-interval 2s --health-timeout 2s --health-retries 30 \
	"$postgres_image" >/dev/null
podman run --detach --name "$keycloak_db" --network "$network" --network-alias keycloak-db \
	--env POSTGRES_DB=keycloak --env POSTGRES_USER=keycloak --env "POSTGRES_PASSWORD=$keycloak_password" \
	--health-cmd 'pg_isready -U keycloak -d keycloak' --health-interval 2s --health-timeout 2s --health-retries 30 \
	"$postgres_image" >/dev/null
wait_healthy "$identity_db" 45
wait_healthy "$keycloak_db" 45

podman run --detach --name "$keycloak" --network "$network" --network-alias keycloak \
	--env KC_DB=postgres --env KC_DB_URL=jdbc:postgresql://keycloak-db:5432/keycloak \
	--env KC_DB_USERNAME=keycloak --env "KC_DB_PASSWORD=$keycloak_password" \
	--env KC_BOOTSTRAP_ADMIN_USERNAME=alpha7-admin --env "KC_BOOTSTRAP_ADMIN_PASSWORD=$admin_password" \
	--env KC_HOSTNAME=http://keycloak:8080 --env KC_HTTP_ENABLED=true \
	--env KC_HEALTH_ENABLED=true --env KC_METRICS_ENABLED=true \
	--health-cmd "{ printf 'HEAD /health/ready HTTP/1.0\\r\\n\\r\\n' >&0; grep 'HTTP/1.0 200'; } 0<>/dev/tcp/127.0.0.1/9000" \
	--health-interval 3s --health-timeout 3s --health-retries 60 \
	"$keycloak_image" start --optimized >/dev/null
wait_healthy "$keycloak" 60
printf 'artificial dependencies healthy\n' >"$evidence_dir/dependencies.txt"

cat >"$app_manifest" <<YAML
apiVersion: orisema.dev/v1alpha4
kind: Application
name: alpha7-mixidentity-lab
networks:
  - name: private
    source:
      podman:
        name: $network
services:
  - name: identity-app
    image: $app_image
    publicEnvironment:
      - {name: APP_NAME, value: "MixIdentity Alpha 7 Lab"}
      - {name: APP_ENV, value: local}
      - {name: APP_KEY, value: "$app_key"}
      - {name: APP_DEBUG, value: "false"}
      - {name: APP_URL, value: "http://identity-web"}
      - {name: LOG_CHANNEL, value: stderr}
      - {name: DB_CONNECTION, value: pgsql}
      - {name: DB_HOST, value: identity-db}
      - {name: DB_PORT, value: "5432"}
      - {name: DB_DATABASE, value: mix_identity}
      - {name: DB_USERNAME, value: mix_identity}
      - {name: DB_PASSWORD, value: "$identity_password"}
      - {name: SESSION_DRIVER, value: file}
      - {name: CACHE_STORE, value: file}
      - {name: QUEUE_CONNECTION, value: sync}
      - {name: KEYCLOAK_BASE_URL, value: "http://keycloak:8080"}
      - {name: KEYCLOAK_PUBLIC_URL, value: "http://keycloak:8080"}
      - {name: KEYCLOAK_REALM, value: mixapps}
      - {name: KEYCLOAK_WEB_CLIENT_ID, value: identity-web}
      - {name: KEYCLOAK_WEB_CLIENT_SECRET, value: "$client_secret"}
    healthcheck:
      command: php-fpm -t && nc -z 127.0.0.1 9000
      interval: 10s
      timeout: 5s
      retries: 6
    networks:
      - name: private
YAML

"$binary" validate "$app_manifest" >"$evidence_dir/app-validate.txt"
"$binary" plan "$app_manifest" >"$evidence_dir/app-plan-before.txt"
grep -Fq '[CREATE]' "$evidence_dir/app-plan-before.txt"
printf 'DEPLOY\n' | "$binary" deploy "$app_manifest" >"$evidence_dir/app-deploy.txt"
wait_healthy "$managed_app" 30

# DNS aliases are deliberately outside the Alpha 7 contract. Reconnect only the
# isolated managed app so the unmodified MixIdentity nginx image can resolve its
# compiled-in `identity-app` upstream name.
podman network disconnect "$network" "$managed_app"
podman network connect --alias identity-app "$network" "$managed_app"
printf 'operator-applied isolated alias: identity-app\n' >"$evidence_dir/dns-alias-boundary.txt"

write_web_manifest() {
	local image=$1 output=$2
	cat >"$output" <<YAML
apiVersion: orisema.dev/v1alpha4
kind: Application
name: alpha7-mixidentity-lab
networks:
  - name: private
    source:
      podman:
        name: $network
services:
  - name: identity-web
    image: $image
    publicEnvironment:
      - {name: ORISEMA_LAB_TOPOLOGY, value: alpha7-mixidentity}
    healthcheck:
      command: wget -q -O /dev/null http://127.0.0.1:8080/health/live || exit 1
      interval: 10s
      timeout: 5s
      retries: 6
    networks:
      - name: private
YAML
}

write_web_manifest "$web_old_image" "$web_old_manifest"
write_web_manifest "$web_image" "$web_manifest"

"$binary" validate "$web_old_manifest" >"$evidence_dir/web-validate.txt"
"$binary" plan "$web_old_manifest" >"$evidence_dir/web-plan-before.txt"
grep -Fq '[CREATE]' "$evidence_dir/web-plan-before.txt"
printf 'DEPLOY\n' | "$binary" deploy "$web_old_manifest" >"$evidence_dir/web-deploy.txt"
wait_healthy "$managed_web" 30
podman exec "$managed_web" wget -q -O /dev/null http://127.0.0.1:8080/health/live
printf 'real app/web health endpoint reachable\n' >"$evidence_dir/reachability.txt"

old_web_id=$(podman inspect --format '{{.Id}}' "$managed_web")
"$binary" update prepare "$web_manifest" identity-web --journal "$evidence_dir/update-journal" >"$evidence_dir/update-prepare.txt"
operation_id=$(awk '/^Update intent persisted:/ {print $4}' "$evidence_dir/update-prepare.txt")
test -n "$operation_id"
printf 'AUTHORIZE UPDATE alpha7-mixidentity-lab/identity-web\n' | \
	"$binary" update authorize "$web_manifest" identity-web --journal "$evidence_dir/update-journal" \
		--operation "$operation_id" >"$evidence_dir/update-authorize.txt"
printf 'APPLY UPDATE alpha7-mixidentity-lab/identity-web\n' | \
	"$binary" update apply "$web_manifest" identity-web --journal "$evidence_dir/update-journal" \
		--operation "$operation_id" >"$evidence_dir/update-apply.txt"

rollback_name="orisema-rollback-${operation_id#update-}"
candidate_id=$(podman inspect --format '{{.Id}}' "$managed_web")
test "$candidate_id" != "$old_web_id"
test "$(podman inspect --format '{{.Id}}' "$rollback_name")" = "$old_web_id"
test "$(podman inspect --format '{{.Config.Image}}' "$managed_web")" = "$web_image"
podman stop --time 0 "$managed_web" >"$evidence_dir/update-candidate-stop.txt"
"$binary" update verify "$web_manifest" identity-web --journal "$evidence_dir/update-journal" \
	--operation "$operation_id" >"$evidence_dir/update-verify.txt"
grep -Fq 'decision: RECOVERY_REQUIRED' "$evidence_dir/update-verify.txt"

failed_name="orisema-failed-${operation_id#update-}"
printf 'ROLLBACK UPDATE alpha7-mixidentity-lab/identity-web %s\n' "$operation_id" | \
	"$binary" update rollback "$web_manifest" identity-web --journal "$evidence_dir/update-journal" \
		--operation "$operation_id" >"$evidence_dir/update-rollback.txt"
test "$(podman inspect --format '{{.Id}}' "$managed_web")" = "$old_web_id"
test "$(podman inspect --format '{{.Id}}' "$failed_name")" = "$candidate_id"
wait_healthy "$managed_web" 30
"$binary" update rollback-verify "$web_manifest" identity-web --journal "$evidence_dir/update-journal" \
	--operation "$operation_id" >"$evidence_dir/update-rollback-verify.txt"
grep -Fq 'decision=ROLLED_BACK' "$evidence_dir/update-rollback-verify.txt"
grep -Fq '"state": "ROLLED_BACK"' "$evidence_dir/update-journal/$operation_id.json"
printf 'configured web update failure and rollback passed\n' >"$evidence_dir/update-result.txt"

{
	sed -n '1,/^services:/p' "$app_manifest"
	sed -n '/^  - name: identity-app/,$p' "$app_manifest"
	sed -n '/^  - name: identity-web/,$p' "$web_old_manifest"
} >"$combined_manifest"
"$binary" validate "$combined_manifest" >"$evidence_dir/combined-validate.txt"
"$binary" plan "$combined_manifest" >"$evidence_dir/combined-plan-after.txt"
test "$(grep -Fc '[NOOP]' "$evidence_dir/combined-plan-after.txt")" -eq 2

for canary in "$identity_password" "$keycloak_password" "$admin_password" "$app_key" "$client_secret"; do
	if grep -FRq -- "$canary" "$evidence_dir"; then
		echo "artificial credential leaked into Alpha 7 MixIdentity evidence" >&2
		exit 1
	fi
done

podman rm --force "$managed_web" "$managed_app" >"$evidence_dir/managed-containers-removed.txt"
podman network exists "$network"
podman container exists "$identity_db"
podman container exists "$keycloak_db"
podman container exists "$keycloak"
printf 'external network and dependencies preserved after managed cleanup\n' >"$evidence_dir/external-resources-preserved.txt"

echo "Alpha 8 isolated MixIdentity configured update and rollback laboratory passed."
