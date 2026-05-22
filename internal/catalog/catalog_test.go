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
