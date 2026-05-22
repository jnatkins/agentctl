package catalog

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

func DiscoverCatalogs(cwd string) ([]string, error) {
	rootCandidates := []string{
		filepath.Join(cwd, "agentctl.toml"),
		filepath.Join(cwd, "state", "agentctl.toml"),
	}
	for _, root := range rootCandidates {
		if _, err := os.Stat(root); err == nil {
			paths := []string{root}
			for _, extra := range []string{
				filepath.Join(cwd, "state", "workloads.toml"),
				filepath.Join(cwd, "state", "automations.toml"),
				filepath.Join(cwd, "state", "integrations.toml"),
			} {
				if extra != root {
					if _, err := os.Stat(extra); err == nil {
						paths = append(paths, extra)
					}
				}
			}
			sort.Strings(paths)
			return paths, nil
		}
	}

	candidates := []string{
		filepath.Join(cwd, "state", "hosts.toml"),
		filepath.Join(cwd, "state", "workloads.toml"),
		filepath.Join(cwd, "state", "automations.toml"),
		filepath.Join(cwd, "state", "integrations.toml"),
	}
	seen := make(map[string]bool)
	var paths []string
	for _, p := range candidates {
		if seen[p] {
			continue
		}
		seen[p] = true
		if _, err := os.Stat(p); err == nil {
			paths = append(paths, p)
		}
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("no catalog supplied and no agentctl catalogs found under %s", cwd)
	}
	sort.Strings(paths)
	return paths, nil
}

func Load(paths []string) (*Catalog, []string, error) {
	if len(paths) == 0 {
		return nil, nil, errors.New("at least one catalog path is required")
	}
	merged := &Catalog{TargetGroups: make(map[string]string)}
	var warnings []string
	for _, path := range paths {
		part := &Catalog{}
		md, err := toml.DecodeFile(path, part)
		if err != nil {
			return nil, warnings, fmt.Errorf("%s: decode TOML: %w", path, err)
		}
		if undecoded := md.Undecoded(); len(undecoded) > 0 {
			var keys []string
			for _, key := range undecoded {
				keys = append(keys, key.String())
			}
			sort.Strings(keys)
			return nil, warnings, fmt.Errorf("%s: unknown keys: %s", path, strings.Join(keys, ", "))
		}
		sourceDir := filepath.Dir(path)
		stampSource(part, sourceDir)
		if err := mergeCatalog(merged, part, path); err != nil {
			return nil, warnings, err
		}
		if len(part.Automations) > 0 {
			warnings = append(warnings, fmt.Sprintf("%s: [[automations]] is a compatibility alias; migrate to [[agent_workloads]] with kind = \"schedule\"", path))
		}
	}
	if merged.Version == 0 {
		merged.Version = 1
	}
	convertLegacyAutomations(merged)
	return merged, warnings, nil
}

func stampSource(c *Catalog, sourceDir string) {
	for i := range c.Hosts {
		c.Hosts[i].sourceDir = sourceDir
	}
	for i := range c.Repos {
		c.Repos[i].sourceDir = sourceDir
	}
	for i := range c.CredentialSources {
		c.CredentialSources[i].sourceDir = sourceDir
	}
	for i := range c.AgentRuntimes {
		c.AgentRuntimes[i].sourceDir = sourceDir
	}
	for i := range c.HarnessConfigs {
		c.HarnessConfigs[i].sourceDir = sourceDir
	}
	for i := range c.HarnessExtensions {
		c.HarnessExtensions[i].sourceDir = sourceDir
	}
	for i := range c.SkillPacks {
		c.SkillPacks[i].sourceDir = sourceDir
	}
	for i := range c.Integrations {
		c.Integrations[i].sourceDir = sourceDir
	}
	for i := range c.AgentWorkloads {
		c.AgentWorkloads[i].sourceDir = sourceDir
	}
	for i := range c.Automations {
		c.Automations[i].sourceDir = sourceDir
	}
	for i := range c.AuxServices {
		c.AuxServices[i].sourceDir = sourceDir
	}
	for i := range c.StateStores {
		c.StateStores[i].sourceDir = sourceDir
	}
	for i := range c.DataSources {
		c.DataSources[i].sourceDir = sourceDir
	}
	for i := range c.CredentialProbes {
		c.CredentialProbes[i].sourceDir = sourceDir
	}
}

func mergeCatalog(dst, src *Catalog, path string) error {
	if src.Version != 0 {
		if src.Version != 1 {
			return fmt.Errorf("%s: unsupported catalog version %d", path, src.Version)
		}
		if dst.Version == 0 {
			dst.Version = src.Version
		}
	}
	for k, v := range src.TargetGroups {
		if existing, ok := dst.TargetGroups[k]; ok && existing != v {
			return fmt.Errorf("%s: duplicate target group with different descriptions: %s", path, k)
		}
		dst.TargetGroups[k] = v
	}
	dst.Hosts = append(dst.Hosts, src.Hosts...)
	dst.Repos = append(dst.Repos, src.Repos...)
	dst.CredentialSources = append(dst.CredentialSources, src.CredentialSources...)
	dst.AgentRuntimes = append(dst.AgentRuntimes, src.AgentRuntimes...)
	dst.HarnessConfigs = append(dst.HarnessConfigs, src.HarnessConfigs...)
	dst.HarnessExtensions = append(dst.HarnessExtensions, src.HarnessExtensions...)
	dst.SkillPacks = append(dst.SkillPacks, src.SkillPacks...)
	dst.Integrations = append(dst.Integrations, src.Integrations...)
	dst.AgentWorkloads = append(dst.AgentWorkloads, src.AgentWorkloads...)
	dst.Automations = append(dst.Automations, src.Automations...)
	dst.AuxServices = append(dst.AuxServices, src.AuxServices...)
	dst.StateStores = append(dst.StateStores, src.StateStores...)
	dst.DataSources = append(dst.DataSources, src.DataSources...)
	dst.CredentialProbes = append(dst.CredentialProbes, src.CredentialProbes...)
	return nil
}

func convertLegacyAutomations(c *Catalog) {
	for _, legacy := range c.Automations {
		w := AgentWorkload(legacy)
		legacyKind := w.Kind
		if w.Kind == "" || w.Kind == "cron" || w.Kind == "heartbeat" {
			w.Kind = "schedule"
		}
		if legacyKind != "" && legacyKind != "cron" {
			w.CodexKind = legacyKind
		}
		c.AgentWorkloads = append(c.AgentWorkloads, w)
	}
	c.Automations = nil
}
