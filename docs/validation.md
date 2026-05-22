# Validation

`agentctl` has two Docker-backed validation paths plus the normal Go test
suite. Docker is useful for clean-room and coordinator/worker checks, but it
does not replace a real macOS canary for launchd, Codex/Claude runtime behavior,
browser access, 1Password, or OAuth-backed connectors.

## Local Unit Checks

```sh
go test ./...
go vet ./...
```

## Clean-Room Container

```sh
./test/scripts/validate-clean-room.sh
```

The scripts expect a working Docker-compatible CLI. Docker Desktop works when
its socket is available to the shell; Colima also works:

```sh
brew install docker docker-compose colima
colima start
```

This builds the test image, creates a fresh fake home, creates a local git
fixture repo, writes a TOML catalog inside the container, then runs:

- `agentctl check`
- `agentctl plan`
- `agentctl render`
- `agentctl apply`
- post-apply assertions for cloned repos, Codex config, Claude MCP config, and
  launchd plist output under the fake home

## Remote SSH Container

```sh
./test/scripts/validate-remote-ssh.sh
```

This starts a worker container running `sshd`, starts a controller container
with an SSH config pointing at that worker, then runs:

```sh
agentctl remote worker check -f /workspace/agentctl/examples/agentctl.toml
```

The test validates the V1 coordinator model without requiring a real fleet.

## Real macOS Canary

The Docker checks intentionally do not claim to validate macOS integration. A
real canary should run on a fresh macOS account or spare Mac with:

```sh
agentctl check
agentctl plan
agentctl apply
```

using a private desired-state catalog. That is where launchd loading, actual
Codex/Claude config behavior, local OAuth sessions, 1Password-backed wrappers,
and browser/devtools integrations are validated.
