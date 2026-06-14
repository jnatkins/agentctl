package catalog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadAndValidateExample(t *testing.T) {
	c, warnings, err := Load([]string{filepath.Join("..", "..", "examples", "agentctl.toml")})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v", warnings)
	}
	result := Validate(c)
	if result.HasErrors() {
		t.Fatalf("validation errors: %#v", result.Diagnostics)
	}
}

func TestStrictUnknownKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agentctl.toml")
	if err := os.WriteFile(path, []byte(`
version = 1
unknown = true
`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := Load([]string{path})
	if err == nil || !strings.Contains(err.Error(), "unknown keys") {
		t.Fatalf("expected unknown key error, got %v", err)
	}
}

func TestAuxServiceTemplateFields(t *testing.T) {
	// Regression: the live catalog registers http daemons + launchd_schedule jobs
	// with plist_template / port / log_dir. Before these struct fields existed the
	// strict loader rejected the whole catalog with "unknown keys", breaking
	// agentctl check/plan/apply on every box.
	dir := t.TempDir()
	path := filepath.Join(dir, "agentctl.toml")
	if err := os.WriteFile(path, []byte(`
version = 1

[[aux_services]]
id = "fsd-monitor"
type = "http"
status = "active"
command = "~/dev/agent-aux/services/fsd-monitor/run.sh"
plist_template = "~/dev/agent-aux/infra/launchd/com.natty.fsd-monitor.plist.template"
port = 8767
log_dir = "~/Library/Logs/agent-aux"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	c, _, err := Load([]string{path})
	if err != nil {
		t.Fatalf("Load() error = %v (plist_template/port/log_dir must be accepted)", err)
	}
	if len(c.AuxServices) != 1 {
		t.Fatalf("expected 1 aux_service, got %d", len(c.AuxServices))
	}
	svc := c.AuxServices[0]
	if svc.PlistTemplate == "" || svc.Port != 8767 || svc.LogDir == "" {
		t.Fatalf("template fields not parsed: PlistTemplate=%q Port=%d LogDir=%q",
			svc.PlistTemplate, svc.Port, svc.LogDir)
	}
}

func TestDiscoverCatalogPrefersAgentctlRoot(t *testing.T) {
	dir := t.TempDir()
	state := filepath.Join(dir, "state")
	if err := os.MkdirAll(state, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"agentctl.toml":    "version = 1\n",
		"automations.toml": "version = 1\n",
		"repos.toml":       "legacy = true\n",
	} {
		if err := os.WriteFile(filepath.Join(state, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	paths, err := DiscoverCatalogs(dir)
	if err != nil {
		t.Fatalf("DiscoverCatalogs() error = %v", err)
	}
	wantAgentctl := filepath.Join(state, "agentctl.toml")
	wantAutomations := filepath.Join(state, "automations.toml")
	if len(paths) != 2 || paths[0] != wantAgentctl || paths[1] != wantAutomations {
		t.Fatalf("paths = %#v, want [%q %q]", paths, wantAgentctl, wantAutomations)
	}
}

func TestLegacyAutomationsBecomeWorkloads(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "prompt.md"), []byte("do work"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "automations.toml")
	if err := os.WriteFile(path, []byte(`
version = 1

[target_groups]
all = "all boxes"

[[hosts]]
id = "local"
hostname = "localhost"
target_groups = ["all"]

[[automations]]
id = "daily-work"
name = "Daily Work"
owner = "ops"
kind = "cron"
status = "active"
schedule = "FREQ=DAILY"
targets = ["all"]
harnesses = ["codex"]
prompt_file = "prompt.md"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	c, warnings, err := Load([]string{path})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings len = %d", len(warnings))
	}
	if len(c.AgentWorkloads) != 1 {
		t.Fatalf("workloads len = %d", len(c.AgentWorkloads))
	}
	if got := c.AgentWorkloads[0].Kind; got != "schedule" {
		t.Fatalf("kind = %q", got)
	}
	result := Validate(c)
	if result.HasErrors() {
		t.Fatalf("validation errors: %#v", result.Diagnostics)
	}
}

func TestLegacyHeartbeatAutomationBecomesSchedule(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "automations.toml")
	if err := os.WriteFile(path, []byte(`
version = 1

[target_groups]
all = "all boxes"

[[automations]]
id = "attended"
owner = "ops"
kind = "heartbeat"
status = "active"
schedule = "FREQ=WEEKLY"
targets = ["all"]
harnesses = ["codex"]
prompt = "ask the user"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	c, _, err := Load([]string{path})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got := c.AgentWorkloads[0].Kind; got != "schedule" {
		t.Fatalf("kind = %q", got)
	}
	if got := c.AgentWorkloads[0].CodexKind; got != "heartbeat" {
		t.Fatalf("codex kind = %q", got)
	}
	result := Validate(c)
	if result.HasErrors() {
		t.Fatalf("validation errors: %#v", result.Diagnostics)
	}
}

func TestInvalidReferences(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agentctl.toml")
	if err := os.WriteFile(path, []byte(`
version = 1

[target_groups]
all = "all boxes"

[[agent_workloads]]
id = "worker"
owner = "ops"
kind = "queue"
status = "active"
targets = ["all"]
command = "worker"
integration_refs = ["missing"]
`), 0o644); err != nil {
		t.Fatal(err)
	}
	c, _, err := Load([]string{path})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	result := Validate(c)
	if !result.HasErrors() {
		t.Fatalf("expected validation errors")
	}
	found := false
	for _, d := range result.Diagnostics {
		if strings.Contains(d.Message, "missing") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected missing ref diagnostic, got %#v", result.Diagnostics)
	}
}

func TestRepoCredentialSourceValidation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agentctl.toml")
	if err := os.WriteFile(path, []byte(`
version = 1

[target_groups]
all = "all boxes"

[[hosts]]
id = "local"
hostname = "localhost"
target_groups = ["all"]

[[credential_sources]]
id = "github"
type = "github_token_env"
env = "AGENTCTL_GITHUB_TOKEN"
targets = ["all"]

[[repos]]
id = "private"
remote = "https://github.com/example/private.git"
path = "~/dev/private"
auth_ref = "github"
targets = ["all"]
`), 0o644); err != nil {
		t.Fatal(err)
	}
	c, _, err := Load([]string{path})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	result := Validate(c)
	if result.HasErrors() {
		t.Fatalf("validation errors: %#v", result.Diagnostics)
	}
}

func TestRepoCredentialSourceUnknownRefFails(t *testing.T) {
	c := &Catalog{
		Version:      1,
		TargetGroups: map[string]string{"all": "all"},
		Hosts:        []Host{{ID: "local", Hostname: "localhost", TargetGroups: []string{"all"}}},
		Repos: []Repo{{
			ID: "private", Remote: "https://github.com/example/private.git", Path: "~/dev/private", AuthRef: "missing", Targets: []string{"all"},
		}},
	}
	result := Validate(c)
	if !result.HasErrors() {
		t.Fatalf("expected validation error")
	}
	found := false
	for _, d := range result.Diagnostics {
		if strings.Contains(d.Message, "auth_ref") && strings.Contains(d.Message, "missing") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected unknown auth_ref diagnostic, got %#v", result.Diagnostics)
	}
}

func TestAuxServiceScheduleDecodes(t *testing.T) {
	doc := `
version = 1
[[hosts]]
id = "h"
hostname = "h"
target_groups = ["all"]
[[aux_services]]
id = "shmem-drain"
type = "launchd_schedule"
status = "active"
command = "~/x/run.sh"
schedule = "StartInterval=300"
restart_policy = "on_interval"
targets = ["all"]
`
	dir := t.TempDir()
	p := filepath.Join(dir, "c.toml")
	if err := os.WriteFile(p, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	c, _, err := Load([]string{p})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if c.AuxServices[0].Schedule != "StartInterval=300" {
		t.Fatalf("Schedule = %q, want StartInterval=300", c.AuxServices[0].Schedule)
	}
}

func TestLaunchdScheduleTypeAllowed(t *testing.T) {
	c := &Catalog{
		Version:      1,
		TargetGroups: map[string]string{"all": "test group"},
		Hosts:        []Host{{ID: "h", Hostname: "h", TargetGroups: []string{"all"}}},
		AuxServices:  []AuxService{{ID: "d", Type: "launchd_schedule", Status: "active", Command: "x", Schedule: "StartInterval=300", Targets: []string{"all"}}},
	}
	if r := Validate(c); r.HasErrors() {
		t.Fatalf("unexpected validation errors for launchd_schedule aux type: %+v", r.Diagnostics)
	}
}
