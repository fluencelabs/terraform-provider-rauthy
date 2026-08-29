#!/usr/bin/env bash
# Remove the container started by rauthy-up.sh.
set -euo pipefail
docker rm -f "${RAUTHY_ACC_CONTAINER:-rauthy-acc}" >/dev/null 2>&1 || true

# The config and bootstrap files were kept alive for the container's sake.
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
rm -rf "${script_dir}/.rauthy-acc"
