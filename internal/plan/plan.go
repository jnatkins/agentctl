package plan

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

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
	for _, store := range c.StateStores {
		if !target.Matches(store.Targets, c, selector) || !isActive(store.Status) {
			continue
		}
		p.Changes = append(p.Changes, planStateStore(store))
	}
	for _, cfg := range c.HarnessConfigs {
		if !target.Matches(cfg.Targets, c, selector) || !isActive(cfg.Status) {
			continue
		}
		p.Changes = append(p.Changes, planHarnessConfig(cfg))
	}
	for _, ext := range c.HarnessExtensions {
		if !target.Matches(ext.Targets, c, selector) || !isActive(ext.Status) {
			continue
		}
		p.Changes = append(p.Changes, planHarnessExtension(ext))
	}
	for _, skill := range c.SkillPacks {
		if !target.Matches(skill.Targets, c, selector) {
			continue
		}
		change, err := planSkillPack(skill)
		p.Changes = append(p.Changes, change)
		if err != nil {
			continue
		}
		p.Changes = append(p.Changes, planSkillAliases(skill)...)
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
	for _, store := range c.StateStores {
		if !target.Matches(store.Targets, c, selector) || !isActive(store.Status) {
			continue
		}
		change, err := applyStateStore(store)
		changes = append(changes, change)
		if err != nil {
			return changes, err
		}
	}
	for _, cfg := range c.HarnessConfigs {
		if !target.Matches(cfg.Targets, c, selector) || !isActive(cfg.Status) {
			continue
		}
		change, err := applyHarnessConfig(cfg)
		changes = append(changes, change)
		if err != nil {
			return changes, err
		}
	}
	for _, ext := range c.HarnessExtensions {
		if !target.Matches(ext.Targets, c, selector) || !isActive(ext.Status) {
			continue
		}
		change, err := applyHarnessExtension(ext)
		changes = append(changes, change)
		if err != nil {
			return changes, err
		}
	}
	for _, skill := range c.SkillPacks {
		if !target.Matches(skill.Targets, c, selector) {
			continue
		}
		change, err := applySkillPack(skill)
		changes = append(changes, change)
		if err != nil {
			return changes, err
		}
		for _, change := range applySkillAliases(skill) {
			changes = append(changes, change)
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

func isActive(status string) bool {
	return status == "" || status == "active"
}

func planStateStore(store catalog.StateStore) Change {
	path := pathutil.Expand(store.Path)
	if store.Type == "external" {
		return Change{Action: "check", Resource: "state_store", ID: store.ID, Path: store.URL, Risk: "low", Reason: "external state is probe-only"}
	}
	if path == "" {
		return Change{Action: "check", Resource: "state_store", ID: store.ID, Risk: "medium", Reason: "path is required"}
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return Change{Action: "create", Resource: "state_store", ID: store.ID, Path: path, Risk: "low", Reason: "path is missing"}
	} else if err != nil {
		return Change{Action: "check", Resource: "state_store", ID: store.ID, Path: path, Risk: "medium", Reason: err.Error()}
	}
	return Change{Action: "no-op", Resource: "state_store", ID: store.ID, Path: path, Risk: "low", Reason: "already present"}
}

func applyStateStore(store catalog.StateStore) (Change, error) {
	planned := planStateStore(store)
	path := pathutil.Expand(store.Path)
	switch store.Type {
	case "external":
		return planned, nil
	case "directory", "queue":
		if err := os.MkdirAll(path, 0o755); err != nil {
			return Change{Action: planned.Action, Resource: "state_store", ID: store.ID, Path: path, Risk: "low", Reason: err.Error()}, err
		}
	case "file", "sqlite":
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return Change{Action: planned.Action, Resource: "state_store", ID: store.ID, Path: path, Risk: "low", Reason: err.Error()}, err
		}
		file, err := os.OpenFile(path, os.O_CREATE, 0o644)
		if err != nil {
			return Change{Action: planned.Action, Resource: "state_store", ID: store.ID, Path: path, Risk: "low", Reason: err.Error()}, err
		}
		if err := file.Close(); err != nil {
			return Change{Action: planned.Action, Resource: "state_store", ID: store.ID, Path: path, Risk: "low", Reason: err.Error()}, err
		}
	}
	if planned.Action == "no-op" {
		return planned, nil
	}
	return Change{Action: planned.Action, Resource: "state_store", ID: store.ID, Path: path, Risk: "low", Reason: "created state path"}, nil
}

func planHarnessConfig(cfg catalog.HarnessConfig) Change {
	path := pathutil.Expand(cfg.Path)
	if path == "" {
		return Change{Action: "check", Resource: "harness_config", ID: cfg.ID, Risk: "medium", Reason: "path is required"}
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return Change{Action: "create", Resource: "harness_config", ID: cfg.ID, Path: path, Risk: "low", Reason: "config file is missing"}
	} else if err != nil {
		return Change{Action: "check", Resource: "harness_config", ID: cfg.ID, Path: path, Risk: "medium", Reason: err.Error()}
	}
	return Change{Action: "no-op", Resource: "harness_config", ID: cfg.ID, Path: path, Risk: "low", Reason: "already present"}
}

func applyHarnessConfig(cfg catalog.HarnessConfig) (Change, error) {
	planned := planHarnessConfig(cfg)
	path := pathutil.Expand(cfg.Path)
	if planned.Action != "create" {
		return planned, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return Change{Action: "create", Resource: "harness_config", ID: cfg.ID, Path: path, Risk: "low", Reason: err.Error()}, err
	}
	content := defaultHarnessConfigContent(cfg, path)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return Change{Action: "create", Resource: "harness_config", ID: cfg.ID, Path: path, Risk: "low", Reason: err.Error()}, err
	}
	return Change{Action: "create", Resource: "harness_config", ID: cfg.ID, Path: path, Risk: "low", Reason: "created missing config file"}, nil
}

func defaultHarnessConfigContent(cfg catalog.HarnessConfig, path string) []byte {
	if strings.HasSuffix(path, ".json") || cfg.Type == "hooks" {
		return []byte("{}\n")
	}
	if strings.HasSuffix(path, ".toml") {
		var b strings.Builder
		b.WriteString("# Created by agentctl.\n")
		keys := make([]string, 0, len(cfg.Settings))
		for k := range cfg.Settings {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(&b, "%s = %q\n", k, cfg.Settings[k])
		}
		return []byte(b.String())
	}
	return []byte("")
}

func planHarnessExtension(ext catalog.HarnessExtension) Change {
	source := pathutil.ResolveRelative(ext.SourceDir(), ext.Source)
	dest := pathutil.Expand(ext.Path)
	if source == "" || dest == "" {
		return Change{Action: "check", Resource: "harness_extension", ID: ext.ID, Path: dest, Risk: "medium", Reason: "source and path are required"}
	}
	info, err := os.Stat(source)
	if err != nil {
		return Change{Action: "check", Resource: "harness_extension", ID: ext.ID, Path: dest, Risk: "medium", Reason: fmt.Sprintf("source unavailable: %v", err)}
	}
	if !info.IsDir() {
		return planManagedFile("harness_extension", ext.ID, source, dest)
	}
	if _, err := os.Stat(dest); os.IsNotExist(err) {
		return Change{Action: "create", Resource: "harness_extension", ID: ext.ID, Path: dest, Risk: "medium", Reason: "install path is missing"}
	} else if err != nil {
		return Change{Action: "check", Resource: "harness_extension", ID: ext.ID, Path: dest, Risk: "medium", Reason: err.Error()}
	}
	differs, count, err := dirDiffers(source, dest)
	if err != nil {
		return Change{Action: "check", Resource: "harness_extension", ID: ext.ID, Path: dest, Risk: "medium", Reason: err.Error()}
	}
	if differs {
		return Change{Action: "update", Resource: "harness_extension", ID: ext.ID, Path: dest, Risk: "medium", Reason: fmt.Sprintf("%d managed file(s) missing or changed", count)}
	}
	return Change{Action: "no-op", Resource: "harness_extension", ID: ext.ID, Path: dest, Risk: "low", Reason: "already current"}
}

func applyHarnessExtension(ext catalog.HarnessExtension) (Change, error) {
	planned := planHarnessExtension(ext)
	source := pathutil.ResolveRelative(ext.SourceDir(), ext.Source)
	dest := pathutil.Expand(ext.Path)
	info, err := os.Stat(source)
	if err != nil {
		return planned, err
	}
	if info.IsDir() {
		if err := copyDirContents(source, dest); err != nil {
			return Change{Action: planned.Action, Resource: "harness_extension", ID: ext.ID, Path: dest, Risk: "medium", Reason: err.Error()}, err
		}
	} else if err := copyFile(source, dest, info.Mode().Perm()); err != nil {
		return Change{Action: planned.Action, Resource: "harness_extension", ID: ext.ID, Path: dest, Risk: "medium", Reason: err.Error()}, err
	}
	if ext.Type == "generated-skill-adapters" && (planned.Action != "no-op" || missing(filepath.Join(dest, ".agent-skills-install.json"))) {
		if err := writeGeneratedSkillManifest(source, dest); err != nil {
			return Change{Action: planned.Action, Resource: "harness_extension", ID: ext.ID, Path: dest, Risk: "medium", Reason: err.Error()}, err
		}
	}
	if planned.Action == "no-op" {
		return planned, nil
	}
	return Change{Action: planned.Action, Resource: "harness_extension", ID: ext.ID, Path: dest, Risk: "medium", Reason: "installed managed extension files"}, nil
}

func writeGeneratedSkillManifest(source, dest string) error {
	skills, err := installableSkillDirs(source)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(skills))
	for _, skill := range skills {
		names = append(names, filepath.Base(skill))
	}
	sort.Strings(names)
	manifest := map[string]any{
		"source":                            source,
		"dest":                              dest,
		"backup":                            "",
		"agents_root":                       filepath.Join(os.Getenv("HOME"), ".agents", "skills"),
		"quarantine_agents_root":            false,
		"generated_skill_count":             len(names),
		"legacy_symlinks_to_remove":         []string{},
		"generated_targets_to_replace":      names,
		"stale_generated_targets_to_remove": []string{},
		"preserved":                         []string{},
		"installed_at":                      time.Now().UTC().Format("20060102T150405Z"),
		"installed_generated_skills":        len(names),
		"status":                            "ok",
		"installed_by":                      "agentctl",
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dest, ".agent-skills-install.json"), append(data, '\n'), 0o644)
}

