#!/usr/bin/env bash
# Regenerate the vendored Rauthy OpenAPI spec.
#
# Rauthy only serves /auth/v1/docs/openapi.json when `swagger_ui_enable=true`,
# which is off in our deployments. So we boot the release image locally with the
# Swagger UI turned on and scrape the spec from it.
#
# Run this by hand when bumping the Rauthy version. It needs Docker; CI does not
# run it.
set -euo pipefail

version="${1:?usage: openapi-refresh.sh <rauthy-version> <output-path>}"
out="${2:?usage: openapi-refresh.sh <rauthy-version> <output-path>}"

image="ghcr.io/sebadob/rauthy:${version}"
name="rauthy-openapi-refresh-$$"
port=8099
# Kept inside the repo rather than /tmp: on macOS the Docker VM only shares the
# user's home directory, and a /var/folders temp dir silently mounts empty.
workdir="$(mktemp -d "$(dirname "$out")/.openapi-refresh.XXXXXX")"

cleanup() {
	docker rm -f "$name" >/dev/null 2>&1 || true
	rm -rf "$workdir"
}
trap cleanup EXIT

# Rauthy refuses to start without a config file. This throwaway config exists
# only to get the process far enough to serve its own OpenAPI document; the
# encryption key below is the example from the Rauthy book and must never be
# used for anything real.
cat >"$workdir/config.toml" <<'TOML'
[cluster]
node_id = 1
nodes = ["1 localhost:8100 localhost:8200"]
secret_raft = "OpenApiRefreshRaftSecret"
secret_api = "OpenApiRefreshApiSecret"

[encryption]
keys = ["q6u26/M0NFQzhSSldCY01rckJNa1JYZ3g2NUFtSnNOVGdoU0E="]
key_active = "q6u26"

[server]
scheme = "http"
pub_url = "localhost:8080"
swagger_ui_enable = true
swagger_ui_public = true

[webauthn]
rp_id = "localhost"
rp_origin = "http://localhost:8080"
TOML

echo "booting $image ..."
docker run -d --name "$name" -p "${port}:8080" \
	-v "$(cd "$workdir" && pwd)/config.toml:/app/config.toml:ro" \
	"$image" >/dev/null

url="http://127.0.0.1:${port}/auth/v1/docs/openapi.json"
for _ in $(seq 1 60); do
	if curl -sf "$url" -o "$out.tmp"; then
		python3 -m json.tool --indent 2 "$out.tmp" >"$out"
		rm -f "$out.tmp"
		echo "wrote $out from $image"
		exit 0
	fi
	sleep 2
done

echo "timed out waiting for $url" >&2
docker logs "$name" >&2 || true
exit 1
