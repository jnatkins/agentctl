package catalog

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

type Diagnostic struct {
	Severity string `json:"severity"`
	Resource string `json:"resource,omitempty"`
	ID       string `json:"id,omitempty"`
	Message  string `json:"message"`
}

type ValidationResult struct {
	Diagnostics []Diagnostic `json:"diagnostics"`
}

func (r ValidationResult) HasErrors() bool {
	for _, d := range r.Diagnostics {
		if d.Severity == "error" {
			return true
		}
	}
	return false
}

func (r *ValidationResult) err(resource, id, msg string) {
	r.Diagnostics = append(r.Diagnostics, Diagnostic{Severity: "error", Resource: resource, ID: id, Message: msg})
}

func (r *ValidationResult) warn(resource, id, msg string) {
	r.Diagnostics = append(r.Diagnostics, Diagnostic{Severity: "warning", Resource: resource, ID: id, Message: msg})
}

var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._:-]*[a-z0-9]$|^[a-z0-9]$`)

var builtinHarnesses = map[string]bool{
	"codex":    true,
	"claude":   true,
	"opencode": true,
}

var allowedWorkloadKinds = set("schedule", "daemon", "queue")
var allowedStatuses = set("", "active", "planned", "disabled", "deprecated")
var allowedRepoPolicies = set("", "check-only", "fast-forward-only", "clone-only", "manual")
var allowedCredentialSourceTypes = set("github_token_env")
var allowedIntegrationTypes = set("mcp_stdio", "mcp_http", "mcp_sse", "mcp_hosted_connector", "cli", "http", "browser_devtools", "dwh_wrapper")
var allowedAuxTypes = set("mcp_gateway", "http_service", "bot_relay", "scheduler", "helper_daemon")
var allowedStateTypes = set("file", "directory", "sqlite", "queue", "external")
var allowedDataTypes = set("slack", "gmail", "google_calendar", "google_drive", "notion", "clickhouse", "filesystem", "web", "github", "queue", "browser", "custom")
var allowedProbeTypes = set("fake", "command", "file_exists", "path_exists", "env", "connector", "http")
var allowedRuntimeTypes = set("", "codex", "claude", "opencode", "custom")

func set(values ...string) map[string]bool {
	m := make(map[string]bool, len(values))
	for _, v := range values {
		m[v] = true
	}
	return m
}

func Validate(c *Catalog) ValidationResult {
	var r ValidationResult
	if c.Version != 1 {
		r.err("catalog", "", fmt.Sprintf("unsupported version %d", c.Version))
	}
	for group := range c.TargetGroups {
		if !validID(group) {
			r.err("target_group", group, "target group id must use lowercase letters, digits, '.', ':', '_' or '-'")
		}
	}

	hostIDs := collect("host", c.Hosts, func(h Host) string { return h.ID }, &r)
	repoIDs := collect("repo", c.Repos, func(x Repo) string { return x.ID }, &r)
	credentialSourceIDs := collect("credential_source", c.CredentialSources, func(x CredentialSource) string { return x.ID }, &r)
	runtimeIDs := collect("agent_runtime", c.AgentRuntimes, func(x AgentRuntime) string { return x.ID }, &r)
	extensionIDs := collect("harness_extension", c.HarnessExtensions, func(x HarnessExtension) string { return x.ID }, &r)
	integrationIDs := collect("integration", c.Integrations, func(x Integration) string { return x.ID }, &r)
	auxIDs := collect("aux_service", c.AuxServices, func(x AuxService) string { return x.ID }, &r)
	stateIDs := collect("state_store", c.StateStores, func(x StateStore) string { return x.ID }, &r)
	dataIDs := collect("data_source", c.DataSources, func(x DataSource) string { return x.ID }, &r)
	probeIDs := collect("credential_probe", c.CredentialProbes, func(x CredentialProbe) string { return x.ID }, &r)
	_ = repoIDs
	credentialSources := make(map[string]CredentialSource, len(c.CredentialSources))
	for _, source := range c.CredentialSources {
		credentialSources[source.ID] = source
	}

	for _, h := range c.Hosts {
		if h.Hostname == "" {
			r.err("host", h.ID, "hostname is required")
		}
		for _, group := range h.TargetGroups {
			if !knownTarget(c, hostIDs, group) {
				r.err("host", h.ID, fmt.Sprintf("unknown target group %q", group))
			}
		}
	}
	for _, repo := range c.Repos {
		if repo.Remote == "" {
			r.err("repo", repo.ID, "remote is required")
		}
		if repo.Path == "" {
			r.err("repo", repo.ID, "path is required")
		}
		checkAllowed(&r, "repo", repo.ID, "update_policy", repo.UpdatePolicy, allowedRepoPolicies)
		checkRef(&r, "repo", repo.ID, "auth_ref", repo.AuthRef, credentialSourceIDs)
		if repo.AuthRef != "" {
			source := credentialSources[repo.AuthRef]
			if source.Type == "github_token_env" && !strings.HasPrefix(repo.Remote, "https://") {
				r.err("repo", repo.ID, "github_token_env auth_ref requires an https remote")
			}
		}
		checkTargets(&r, c, hostIDs, "repo", repo.ID, repo.Targets)
	}
	for _, source := range c.CredentialSources {
		checkAllowed(&r, "credential_source", source.ID, "type", source.Type, allowedCredentialSourceTypes)
		checkAllowed(&r, "credential_source", source.ID, "status", source.Status, allowedStatuses)
		if source.Env == "" {
			r.err("credential_source", source.ID, "env is required")
		}
		checkTargets(&r, c, hostIDs, "credential_source", source.ID, source.Targets)
	}
	for _, rt := range c.AgentRuntimes {
		checkAllowed(&r, "agent_runtime", rt.ID, "type", rt.Type, allowedRuntimeTypes)
		checkAllowed(&r, "agent_runtime", rt.ID, "status", rt.Status, allowedStatuses)
		checkTargets(&r, c, hostIDs, "agent_runtime", rt.ID, rt.Targets)
	}
	for _, cfg := range c.HarnessConfigs {
		checkAllowed(&r, "harness_config", cfg.ID, "status", cfg.Status, allowedStatuses)
		checkHarness(&r, runtimeIDs, "harness_config", cfg.ID, cfg.Harness)
		checkTargets(&r, c, hostIDs, "harness_config", cfg.ID, cfg.Targets)
	}
	for _, ext := range c.HarnessExtensions {
		checkAllowed(&r, "harness_extension", ext.ID, "status", ext.Status, allowedStatuses)
		for _, h := range ext.Harnesses {
			checkHarness(&r, runtimeIDs, "harness_extension", ext.ID, h)
		}
		checkTargets(&r, c, hostIDs, "harness_extension", ext.ID, ext.Targets)
	}
	for _, skill := range c.SkillPacks {
		for _, h := range skill.Harnesses {
			checkHarness(&r, runtimeIDs, "skill_pack", skill.ID, h)
		}
		if skill.Source == "" {
			r.err("skill_pack", skill.ID, "source is required")
		}
		checkTargets(&r, c, hostIDs, "skill_pack", skill.ID, skill.Targets)
	}
	for _, in := range c.Integrations {
		checkAllowed(&r, "integration", in.ID, "type", in.Type, allowedIntegrationTypes)
		checkAllowed(&r, "integration", in.ID, "status", in.Status, allowedStatuses)
		for _, h := range in.Harnesses {
			checkHarness(&r, runtimeIDs, "integration", in.ID, h)
		}
		if strings.HasPrefix(in.Type, "mcp_") {
			switch in.Type {
			case "mcp_stdio":
				if in.Command == "" {
					r.err("integration", in.ID, "mcp_stdio integration requires command")
				}
			case "mcp_http", "mcp_sse", "mcp_hosted_connector":
				if in.URL == "" && in.Provider == "" {
					r.err("integration", in.ID, in.Type+" integration requires url or provider")
				}
			}
		}
		checkRef(&r, "integration", in.ID, "extension_ref", in.ExtensionRef, extensionIDs)
		checkRef(&r, "integration", in.ID, "aux_service_ref", in.AuxServiceRef, auxIDs)
		checkRefs(&r, "integration", in.ID, "data_source_refs", in.DataSourceRefs, dataIDs)
		checkRefs(&r, "integration", in.ID, "credential_probe_refs", in.CredentialProbeRefs, probeIDs)
		checkTargets(&r, c, hostIDs, "integration", in.ID, in.Targets)
	}
	for _, w := range c.AgentWorkloads {
		if w.Owner == "" {
			r.err("agent_workload", w.ID, "owner is required")
		}
		checkAllowed(&r, "agent_workload", w.ID, "kind", w.Kind, allowedWorkloadKinds)
		checkAllowed(&r, "agent_workload", w.ID, "status", w.Status, allowedStatuses)
		for _, h := range w.Harnesses {
			checkHarness(&r, runtimeIDs, "agent_workload", w.ID, h)
		}
		if w.Kind == "schedule" && w.Schedule == "" {
			r.err("agent_workload", w.ID, "schedule workload requires schedule")
		}
		if (w.Kind == "daemon" || w.Kind == "queue") && w.Command == "" {
			r.err("agent_workload", w.ID, "daemon and queue workloads require command")
		}
		if w.Kind == "queue" && len(w.StateStoreRefs) == 0 && len(w.IntegrationRefs) == 0 {
			r.err("agent_workload", w.ID, "queue workload requires state_store_refs or integration_refs")
		}
		if w.Kind == "schedule" && w.Command == "" && w.Prompt == "" && w.PromptFile == "" {
			r.err("agent_workload", w.ID, "schedule workload requires command, prompt, or prompt_file")
		}
		checkRefs(&r, "agent_workload", w.ID, "integration_refs", w.IntegrationRefs, integrationIDs)
		checkRefs(&r, "agent_workload", w.ID, "state_store_refs", w.StateStoreRefs, stateIDs)
		checkTargets(&r, c, hostIDs, "agent_workload", w.ID, w.Targets)
	}
	for _, svc := range c.AuxServices {
		checkAllowed(&r, "aux_service", svc.ID, "type", svc.Type, allowedAuxTypes)
		checkAllowed(&r, "aux_service", svc.ID, "status", svc.Status, allowedStatuses)
		if svc.Status != "disabled" && svc.Command == "" && svc.URL == "" {
			r.err("aux_service", svc.ID, "active/planned service requires command or url")
		}
		checkRefs(&r, "aux_service", svc.ID, "state_store_refs", svc.StateStoreRefs, stateIDs)
		checkTargets(&r, c, hostIDs, "aux_service", svc.ID, svc.Targets)
	}
	for _, ss := range c.StateStores {
		checkAllowed(&r, "state_store", ss.ID, "type", ss.Type, allowedStateTypes)
		checkAllowed(&r, "state_store", ss.ID, "status", ss.Status, allowedStatuses)
		if ss.Type != "external" && ss.Path == "" {
			r.err("state_store", ss.ID, "non-external state_store requires path")
		}
		checkTargets(&r, c, hostIDs, "state_store", ss.ID, ss.Targets)
	}
	for _, ds := range c.DataSources {
		checkAllowed(&r, "data_source", ds.ID, "type", ds.Type, allowedDataTypes)
		checkAllowed(&r, "data_source", ds.ID, "status", ds.Status, allowedStatuses)
		checkTargets(&r, c, hostIDs, "data_source", ds.ID, ds.Targets)
	}
	for _, probe := range c.CredentialProbes {
		checkAllowed(&r, "credential_probe", probe.ID, "type", probe.Type, allowedProbeTypes)
		checkAllowed(&r, "credential_probe", probe.ID, "status", probe.Status, allowedStatuses)
		if probe.Type == "command" && probe.Command == "" {
			r.err("credential_probe", probe.ID, "command probe requires command")
		}
		if (probe.Type == "file_exists" || probe.Type == "path_exists") && probe.Path == "" {
			r.err("credential_probe", probe.ID, probe.Type+" probe requires path")
		}
		if probe.Type == "env" && probe.Env == "" {
			r.err("credential_probe", probe.ID, "env probe requires env")
		}
		checkRef(&r, "credential_probe", probe.ID, "integration_ref", probe.IntegrationRef, integrationIDs)
		checkRef(&r, "credential_probe", probe.ID, "data_source_ref", probe.DataSourceRef, dataIDs)
		checkTargets(&r, c, hostIDs, "credential_probe", probe.ID, probe.Targets)
	}
	sortDiagnostics(r.Diagnostics)
	return r
}

func collect[T any](resource string, items []T, idFn func(T) string, r *ValidationResult) map[string]bool {
	ids := make(map[string]bool, len(items))
	for _, item := range items {
		id := idFn(item)
		if id == "" {
			r.err(resource, "", "id is required")
			continue
		}
		if !validID(id) {
			r.err(resource, id, "id must use lowercase letters, digits, '.', ':', '_' or '-'")
		}
		if ids[id] {
			r.err(resource, id, "duplicate id")
		}
		ids[id] = true
	}
	return ids
}

func validID(id string) bool {
	return idPattern.MatchString(id)
}

func checkAllowed(r *ValidationResult, resource, id, field, value string, allowed map[string]bool) {
	if !allowed[value] {
		var values []string
		for v := range allowed {
			if v != "" {
				values = append(values, v)
			}
		}
		sort.Strings(values)
		r.err(resource, id, fmt.Sprintf("unsupported %s %q; allowed: %s", field, value, strings.Join(values, ", ")))
	}
}

func checkHarness(r *ValidationResult, runtimeIDs map[string]bool, resource, id, harness string) {
	if harness == "" {
		r.err(resource, id, "harness reference cannot be empty")
		return
	}
	if runtimeIDs[harness] || builtinHarnesses[harness] {
		return
	}
	r.err(resource, id, fmt.Sprintf("unknown harness %q", harness))
}

func checkTargets(r *ValidationResult, c *Catalog, hostIDs map[string]bool, resource, id string, targets []string) {
	for _, target := range targets {
		if !knownTarget(c, hostIDs, target) {
			r.err(resource, id, fmt.Sprintf("unknown target %q", target))
		}
	}
}

func knownTarget(c *Catalog, hostIDs map[string]bool, target string) bool {
	if target == "" {
		return false
	}
	_, isGroup := c.TargetGroups[target]
	return isGroup || hostIDs[target]
}

func checkRef(r *ValidationResult, resource, id, field, ref string, ids map[string]bool) {
	if ref == "" {
		return
	}
	if !ids[ref] {
		r.err(resource, id, fmt.Sprintf("%s references unknown id %q", field, ref))
	}
}

func checkRefs(r *ValidationResult, resource, id, field string, refs []string, ids map[string]bool) {
	for _, ref := range refs {
		checkRef(r, resource, id, field, ref, ids)
	}
}

func sortDiagnostics(ds []Diagnostic) {
	sort.Slice(ds, func(i, j int) bool {
		a, b := ds[i], ds[j]
		if a.Severity != b.Severity {
			return a.Severity < b.Severity
		}
		if a.Resource != b.Resource {
			return a.Resource < b.Resource
		}
		if a.ID != b.ID {
			return a.ID < b.ID
		}
		return a.Message < b.Message
	})
}
