#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BACKEND_DIR="$ROOT/backend"
DIST_DIR="$BACKEND_DIR/dist/control-plane"
TELEMETRY_DIR="$ROOT/telemetry-ingest"
TELEMETRY_DIST_DIR="$TELEMETRY_DIR/dist"

require_command() {
  local command_name="$1"
  if ! command -v "$command_name" >/dev/null 2>&1; then
    echo "Missing required command: $command_name" >&2
    return 127
  fi
}

require_command go
require_command zip

VERSION="${HOMESIGNAL_VERSION:-}"
if [[ -z "$VERSION" ]] && command -v git >/dev/null 2>&1; then
  VERSION="$(git -C "$ROOT" rev-parse --short HEAD 2>/dev/null || true)"
fi
VERSION="${VERSION:-dev}"

mkdir -p "$DIST_DIR"
mkdir -p "$TELEMETRY_DIST_DIR"

echo "Building control-plane local binary at version $VERSION"
(
  cd "$BACKEND_DIR"
  go build -ldflags "-X main.version=$VERSION" -o "$DIST_DIR/control-plane" ./cmd/control-plane
)

echo "Building control-plane Lambda bootstrap for linux/arm64"
(
  cd "$BACKEND_DIR"
  GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "-X main.version=$VERSION" -o "$DIST_DIR/bootstrap" ./cmd/control-plane
)

echo "Packaging $DIST_DIR/bootstrap.zip"
(
  cd "$DIST_DIR"
  zip -q -j bootstrap.zip bootstrap
)

echo "Building telemetry-ingest local binary at version $VERSION"
(
  cd "$TELEMETRY_DIR"
  go build -ldflags "-X main.version=$VERSION -X main.commit=$VERSION" -o "$TELEMETRY_DIST_DIR/telemetry-ingest" ./cmd/telemetry-ingest
)

echo "Building telemetry-ingest linux/arm64 container binary"
(
  cd "$TELEMETRY_DIR"
  GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "-X main.version=$VERSION -X main.commit=$VERSION" -o "$TELEMETRY_DIST_DIR/telemetry-ingest-linux-arm64" ./cmd/telemetry-ingest
)

echo "Build complete"
echo "  local:  $DIST_DIR/control-plane"
echo "  lambda: $DIST_DIR/bootstrap.zip"
echo "  telemetry local:     $TELEMETRY_DIST_DIR/telemetry-ingest"
echo "  telemetry container: $TELEMETRY_DIST_DIR/telemetry-ingest-linux-arm64"
