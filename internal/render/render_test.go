package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jnatkins/agentctl/internal/catalog"
	"github.com/jnatkins/agentctl/internal/pathutil"
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

func TestRenderFromTemplateSubstitutes(t *testing.T) {
	tmp := t.TempDir()
	// Build tmp/infra/launchd/com.natty.claude.demo.plist.template
	launchdDir := filepath.Join(tmp, "infra", "launchd")
	if err := os.MkdirAll(launchdDir, 0o755); err != nil {
		t.Fatal(err)
	}
	tmplContent := `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict>
  <key>Label</key><string>com.natty.claude.demo</string>
  <key>ProgramArguments</key><array>
    <string>__REPO_ROOT__/services/x.sh</string>
  </array>
  <key>StandardOutPath</key>
  <string>/Users/__USER__/Library/Logs/x.log</string>
</dict></plist>`
	tmplPath := filepath.Join(launchdDir, "com.natty.claude.demo.plist.template")
	if err := os.WriteFile(tmplPath, []byte(tmplContent), 0o644); err != nil {
		t.Fatal(err)
	}

	c := &catalog.Catalog{
		TargetGroups: map[string]string{"all": "all"},
		Hosts:        []catalog.Host{{ID: "local", Hostname: "localhost", TargetGroups: []string{"all"}}},
		AuxServices: []catalog.AuxService{{
			ID:            "demo",
			Status:        "active",
			Targets:       []string{"all"},
			PlistTemplate: tmplPath,
		}},
	}
	files, err := Launchd(c, Options{Selector: target.Selector{TargetGroups: []string{"all"}}, OutputDir: "/tmp/out"})
	if err != nil {
		t.Fatalf("Launchd() error = %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("want 1 file, got %d", len(files))
	}
	f := files[0]
	if filepath.Base(f.Path) != "com.natty.claude.demo.plist" {
		t.Fatalf("install filename = %q, want com.natty.claude.demo.plist", filepath.Base(f.Path))
	}
	content := string(f.Content)
	if strings.Contains(content, "__REPO_ROOT__") {
		t.Fatalf("__REPO_ROOT__ placeholder not replaced:\n%s", content)
	}
	if strings.Contains(content, "__USER__") {
		t.Fatalf("__USER__ placeholder not replaced:\n%s", content)
	}
	if !strings.Contains(content, tmp+"/services/x.sh") {
		t.Fatalf("repoRoot substitution wrong, want %q in:\n%s", tmp+"/services/x.sh", content)
	}
	user := os.Getenv("USER")
	if user == "" {
		if home, err := os.UserHomeDir(); err == nil {
			user = filepath.Base(home)
		}
	}
	if user == "" {
		t.Fatal("could not determine user for assertion")
	}
	if !strings.Contains(content, "/Users/"+user+"/Library/Logs/x.log") {
		t.Fatalf("user substitution wrong, want /Users/%s/... in:\n%s", user, content)
	}
}

func TestAuxServiceWithTemplatePreferredOverGenerated(t *testing.T) {
	tmp := t.TempDir()
	launchdDir := filepath.Join(tmp, "infra", "launchd")
	if err := os.MkdirAll(launchdDir, 0o755); err != nil {
		t.Fatal(err)
	}
	tmplPath := filepath.Join(launchdDir, "com.natty.claude.mysvc.plist.template")
	if err := os.WriteFile(tmplPath, []byte(`<plist><dict><key>Label</key><string>com.natty.claude.mysvc</string></dict></plist>`), 0o644); err != nil {
		t.Fatal(err)
	}

	c := &catalog.Catalog{
		TargetGroups: map[string]string{"all": "all"},
		Hosts:        []catalog.Host{{ID: "local", Hostname: "localhost", TargetGroups: []string{"all"}}},
		AuxServices: []catalog.AuxService{{
			ID:            "mysvc",
			Status:        "active",
			Targets:       []string{"all"},
			Command:       "/usr/local/bin/mysvc",
			PlistTemplate: tmplPath,
		}},
	}
	files, err := Launchd(c, Options{Selector: target.Selector{TargetGroups: []string{"all"}}, OutputDir: "/tmp/out"})
	if err != nil {
		t.Fatalf("Launchd() error = %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("want exactly 1 file, got %d: %v", len(files), func() []string {
			var names []string
			for _, f := range files {
				names = append(names, filepath.Base(f.Path))
			}
			return names
		}())
	}
	base := filepath.Base(files[0].Path)
	if base != "com.natty.claude.mysvc.plist" {
		t.Fatalf("filename = %q, want com.natty.claude.mysvc.plist (not dev.agentctl.service.*)", base)
	}
}