func missing(path string) bool {
	_, err := os.Stat(path)
	return os.IsNotExist(err)
}

func planManagedFile(resource, id, source, dest string) Change {
	if _, err := os.Stat(dest); os.IsNotExist(err) {
		return Change{Action: "create", Resource: resource, ID: id, Path: dest, Risk: "medium", Reason: "install path is missing"}
	} else if err != nil {
		return Change{Action: "check", Resource: resource, ID: id, Path: dest, Risk: "medium", Reason: err.Error()}
	}
	same, err := filesEqual(source, dest)
	if err != nil {
		return Change{Action: "check", Resource: resource, ID: id, Path: dest, Risk: "medium", Reason: err.Error()}
	}
	if !same {
		return Change{Action: "update", Resource: resource, ID: id, Path: dest, Risk: "medium", Reason: "managed file differs"}
	}
	return Change{Action: "no-op", Resource: resource, ID: id, Path: dest, Risk: "low", Reason: "already current"}
}

func planSkillPack(skill catalog.SkillPack) (Change, error) {
	source := pathutil.ResolveRelative(skill.SourceDir(), skill.Source)
	dest := pathutil.Expand(skill.InstallPath)
	if dest == "" {
		return Change{Action: "check", Resource: "skill_pack", ID: skill.ID, Risk: "medium", Reason: "install_path is required"}, fmt.Errorf("skill_pack %s: install_path is required", skill.ID)
	}
	info, err := os.Stat(source)
	if err != nil {
		return Change{Action: "check", Resource: "skill_pack", ID: skill.ID, Path: dest, Risk: "medium", Reason: fmt.Sprintf("source unavailable: %v", err)}, err
	}
	if !info.IsDir() {
		return Change{Action: "check", Resource: "skill_pack", ID: skill.ID, Path: dest, Risk: "medium", Reason: "source must be a directory"}, fmt.Errorf("skill_pack %s: source must be a directory", skill.ID)
	}
	skills, err := installableSkillDirs(source)
	if err != nil {
		return Change{Action: "check", Resource: "skill_pack", ID: skill.ID, Path: dest, Risk: "medium", Reason: err.Error()}, err
	}
	if len(skills) == 0 {
		return Change{Action: "check", Resource: "skill_pack", ID: skill.ID, Path: dest, Risk: "low", Reason: "no top-level installable skills found"}, nil
	}
	if _, err := os.Stat(dest); os.IsNotExist(err) {
		return Change{Action: "create", Resource: "skill_pack", ID: skill.ID, Path: dest, Risk: "medium", Reason: "install path is missing"}, nil
	} else if err != nil {
		return Change{Action: "check", Resource: "skill_pack", ID: skill.ID, Path: dest, Risk: "medium", Reason: err.Error()}, err
	}
	differs, count, err := skillDirsDiffer(skills, dest)
	if err != nil {
		return Change{Action: "check", Resource: "skill_pack", ID: skill.ID, Path: dest, Risk: "medium", Reason: err.Error()}, err
	}
	if differs {
		return Change{Action: "update", Resource: "skill_pack", ID: skill.ID, Path: dest, Risk: "medium", Reason: fmt.Sprintf("%d managed file(s) missing or changed", count)}, nil
	}
	return Change{Action: "no-op", Resource: "skill_pack", ID: skill.ID, Path: dest, Risk: "low", Reason: "already current"}, nil
}

