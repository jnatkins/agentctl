package plan

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/jnatkins/agentctl/internal/catalog"
	"github.com/jnatkins/agentctl/internal/target"
)

func TestBuildPlansMissingRepo(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "repo")
	c := &catalog.Catalog{
		Version:      1,
		TargetGroups: map[string]string{"all": "all"},
		Hosts:        []catalog.Host{{ID: "local", Hostname: "localhost", TargetGroups: []string{"all"}}},
		Repos:        []catalog.Repo{{ID: "repo", Remote: "git@example.com:repo.git", Path: missing, UpdatePolicy: "fast-forward-only", Targets: []string{"all"}}},
	}
	p, err := Build(c, target.Selector{TargetGroups: []string{"all"}})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(p.Changes) == 0 || p.Changes[0].Action != "create" {
		t.Fatalf("plan changes = %#v", p.Changes)
	}
}

func TestApplyWritesRenderedFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	c := &catalog.Catalog{
		Version:      1,
		TargetGroups: map[string]string{"all": "all"},
		Hosts:        []catalog.Host{{ID: "local", Hostname: "localhost", TargetGroups: []string{"all"}}},
		AgentWorkloads: []catalog.AgentWorkload{{
			ID: "daily", Name: "Daily", Owner: "ops", Kind: "schedule", Status: "active", Schedule: "FREQ=DAILY",
			Targets: []string{"all"}, Harnesses: []string{"codex"}, Prompt: "do work",
		}},
	}
	changes, err := Apply(c, target.Selector{TargetGroups: []string{"all"}})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if len(changes) == 0 {
		t.Fatalf("expected changes")
	}
	path := filepath.Join(home, ".codex", "automations", "daily", "automation.toml")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected rendered file: %v", err)
	}
}