func TestWorkloadTemplateUsesCatalogVariablesAndRejectsUnresolvedTokens(t *testing.T) {
	tmp := t.TempDir()
	launchdDir := filepath.Join(tmp, "infra", "launchd")
	if err := os.MkdirAll(launchdDir, 0o755); err != nil {
		t.Fatal(err)
	}
	tmplPath := filepath.Join(launchdDir, "com.natty.fsd.demo.plist.template")
	tmpl := `<plist><dict><key>Label</key><string>com.natty.fsd.demo</string>` +
		`<key>ProgramArguments</key><array><string>__REPO_ROOT__/bin/fsd-natty</string>` +
		`<string>__STATE_DIR__</string></array><key>EnvironmentVariables</key><dict>` +
		`<key>CHANNEL</key><string>__CHANNEL__</string></dict></dict></plist>`
	if err := os.WriteFile(tmplPath, []byte(tmpl), 0o644); err != nil {
		t.Fatal(err)
	}
	c := &catalog.Catalog{
		TargetGroups: map[string]string{"all": "all"},
		Hosts:        []catalog.Host{{ID: "local", Hostname: "localhost", TargetGroups: []string{"all"}}},
		AgentWorkloads: []catalog.AgentWorkload{{
			ID: "demo", Owner: "fsd", Kind: "schedule", Status: "active",
			Targets: []string{"all"}, PlistTemplate: tmplPath,
			TemplateVars: map[string]string{
				"__STATE_DIR__": "~/state/fsd", "__CHANNEL__": "C0123",
			},
		}},
	}
	files, err := Launchd(c, Options{Selector: target.Selector{TargetGroups: []string{"all"}}, OutputDir: "/tmp/out"})
	if err != nil {
		t.Fatalf("Launchd() error = %v", err)
	}
	if len(files) != 1 || strings.Contains(string(files[0].Content), "__") {
		t.Fatalf("template variables were not fully resolved: %v", files)
	}
	if !strings.Contains(string(files[0].Content), pathutil.Expand("~/state/fsd")) {
		t.Fatalf("home-relative catalog path was not expanded: %s", files[0].Content)
	}

	c.AgentWorkloads[0].TemplateVars = map[string]string{"__STATE_DIR__": "~/state/fsd"}
	if _, err := Launchd(c, Options{Selector: target.Selector{TargetGroups: []string{"all"}}, OutputDir: "/tmp/out"}); err == nil || !strings.Contains(err.Error(), "__CHANNEL__") {
		t.Fatalf("expected unresolved token failure, got %v", err)
	}
	c.AgentWorkloads[0].TemplateVars = map[string]string{
		"__STATE_DIR__": "__CHANNEL__/state", "__CHANNEL__": "C0123",
	}
	if _, err := Launchd(c, Options{Selector: target.Selector{TargetGroups: []string{"all"}}, OutputDir: "/tmp/out"}); err == nil || !strings.Contains(err.Error(), "nested template variable") {
		t.Fatalf("expected nested template-variable failure, got %v", err)
	}
	c.AgentWorkloads[0].TemplateVars = map[string]string{
		"__STATE_DIR__": "bad\x01value", "__CHANNEL__": "C0123",
	}
	if _, err := Launchd(c, Options{Selector: target.Selector{TargetGroups: []string{"all"}}, OutputDir: "/tmp/out"}); err == nil || !strings.Contains(err.Error(), "unsafe control character") {
		t.Fatalf("expected XML control-character failure, got %v", err)
	}
}

func TestScheduledWorkloadWithTemplateRenders(t *testing.T) {
	tmp := t.TempDir()
	launchdDir := filepath.Join(tmp, "infra", "launchd")
	if err := os.MkdirAll(launchdDir, 0o755); err != nil {
		t.Fatal(err)
	}
	tmplPath := filepath.Join(launchdDir, "com.natty.claude.nightly-report.plist.template")
	if err := os.WriteFile(tmplPath, []byte(`<plist><dict><key>Label</key><string>com.natty.claude.nightly-report</string></dict></plist>`), 0o644); err != nil {
		t.Fatal(err)
	}

	c := &catalog.Catalog{
		TargetGroups: map[string]string{"all": "all"},
		Hosts:        []catalog.Host{{ID: "local", Hostname: "localhost", TargetGroups: []string{"all"}}},
		AgentWorkloads: []catalog.AgentWorkload{{
			ID:            "nightly-report",
			Kind:          "schedule",
			Status:        "active",
			Targets:       []string{"all"},
			Schedule:      "StartCalendarInterval Hour=3 Minute=0",
			PlistTemplate: tmplPath,
		}},
	}
	files, err := Launchd(c, Options{Selector: target.Selector{TargetGroups: []string{"all"}}, OutputDir: "/tmp/out"})
	if err != nil {
		t.Fatalf("Launchd() error = %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("want 1 file for schedule workload with template, got %d", len(files))
	}
	base := filepath.Base(files[0].Path)
	if base != "com.natty.claude.nightly-report.plist" {
		t.Fatalf("filename = %q, want com.natty.claude.nightly-report.plist", base)
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
