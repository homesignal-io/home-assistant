#!/usr/bin/env bash
set -euo pipefail

ENVIRONMENT="${1:-}"
DEVICE_ID="${2:-}"

if [[ "$ENVIRONMENT" != "staging" || -z "$DEVICE_ID" ]]; then
  echo "Usage: scripts/cleanup-staging-fixtures.sh staging <dev_smoke-...>" >&2
  exit 2
fi

"$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/staging-fixtures.sh" staging cleanup-telemetry-device "$DEVICE_ID"
