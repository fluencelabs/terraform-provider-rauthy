#!/usr/bin/env bash
# Shared helpers for the scripts that boot a Rauthy container:
# openapi-refresh.sh (scrapes the OpenAPI document) and rauthy-up.sh (stands up
# an instance for the acceptance tests).

# rauthy_require_docker fails with an actionable message when there is no
# reachable container runtime.
rauthy_require_docker() {
	if ! command -v docker >/dev/null 2>&1; then
		cat >&2 <<-EOF
			docker not found.

			This target boots the Rauthy release image locally, so a container runtime is
			required. Install Docker (or start Colima / Docker Desktop if it is installed
			but not running) and retry.
		EOF
		return 1
	fi
	if ! docker info >/dev/null 2>&1; then
		echo "docker is installed but the daemon is not reachable; start it and retry." >&2
		return 1
	fi
}

# rauthy_pull_image <image> <workdir> makes the image available locally.
#
# `docker pull` from ghcr.io fails on some macOS setups: the osxkeychain
# credential helper hangs waiting on a keychain prompt, or the pull is refused
# with "denied" even though the image is anonymously pullable. skopeo talks to
# the registry itself and sidesteps the whole credential path, so we copy the
# image to a tarball and load that.
rauthy_pull_image() {
	local image="$1" workdir="$2"

	if docker image inspect "$image" >/dev/null 2>&1; then
		echo "using local image $image" >&2
		return 0
	fi

	echo "pulling $image ..." >&2
	if docker pull "$image" 2>&1 | sed 's/^/  docker: /' >&2; then
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
			return 1
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

# rauthy_wait_ready <url> <attempts> polls until the URL answers.
rauthy_wait_ready() {
	local url="$1" attempts="${2:-60}"
	local i
	for ((i = 0; i < attempts; i++)); do
		if curl -sf -o /dev/null "$url"; then
			return 0
		fi
		sleep 2
	done
	return 1
}
