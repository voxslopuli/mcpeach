// Package permission implements allow/block tool filters and group resolution.
package permission

import (
	"sort"
	"strings"

	"github.com/mcpeach/mcpeach/internal/config"
)

// Filter is an allow/block filter over canonical "<server>__<tool>" names.
type Filter struct {
	Mode string   // "allow" | "block" | "" (allow all)
	List []string // canonical tool names
}

// FromConfig builds a Filter from a config ToolConfig.
func FromConfig(tc config.ToolConfig) Filter {
	return Filter{Mode: tc.Mode, List: tc.List}
}

// Allows reports whether the canonical tool name passes the filter.
func (f Filter) Allows(tool string) bool {
	switch f.Mode {
	case "allow":
		return contains(f.List, tool)
	case "block":
		return !contains(f.List, tool)
	default:
		return true
	}
}

// ResolveGroup computes the set of canonical tool names a group exposes, given
// the full catalog of available tools per server. Exclusion is applied last.
// Tools not present in the catalog are dropped.
func ResolveGroup(g config.GroupConfig, catalog map[string][]string) []string {
	// Start from included servers.
	seen := map[string]bool{}
	for _, s := range g.IncludedServers {
		for _, t := range catalog[s] {
			seen[t] = true
		}
	}
	// Add explicitly included tools (if present in catalog).
	for _, t := range g.IncludedTools {
		if _, ok := catalog[serverOf(t)]; ok {
			seen[t] = true
		}
	}
	// Apply exclusions last.
	for _, t := range g.ExcludedTools {
		delete(seen, t)
	}
	out := make([]string, 0, len(seen))
	for t := range seen {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// serverOf returns the server prefix of a canonical "<server>__<tool>" name.
func serverOf(tool string) string {
	if i := strings.Index(tool, "__"); i >= 0 {
		return tool[:i]
	}
	return ""
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