func applySkillPack(skill catalog.SkillPack) (Change, error) {
	planned, err := planSkillPack(skill)
	if err != nil {
		return planned, err
	}
	source := pathutil.ResolveRelative(skill.SourceDir(), skill.Source)
	dest := pathutil.Expand(skill.InstallPath)
	skills, err := installableSkillDirs(source)
	if err != nil {
		return planned, err
	}
	if len(skills) == 0 {
		return planned, nil
	}
	if err := copySkillDirs(skills, dest); err != nil {
		return Change{Action: "update", Resource: "skill_pack", ID: skill.ID, Path: dest, Risk: "medium", Reason: err.Error()}, err
	}
	if planned.Action == "no-op" {
		return planned, nil
	}
	return Change{Action: planned.Action, Resource: "skill_pack", ID: skill.ID, Path: dest, Risk: "medium", Reason: "installed managed skill files"}, nil
}

func installableSkillDirs(source string) ([]string, error) {
	entries, err := os.ReadDir(source)
	if err != nil {
		return nil, err
	}
	var skills []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(source, entry.Name())
		if _, err := os.Stat(filepath.Join(path, "SKILL.md")); err == nil {
			skills = append(skills, path)
		} else if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
	}
	return skills, nil
}

func skillDirsDiffer(skills []string, dest string) (bool, int, error) {
	count := 0
	for _, skill := range skills {
		name := filepath.Base(skill)
		differs, changed, err := dirDiffers(skill, filepath.Join(dest, name))
		if err != nil {
			return false, 0, err
		}
		if differs {
			count += changed
		}
	}
	return count > 0, count, nil
}

