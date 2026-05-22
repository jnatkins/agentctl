package plan

import (
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
