#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENVIRONMENT="${1:-}"
REGION="${HOMESIGNAL_AWS_REGION:-${AWS_REGION:-${AWS_DEFAULT_REGION:-us-east-1}}}"
VERSION="${HOMESIGNAL_VERSION:-}"
BUDGET_ALERT_EMAIL="${HOMESIGNAL_BUDGET_ALERT_EMAIL:-}"
BUDGET_GUARDRAIL_CONFIRMED="${HOMESIGNAL_BUDGET_GUARDRAIL_CONFIRMED:-}"
CREATE_BUDGET="${HOMESIGNAL_CREATE_STAGING_BUDGET:-0}"
MONTHLY_BUDGET_AMOUNT="${HOMESIGNAL_STAGING_BUDGET_AMOUNT:-25}"
OWNER_TAG="${HOMESIGNAL_OWNER_TAG:-platform}"
TELEMETRY_IMAGE_TAG="${HOMESIGNAL_TELEMETRY_IMAGE_TAG:-}"

fail() {
  echo "$1" >&2
  exit "${2:-1}"
}

require_command() {
  local command_name="$1"
  if ! command -v "$command_name" >/dev/null 2>&1; then
    fail "Missing required command: $command_name" 127
  fi
}

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
  fail "Usage: scripts/deploy.sh staging"
fi

if [[ "$REGION" != "us-east-1" ]]; then
  fail "First deploy is pinned to us-east-1 to stay colocated with staging Neon."
fi

require_command aws
require_command docker

TF_BIN="$(terraform_command || true)"
if [[ -z "$TF_BIN" ]]; then
  fail "Missing required command: tofu or terraform" 127
fi

export AWS_REGION="$REGION"
export AWS_DEFAULT_REGION="$REGION"

ACCOUNT_ID="$(aws sts get-caller-identity --query Account --output text)"
CALLER_ARN="$(aws sts get-caller-identity --query Arn --output text)"
if [[ "$CALLER_ARN" == arn:aws:iam::*:root ]]; then
  fail "Refusing to deploy with root AWS credentials. Use a named deploy principal."
fi

if [[ -z "$VERSION" ]] && command -v git >/dev/null 2>&1; then
  VERSION="$(git -C "$ROOT" rev-parse --short HEAD 2>/dev/null || true)"
fi
VERSION="${VERSION:-dev}"
TELEMETRY_IMAGE_TAG="${TELEMETRY_IMAGE_TAG:-$VERSION}"

echo "Deploying HomeSignal staging"
echo "  account: $ACCOUNT_ID"
echo "  caller:  $CALLER_ARN"
echo "  region:  $REGION"
echo "  version: $VERSION"

if [[ -z "$BUDGET_ALERT_EMAIL" && "$BUDGET_GUARDRAIL_CONFIRMED" != "1" ]]; then
  fail "Missing staging budget guardrail input. Set HOMESIGNAL_BUDGET_ALERT_EMAIL for the payer-account budget task, or set HOMESIGNAL_BUDGET_GUARDRAIL_CONFIRMED=1 if it already exists." 2
fi

if [[ -z "$BUDGET_ALERT_EMAIL" && "$BUDGET_GUARDRAIL_CONFIRMED" == "1" ]]; then
  echo "Budget guardrail marked as already configured; Terraform will skip the AWS Budget resource."
fi

if [[ "$CREATE_BUDGET" == "1" && -z "$BUDGET_ALERT_EMAIL" ]]; then
  fail "HOMESIGNAL_CREATE_STAGING_BUDGET=1 requires HOMESIGNAL_BUDGET_ALERT_EMAIL." 2
fi

if [[ "$CREATE_BUDGET" != "1" && -n "$BUDGET_ALERT_EMAIL" ]]; then
  echo "Budget alert email captured. Skipping budget creation from the member account; create/enable the budget in the payer account or rerun with HOMESIGNAL_CREATE_STAGING_BUDGET=1 after payer-account Budgets are enabled."
fi

HOMESIGNAL_VERSION="$VERSION" "$ROOT/scripts/build.sh"

(
  cd "$ROOT/infra/envs/staging"
  "$TF_BIN" init

  "$TF_BIN" apply \
    -auto-approve \
    -target=aws_ecr_repository.telemetry_ingest \
    -var="aws_region=$REGION" \
    -var="lambda_package_path=$ROOT/backend/dist/control-plane/bootstrap.zip" \
    -var="artifact_version=$VERSION" \
    -var="budget_alert_email=$BUDGET_ALERT_EMAIL" \
    -var="create_budget=$([[ "$CREATE_BUDGET" == "1" ]] && echo true || echo false)" \
    -var="monthly_budget_amount=$MONTHLY_BUDGET_AMOUNT" \
    -var="owner_tag=$OWNER_TAG" \
    -var="telemetry_ingest_image=bootstrap"

  TELEMETRY_ECR_REPOSITORY_URL="$("$TF_BIN" output -raw telemetry_ingest_ecr_repository_url)"
  TELEMETRY_ECR_REGISTRY="${TELEMETRY_ECR_REPOSITORY_URL%/*}"
  TELEMETRY_IMAGE="$TELEMETRY_ECR_REPOSITORY_URL:$TELEMETRY_IMAGE_TAG"

  echo "Logging into telemetry-ingest ECR registry"
  aws ecr get-login-password --region "$REGION" | docker login --username AWS --password-stdin "$TELEMETRY_ECR_REGISTRY"

  echo "Building telemetry-ingest container image"
  docker build --platform linux/arm64 -t "$TELEMETRY_IMAGE" "$ROOT/telemetry-ingest"

  echo "Pushing telemetry-ingest container image"
  docker push "$TELEMETRY_IMAGE"

  "$TF_BIN" apply \
    -auto-approve \
    -var="aws_region=$REGION" \
    -var="lambda_package_path=$ROOT/backend/dist/control-plane/bootstrap.zip" \
    -var="artifact_version=$VERSION" \
    -var="budget_alert_email=$BUDGET_ALERT_EMAIL" \
    -var="create_budget=$([[ "$CREATE_BUDGET" == "1" ]] && echo true || echo false)" \
    -var="monthly_budget_amount=$MONTHLY_BUDGET_AMOUNT" \
    -var="owner_tag=$OWNER_TAG" \
    -var="telemetry_ingest_image=$TELEMETRY_IMAGE"

  echo "Deploy complete"
  echo "  staging_base_url: $("$TF_BIN" output -raw staging_base_url)"
  echo "  telemetry_ingest_image: $TELEMETRY_IMAGE"
)