func copySkillDirs(skills []string, dest string) error {
	for _, skill := range skills {
		if err := copyDirContents(skill, filepath.Join(dest, filepath.Base(skill))); err != nil {
			return err
		}
	}
	return nil
}

func planSkillAliases(skill catalog.SkillPack) []Change {
	dest := pathutil.Expand(skill.InstallPath)
	var changes []Change
	for _, alias := range skill.CompatibilityAliases {
		path := pathutil.Expand(alias)
		if path == "" {
			continue
		}
		if target, err := os.Readlink(path); err == nil {
			if target == dest {
				changes = append(changes, Change{Action: "no-op", Resource: "skill_alias", ID: skill.ID, Path: path, Risk: "low", Reason: "already current"})
			} else {
				changes = append(changes, Change{Action: "check", Resource: "skill_alias", ID: skill.ID, Path: path, Risk: "medium", Reason: "alias symlink points elsewhere"})
			}
			continue
		}
		if _, err := os.Lstat(path); os.IsNotExist(err) {
			changes = append(changes, Change{Action: "create", Resource: "skill_alias", ID: skill.ID, Path: path, Risk: "low", Reason: "alias path is missing"})
		} else if err != nil {
			changes = append(changes, Change{Action: "check", Resource: "skill_alias", ID: skill.ID, Path: path, Risk: "medium", Reason: err.Error()})
		} else {
			changes = append(changes, Change{Action: "check", Resource: "skill_alias", ID: skill.ID, Path: path, Risk: "low", Reason: "path exists; leaving unmanaged"})
		}
	}
	return changes
}

