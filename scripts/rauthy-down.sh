#!/usr/bin/env bash
# Remove the container started by rauthy-up.sh.
set -euo pipefail
docker rm -f "${RAUTHY_ACC_CONTAINER:-rauthy-acc}" >/dev/null 2>&1 || true
