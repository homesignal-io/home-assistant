#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENVIRONMENT="${1:-}"
BASE_URL="${HOMESIGNAL_STAGING_BASE_URL:-}"
REGION="${HOMESIGNAL_AWS_REGION:-${AWS_REGION:-${AWS_DEFAULT_REGION:-us-east-1}}}"

terraform_command() {
  if command -v tofu >/dev/null 2>&1; then
    echo "tofu"
    return 0
  fi
  if command -v terraform >/dev/null 2>&1; then
    echo "terraform"
    return 0
  fi
  return 1
}

if [[ "$ENVIRONMENT" != "staging" ]]; then
  echo "Usage: scripts/smoke.sh staging" >&2
  exit 2
fi

if [[ -z "$BASE_URL" ]]; then
  TF_BIN="$(terraform_command || true)"
  if [[ -n "$TF_BIN" ]]; then
    BASE_URL="$(
      cd "$ROOT/infra/envs/staging"
      "$TF_BIN" output -raw staging_base_url 2>/dev/null || true
    )"
  fi
fi

if [[ -z "$BASE_URL" ]]; then
  echo "Missing staging base URL." >&2
  echo "Set HOMESIGNAL_STAGING_BASE_URL or run from an applied staging Terraform workspace." >&2
  exit 2
fi

if ! command -v curl >/dev/null 2>&1; then
  echo "Missing required command: curl" >&2
  exit 127
fi
if ! command -v aws >/dev/null 2>&1; then
  echo "Missing required command: aws" >&2
  exit 127
fi

BASE_URL="${BASE_URL%/}"
for path in /healthz /readyz /version; do
  echo "Checking $BASE_URL$path"
  curl -fsS "$BASE_URL$path" >/dev/null
done

echo "Checking public API shell requires authentication"
PUBLIC_STATUS="$(
  curl -sS \
    -o /dev/null \
    -w "%{http_code}" \
    "$BASE_URL/api/v1/dashboard"
)"
if [[ "$PUBLIC_STATUS" != "401" ]]; then
  echo "Unexpected /api/v1/dashboard status: $PUBLIC_STATUS" >&2
  exit 1
fi

TF_BIN="$(terraform_command || true)"
if [[ -z "$TF_BIN" ]]; then
  echo "Missing required command: tofu or terraform" >&2
  exit 127
fi

terraform_output() {
  local name="$1"
  (
    cd "$ROOT/infra/envs/staging"
    "$TF_BIN" output -raw "$name"
  )
}

TELEMETRY_CLUSTER="$(terraform_output telemetry_ingest_cluster_name)"
TELEMETRY_SERVICE="$(terraform_output telemetry_ingest_service_name)"
IOT_POLICY_NAME="$(terraform_output iot_device_policy_name)"
IOT_RULE_NAME="$(terraform_output iot_lifecycle_rule_name)"

echo "Waiting for telemetry-ingest ECS service to be stable"
aws ecs wait services-stable \
  --region "$REGION" \
  --cluster "$TELEMETRY_CLUSTER" \
  --services "$TELEMETRY_SERVICE"

TASK_ARN="$(
  aws ecs list-tasks \
    --region "$REGION" \
    --cluster "$TELEMETRY_CLUSTER" \
    --service-name "$TELEMETRY_SERVICE" \
    --desired-status RUNNING \
    --query 'taskArns[0]' \
    --output text
)"
if [[ -z "$TASK_ARN" || "$TASK_ARN" == "None" ]]; then
  echo "No running telemetry-ingest task found." >&2
  exit 1
fi

ENI_ID="$(
  aws ecs describe-tasks \
    --region "$REGION" \
    --cluster "$TELEMETRY_CLUSTER" \
    --tasks "$TASK_ARN" \
    --query 'tasks[0].attachments[0].details[?name==`networkInterfaceId`].value | [0]' \
    --output text
)"
PUBLIC_IP="$(
  aws ec2 describe-network-interfaces \
    --region "$REGION" \
    --network-interface-ids "$ENI_ID" \
    --query 'NetworkInterfaces[0].Association.PublicIp' \
    --output text
)"
if [[ -z "$PUBLIC_IP" || "$PUBLIC_IP" == "None" ]]; then
  echo "Telemetry-ingest task has no public IP for direct staging smoke." >&2
  exit 1