func applySkillAliases(skill catalog.SkillPack) []Change {
	dest := pathutil.Expand(skill.InstallPath)
	var changes []Change
	for _, alias := range skill.CompatibilityAliases {
		path := pathutil.Expand(alias)
		if path == "" {
			continue
		}
		if target, err := os.Readlink(path); err == nil {
			if target == dest {
				changes = append(changes, Change{Action: "no-op", Resource: "skill_alias", ID: skill.ID, Path: path, Risk: "low", Reason: "already current"})
			} else {
				changes = append(changes, Change{Action: "check", Resource: "skill_alias", ID: skill.ID, Path: path, Risk: "medium", Reason: "alias symlink points elsewhere"})
			}
			continue
		}
		if _, err := os.Lstat(path); os.IsNotExist(err) {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				changes = append(changes, Change{Action: "create", Resource: "skill_alias", ID: skill.ID, Path: path, Risk: "low", Reason: err.Error()})
				continue
			}
			if err := os.Symlink(dest, path); err != nil {
				changes = append(changes, Change{Action: "create", Resource: "skill_alias", ID: skill.ID, Path: path, Risk: "low", Reason: err.Error()})
				continue
			}
			changes = append(changes, Change{Action: "create", Resource: "skill_alias", ID: skill.ID, Path: path, Risk: "low", Reason: "created compatibility symlink"})
		} else if err != nil {
			changes = append(changes, Change{Action: "check", Resource: "skill_alias", ID: skill.ID, Path: path, Risk: "medium", Reason: err.Error()})
		} else {
			changes = append(changes, Change{Action: "check", Resource: "skill_alias", ID: skill.ID, Path: path, Risk: "low", Reason: "path exists; leaving unmanaged"})
		}
	}
	return changes
}

func dirDiffers(source, dest string) (bool, int, error) {
	count := 0
	err := filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		destPath := filepath.Join(dest, rel)
		if entry.Type()&os.ModeSymlink != 0 {
			same, err := symlinksEqual(path, destPath)
			if err != nil {
				if os.IsNotExist(err) {
					count++
					return nil
				}
				return err
			}
			if !same {
				count++
			}
			return nil
		}
		same, err := filesEqual(path, destPath)
		if err != nil {
			if os.IsNotExist(err) {
				count++
				return nil
			}
			return err
		}
		if !same {
			count++
		}
		return nil
	})
	return count > 0, count, err
}

func filesEqual(a, b string) (bool, error) {
	left, err := os.ReadFile(a)
	if err != nil {
		return false, err
	}
	right, err := os.ReadFile(b)
	if err != nil {
		return false, err
	}
	return bytes.Equal(left, right), nil
}

func copyDirContents(source, dest string) error {
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(dest, 0o755)
		}
		destPath := filepath.Join(dest, rel)
		if entry.Type()&os.ModeSymlink != 0 {
			return copySymlink(path, destPath)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.MkdirAll(destPath, info.Mode().Perm())
		}
		return copyFile(path, destPath, info.Mode().Perm())
	})
}

func copyFile(source, dest string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dest + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp, mode); err != nil {
		return err
	}
	return os.Rename(tmp, dest)
}

func symlinksEqual(source, dest string) (bool, error) {
	left, err := os.Readlink(source)
	if err != nil {
		return false, err
	}
	right, err := os.Readlink(dest)
	if err != nil {
		return false, err
	}
	return left == right, nil
}

func copySymlink(source, dest string) error {
	target, err := os.Readlink(source)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	tmp := dest + ".tmp"
	if err := os.RemoveAll(tmp); err != nil {
		return err
	}
	if err := os.Symlink(target, tmp); err != nil {
		return err
	}
	if err := os.RemoveAll(dest); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dest)
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
