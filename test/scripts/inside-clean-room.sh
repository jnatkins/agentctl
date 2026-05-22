#!/usr/bin/env bash
set -euo pipefail

export HOME=/tmp/agent-home
export CODEX_HOME="$HOME/.codex"
export GOCACHE=/tmp/go-cache
export GOMODCACHE=/go/pkg/mod

ROOT=/workspace/agentctl
FIXTURES=/tmp/agentctl-fixtures
SOURCE_REPO="$FIXTURES/repos/example-source"
CATALOG="$FIXTURES/catalogs/clean-room.toml"
RENDER_OUT="$FIXTURES/rendered"

mkdir -p "$HOME" "$FIXTURES/repos" "$FIXTURES/catalogs"

git init "$SOURCE_REPO" >/dev/null
git -C "$SOURCE_REPO" config user.name "Agentctl Test"
git -C "$SOURCE_REPO" config user.email "agentctl@example.test"
printf "fixture repo\n" > "$SOURCE_REPO/README.md"
git -C "$SOURCE_REPO" add README.md
git -C "$SOURCE_REPO" commit -m "Initial fixture repo" >/dev/null
git -C "$SOURCE_REPO" branch -M main

cat > "$CATALOG" <<CATALOG_TOML
version = 1

[target_groups]
all-agent-boxes = "All clean-room agent boxes."
coordinator = "Coordinator box."

[[hosts]]
id = "clean-room"
hostname = "clean-room"
target_groups = ["all-agent-boxes", "coordinator"]

[[agent_runtimes]]
id = "codex"
type = "codex"
status = "active"
targets = ["all-agent-boxes"]

[[agent_runtimes]]
id = "claude"
type = "claude"
status = "active"
targets = ["all-agent-boxes"]

[[repos]]
id = "fixture-repo"
remote = "file://$SOURCE_REPO"
path = "~/dev/fixture-repo"
branch = "main"
update_policy = "clone-only"
targets = ["all-agent-boxes"]

[[integrations]]
id = "playwright"
type = "mcp_stdio"
status = "active"
command = "npx"
args = ["-y", "@playwright/mcp"]
harnesses = ["claude"]
allowed_tools = ["mcp__playwright__*"]
targets = ["all-agent-boxes"]

[[credential_probes]]
id = "home-exists"
type = "path_exists"
path = "$HOME"
targets = ["all-agent-boxes"]

[[state_stores]]
id = "local-queue"
type = "sqlite"
path = "~/.local/share/agentctl/queues/local.sqlite3"
targets = ["coordinator"]

[[agent_workloads]]
id = "daily-clean-room"
name = "Daily Clean Room"
owner = "agentctl:test"
kind = "schedule"
status = "active"
schedule = "FREQ=DAILY;BYHOUR=6;BYMINUTE=15;BYSECOND=0"
targets = ["all-agent-boxes"]
harnesses = ["codex"]
prompt = "Run the clean-room scheduled workload."
model = "gpt-5.4"
reasoning = "low"
execution_environment = "local"

[[agent_workloads]]
id = "queue-worker"
name = "Queue Worker"
owner = "agentctl:test"
kind = "queue"
status = "active"
targets = ["coordinator"]
command = "/usr/local/bin/agent-worker"
args = ["--queue", "local-queue"]
state_store_refs = ["local-queue"]
restart_policy = "always"
log_path = "~/.local/state/agentctl/queue-worker.log"

[[aux_services]]
id = "local-mcp-gateway"
type = "mcp_gateway"
status = "active"
targets = ["coordinator"]
command = "/usr/local/bin/mcp-gateway"
args = ["serve"]
restart_policy = "always"
CATALOG_TOML

cd "$ROOT"
go test ./...
go vet ./...

agentctl check -f "$CATALOG" --host clean-room
agentctl plan -f "$CATALOG" --host clean-room
agentctl render -f "$CATALOG" --host clean-room --out "$RENDER_OUT"

test -f "$RENDER_OUT/codex/automations/daily-clean-room/automation.toml"
test -f "$RENDER_OUT/claude/agentctl.mcp.json"
test -f "$RENDER_OUT/launchd/dev.agentctl.workload.queue-worker.plist"
test -f "$RENDER_OUT/launchd/dev.agentctl.service.local-mcp-gateway.plist"

agentctl apply -f "$CATALOG" --host clean-room

test -d "$HOME/dev/fixture-repo/.git"
test -f "$CODEX_HOME/automations/daily-clean-room/automation.toml"
test -f "$HOME/.claude/agentctl.mcp.json"
test -f "$HOME/Library/LaunchAgents/dev.agentctl.workload.queue-worker.plist"
test -f "$HOME/Library/LaunchAgents/dev.agentctl.service.local-mcp-gateway.plist"

echo "clean-room validation ok"
