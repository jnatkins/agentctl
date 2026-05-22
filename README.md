# agentctl

`agentctl` is a harness-neutral control tool for declaring, checking, planning,
rendering, and applying local agent-box desired state.

The project is intentionally split from any private machine inventory. The CLI
and schemas live here; user- or fleet-specific catalogs should live in a
separate repo such as `agent-workbench`.

## Status

This is a V1 foundation. It is macOS-first, Linux-shaped, and local-state-only:

- validates strict TOML catalogs
- models agent workloads, integrations, skills, repos, services, state stores,
  data sources, and credential probes
- renders Codex scheduled workloads, Claude MCP config, and launchd service
  wrappers
- plans local changes before applying them
- applies local repo/config/service-file changes only
- dispatches read-only or mutating subcommands over existing SSH config

External SaaS systems are check-only in V1. `agentctl` does not create Slack
apps, Notion databases, hosted connectors, OAuth clients, SSH keys, or tailnet
inventory.

## Install From Source

```sh
go install github.com/jnatkins/agentctl/cmd/agentctl@latest
```

During local development:

```sh
go test ./...
go run ./cmd/agentctl check -f examples/agentctl.toml
go run ./cmd/agentctl plan -f examples/agentctl.toml
go run ./cmd/agentctl render -f examples/agentctl.toml --out /tmp/agentctl-render
```

## Commands

```text
agentctl check   - validate catalogs and run safe local/read-only checks
agentctl plan    - show intended local changes without mutating state
agentctl render  - write rendered harness/service config to an output directory
agentctl apply   - apply local repo/config/service-file changes
agentctl remote  - dispatch check/plan/render/apply to hosts over SSH
```

Global flags:

```text
-f, --catalog PATH      TOML catalog path; repeatable
--host ID              host id/hostname/alias to target
--target-group NAME    target group to target; repeatable
--format text|json     output format for check/plan
```

If no catalog is supplied, `agentctl` looks for `agentctl.toml` and `state/*.toml`
under the current directory.

## Resource Model

`agent_workload` is the primary resource for runnable agentic work. Scheduled
automations are represented as `kind = "schedule"` workloads; long-lived agents
use `kind = "daemon"` or `kind = "queue"`.

MCP is not a top-level resource. It is modeled as typed integrations and, when
needed, supporting services:

- hosted connector MCP: `integration` + `credential_probe` + `data_source`
- local stdio MCP: `integration` with `type = "mcp_stdio"`
- local MCP gateway: `aux_service` with `type = "mcp_gateway"` plus an
  `integration`
- harness registration: rendered from `integration` into Codex or Claude config

See [docs/architecture.md](docs/architecture.md) and
[examples/agentctl.toml](examples/agentctl.toml).
