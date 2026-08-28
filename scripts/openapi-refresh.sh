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
port="${RAUTHY_OPENAPI_PORT:-8099}"

if ! command -v docker >/dev/null 2>&1; then
	cat >&2 <<-EOF
		docker not found.

		This target boots the Rauthy release image locally to scrape its OpenAPI
		document, so a container runtime is required. Install Docker (or start
		Colima / Docker Desktop if it is installed but not running) and retry.
	EOF
	exit 1
fi

if ! docker info >/dev/null 2>&1; then
	echo "docker is installed but the daemon is not reachable; start it and retry." >&2
	exit 1
fi

# Pull the image, falling back to skopeo.
#
# `docker pull` from ghcr.io fails on some macOS setups: the osxkeychain
# credential helper hangs waiting on a keychain prompt, or the pull is refused
# with "denied" even though the image is anonymously pullable. skopeo talks to
# the registry itself and sidesteps the whole credential path, so we copy the
# image to a tarball and load that.
pull_image() {
	if docker image inspect "$image" >/dev/null 2>&1; then
		echo "using local image $image"
		return 0
	fi

	echo "pulling $image ..."
	if docker pull "$image" 2>&1 | sed 's/^/  docker: /'; then
		return 0
	fi

	echo "docker pull failed; trying skopeo (this is expected on macOS with the osxkeychain helper)" >&2

	local skopeo=(skopeo)
	if ! command -v skopeo >/dev/null 2>&1; then
		if command -v nix >/dev/null 2>&1; then
			skopeo=(nix run nixpkgs#skopeo --)
		else
			cat >&2 <<-EOF

				Could not pull $image.

				docker pull failed and skopeo is not installed, so there is no way left to
				fetch the image. Either:
				  * fix the docker pull (on macOS this is usually the osxkeychain credential
				    helper: remove "credsStore" from ~/.docker/config.json, or approve the
				    keychain prompt), or
				  * install skopeo (brew install skopeo / nix profile install nixpkgs#skopeo)
				    and rerun this script, or
				  * pull the image by any other means; if $image is already present locally
				    this script uses it as is.
			EOF
			exit 1
		fi
	fi

	local arch tar policy
	arch="$(docker info --format '{{.Architecture}}' 2>/dev/null || uname -m)"
	case "$arch" in
	aarch64 | arm64) arch=arm64 ;;
	x86_64 | amd64) arch=amd64 ;;
	esac

	tar="${workdir}/image.tar"
	policy="${workdir}/policy.json"
	printf '{"default":[{"type":"insecureAcceptAnything"}]}' >"$policy"

	"${skopeo[@]}" --policy "$policy" copy \
		--override-os linux --override-arch "$arch" \
		"docker://${image}" "docker-archive:${tar}:${image}"
	docker load -i "$tar" >/dev/null
	rm -f "$tar"
}

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

pull_image

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
