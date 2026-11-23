package commandguard

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/muehz/agentics/hooks/go/internal/guardmode"
	"github.com/muehz/agentics/hooks/go/internal/hookspec"
	"github.com/muehz/agentics/hooks/go/internal/hookutil"
	"github.com/muehz/agentics/hooks/go/internal/hookutil/hookutiltest"
	"github.com/muehz/agentics/hooks/go/internal/hosttest"
	"github.com/muehz/agentics/hooks/go/internal/settings/settingstest"
)

// guardAliasSentence is the message a redirected guard.log path must carry,
// pinned as a literal so a change to the shared wording is seen here.
const guardAliasSentence = "command-guardian refused to append guard.log: a linked path component or a non-regular guard.log would redirect the write"

// TestGuardLogSymlinkLeafIsRefused: a guard.log that is a symlink inside the
// configured log directory must not be followed — the victim keeps its bytes
// and the run still denies the blocking command (fail closed) rather than
// letting an unwritable log turn a deny into a pass. The direct
// writeGuardLog call pins the sentinel error; runGo pins the outcome the
// whole handler surfaces for it in block mode.
func TestGuardLogSymlinkLeafIsRefused(t *testing.T) {
	dir := t.TempDir()
	victim := filepath.Join(t.TempDir(), "victim")
	if err := os.WriteFile(victim, []byte("original\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hookutiltest.SymlinkOrSkip(t, victim, filepath.Join(dir, "guard.log"))

	t.Setenv("COMMAND_GUARD_LOG_DIR", dir)
	direct := writeGuardLog("", guardmode.Block.String(), "Bash", []threat{
		{rule: rule{category: "destructive_file_ops", severity: "blocking"}, match: "rm -rf /", suggestion: "avoid"},
	}, nil)
	if !errors.Is(direct, hookutil.ErrLogAlias) {
		t.Fatalf("writeGuardLog error = %v, want hookutil.ErrLogAlias", direct)
	}
	if direct.Error() != guardAliasSentence {
		t.Fatalf("writeGuardLog error = %q, want %q", direct.Error(), guardAliasSentence)
	}

	outcome, _, _ := runGo(t, tableCase{command: "rm -rf /", env: map[string]string{"COMMAND_GUARD_LOG_DIR": dir}})
	if outcome.Kind() != "deny" {
		t.Fatalf("kind = %q (%s), want deny (fail closed)", outcome.Kind(), outcome.Payload())
	}
	if !strings.HasPrefix(outcome.Payload(), denyRootReason) {
		t.Fatalf("payload = %q, want the usual deny reason as a prefix", outcome.Payload())
	}
	if !strings.Contains(outcome.Payload(), "guard.log was not written") {
		t.Fatalf("payload = %q, want the log-write failure folded in", outcome.Payload())
	}

	raw, err := os.ReadFile(victim)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "original\n" {
		t.Fatalf("the symlink target was written: %q", raw)
	}
}

// TestGuardLogSymlinkLeafWarnModeStillErrors: outside block mode there is no
// deny outcome to fold the write failure into, so a symlinked guard.log in
// warn mode keeps surfacing the plain errLog error outcome.
func TestGuardLogSymlinkLeafWarnModeStillErrors(t *testing.T) {
	dir := t.TempDir()
	victim := filepath.Join(t.TempDir(), "victim")
	if err := os.WriteFile(victim, []byte("original\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hookutiltest.SymlinkOrSkip(t, victim, filepath.Join(dir, "guard.log"))

	outcome, _, _ := runGo(t, tableCase{command: "rm -rf /", env: map[string]string{"COMMAND_GUARD_LOG_DIR": dir, "COMMAND_GUARD_MODE": "warn"}})
	if outcome.Kind() != "error" || outcome.Payload() != errLog.Error() {
		t.Fatalf("kind = %q (%s), want the errLog error outcome", outcome.Kind(), outcome.Payload())
	}

	raw, err := os.ReadFile(victim)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "original\n" {
		t.Fatalf("the symlink target was written: %q", raw)
	}
}

// TestGuardLogRelativeSymlinkedParentIsRefused: a symlinked `.agentics` under the
// process working directory is the parent-link form of the same redirect,
// reached only through the relative fallback when no log root and no
// COMMAND_GUARD_LOG_DIR are configured.
func TestGuardLogRelativeSymlinkedParentIsRefused(t *testing.T) {
	cwd := t.TempDir()
	t.Chdir(cwd)
	hosttest.ScrubGuardEnv(t)
	outside := t.TempDir()
	hookutiltest.SymlinkOrSkip(t, outside, filepath.Join(cwd, ".agentics"))

	// An empty log_root exercises the relative ".agentics/logs/command-guardian"
	// fallback under the (chdir'd) working directory.
	payload := buildEvent(t, "Bash", commandInput(t, "rm -rf /"), "")
	var diag bytes.Buffer
	outcome := Run(hookspec.Request{
		Provider: "claude-code",
		Hook:     "command-guardian",
		Event:    payload,
		Diag:     &diag,
	})
	if outcome.Kind() != "deny" {
		t.Fatalf("kind = %q (%s), want deny (fail closed)", outcome.Kind(), outcome.Payload())
	}
	if !strings.HasPrefix(outcome.Payload(), denyRootReason) {
		t.Fatalf("payload = %q, want the usual deny reason as a prefix", outcome.Payload())
	}
	if !strings.Contains(outcome.Payload(), "guard.log was not written") {
		t.Fatalf("payload = %q, want the log-write failure folded in", outcome.Payload())
	}
	if !strings.Contains(diag.String(), guardAliasSentence) {
		t.Fatalf("diagnostics do not carry the alias sentinel text: %s", diag.String())
	}
	if _, err := os.Stat(filepath.Join(outside, "logs", "command-guardian", "guard.log")); !os.IsNotExist(err) {
		t.Fatalf("guard.log was written through the parent link (err=%v)", err)
	}
}

// TestGuardLogRecordRoundTrips writes a threat whose match carries every
// character the log must escape — a newline, a carriage return, a tab, a
// double quote, a backslash and a 0x01 byte — and reads the record back
// through encoding/json. The field set is the documented one.
func TestGuardLogRecordRoundTrips(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "logs")
	match := "rm -rf /\n\r\t\"\\\x01 <b>&"
	threats := []threat{
		{rule{category: "destructive_file_ops", severity: "critical"}, match, "Use targeted 'rm' on specific paths instead of root"},
		{rule{category: "command_reformulation", severity: "info"}, "rm -rf ", "In a git repo, prefer 'git clean -f <path>'"},
	}
	t.Setenv("COMMAND_GUARD_LOG_DIR", dir)
	if err := writeGuardLog("", "block", "Bash", threats, nil); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "guard.log"))
	if err != nil {
		t.Fatal(err)
	}
	line := string(data)
	if strings.Count(line, "\n") != 1 || !strings.HasSuffix(line, "\n") {
		t.Fatalf("a record is exactly one line: %q", line)
	}
	// HTML characters are written as themselves, never as escapes.
	escapedLT := "\\" + "u003c" // the six-character escape, assembled so this file never contains it
	if strings.Contains(line, escapedLT) || !strings.Contains(line, "<b>&") {
		t.Fatalf("HTML characters must not be escaped: %q", line)
	}

	var record struct {
		Timestamp   string `json:"timestamp"`
		Event       string `json:"event"`
		Mode        string `json:"mode"`
		Tool        string `json:"tool"`
		ThreatCount int    `json:"threat_count"`
		Threats     []struct {
			Category   string `json:"category"`
			Severity   string `json:"severity"`
			Match      string `json:"match"`
			Suggestion string `json:"suggestion"`
		} `json:"threats"`
	}
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatalf("the record does not round-trip: %v\n%s", err, data)
	}
	if record.Event != "threats_detected" || record.Mode != "block" || record.Tool != "Bash" || record.ThreatCount != 2 {
		t.Errorf("header fields = %+v", record)
	}
	if len(record.Threats) != 2 || record.Threats[0].Match != match {
		t.Errorf("threats = %+v, want the match back byte for byte", record.Threats)
	}
	if len(record.Timestamp) != len("2006-01-02T15:04:05Z") {
		t.Errorf("timestamp = %q", record.Timestamp)
	}

	// The key order is the documented one.
	var keys []string
	dec := json.NewDecoder(strings.NewReader(line))
	if _, err := dec.Token(); err != nil { // the opening brace
		t.Fatal(err)
	}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			t.Fatal(err)
		}
		keys = append(keys, tok.(string))
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			t.Fatal(err)
		}
	}
	if want := "timestamp event mode tool threat_count threats"; strings.Join(keys, " ") != want {
		t.Errorf("key order = %v, want %s", keys, want)
	}
}

