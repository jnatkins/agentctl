package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jnatkins/agentctl/internal/catalog"
	"github.com/jnatkins/agentctl/internal/target"
)

func TestCodexRenderLegacyAutomation(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "prompt.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "automations.toml")
	if err := os.WriteFile(path, []byte(`
version = 1

[target_groups]
all = "all"

[[hosts]]
id = "local"
hostname = "localhost"
target_groups = ["all"]

[[automations]]
id = "daily"
name = "Daily"
owner = "ops"
kind = "cron"
status = "active"
schedule = "FREQ=DAILY"
targets = ["all"]
harnesses = ["codex"]
prompt_file = "prompt.md"
model = "gpt-5.4"
reasoning = "low"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	c, _, err := catalog.Load([]string{path})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	files, err := Codex(c, Options{Selector: target.Selector{TargetGroups: []string{"all"}}, OutputDir: "/tmp/out"})
	if err != nil {
		t.Fatalf("Codex() error = %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("files len = %d", len(files))
	}
	got := string(files[0].Content)
	for _, want := range []string{`version = 1`, `id = "daily"`, `kind = "cron"`, `rrule = "FREQ=DAILY"`, `model = "gpt-5.4"`, `reasoning_effort = "low"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("render missing %q:\n%s", want, got)
		}
	}
}

func TestClaudeMCPRender(t *testing.T) {
	c := &catalog.Catalog{
		Version:      1,
		TargetGroups: map[string]string{"all": "all"},
		Hosts:        []catalog.Host{{ID: "local", Hostname: "localhost", TargetGroups: []string{"all"}}},
		Integrations: []catalog.Integration{{
			ID: "playwright", Type: "mcp_stdio", Command: "npx", Args: []string{"-y", "@playwright/mcp"},
			Harnesses: []string{"claude"}, AllowedTools: []string{"mcp__playwright__*"}, Targets: []string{"all"},
		}},
	}
	files, err := Claude(c, Options{Selector: target.Selector{TargetGroups: []string{"all"}}, OutputDir: "/tmp/out"})
	if err != nil {
		t.Fatalf("Claude() error = %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("files len = %d", len(files))
	}
	got := string(files[0].Content)
	for _, want := range []string{`"mcpServers"`, `"playwright"`, `"command": "npx"`, `"mcp__playwright__*"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("render missing %q:\n%s", want, got)
		}
	}
}

func TestLaunchdRenderQueueWorkload(t *testing.T) {
	c := &catalog.Catalog{
		Version:      1,
		TargetGroups: map[string]string{"all": "all"},
		Hosts:        []catalog.Host{{ID: "local", Hostname: "localhost", TargetGroups: []string{"all"}}},
		AgentWorkloads: []catalog.AgentWorkload{{
			ID: "queue-worker", Owner: "ops", Kind: "queue", Status: "active", Targets: []string{"all"},
			Command: "/usr/local/bin/worker", Args: []string{"--queue", "main"}, StateStoreRefs: []string{"queue"},
		}},
	}
	files, err := Launchd(c, Options{Selector: target.Selector{TargetGroups: []string{"all"}}, OutputDir: "/tmp/out"})
	if err != nil {
		t.Fatalf("Launchd() error = %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("files len = %d", len(files))
	}
	got := string(files[0].Content)
	for _, want := range []string{"dev.agentctl.workload.queue-worker", "/usr/local/bin/worker", "--queue", "KeepAlive"} {
		if !strings.Contains(got, want) {
			t.Fatalf("render missing %q:\n%s", want, got)
		}
	}
}

func TestLaunchdSkipsPlannedServices(t *testing.T) {
	c := &catalog.Catalog{
		Version:      1,
		TargetGroups: map[string]string{"all": "all"},
		Hosts:        []catalog.Host{{ID: "local", Hostname: "localhost", TargetGroups: []string{"all"}}},
		AuxServices: []catalog.AuxService{{
			ID: "planned", Type: "mcp_gateway", Status: "planned", Targets: []string{"all"}, Command: "/usr/local/bin/planned",
		}},
	}
	files, err := Launchd(c, Options{Selector: target.Selector{TargetGroups: []string{"all"}}, OutputDir: "/tmp/out"})
	if err != nil {
		t.Fatalf("Launchd() error = %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("files len = %d, want 0", len(files))
	}
}

func TestLaunchdRenderStartInterval(t *testing.T) {
	c := &catalog.Catalog{
		AuxServices: []catalog.AuxService{{
			ID: "shmem-drain", Type: "launchd_schedule", Status: "active",
			Command: "~/x/run.sh", Schedule: "StartInterval=300",
			RestartPolicy: "on_interval", Targets: []string{"all"},
		}},
	}
	files, err := Launchd(c, Options{Selector: target.Selector{TargetGroups: []string{"all"}}, OutputDir: "/tmp/out"})
	if err != nil {
		t.Fatalf("Launchd() error = %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("want 1 file, got %d", len(files))
	}
	s := string(files[0].Content)
	if !strings.Contains(s, "<key>StartInterval</key>") || !strings.Contains(s, "<integer>300</integer>") {
		t.Fatalf("missing StartInterval:\n%s", s)
	}
	if !strings.Contains(s, "<key>KeepAlive</key>\n  <false/>") {
		t.Fatalf("scheduled service must set KeepAlive false:\n%s", s)
	}
}
