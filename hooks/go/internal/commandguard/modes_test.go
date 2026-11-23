package commandguard

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hookspec"
	"github.com/muehz/agentics/hooks/go/internal/hosttest"
)

// TestModePrecedence pins the mode rule at the handler seam: with no
// COMMAND_GUARD_MODE the guard blocks, warn and block select their mode, and
// any other value is block with a diagnostic naming it.
func TestModePrecedence(t *testing.T) {
	cases := []struct {
		name     string
		env      map[string]string
		wantKind string
		wantMode string
		wantDiag string
	}{
		{"no env", nil, "deny", "block", ""},
		{"env warn", map[string]string{"COMMAND_GUARD_MODE": "warn"}, "context", "warn", ""},
		{"env block", map[string]string{"COMMAND_GUARD_MODE": "block"}, "deny", "block", ""},
		{"env typo is block with a diagnostic", map[string]string{"COMMAND_GUARD_MODE": "blcok"}, "deny", "block",
			"command-guardian: COMMAND_GUARD_MODE=\"blcok\" is not warn or block; using block\n"},
		{"env typo cannot select warn", map[string]string{"COMMAND_GUARD_MODE": "Warn"}, "deny", "block",
			"command-guardian: COMMAND_GUARD_MODE=\"Warn\" is not warn or block; using block\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, diag, logRoot := runGo(t, tableCase{
				name: tc.name, command: "rm -rf /", env: tc.env,
			})
			if kind := raw.Kind(); kind != tc.wantKind {
				t.Errorf("kind = %q, want %q", kind, tc.wantKind)
			}
			if !strings.HasPrefix(diag, tc.wantDiag) {
				t.Errorf("diag = %q, want prefix %q", diag, tc.wantDiag)
			}
			if tc.wantDiag == "" && strings.Contains(diag, "COMMAND_GUARD_MODE=") {
				t.Errorf("unexpected mode diagnostic: %q", diag)
			}
			data, err := os.ReadFile(guardLogPath(logRoot))
			if err != nil {
				t.Fatal(err)
			}
			if want := `"mode":"` + tc.wantMode + `"`; !bytes.Contains(data, []byte(want)) {
				t.Errorf("guard log = %s, want %s", data, want)
			}
		})
	}
}

// An unrecognized COMMAND_GUARD_MODE must not disable the guard: the whole
// path through Run denies, and the diagnostic names the value.
func TestUnknownEnvModeFailsClosed(t *testing.T) {
	raw, diag, _ := runGo(t, tableCase{
		name: "env_mode_unknown", command: "rm -rf /",
		env: map[string]string{"COMMAND_GUARD_MODE": "blcok"},
	})
	if kind := raw.Kind(); kind != "deny" {
		t.Fatalf("kind = %v, want deny\n%s", kind, raw)
	}
	if !strings.Contains(diag, `COMMAND_GUARD_MODE="blcok" is not warn or block`) {
		t.Fatalf("diag does not name the bad value: %q", diag)
	}
	if !strings.HasPrefix(raw.Payload(), unknownModeWarning+"\n\n") {
		t.Fatalf("reason does not lead with the warning: %q", raw.Payload())
	}
}

// unknownModeWarning is the diagnostic an unusable COMMAND_GUARD_MODE value
// "blcok" produces. Providers discard the diagnostic stream on pass, deny, and
// context outcomes, so the warning must also lead the reported message.
const unknownModeWarning = `command-guardian: COMMAND_GUARD_MODE="blcok" is not warn or block; using block`

