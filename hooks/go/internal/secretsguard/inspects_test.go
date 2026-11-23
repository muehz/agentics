package secretsguard

import (
	"sort"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/toolcall"
)

// TestInspectsEveryRosterName requires Inspects to answer true for every name
// in the four rosters, for an MCP name under any namespace prefix, and false for
// names that only resemble a roster name.
func TestInspectsEveryRosterName(t *testing.T) {
	var local []string
	for name := range inspectedTools {
		local = append(local, name)
	}
	for _, name := range []string{"Write", "Edit", "MultiEdit", "NotebookEdit"} {
		if !toolcall.IsFileWrite(name) {
			t.Errorf("toolcall.IsFileWrite(%q) = false for a write roster name", name)
		}
		local = append(local, name)
	}
	var remote []string
	for _, roster := range []map[string]bool{mcpReadTools, mcpWriteTools} {
		for name := range roster {
			remote = append(remote, name)
		}
	}
	sort.Strings(local)
	sort.Strings(remote)
	if len(local) == 0 || len(remote) == 0 {
		t.Fatal("a roster is empty")
	}

	for _, name := range local {
		if !Inspects(name) {
			t.Errorf("Inspects(%q) = false for a local roster name", name)
		}
	}
	for _, name := range remote {
		for _, spelled := range []string{name, "mcp__server__" + name, "host." + name} {
			if !Inspects(spelled) {
				t.Errorf("Inspects(%q) = false for an MCP roster name", spelled)
			}
		}
	}

	for _, name := range []string{"host.Bash", "bash", "write", "mcp__x__create_pull_request"} {
		if Inspects(name) {
			t.Errorf("Inspects(%q) = true, want false", name)
		}
	}
}
