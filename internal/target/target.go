package target

import (
	"os"

	"github.com/jnatkins/agentctl/internal/catalog"
)

type Selector struct {
	Host         string
	TargetGroups []string
}

func HostID(c *catalog.Catalog, selector Selector) string {
	if selector.Host != "" {
		return selector.Host
	}
	name, err := os.Hostname()
	if err != nil {
		return ""
	}
	for _, h := range c.Hosts {
		if h.Hostname == name || h.ID == name || contains(h.Aliases, name) {
			return h.ID
		}
	}
	return name
}

func Matches(targets []string, c *catalog.Catalog, selector Selector) bool {
	if len(targets) == 0 {
		return true
	}
	hostID := HostID(c, selector)
	groups := make(map[string]bool)
	for _, g := range selector.TargetGroups {
		groups[g] = true
	}
	if len(groups) == 0 {
		if h, ok := FindHost(c, hostID); ok {
			for _, g := range h.TargetGroups {
				groups[g] = true
			}
		} else if _, ok := c.TargetGroups["all-agent-boxes"]; ok {
			groups["all-agent-boxes"] = true
		}
	}
	for _, t := range targets {
		if t == hostID || groups[t] {
			return true
		}
	}
	return false
}

func FindHost(c *catalog.Catalog, selector string) (catalog.Host, bool) {
	for _, h := range c.Hosts {
		if h.ID == selector || h.Hostname == selector || h.SSHAlias == selector || contains(h.Aliases, selector) {
			return h, true
		}
	}
	return catalog.Host{}, false
}

func HostsForSelector(c *catalog.Catalog, selector string) []catalog.Host {
	var hosts []catalog.Host
	if selector == "" {
		return hosts
	}
	if h, ok := FindHost(c, selector); ok {
		return []catalog.Host{h}
	}
	if _, ok := c.TargetGroups[selector]; ok {
		for _, h := range c.Hosts {
			if contains(h.TargetGroups, selector) {
				hosts = append(hosts, h)
			}
		}
	}
	return hosts
}

func SSHName(h catalog.Host) string {
	if h.SSHAlias != "" {
		return h.SSHAlias
	}
	if h.Hostname != "" {
		return h.Hostname
	}
	return h.ID
}

func contains(values []string, needle string) bool {
	for _, v := range values {
		if v == needle {
			return true
		}
	}
	return false
}
