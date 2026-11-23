package secretsguard

// Branch coverage for paths the behavior corpus cannot reach: a nil
// diagnostic writer, an unreadable neutral event, a tool input the decoder
// cannot represent, and the mode rule.

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hookspec"
	"github.com/muehz/agentics/hooks/go/internal/hosttest"
)

// A nil diagnostic stream must not panic at either write the handler makes: the
// invalid-mode warning and the report of a finding.
func TestRunNeverPanicsWithNilDiag(t *testing.T) {
	hosttest.ScrubGuardEnv(t)
	t.Setenv("SECRETS_GUARD_MODE", "blcok")
	payload := buildEvent(t, "Read", map[string]any{"file_path": "/tmp/x/.env"})
	got := Run(hookspec.Request{Event: payload, Diag: nil})
	if got.Kind() != "deny" {
		t.Fatalf("kind = %v, want deny for a credential read", got.Kind())
	}
	const wantPrefix = "secrets-guardian: SECRETS_GUARD_MODE=\"blcok\" is not warn or block; using block\n\n"
	if !strings.HasPrefix(got.Payload(), wantPrefix) {
		t.Fatalf("payload = %q, want it to start with the invalid-mode warning and a blank line", got.Payload())
	}
}

func TestUnreadableEventIsAnError(t *testing.T) {
	kind, message := run(t, buildEvent(t, "Read", map[string]any{"path": "\x00"}), nil)
	if kind != "error" || message != "secrets-guardian could not validate normalized input" {
		t.Fatalf("kind=%q message=%q", kind, message)
	}
}

// TestUndecodableToolInputFailsClosed pins the read gate against an input the
// decoder cannot represent. tool_input is agent-controlled, so an integer
// beyond float64's range would otherwise leave the guard with no path to
// classify and let the credential read through.
func TestUndecodableToolInputFailsClosed(t *testing.T) {
	huge := "1" + strings.Repeat("0", 400)
	payload := buildEvent(t, "Read", map[string]any{"file_path": "/tmp/x/.env", "n": json.Number(huge)})
	kind, message := run(t, payload, nil)
	if kind != "deny" || message != "secrets-guardian could not read normalized input" {
		t.Fatalf("kind=%q message=%q, want a deny with the read-stage error", kind, message)
	}
}

// A tool this hook does not inspect is passed before the decode, so an input
// the decoder cannot represent is not an error there: there was nothing to
// gate in the first place.
func TestUndecodableToolInputOnAnUninspectedToolPasses(t *testing.T) {
	huge := "1" + strings.Repeat("0", 400)
	payload := buildEvent(t, "WebFetch", map[string]any{"url": "https://example.test", "n": json.Number(huge)})
	if kind, _ := run(t, payload, nil); kind != "pass" {
		t.Fatalf("kind = %q, want pass", kind)
	}
}

// The mode rule through the whole handler: the environment selects warn or
// block for the whole handler, and an unusable value keeps block.
func TestEnvModeSelectsMode(t *testing.T) {
	payload := buildEvent(t, "Read", map[string]any{"file_path": "/tmp/x/.env"})

	if kind, _ := run(t, payload,
		map[string]string{"SECRETS_GUARD_MODE": "warn"}); kind != "context" {
		t.Errorf("env warn: kind = %q, want context", kind)
	}
	if kind, _ := run(t, payload,
		map[string]string{"SECRETS_GUARD_MODE": "block"}); kind != "deny" {
		t.Errorf("env block: kind = %q, want deny", kind)
	}

	hosttest.ScrubGuardEnv(t)
	t.Setenv("SECRETS_GUARD_MODE", "blcok")
	var diag bytes.Buffer
	outcome := Run(hookspec.Request{
		Provider: "claude-code",
		Hook:     "secrets-guardian",
		Event:    payload,
		Diag:     &diag,
	})
	if outcome.Kind() != "deny" {
		t.Errorf("invalid env mode outcome = %q, want deny", outcome.Kind())
	}
	const wantPrefix = "secrets-guardian: SECRETS_GUARD_MODE=\"blcok\" is not warn or block; using block\n"
	if !strings.HasPrefix(diag.String(), wantPrefix) || !strings.Contains(diag.String(), "credential file read") {
		t.Errorf("diagnostic = %q, want invalid-mode line followed by the deny explanation", diag.String())
	}
}

// The path allowlist is observed through the read decision rather than the
// parser's return shape: entries are trimmed, case-folded, slash-normalized,
// and empty fields do not allow an unrelated credential path.
func TestPathAllowlistNormalizationAtHandlerSeam(t *testing.T) {
	const allowlist = ` Fixtures\Data , ,testdata/,`
	cases := []struct {
		name string
		path string
		want string
	}{
		{"case and backslashes", `C:\ROOT\FIXTURES\DATA\.env`, "pass"},
		{"trimmed slash entry", "/root/testdata/.env", "pass"},
		{"empty field ignored", "/root/other/.env", "deny"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload := buildEvent(t, "Read", map[string]any{"file_path": tc.path})
			kind, _ := run(t, payload,
				map[string]string{"SECRETS_GUARD_ALLOWLIST": allowlist})
			if kind != tc.want {
				t.Errorf("kind = %q, want %q", kind, tc.want)
			}
		})
	}
}