func TestApplyInstallsSkillPackTopLevelSkills(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	source := filepath.Join(home, "generated-skills")
	if err := os.MkdirAll(filepath.Join(source, "agent-ops-bootstrap"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "agent-ops-bootstrap", "SKILL.md"), []byte("---\nname: agent-ops-bootstrap\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(source, "plugin-source", "skills", "raw-skill"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "plugin-source", "skills", "raw-skill", "SKILL.md"), []byte("---\nname: raw\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := &catalog.Catalog{
		Version:      1,
		TargetGroups: map[string]string{"all": "all"},
		Hosts:        []catalog.Host{{ID: "local", Hostname: "localhost", TargetGroups: []string{"all"}}},
		SkillPacks: []catalog.SkillPack{{
			ID: "skills", Source: source, InstallPath: "~/.codex/skills", Targets: []string{"all"},
		}},
	}
	if _, err := Apply(c, target.Selector{TargetGroups: []string{"all"}}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".codex", "skills", "agent-ops-bootstrap", "SKILL.md")); err != nil {
		t.Fatalf("expected installed skill: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".codex", "skills", "plugin-source", "skills", "raw-skill", "SKILL.md")); !os.IsNotExist(err) {
		t.Fatalf("expected nested plugin source not to be copied, err=%v", err)
	}
}

func TestApplyInstallsHarnessExtensionsAndStateStores(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	source := filepath.Join(home, "bin")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "op-sa"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	c := &catalog.Catalog{
		Version:      1,
		TargetGroups: map[string]string{"all": "all"},
		Hosts:        []catalog.Host{{ID: "local", Hostname: "localhost", TargetGroups: []string{"all"}}},
		HarnessExtensions: []catalog.HarnessExtension{{
			ID: "wrappers", Status: "active", Source: source, Path: "~/.local/bin", Targets: []string{"all"},
		}},
		StateStores: []catalog.StateStore{{
			ID: "state", Type: "directory", Status: "active", Path: "~/.local/state/agentctl", Targets: []string{"all"},
		}},
	}
	if _, err := Apply(c, target.Selector{TargetGroups: []string{"all"}}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".local", "bin", "op-sa")); err != nil {
		t.Fatalf("expected installed wrapper: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".local", "state", "agentctl")); err != nil {
		t.Fatalf("expected state store: %v", err)
	}
}

func TestApplyCreatesOwnerOnlySQLiteStateAndLogDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	c := &catalog.Catalog{
		Version: 1, TargetGroups: map[string]string{"all": "all"},
		Hosts: []catalog.Host{{ID: "local", Hostname: "localhost", TargetGroups: []string{"all"}}},
		StateStores: []catalog.StateStore{
			{ID: "ledger", Type: "sqlite", Status: "active", Path: "~/.local/state/fsd/news.db", Mode: "0600", ParentMode: "0700", Targets: []string{"all"}},
			{ID: "logs", Type: "directory", Status: "active", Path: "~/Library/Logs/fsd", Mode: "0700", Targets: []string{"all"}},
		},
	}
	if _, err := Apply(c, target.Selector{TargetGroups: []string{"all"}}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	for path, want := range map[string]os.FileMode{
		filepath.Join(home, ".local", "state", "fsd"):            0o700,
		filepath.Join(home, ".local", "state", "fsd", "news.db"): 0o600,
		filepath.Join(home, "Library", "Logs", "fsd"):            0o700,
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if info.Mode().Perm() != want {
			t.Fatalf("%s mode = %04o; want %04o", path, info.Mode().Perm(), want)
		}
	}
}

func TestApplyDoesNotBroadenExistingStateModeWithoutExplicitMode(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(home, ".local", "state", "private")
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
	c := &catalog.Catalog{
		Version: 1, TargetGroups: map[string]string{"all": "all"},
		Hosts: []catalog.Host{{ID: "local", Hostname: "localhost", TargetGroups: []string{"all"}}},
		StateStores: []catalog.StateStore{{
			ID: "private", Type: "directory", Status: "active", Path: path,
			Targets: []string{"all"},
		}},
	}
	if _, err := Apply(c, target.Selector{TargetGroups: []string{"all"}}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("existing state mode = %v, err=%v; want 0700", info.Mode().Perm(), err)
	}
}

func TestApplyInstallsCLIWrappersAsSymlinks(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	source := filepath.Join(home, "repo", "bin")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	wrapper := filepath.Join(source, "gmail")
	if err := os.WriteFile(wrapper, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "gmail"), []byte("old copied wrapper\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	c := &catalog.Catalog{
		Version:      1,
		TargetGroups: map[string]string{"all": "all"},
		Hosts:        []catalog.Host{{ID: "local", Hostname: "localhost", TargetGroups: []string{"all"}}},
		HarnessExtensions: []catalog.HarnessExtension{{
			ID: "wrappers", Type: "cli-wrappers", Status: "active", Source: source, Path: "~/.local/bin", Targets: []string{"all"},
		}},
	}
	if _, err := Apply(c, target.Selector{TargetGroups: []string{"all"}}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	targetPath, err := os.Readlink(filepath.Join(dest, "gmail"))
	if err != nil {
		t.Fatalf("expected wrapper symlink: %v", err)
	}
	if targetPath != wrapper {
		t.Fatalf("symlink target = %q, want %q", targetPath, wrapper)
	}
}

func TestApplyCreatesHarnessConfigAndGeneratedSkillManifest(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	source := filepath.Join(home, "generated")
	if err := os.MkdirAll(filepath.Join(source, "agent-ops-bootstrap"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "agent-ops-bootstrap", "SKILL.md"), []byte("---\nname: agent-ops-bootstrap\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := &catalog.Catalog{
		Version:      1,
		TargetGroups: map[string]string{"all": "all"},
		Hosts:        []catalog.Host{{ID: "local", Hostname: "localhost", TargetGroups: []string{"all"}}},
		HarnessConfigs: []catalog.HarnessConfig{{
			ID: "codex-config", Harness: "codex", Type: "user-config", Status: "active", Path: "~/.codex/config.toml", Targets: []string{"all"},
		}},
		HarnessExtensions: []catalog.HarnessExtension{{
			ID: "skills", Type: "generated-skill-adapters", Status: "active", Source: source, Path: "~/.codex/skills", Targets: []string{"all"},
		}},
	}
	if _, err := Apply(c, target.Selector{TargetGroups: []string{"all"}}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".codex", "config.toml")); err != nil {
		t.Fatalf("expected harness config: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(home, ".codex", "skills", ".agent-skills-install.json"))
	if err != nil {
		t.Fatalf("expected install manifest: %v", err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("manifest JSON: %v", err)
	}
	if manifest["status"] != "ok" || manifest["installed_by"] != "agentctl" {
		t.Fatalf("manifest = %#v", manifest)
	}
}

func TestBuildReportsMissingCredentialSourceEnv(t *testing.T) {
	t.Setenv("AGENTCTL_TEST_GITHUB_TOKEN", "")
	missing := filepath.Join(t.TempDir(), "repo")
	c := &catalog.Catalog{
		Version:      1,
		TargetGroups: map[string]string{"all": "all"},
		Hosts:        []catalog.Host{{ID: "local", Hostname: "localhost", TargetGroups: []string{"all"}}},
		CredentialSources: []catalog.CredentialSource{{
			ID: "github", Type: "github_token_env", Env: "AGENTCTL_TEST_GITHUB_TOKEN", Targets: []string{"all"},
		}},
		Repos: []catalog.Repo{{
			ID: "repo", Remote: "https://github.com/example/private.git", Path: missing, UpdatePolicy: "fast-forward-only", AuthRef: "github", Targets: []string{"all"},
		}},
	}
	p, err := Build(c, target.Selector{TargetGroups: []string{"all"}})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	found := false
	for _, change := range p.Changes {
		if change.Resource == "credential_source" && change.ID == "github" && change.Risk == "high" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected missing credential source env in plan, got %#v", p.Changes)
	}
}

func TestApplySkillPackCleansUpLeftoverTmp(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	source := filepath.Join(home, "generated-skills")
	if err := os.MkdirAll(filepath.Join(source, "my-skill"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "my-skill", "SKILL.md"), []byte("---\nname: my-skill\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(home, ".codex", "skills")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	// Simulate a leftover .tmp from a prior crashed install.
	leftover := filepath.Join(dest, "my-skill.tmp")
	if err := os.MkdirAll(leftover, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(leftover, "stale.md"), []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := &catalog.Catalog{
		Version:      1,
		TargetGroups: map[string]string{"all": "all"},
		Hosts:        []catalog.Host{{ID: "local", Hostname: "localhost", TargetGroups: []string{"all"}}},
		SkillPacks: []catalog.SkillPack{{
			ID: "skills", Source: source, InstallPath: "~/.codex/skills", Targets: []string{"all"},
		}},
	}
	if _, err := Apply(c, target.Selector{TargetGroups: []string{"all"}}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	// .tmp should be gone.
	if _, err := os.Stat(leftover); !os.IsNotExist(err) {
		t.Fatalf("expected leftover .tmp to be removed, err=%v", err)
	}
	// Final skill dir should contain only source files.
	if _, err := os.Stat(filepath.Join(dest, "my-skill", "SKILL.md")); err != nil {
		t.Fatalf("expected installed SKILL.md: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "my-skill", "stale.md")); !os.IsNotExist(err) {
		t.Fatalf("expected stale.md to be gone after atomic replace")
	}
}

func TestApplyRepoRequiresCredentialSourceEnv(t *testing.T) {
	t.Setenv("AGENTCTL_TEST_GITHUB_TOKEN", "")
	missing := filepath.Join(t.TempDir(), "repo")
	c := &catalog.Catalog{
		Version:      1,
		TargetGroups: map[string]string{"all": "all"},
		Hosts:        []catalog.Host{{ID: "local", Hostname: "localhost", TargetGroups: []string{"all"}}},
		CredentialSources: []catalog.CredentialSource{{
			ID: "github", Type: "github_token_env", Env: "AGENTCTL_TEST_GITHUB_TOKEN", Targets: []string{"all"},
		}},
		Repos: []catalog.Repo{{
			ID: "repo", Remote: "https://github.com/example/private.git", Path: missing, UpdatePolicy: "fast-forward-only", AuthRef: "github", Targets: []string{"all"},
		}},
	}
	changes, err := Apply(c, target.Selector{TargetGroups: []string{"all"}})
	if err == nil {
		t.Fatalf("expected apply error")
	}
	if len(changes) != 1 || changes[0].Resource != "repo" || changes[0].Risk != "high" {
		t.Fatalf("changes = %#v", changes)
	}
}
