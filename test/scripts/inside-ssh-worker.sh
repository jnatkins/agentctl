#!/usr/bin/env bash
set -euo pipefail

mkdir -p /run/sshd /root/.ssh
cp /ssh/id_ed25519.pub /root/.ssh/authorized_keys
chmod 700 /root/.ssh
chmod 600 /root/.ssh/authorized_keys
ssh-keygen -A

exec /usr/sbin/sshd -D -e
