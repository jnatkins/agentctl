#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
DOCKER="${DOCKER:-docker}"
PROJECT="${AGENTCTL_COMPOSE_PROJECT:-agentctl-remote-validation}"
SSH_DIR="$ROOT/.agentctl-test/ssh"

if ! command -v "$DOCKER" >/dev/null 2>&1; then
  echo "docker CLI not found. Set DOCKER=/path/to/docker or install the Docker CLI." >&2
  exit 127
fi

if [[ -z "${COMPOSE:-}" ]]; then
  if "$DOCKER" compose version >/dev/null 2>&1; then
    COMPOSE="$DOCKER compose"
  elif command -v docker-compose >/dev/null 2>&1; then
    COMPOSE="docker-compose"
  else
    echo "Docker Compose not found. Install docker-compose or set COMPOSE." >&2
    exit 127
  fi
fi

rm -rf "$SSH_DIR"
mkdir -p "$SSH_DIR"
ssh-keygen -t ed25519 -f "$SSH_DIR/id_ed25519" -N "" -C "agentctl-remote-validation" >/dev/null

cleanup() {
  AGENTCTL_SSH_DIR="$SSH_DIR" COMPOSE_PROJECT_NAME="$PROJECT" $COMPOSE -f "$ROOT/test/docker/compose.yaml" down -v >/dev/null 2>&1 || true
}
trap cleanup EXIT

AGENTCTL_SSH_DIR="$SSH_DIR" COMPOSE_PROJECT_NAME="$PROJECT" $COMPOSE -f "$ROOT/test/docker/compose.yaml" build
AGENTCTL_SSH_DIR="$SSH_DIR" COMPOSE_PROJECT_NAME="$PROJECT" $COMPOSE -f "$ROOT/test/docker/compose.yaml" up -d worker
AGENTCTL_SSH_DIR="$SSH_DIR" COMPOSE_PROJECT_NAME="$PROJECT" $COMPOSE -f "$ROOT/test/docker/compose.yaml" run --rm controller
