# Agentctl Architecture

`agentctl` reconciles concrete local agent-box state from harness-neutral TOML
catalogs. It is not a hosted control plane and it is not Terraform for SaaS
objects. It is closer to a local desired-state engine for agent runtimes,
integrations, workloads, repos, and service wrappers.

## Boundaries

- Product code and schemas live in this repo.
- Private host and workload catalogs live outside this repo.
- V1 mutates local machine state only.
- External systems are verified through read-only probes.
- SSH dispatch uses the user's existing SSH config and agent.

## Resources

| Resource | Purpose |
| --- | --- |
| `host` | Machine inventory, target groups, roles, SSH aliases. |
| `repo` | Managed git checkout and update policy. |
| `credential_source` | Secretless pointer to bootstrap credentials supplied outside the catalog. |
| `agent_runtime` | Codex, Claude, or another harness runtime expectation. |
| `harness_config` | Low-level harness settings such as hooks, trust, approval defaults, model defaults, enabled plugins. |
| `harness_extension` | Installed plugin, adapter, wrapper, or package that extends a harness. |
| `skill_pack` | Skill source, install target, compatibility aliases. |
| `integration` | Callable capability agents use: MCP, CLI, HTTP, hosted connector, browser/devtools, DWH wrapper. |
| `agent_workload` | Runnable agent work: schedule, daemon, or queue. |
| `aux_service` | Supporting long-lived service: MCP gateway, local API, bot relay, helper daemon. |
| `state_store` | Local or external state used by workloads. |
| `data_source` | Business/data surface reached by an integration. |
| `credential_probe` | Read-only proof that auth, session, or permissions work. |

## Workloads

`agent_workload` replaces the durable schema role previously held by
"automation". It supports:

- `schedule`: cron/RRULE-style scheduled agent work.
- `daemon`: long-lived agent process.
- `queue`: long-lived or repeated agent process consuming a queue.

Workloads declare what runs using `command`, `args`, `cwd`, `env_refs`, and logs,
with optional agent metadata such as `harness`, `model`, `reasoning`, `prompt`,
`prompt_file`, and `skill`.

Queue workloads must reference a `state_store` or `integration` so the backing
queue is explicit.

## MCP

MCP is treated as a protocol/interface boundary, not a primary resource.
`agentctl` models the concrete things it can reconcile:

- the installed server/adapter as a `harness_extension` or `integration`
- the harness registration rendered from `integration`
- the long-running gateway as an `aux_service`, when applicable
- the auth/session check as a `credential_probe`
- the business surface as a `data_source`

V1 has built-in integration types for `mcp_stdio`, `mcp_http`, `mcp_sse`, and
`mcp_hosted_connector`.

## Bootstrap Credentials

`agentctl` follows the Terraform-style boundary for credentials: the catalog can
declare where a credential should come from, but it does not contain the secret
value. For V1, `credential_source` supports `type = "github_token_env"` so a
repo can declare `auth_ref = "github-agent-repos"` and receive an HTTPS GitHub
token through a scoped environment variable during clone or pull.

The token is passed to `git` with a temporary askpass helper and is never written
into repo remotes or rendered config. Missing bootstrap credentials show up in
`plan` as high-risk checks and fail `apply` with an explicit error before
interactive Git prompts can hang a remote run.

## Render Targets

V1 renderers:

- Codex scheduled workload TOML
- Claude MCP config JSON
- macOS launchd plist wrappers for daemon/queue workloads and auxiliary services

Linux/systemd appears in the renderer interface and schema language, but V1
apply intentionally reports it as unsupported.
