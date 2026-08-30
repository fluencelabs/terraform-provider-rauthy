#!/usr/bin/env bash
# Boot a throwaway Rauthy for the acceptance tests and print the environment
# they need.
#
#   eval "$(scripts/rauthy-up.sh 0.36.2)"
#   TF_ACC=1 go test ./... -p 1
#   scripts/rauthy-down.sh
#
# Pass --github-env to print `KEY=value` instead of `export KEY='value'`, the
# form GitHub Actions expects when appending to $GITHUB_ENV.
#
# The API key is seeded at first start through Rauthy's bootstrap: the secret is
# pinned here rather than generated, so the caller knows the credential before
# the container exists and never has to scrape it out of the logs.
set -euo pipefail

version="${1:?usage: rauthy-up.sh <rauthy-version> [--github-env]}"
output_format="${2:-shell}"

case "$output_format" in
shell | --github-env) ;;
*)
	echo "unknown output format '${output_format}'; expected --github-env or nothing" >&2
	exit 2
	;;
esac


image="ghcr.io/sebadob/rauthy:${version}"
name="${RAUTHY_ACC_CONTAINER:-rauthy-acc}"
port="${RAUTHY_ACC_PORT:-8098}"

# The key name must match ^[a-zA-Z0-9_-/]{2,24}$ and the secret must be at least
# 64 alphanumeric characters. Both are throwaway test credentials for a
# container that is destroyed afterwards.
key_name="tfacc"
key_secret="tfaccSecretDoNotUseAnywhereElse0123456789abcdefghijklmnopqrstuvwxyz"

# Every access group this provider touches. password_policy is Secrets:update,
# not a group of its own — Rauthy has no `Config` group. Rauthy 0.36 moved
# /users/attr off the `Users` group onto `UserAttributes`; without that entry
# every user-attribute call 403s before a test runs. `Blacklist` guards
# /blacklist, taken from the live 403 message rather than guessed.
read -r -d '' key_json <<'JSON' || true
{
  "name": "tfacc",
  "access": [
    {"group": "Clients", "access_rights": ["read", "create", "update", "delete"]},
    {"group": "Secrets", "access_rights": ["read", "update"]},
    {"group": "Roles",   "access_rights": ["read", "create", "update", "delete"]},
    {"group": "Groups",  "access_rights": ["read", "create", "update", "delete"]},
    {"group": "Scopes",  "access_rights": ["read", "create", "update", "delete"]},
    {"group": "Users",   "access_rights": ["read", "create", "update", "delete"]},
    {"group": "UserAttributes", "access_rights": ["read", "create", "update", "delete"]},
    {"group": "AuthProviders", "access_rights": ["read", "create", "update", "delete"]},
    {"group": "Pam",     "access_rights": ["read", "create", "update", "delete"]},
    {"group": "Blacklist", "access_rights": ["read", "create", "update", "delete"]}
  ]
}
JSON

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=scripts/lib-rauthy.sh
. "${script_dir}/lib-rauthy.sh"

# The container outlives this script — rauthy-down.sh tears it down later — so
# the files it bind-mounts have to outlive it too. Deleting them on exit works
# on Linux only because the mount pins the inode; with Docker Desktop or Colima
# the share is path-based and the mounted file goes unreadable the moment the
# host copy disappears. The directory has a fixed name so rauthy-down.sh can
# clean it up, and is recreated from scratch on every boot.
workdir="${script_dir}/.rauthy-acc"
rm -rf "$workdir"
mkdir -p "$workdir"

# Seed the user attributes the acceptance tests map into tokens. Rauthy filters
# a scope's attr_include_* against the attributes that already exist and drops
# the rest silently, so without these the mapping half of rauthy_scope cannot be
# exercised at all.
mkdir -p "$workdir/bootstrap"
cat >"$workdir/bootstrap/user_attributes.json" <<'JSON'
[
  {"name": "department", "desc": "Acceptance test attribute"},
  {"name": "cost_center", "desc": "Acceptance test attribute"}
]
JSON

cat >"$workdir/config.toml" <<TOML
[cluster]
node_id = 1
nodes = ["1 localhost:8100 localhost:8200"]
secret_raft = "AcceptanceTestRaftSecretNotForRealUse"
secret_api = "AcceptanceTestApiSecretNotForRealUse"

[encryption]
keys = ["q6u26/M0NFQzhSSldCY01rckJNa1JYZ3g2NUFtSnNOVGdoU0E="]
key_active = "q6u26"

[server]
scheme = "http"
pub_url = "localhost:${port}"

[webauthn]
rp_id = "localhost"
rp_origin = "http://localhost:${port}"
TOML

rauthy_pull_image "$image" "$workdir"

docker rm -f "$name" >/dev/null 2>&1 || true
docker run -d --name "$name" -p "${port}:8080" \
	-e "BOOTSTRAP_API_KEY=$(printf '%s' "$key_json" | base64 | tr -d '\n')" \
	-e "BOOTSTRAP_API_KEY_SECRET=${key_secret}" \
	-e "BOOTSTRAP_DIR=/app/bootstrap" \
	-v "$(cd "$workdir" && pwd)/config.toml:/app/config.toml:ro" \
	-v "$(cd "$workdir" && pwd)/bootstrap:/app/bootstrap:ro" \
	"$image" >/dev/null

url="http://127.0.0.1:${port}"
if ! rauthy_wait_ready "${url}/auth/v1/ping" 90; then
	echo "rauthy did not become ready; container logs follow" >&2
	docker logs "$name" >&2 || true
	docker rm -f "$name" >/dev/null 2>&1 || true
	rm -rf "$workdir"
	exit 1
fi

# Prove the seeded key actually works before handing it to the caller: a
# bootstrap that silently did nothing would otherwise surface as a wall of
# confusing 401s inside the test run.
if ! curl -sf -o /dev/null -H "Authorization: API-Key ${key_name}\$${key_secret}" \
	"${url}/auth/v1/clients"; then
	echo "the bootstrapped API key was rejected by ${url}/auth/v1/clients" >&2
	docker logs "$name" >&2 || true
	docker rm -f "$name" >/dev/null 2>&1 || true
	rm -rf "$workdir"
	exit 1
fi

case "$output_format" in
--github-env)
	echo "RAUTHY_URL=${url}"
	echo "RAUTHY_API_KEY=${key_name}\$${key_secret}"
	;;
*)
	echo "export RAUTHY_URL='${url}'"
	echo "export RAUTHY_API_KEY='${key_name}\$${key_secret}'"
	;;
esac