fi

TELEMETRY_URL="http://$PUBLIC_IP:8080"
for path in /healthz /readyz /version; do
  echo "Checking $TELEMETRY_URL$path"
  curl -fsS "$TELEMETRY_URL$path" >/dev/null
done

FIXTURE="$ROOT/testdata/contracts/runtime/agent_https_telemetry_device_health_snapshot_v1_valid.json"
FIRST_FIXTURE="$(mktemp)"
SECOND_FIXTURE="$(mktemp)"
trap 'rm -f "$FIRST_FIXTURE" "$SECOND_FIXTURE"' EXIT
SMOKE_ID_PREFIX="smoke-$(date +%s)-$$-$RANDOM"
SMOKE_DEVICE_ID="dev_${SMOKE_ID_PREFIX}"
sed "s/\"message_id\": \"01J00000000000000000000000\"/\"message_id\": \"${SMOKE_ID_PREFIX}-1\"/" "$FIXTURE" >"$FIRST_FIXTURE"
sed "s/\"message_id\": \"01J00000000000000000000000\"/\"message_id\": \"${SMOKE_ID_PREFIX}-2\"/" "$FIXTURE" >"$SECOND_FIXTURE"

echo "Checking telemetry-ingest accepts first health snapshot"
FIRST_RESPONSE="$(
  curl -fsS \
    -H "Content-Type: application/json" \
    -H "X-HomeSignal-Device-ID: $SMOKE_DEVICE_ID" \
    -H "X-HomeSignal-Site-ID: site_01J00000000000000000000000" \
    -H "X-HomeSignal-Org-ID: org_01J00000000000000000000000" \
    -H "X-Client-Cert-Fingerprint: SHA256:fixture" \
    -H "X-Client-Cert-Serial: 01J00000000000000000000000" \
    --data-binary @"$FIRST_FIXTURE" \
    "$TELEMETRY_URL/agent/telemetry"
)"
if [[ "$FIRST_RESPONSE" != *'"accepted":true'* || "$FIRST_RESPONSE" != *'"written":true'* ]]; then
  echo "Unexpected first telemetry response: $FIRST_RESPONSE" >&2
  exit 1
fi

echo "Checking telemetry-ingest suppresses unchanged health snapshot"
SECOND_RESPONSE="$(
  curl -fsS \
    -H "Content-Type: application/json" \
    -H "X-HomeSignal-Device-ID: $SMOKE_DEVICE_ID" \
    -H "X-HomeSignal-Site-ID: site_01J00000000000000000000000" \
    -H "X-HomeSignal-Org-ID: org_01J00000000000000000000000" \
    -H "X-Client-Cert-Fingerprint: SHA256:fixture" \
    -H "X-Client-Cert-Serial: 01J00000000000000000000000" \
    --data-binary @"$SECOND_FIXTURE" \
    "$TELEMETRY_URL/agent/telemetry"
)"
if [[ "$SECOND_RESPONSE" != *'"accepted":true'* || "$SECOND_RESPONSE" != *'"suppressed":true'* || "$SECOND_RESPONSE" != *'"suppression_reason":"unchanged_material"'* ]]; then
  echo "Unexpected second telemetry response: $SECOND_RESPONSE" >&2
  exit 1
fi

echo "Checking AWS IoT endpoint and staging policy/rule"
aws iot describe-endpoint \
  --region "$REGION" \
  --endpoint-type iot:Data-ATS \
  --query endpointAddress \
  --output text >/dev/null
aws iot get-policy \
  --region "$REGION" \
  --policy-name "$IOT_POLICY_NAME" >/dev/null
aws iot get-topic-rule \
  --region "$REGION" \
  --rule-name "$IOT_RULE_NAME" >/dev/null

"$ROOT/scripts/migrate.sh" staging status-if-configured

echo "Smoke checks passed"
