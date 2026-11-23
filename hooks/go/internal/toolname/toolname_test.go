package toolname

import "testing"

func TestBase(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		// A local tool carries no namespace and must survive unchanged, so a
		// caller can normalize every name without branching first.
		{"local tool", "Bash", "Bash"},
		{"local tool with camel case", "NotebookEdit", "NotebookEdit"},
		{"bare server name", "create_pull_request", "create_pull_request"},

		// The four namespace spellings hosts use.
		{"double underscore", "mcp__github__create_pull_request", "create_pull_request"},
		{"dot", "github.create_pull_request", "create_pull_request"},
		{"slash", "github/create_pull_request", "create_pull_request"},
		{"colon", "github:create_pull_request", "create_pull_request"},

		// A single underscore is NOT a separator. Treating it as one would
		// reduce every snake_case server name to its last word, and
		// `request` would then gate a dozen unrelated tools.
		{"single underscore is not a separator", "get_file_contents", "get_file_contents"},

		// The reason this is a component comparison rather than a suffix
		// test. A neighbouring tool whose name ENDS in a gated name must
		// resolve to itself, so no rule holding the shorter name reaches it.
		{"longer neighbour keeps its own name",
			"mcp__github__create_pull_request_with_copilot", "create_pull_request_with_copilot"},
		{"prefixed neighbour keeps its own name",
			"mcp__github__force_create_pull_request", "force_create_pull_request"},

		// A gated name appearing anywhere but last names nothing.
		{"gated name in the namespace position", "mcp__create_pull_request__ping", "ping"},

		// Degenerate inputs must not invent a name a rule could match.
		{"empty", "", ""},
		{"separator only", "mcp__github__", ""},
		{"trailing dot", "github.", ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Base(c.in); got != c.want {
				t.Fatalf("Base(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