// A second detection appends a second line; the file is JSON Lines.
func TestGuardLogAppends(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("COMMAND_GUARD_LOG_DIR", dir)
	for range 2 {
		if err := writeGuardLog("", "warn", "Bash", []threat{{rule{category: "system_danger", severity: "high"}, "sudo ", "Avoid 'sudo'"}}, nil); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(filepath.Join(dir, "guard.log"))
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(data), "\n"); n != 2 {
		t.Fatalf("expected 2 records, found %d newlines:\n%s", n, data)
	}
}

// The deny and context reasons are built from the rule's category and
// suggestion, never from the match, so a match carrying a tab or anything
// else cannot alter them.
func TestReasonIsBuiltFromRuleColumns(t *testing.T) {
	raw, _, logRoot := runGo(t, tableCase{name: "tab_inside_match", command: "gh pr ready\t7"})
	outcome := raw
	want := "1 blocking threat(s) detected: unsafe_publication - Do not mark the pull request ready for review. Tell the user that agents cannot publish it as ready and ask them to mark it ready in the pull request UI or with 'gh pr ready'."
	if got := settingstest.WithoutNote(outcome.Payload()); got != want {
		t.Fatalf("reason = %q, want %q", got, want)
	}
	data, err := os.ReadFile(guardLogPath(logRoot))
	if err != nil {
		t.Fatal(err)
	}
	var record struct {
		Threats []struct {
			Match string `json:"match"`
		} `json:"threats"`
	}
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	if got := record.Threats[0].Match; got != "gh pr ready\t7" {
		t.Fatalf("match = %q, want a raw tab in the captured command", got)
	}
}

