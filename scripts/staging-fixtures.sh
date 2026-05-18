#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENVIRONMENT="${1:-}"
ACTION="${2:-}"
DEVICE_ID="${3:-}"
REGION="${HOMESIGNAL_AWS_REGION:-${AWS_REGION:-${AWS_DEFAULT_REGION:-us-east-1}}}"
SECRET_NAME="${HOMESIGNAL_DATABASE_URL_SECRET_NAME:-/homesignal/${ENVIRONMENT}/platform/database_url}"

fail() {
  echo "$1" >&2
  exit "${2:-1}"
}

usage() {
  cat >&2 <<USAGE
Usage: scripts/staging-fixtures.sh staging [seed-telemetry-device|cleanup-telemetry-device] <dev_smoke-...>
USAGE
}

if [[ "$ENVIRONMENT" != "staging" ]]; then
  usage
  exit 2
fi

case "$ACTION" in
  seed-telemetry-device|cleanup-telemetry-device)
    ;;
  *)
    usage
    exit 2
    ;;
esac

if [[ -z "$DEVICE_ID" ]]; then
  fail "Missing smoke device ID." 2
fi

if [[ "$DEVICE_ID" != dev_smoke-* ]]; then
  fail "Refusing to mutate non-smoke device ID: $DEVICE_ID" 2
fi

if ! command -v go >/dev/null 2>&1; then
  fail "Missing required command: go" 127
fi

resolve_database_url() {
  if [[ -n "${HOMESIGNAL_DATABASE_URL:-}" ]]; then
    echo "$HOMESIGNAL_DATABASE_URL"
    return 0
  fi
  if [[ -n "${DATABASE_URL:-}" ]]; then
    echo "$DATABASE_URL"
    return 0
  fi
  if ! command -v aws >/dev/null 2>&1; then
    return 1
  fi

  local value
  value="$(
    aws secretsmanager get-secret-value \
      --region "$REGION" \
      --secret-id "$SECRET_NAME" \
      --query SecretString \
      --output text 2>/dev/null || true
  )"
  if [[ -z "$value" || "$value" == "None" ]]; then
    return 1
  fi
  echo "$value"
}

DATABASE_URL_VALUE="$(resolve_database_url || true)"
if [[ -z "$DATABASE_URL_VALUE" ]]; then
  fail "Missing database URL. Populate $SECRET_NAME or set HOMESIGNAL_DATABASE_URL." 2
fi

(
  cd "$ROOT/backend"
  HOMESIGNAL_DATABASE_URL="$DATABASE_URL_VALUE" go run ./cmd/staging-fixtures \
    -mode "$ACTION" \
    -device-id "$DEVICE_ID"
)
