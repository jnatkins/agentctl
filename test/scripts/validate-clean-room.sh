#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
DOCKER="${DOCKER:-docker}"
IMAGE="${AGENTCTL_TEST_IMAGE:-agentctl-validation:local}"

if ! command -v "$DOCKER" >/dev/null 2>&1; then
  echo "docker CLI not found. Set DOCKER=/path/to/docker or install the Docker CLI." >&2
  exit 127
fi

"$DOCKER" build -f "$ROOT/test/docker/Dockerfile" -t "$IMAGE" "$ROOT"
"$DOCKER" run --rm "$IMAGE" /workspace/agentctl/test/scripts/inside-clean-room.sh
