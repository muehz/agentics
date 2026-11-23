package commandguard

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestUnsafeGitCleanAndResetParsing(t *testing.T) {
	for _, tc := range []struct {
		name, command string
		deny          bool
	}{
		{"global clean", "git --no-pager -C repo clean -fdx", true},
		{"clean attached exclude", "git clean -feconfig src", false},
		{"clean separate exclude", "git clean -e '*.log' -f src", false},
		{"clean dry run", "git clean -n -f .", false},
		{"dry run negated", "git clean --dry-run --no-dry-run --force .", true},
		{"dry run last wins", "git clean --no-dry-run --dry-run --force .", false},
		{"clean post marker data is not dry run", "git clean -fx -- --dry-run", true},
		{"incomplete reset exact head is not safe", "xargs -- git reset --hard HEAD", true},
		{"incomplete clean dry run is not safe", "xargs -- git clean -n -f .", true},
		{"incomplete clean target is not safe", "xargs -- git clean -f src", true},
		{"incomplete clean preview is not safe", "xargs -- git clean -n", true},
		{"incomplete reset soft is not safe", "xargs -- git reset --soft HEAD", true},
		{"incomplete status remains safe", "xargs -- git status --short", false},
		{"global reset", "git --config-env=core.editor=GIT_EDITOR reset --hard HEAD~1", true},
		{"exact head", "git -C repo reset --hard HEAD", false},
		{"post marker hard is target", "git reset --hard -- --hard", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := recordCase(t, tableCase{command: tc.command})
			// The deny payload names only the leading category, so the guard
			// log's threats show whether the clean or reset rule fired.
			var log guardLogRecord
			if got.LogExists {
				if err := json.Unmarshal([]byte(got.GuardLog), &log); err != nil {
					t.Fatalf("%q: guard.log is not a semantic record: %v", tc.command, err)
				}
			}
			denied := false
			for _, threat := range log.Threats {
				if threat.Category == "destructive_git_ops" {
					denied = true
				}
			}
			if denied != tc.deny || (denied && got.Outcome.Kind() != "deny") {
				t.Fatalf("%q: destructive_git_ops=%v, want %v (outcome %s, log %s)", tc.command, denied, tc.deny, got.Outcome, got.GuardLog)
			}
		})
	}
}

// TestGitUnreadablePosture pins what command-guardian does with a git line the
// grammar cannot read to its verb. The clean and reset rules read nothing after
// an option they cannot place, so such a line passes them, while the lenient
// walk for -c and --config-env still finds a hooks-path override before an
// unreadable verb or past an unknown option.
func TestGitUnreadablePosture(t *testing.T) {
	for _, tc := range []struct {
		command, want, category string
	}{
		{"git --attr-source HEAD clean -fdx", "pass", ""},
		{"git -c core.hooksPath=x $VERB", "deny", "git_hook_bypass"},
		{"git --unknown -c core.hooksPath=x status", "deny", "git_hook_bypass"},
		{"git --version", "pass", ""},
	} {
		t.Run(tc.command, func(t *testing.T) {
			outcome, diag, _ := runGo(t, tableCase{command: tc.command})
			if kind := outcome.Kind(); kind != tc.want {
				t.Fatalf("%q: kind = %q, want %q (outcome %s, diag %q)", tc.command, kind, tc.want, outcome, diag)
			}
			if tc.category != "" && !strings.Contains(outcome.Payload(), "detected: "+tc.category+" - ") {
				t.Fatalf("%q: payload %q does not name %s", tc.command, outcome.Payload(), tc.category)
			}
		})
	}
}

// TestForcedPushToProtectedBranchIsNotDenied pins that a forced push is not a
// command rule, while the other destructive Git checks still deny.
func TestForcedPushToProtectedBranchIsNotDenied(t *testing.T) {
	for _, tc := range []tableCase{
		{name: "force to main", command: "git push --force origin main", want: "pass"},
		{name: "short force to master", command: "git push -f origin master", want: "pass"},
		{name: "plus refspec to main", command: "git push origin +main", want: "pass"},
		{name: "incomplete push", command: "xargs -- git push origin main", want: "pass"},
		{name: "reset hard to a ref", command: "git reset --hard origin/main", want: "deny"},
		{name: "clean with ignored files", command: "git clean -fdx", want: "deny"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			outcome, diag, _ := runGo(t, tc)
			if kind := outcome.Kind(); kind != tc.want {
				t.Fatalf("%q: kind = %q, want %q (outcome %s, diag %q)", tc.command, kind, tc.want, outcome, diag)
			}
		})
	}
}
