package secretsguard

import (
	"strings"
	"testing"
)

// A relative glob is resolved only against a working directory that every tool
// argument agrees with; a competing workdir or nested cwd leaves it unknown.
func TestGlobWorkingDirectoryRequiresAgreement(t *testing.T) {
	root := globFixture(t, ".env")
	other := globFixture(t)
	command := "cat .e[n]v"
	for _, tc := range []struct {
		name     string
		input    map[string]any
		fragment []string
	}{
		{"agreeing cwd", map[string]any{"command": command, "cwd": root}, []string{"(dotenv, critical)"}},
		{"competing workdir", map[string]any{"command": command, "workdir": other}, []string{"could not inspect glob", "relative wildcard has no known absolute working directory"}},
		{"competing nested cwd", map[string]any{"command": command, "value": map[string]any{"command": command, "cwd": other}}, []string{"could not inspect glob", "relative wildcard has no known absolute working directory"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			kind, message := run(t, buildEventAt(t, "Bash", tc.input, root), nil)
			if kind != "deny" {
				t.Fatalf("kind = %s, want deny (%s)", kind, message)
			}
			for _, fragment := range tc.fragment {
				if !strings.Contains(message, fragment) {
					t.Errorf("message missing %q:\n%s", fragment, message)
				}
			}
		})
	}
}
