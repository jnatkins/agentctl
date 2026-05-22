package plan

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jnatkins/agentctl/internal/catalog"
	"github.com/jnatkins/agentctl/internal/pathutil"
	"github.com/jnatkins/agentctl/internal/render"
	"github.com/jnatkins/agentctl/internal/target"
)

type Change struct {
	Action   string `json:"action"`
	Resource string `json:"resource"`
	ID       string `json:"id"`
	Path     string `json:"path,omitempty"`
	Risk     string `json:"risk"`
	Reason   string `json:"reason"`
}

type Plan struct {
	Changes []Change `json:"changes"`
}

func Build(c *catalog.Catalog, selector target.Selector) (Plan, error) {
	var p Plan
	for _, repo := range c.Repos {
		if !target.Matches(repo.Targets, c, selector) {
			continue
		}
		path := pathutil.Expand(repo.Path)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			p.Changes = append(p.Changes, Change{Action: "create", Resource: "repo", ID: repo.ID, Path: path, Risk: "medium", Reason: "checkout is missing"})
		} else if err != nil {
			p.Changes = append(p.Changes, Change{Action: "check", Resource: "repo", ID: repo.ID, Path: path, Risk: "medium", Reason: err.Error()})
		} else if repo.UpdatePolicy == "fast-forward-only" {
			p.Changes = append(p.Changes, Change{Action: "update", Resource: "repo", ID: repo.ID, Path: path, Risk: "medium", Reason: "fast-forward-only policy"})
		} else {
			p.Changes = append(p.Changes, Change{Action: "check", Resource: "repo", ID: repo.ID, Path: path, Risk: "low", Reason: "repo exists"})
		}
	}
	files, err := render.All(c, render.Options{Selector: selector, Install: true})
	if err != nil {
		return p, err
	}
	for _, f := range files {
		action := "create"
		reason := f.Reason
		if existing, err := os.ReadFile(f.Path); err == nil {
			if bytes.Equal(existing, f.Content) {
				action = "no-op"
				reason = "already current"
			} else {
				action = "update"
				reason = f.Reason + " differs"
			}
		} else if !os.IsNotExist(err) {
			action = "check"
			reason = err.Error()
		}
		p.Changes = append(p.Changes, Change{Action: action, Resource: "rendered_file", Path: f.Path, Risk: "low", Reason: reason})
	}
	if len(p.Changes) == 0 {
		p.Changes = append(p.Changes, Change{Action: "no-op", Resource: "catalog", Risk: "low", Reason: "no target resources"})
	}
	return p, nil
}

func Apply(c *catalog.Catalog, selector target.Selector) ([]Change, error) {
	var changes []Change
	for _, repo := range c.Repos {
		if !target.Matches(repo.Targets, c, selector) || repo.UpdatePolicy == "check-only" || repo.UpdatePolicy == "manual" {
			continue
		}
		change, err := applyRepo(repo)
		changes = append(changes, change)
		if err != nil {
			return changes, err
		}
	}
	files, err := render.All(c, render.Options{Selector: selector, Install: true})
	if err != nil {
		return changes, err
	}
	for _, f := range files {
		change, err := writeFile(f)
		changes = append(changes, change)
		if err != nil {
			return changes, err
		}
	}
	return changes, nil
}

func applyRepo(repo catalog.Repo) (Change, error) {
	path := pathutil.Expand(repo.Path)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		args := []string{"clone"}
		if repo.Branch != "" {
			args = append(args, "--branch", repo.Branch)
		}
		args = append(args, repo.Remote, path)
		cmd := exec.Command("git", args...)
		out, err := cmd.CombinedOutput()
		change := Change{Action: "create", Resource: "repo", ID: repo.ID, Path: path, Risk: "medium", Reason: strings.TrimSpace(string(out))}
		if err != nil {
			return change, fmt.Errorf("clone repo %s: %w", repo.ID, err)
		}
		return change, nil
	}
	if repo.UpdatePolicy == "fast-forward-only" {
		cmd := exec.Command("git", "-C", path, "pull", "--ff-only")
		out, err := cmd.CombinedOutput()
		change := Change{Action: "update", Resource: "repo", ID: repo.ID, Path: path, Risk: "medium", Reason: strings.TrimSpace(string(out))}
		if err != nil {
			return change, fmt.Errorf("update repo %s: %w", repo.ID, err)
		}
		return change, nil
	}
	return Change{Action: "check", Resource: "repo", ID: repo.ID, Path: path, Risk: "low", Reason: "repo exists"}, nil
}

func writeFile(f render.File) (Change, error) {
	if existing, err := os.ReadFile(f.Path); err == nil && bytes.Equal(existing, f.Content) {
		return Change{Action: "no-op", Resource: "rendered_file", Path: f.Path, Risk: "low", Reason: "already current"}, nil
	}
	if err := os.MkdirAll(filepath.Dir(f.Path), 0o755); err != nil {
		return Change{Action: "update", Resource: "rendered_file", Path: f.Path, Risk: "low", Reason: err.Error()}, err
	}
	tmp := f.Path + ".tmp"
	if err := os.WriteFile(tmp, f.Content, os.FileMode(f.Mode)); err != nil {
		return Change{Action: "update", Resource: "rendered_file", Path: f.Path, Risk: "low", Reason: err.Error()}, err
	}
	if err := os.Rename(tmp, f.Path); err != nil {
		return Change{Action: "update", Resource: "rendered_file", Path: f.Path, Risk: "low", Reason: err.Error()}, err
	}
	return Change{Action: "update", Resource: "rendered_file", Path: f.Path, Risk: "low", Reason: f.Reason}, nil
}
