#!/usr/bin/env bash
set -euo pipefail

export HOME=/root

mkdir -p "$HOME/.ssh"
cp /ssh/id_ed25519 "$HOME/.ssh/id_ed25519"
chmod 600 "$HOME/.ssh/id_ed25519"

cat > "$HOME/.ssh/config" <<'SSH_CONFIG'
Host worker
  HostName worker
  User root
  IdentityFile /root/.ssh/id_ed25519
  StrictHostKeyChecking no
  UserKnownHostsFile /dev/null
SSH_CONFIG
chmod 600 "$HOME/.ssh/config"

cat > /tmp/remote-hosts.toml <<'HOSTS_TOML'
version = 1

[target_groups]
all-agent-boxes = "All remote test boxes."

[[hosts]]
id = "worker"
hostname = "worker"
ssh_alias = "worker"
target_groups = ["all-agent-boxes"]
HOSTS_TOML

for _ in $(seq 1 20); do
  if ssh worker true 2>/dev/null; then
    break
  fi
  sleep 1
done

agentctl remote -f /tmp/remote-hosts.toml --remote-path /workspace/agentctl worker check -f /workspace/agentctl/examples/agentctl.toml

echo "remote ssh validation ok"
