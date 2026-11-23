package surface

import (
	"slices"
	"testing"
)

func TestParseAcceptsExactlyTheFourNames(t *testing.T) {
	cases := []struct {
		name string
		want ID
	}{
		{"antigravity", Antigravity},
		{"claude-code", ClaudeCode},
		{"copilot", Copilot},
		{"codex", Codex},
	}
	for _, c := range cases {
		got, ok := Parse(c.name)
		if !ok || got != c.want {
			t.Errorf("Parse(%q) = (%q, %v), want (%q, true)", c.name, got, ok, c.want)
		}
	}
}

func TestParseRefusesEveryOtherString(t *testing.T) {
	for _, name := range []string{
		"copilot-cli", "Copilot", "CODEX", "Antigravity", "", " claude-code", "codex\x00",
		"flow", "help", "pre-tool", "claude-code ", "claude", "codex\n",
	} {
		got, ok := Parse(name)
		if ok || got != ID("") {
			t.Errorf("Parse(%q) = (%q, %v), want (%q, false)", name, got, ok, ID(""))
		}
	}
}

func TestAllIsTheSortedRosterAndRoundTrips(t *testing.T) {
	all := All()
	if len(all) != 4 {
		t.Fatalf("All() has %d elements, want 4", len(all))
	}
	if !slices.IsSorted(all) {
		t.Errorf("All() = %v, want sorted by name", all)
	}
	for _, id := range all {
		got, ok := Parse(string(id))
		if !ok || got != id {
			t.Errorf("Parse(%q) = (%q, %v), want it back", id, got, ok)
		}
	}
	if slices.Contains(all, ID("")) {
		t.Error("the zero ID is in All()")
	}
}

func TestAllReturnsAFreshSlice(t *testing.T) {
	first := All()
	first[0] = ID("mutated")
	if got := All()[0]; got == ID("mutated") {
		t.Error("All() shares its backing array between calls")
	}
}

func TestLabel(t *testing.T) {
	for id, want := range map[ID]string{Antigravity: "Antigravity", ClaudeCode: "Claude", Copilot: "Copilot", Codex: "Codex"} {
		if got := id.Label(); got != want {
			t.Errorf("%q.Label() = %q, want %q", id, got, want)
		}
	}
}

func TestPreToolEventRoster(t *testing.T) {
	rows := map[ID]string{
		Antigravity: "PreToolUse",
		ClaudeCode:  "PreToolUse",
		Codex:       "PreToolUse",
		Copilot:     "preToolUse",
		ID("bogus"): "",
		ID(""):      "",
	}
	for _, id := range All() {
		if want, ok := rows[id]; !ok || want == "" {
			t.Errorf("the roster has no event row for %q", id)
		}
	}
	for id, want := range rows {
		if got := id.PreToolEvent(); got != want {
			t.Errorf("%q.PreToolEvent() = %q, want %q", id, got, want)
		}
	}
}
