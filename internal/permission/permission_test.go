package permission

import (
	"testing"

	"github.com/voxslopuli/mcpeach/internal/config"
)

func TestFilterAllows(t *testing.T) {
	tests := []struct {
		name string
		mode string
		list []string
		tool string
		want bool
	}{
		{"empty mode allows all", "", nil, "anything", true},
		{"allow mode with matching tool", "allow", []string{"a", "b"}, "b", true},
		{"allow mode with non-matching tool", "allow", []string{"a", "b"}, "c", false},
		{"allow mode empty list blocks all", "allow", nil, "a", false},
		{"block mode with matching tool", "block", []string{"a"}, "a", false},
		{"block mode with non-matching tool", "block", []string{"a"}, "b", true},
		{"block mode empty list allows all", "block", nil, "a", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Filter{Mode: tt.mode, List: tt.list}
			if got := f.Allows(tt.tool); got != tt.want {
				t.Errorf("Allows(%q) = %v, want %v", tt.tool, got, tt.want)
			}
		})
	}
}

func TestFilterFromConfig(t *testing.T) {
	f := FromConfig(config.ToolConfig{Mode: "allow", List: []string{"x"}})
	if f.Mode != "allow" || len(f.List) != 1 {
		t.Errorf("FromConfig = %+v, want allow [x]", f)
	}
}

func TestGroupResolve(t *testing.T) {
	// Build a catalog of canonical tools per server.
	catalog := map[string][]string{
		"a": {"a__t1", "a__t2"},
		"b": {"b__t1", "b__t2"},
		"c": {"c__t1"},
	}

	tests := []struct {
		name  string
		group config.GroupConfig
		want  []string
	}{
		{
			"empty group includes nothing",
			config.GroupConfig{},
			nil,
		},
		{
			"included servers",
			config.GroupConfig{IncludedServers: []string{"a", "b"}},
			[]string{"a__t1", "a__t2", "b__t1", "b__t2"},
		},
		{
			"included tools",
			config.GroupConfig{IncludedTools: []string{"a__t1", "c__t1"}},
			[]string{"a__t1", "c__t1"},
		},
		{
			"excluded tools applied last",
			config.GroupConfig{
				IncludedServers: []string{"a", "b"},
				ExcludedTools:   []string{"a__t2"},
			},
			[]string{"a__t1", "b__t1", "b__t2"},
		},
		{
			"included tool not in catalog dropped",
			config.GroupConfig{IncludedTools: []string{"a__t1", "z__nope"}},
			[]string{"a__t1"},
		},
		{
			"included tool with existing server but missing tool dropped",
			config.GroupConfig{IncludedTools: []string{"a__t1", "a__missing"}},
			[]string{"a__t1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveGroup(tt.group, catalog)
			if !equalStrings(got, tt.want) {
				t.Errorf("ResolveGroup = %v, want %v", got, tt.want)
			}
		})
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := map[string]int{}
	for _, s := range a {
		seen[s]++
	}
	for _, s := range b {
		seen[s]--
		if seen[s] < 0 {
			return false
		}
	}
	return true
}