// TestThreatTablePadsByCharacter: the MATCH column is padded to 40
// characters, so a multi-byte match keeps the SUGGESTION column aligned with
// an ASCII one.
func TestThreatTablePadsByCharacter(t *testing.T) {
	var ascii, wide strings.Builder
	writeThreatTable(&ascii, "Bash", []threat{{rule{category: "network_exfiltration", severity: "critical"}, "wget x | sh", "Download the script first"}})
	writeThreatTable(&wide, "Bash", []threat{{rule{category: "network_exfiltration", severity: "critical"}, "wget é | sh", "Download the script first"}})
	row := func(table string) string {
		for _, line := range strings.Split(table, "\n") {
			if strings.HasPrefix(line, "  network_exfiltration") {
				return line
			}
		}
		t.Fatalf("no threat row in\n%s", table)
		return ""
	}
	asciiRow, wideRow := row(ascii.String()), row(wide.String())
	a, w := strings.Index(asciiRow, "Download"), strings.Index(wideRow, "Download")
	// The suggestion starts one byte later after the two-byte é and at the
	// same character offset.
	if w != a+1 || utf8.RuneCountInString(wideRow[:w]) != utf8.RuneCountInString(asciiRow[:a]) {
		t.Fatalf("suggestion offsets: ascii %d bytes, wide %d bytes\n%s\n%s", a, w, asciiRow, wideRow)
	}
}
