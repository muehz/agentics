package settingstest_test

import (
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/settings"
	"github.com/muehz/agentics/hooks/go/internal/settings/settingstest"
)

func TestPolicyWithNoKeyOffEnablesEveryKey(t *testing.T) {
	t.Parallel()
	stub := settingstest.Policy()
	res := stub.Source().Resolve("a")
	if len(res.Invalid) != 0 {
		t.Fatalf("Invalid = %v, want none", res.Invalid)
	}
	for _, k := range settings.Keys {
		if !res.Policy.Enabled(k) {
			t.Errorf("%s is off, want on", k)
		}
	}
}

func TestPolicyTurnsOffOnlyTheGivenKeys(t *testing.T) {
	t.Parallel()
	res := settingstest.Policy(settings.DraftPRs, settings.CommandReformulation).Source().Resolve("a")
	for _, k := range settings.Keys {
		want := k != settings.DraftPRs && k != settings.CommandReformulation
		if got := res.Policy.Enabled(k); got != want {
			t.Errorf("%s enabled = %v, want %v", k, got, want)
		}
	}
}

func TestPolicyWithEveryKeyOff(t *testing.T) {
	t.Parallel()
	res := settingstest.Policy(settings.Keys...).Source().Resolve("a")
	for _, k := range settings.Keys {
		if res.Policy.Enabled(k) {
			t.Errorf("%s is on, want off", k)
		}
	}
}

func TestPolicyDoesNotShareStateBetweenStubs(t *testing.T) {
	t.Parallel()
	off := settingstest.Policy(settings.DraftPRs).Source().Resolve("a")
	on := settingstest.Policy().Source().Resolve("a")
	if off.Policy.Enabled(settings.DraftPRs) || !on.Policy.Enabled(settings.DraftPRs) {
		t.Fatalf("stubs share policy: off=%v on=%v", off.Policy, on.Policy)
	}
}

func TestCallsCountsResolverCallsAcrossDirectories(t *testing.T) {
	t.Parallel()
	stub := settingstest.Policy()
	if got := stub.Calls(); got != 0 {
		t.Fatalf("Calls before any resolve = %d, want 0", got)
	}
	source := stub.Source()
	source.Resolve("a")
	if got := stub.Calls(); got != 1 {
		t.Fatalf("Calls after one directory = %d, want 1", got)
	}
	source.Resolve("a")
	if got := stub.Calls(); got != 1 {
		t.Fatalf("Calls after repeating a directory = %d, want 1", got)
	}
	source.Resolve("b")
	if got := stub.Calls(); got != 2 {
		t.Fatalf("Calls after a second directory = %d, want 2", got)
	}
}

func TestSourceReturnsTheSameSourceEachTime(t *testing.T) {
	t.Parallel()
	stub := settingstest.Policy()
	if stub.Source() != stub.Source() {
		t.Fatal("Source returned different values")
	}
}

func TestWithoutNote(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, message, want string }{
		{"no note", "denied: reason", "denied: reason"},
		{"note", "denied: reason\n\nSettings: policy.draft_prs is on", "denied: reason"},
		{"first marker wins", "a\n\nSetting one\n\nSetting two", "a"},
		{"single newline is not a note", "a\nSetting", "a\nSetting"},
		{"empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := settingstest.WithoutNote(tc.message); got != tc.want {
				t.Fatalf("WithoutNote(%q) = %q, want %q", tc.message, got, tc.want)
			}
		})
	}
}