// TestUnknownModePrefixesEveryMessage runs each input twice on the same log
// directory, without COMMAND_GUARD_MODE and with an unusable value. The second
// message is the warning, a blank line, then the first message, and the
// outcome kind is the same. The log-failure error keeps its fixed text, as
// error outcomes do in secrets-guardian.
func TestUnknownModePrefixesEveryMessage(t *testing.T) {
	const advice = "find . -exec grep x {} +"
	// A directory below a regular file cannot be created on any host, so the
	// log rows fail the guard.log write without relying on symbolic links.
	brokenLogDir := func(t *testing.T) string {
		parent := filepath.Join(t.TempDir(), "file")
		if err := os.WriteFile(parent, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		return filepath.Join(parent, "logs")
	}
	shell := func(command string) func(*testing.T) hookspec.Event {
		return func(t *testing.T) hookspec.Event { return buildEvent(t, "Bash", commandInput(t, command), "") }
	}
	cases := []struct {
		name      string
		event     func(t *testing.T) hookspec.Event
		logDir    func(t *testing.T) string
		wantKind  string
		check     func(t *testing.T, unset string)
		noWarning bool
	}{
		{name: "validate", event: shell("printf \x00safe"), wantKind: "deny", check: wantExactly(errValidate.Error())},
		{name: "extract", event: outOfRangeIntegerEvent, wantKind: "deny", check: wantExactly(errExtract.Error())},
		{name: "advice", event: shell(advice), wantKind: "context",
			check: wantPrefix("1 threat(s) detected: command_reformulation - ")},
		{name: "log-failure deny", event: shell("rm -rf /"), logDir: brokenLogDir, wantKind: "deny",
			check: func(t *testing.T, unset string) {
				wantPrefix("1 blocking threat(s) detected: destructive_file_ops - ")(t, unset)
				if !strings.Contains(unset, "; guard.log was not written: ") {
					t.Errorf("reason = %q, want the guard.log failure", unset)
				}
			}},
		{name: "log-failure error", event: shell(advice), logDir: brokenLogDir, wantKind: "error",
			check: wantExactly(errLog.Error()), noWarning: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			logDir := t.TempDir()
			if tc.logDir != nil {
				logDir = tc.logDir(t)
			}
			run := func(mode string) hookspec.Outcome {
				hosttest.ScrubGuardEnv(t)
				t.Setenv("COMMAND_GUARD_LOG_DIR", logDir)
				if mode != "" {
					t.Setenv("COMMAND_GUARD_MODE", mode)
				}
				return Run(hookspec.Request{Event: tc.event(t), Diag: &bytes.Buffer{}})
			}
			unset, typo := run(""), run("blcok")
			if unset.Kind() != tc.wantKind || typo.Kind() != tc.wantKind {
				t.Fatalf("kinds = %q and %q, want %q", unset.Kind(), typo.Kind(), tc.wantKind)
			}
			tc.check(t, unset.Payload())
			want := unknownModeWarning + "\n\n" + unset.Payload()
			if tc.noWarning {
				want = unset.Payload()
			}
			if typo.Payload() != want {
				t.Errorf("payload with the unusable mode = %q, want %q", typo.Payload(), want)
			}
		})
	}
}

func wantExactly(want string) func(*testing.T, string) {
	return func(t *testing.T, got string) {
		t.Helper()
		if got != want {
			t.Errorf("payload = %q, want %q", got, want)
		}
	}
}

func wantPrefix(want string) func(*testing.T, string) {
	return func(t *testing.T, got string) {
		t.Helper()
		if !strings.HasPrefix(got, want) {
			t.Errorf("payload = %q, want prefix %q", got, want)
		}
	}
}

// The explicit log directory must win over the event's log root so users can
// keep audit records in a stable location across hosts.
func TestLogDirectoryOverride(t *testing.T) {
	override := filepath.Join(t.TempDir(), "override")
	raw, _, logRoot := runGo(t, tableCase{
		name: "log_directory_override", command: "rm -rf /",
		env: map[string]string{"COMMAND_GUARD_LOG_DIR": override},
	})
	if kind := raw.Kind(); kind != "deny" {
		t.Fatalf("kind = %v, want deny", kind)
	}
	if _, err := os.ReadFile(filepath.Join(override, "guard.log")); err != nil {
		t.Fatalf("the override received no record: %v", err)
	}
	if _, err := os.Stat(guardLogPath(logRoot)); !os.IsNotExist(err) {
		t.Fatalf("the event log root received a record (err=%v)", err)
	}
}

// TestLogDirectoryIsCreatedOnlyForARecord runs the guard with no log root and
// no override from a fresh working directory: a clean invocation creates
// nothing, and a detection creates the hidden directory and writes the
// record there.
func TestLogDirectoryIsCreatedOnlyForARecord(t *testing.T) {
	hosttest.ScrubGuardEnv(t)
	t.Chdir(t.TempDir())
	hidden := filepath.Join(".agentics", "logs", "command-guardian")

	run := func(command string) hookspec.Outcome {
		var diag bytes.Buffer
		raw := Run(hookspec.Request{
			Event: buildEvent(t, "Bash", commandInput(t, command), ""),
			Diag:  &diag,
		})
		return raw
	}

	if kind := run("printf safe").Kind(); kind != "pass" {
		t.Fatalf("clean invocation: kind = %v", kind)
	}
	if _, err := os.Stat(".agentics"); !os.IsNotExist(err) {
		t.Fatalf("a clean invocation created .agentics (err=%v)", err)
	}

	if kind := run("rm -rf /").Kind(); kind != "deny" {
		t.Fatalf("detection: kind = %v", kind)
	}
	data, err := os.ReadFile(filepath.Join(hidden, "guard.log"))
	if err != nil {
		t.Fatalf("the record was not written under %s: %v", hidden, err)
	}
	if !strings.Contains(string(data), `"event":"threats_detected"`) {
		t.Fatalf("unexpected record: %s", data)
	}
}
