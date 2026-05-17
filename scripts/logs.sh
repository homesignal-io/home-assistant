#!/usr/bin/env bash
set -euo pipefail

ENVIRONMENT="${1:-}"
SERVICE="${2:-control-plane}"
REGION="${HOMESIGNAL_AWS_REGION:-${AWS_REGION:-${AWS_DEFAULT_REGION:-us-east-1}}}"

if [[ "$ENVIRONMENT" != "staging" ]]; then
  echo "Usage: scripts/logs.sh staging [control-plane|telemetry-ingest|iot-lifecycle]" >&2
  exit 2
fi

if ! command -v aws >/dev/null 2>&1; then
  echo "Missing required command: aws" >&2
  exit 127
fi

case "$SERVICE" in
  control-plane)
    LOG_GROUP="/homesignal/staging/control-plane"
    ;;
  telemetry-ingest)
    LOG_GROUP="/homesignal/staging/telemetry-ingest"
    ;;
  iot-lifecycle)
    LOG_GROUP="/homesignal/staging/iot/lifecycle"
    ;;
  *)
    echo "Unknown service: $SERVICE" >&2
    echo "Usage: scripts/logs.sh staging [control-plane|telemetry-ingest|iot-lifecycle]" >&2
    exit 2
    ;;
esac

aws logs tail "$LOG_GROUP" --follow --region "$REGION"
