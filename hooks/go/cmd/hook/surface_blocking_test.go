package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hosttest"
	"github.com/muehz/agentics/hooks/go/internal/surface"
)

// blockingForm says whether a surface's host treats a hook's response as
// blocking, and returns the reason the host shows. Each reader states one
// surface's Hook protocol from docs/surfaces/. They are written here, not
// taken from hosttest.Read, so the statement of each protocol does not share a
// reader with the tests it checks.
type blockingForm func(exit int, stdout, stderr string) (reason string, blocking bool)

// blockingForms is the only per-surface logic in this file. A surface with no
// row fails the test, so a new surface cannot ship without stating how its
// host reads a deny.
var blockingForms = map[surface.ID]blockingForm{
	// Exit 0 with one flat decision document: decision deny and a reason.
	surface.Antigravity: func(exit int, stdout, _ string) (string, bool) {
		if exit != 0 {
			return "", false
		}
		var document struct {
			Decision string `json:"decision"`
			Reason   string `json:"reason"`
		}
		if !decodeOneDocument(stdout, &document) {
			return "", false
		}
		return document.Reason, document.Decision == "deny" && document.Reason != ""
	},
	// Exit 2 with nothing on stdout; the reason is stderr.
	surface.ClaudeCode: func(exit int, stdout, stderr string) (string, bool) {
		return stderr, exit == 2 && stdout == "" && strings.TrimSpace(stderr) != ""
	},
	// Exit 0 with one flat document: permissionDecision deny and a reason.
	surface.Copilot: func(exit int, stdout, _ string) (string, bool) {
		if exit != 0 {
			return "", false
		}
		var document struct {
			Decision string `json:"permissionDecision"`
			Reason   string `json:"permissionDecisionReason"`
		}
		if !decodeOneDocument(stdout, &document) {
			return "", false
		}
		return document.Reason, document.Decision == "deny" && document.Reason != ""
	},
	// Exit 0 with one hookSpecificOutput document naming PreToolUse.
	surface.Codex: func(exit int, stdout, _ string) (string, bool) {
		if exit != 0 {
			return "", false
		}
		var document struct {
			Output struct {
				Event    string `json:"hookEventName"`
				Decision string `json:"permissionDecision"`
				Reason   string `json:"permissionDecisionReason"`
			} `json:"hookSpecificOutput"`
		}
		if !decodeOneDocument(stdout, &document) {
			return "", false
		}
		out := document.Output
		return out.Reason, out.Event == "PreToolUse" && out.Decision == "deny" && out.Reason != ""
	},
}

// decodeOneDocument decodes stdout as exactly one JSON object, with no
// trailing data.
func decodeOneDocument(stdout string, into any) bool {
	decoder := json.NewDecoder(bytes.NewReader([]byte(stdout)))
	if err := decoder.Decode(into); err != nil {
		return false
	}
	return !decoder.More()
}

func TestEverySurfaceRendersADenyAsBlocking(t *testing.T) {
	hosttest.ScrubGuardEnv(t)
	for _, id := range surface.All() {
		read, ok := blockingForms[id]
		if !ok {
			t.Errorf("surface %q has no blocking-form reader: state how its host reads a deny", id)
			continue
		}
		name := string(id)
		for _, hook := range []string{"pre-tool", "command-guardian"} {
			t.Run(name+"/"+hook, func(t *testing.T) {
				payload := string(hosttest.Stdin(id, hosttest.Call{
					Tool:  "Bash",
					Input: map[string]any{"command": "rm -rf /"},
					Cwd:   t.TempDir(),
				}))
				argv := []string{name, hook}
				if hook == "command-guardian" {
					argv = append(argv, legalArgs(id, hook)...)
				}
				code, stdout, stderr := invoke(t, payload, argv...)
				reason, blocking := read(code, stdout, stderr)
				if !blocking {
					t.Fatalf("a deny is not blocking on %s: exit=%d stdout=%q stderr=%q", name, code, stdout, stderr)
				}
				if !strings.Contains(reason, "destructive_file_ops") {
					t.Fatalf("blocking reason on %s = %q, want it to contain destructive_file_ops", name, reason)
				}
			})
		}
	}
}

func TestUnknownSurfaceIsAUsageError(t *testing.T) {
	for _, name := range []string{"copilot-cli", "Copilot", "CODEX", "", " claude-code"} {
		t.Run("["+name+"]", func(t *testing.T) {
			code, stdout, stderr := invoke(t, "", name, "pre-tool")
			if code != 1 || stdout != "" || !strings.HasPrefix(stderr, "agx: unknown provider") {
				t.Fatalf("exit=%d stdout=%q stderr=%q, want exit 1, empty stdout, stderr starting with %q",
					code, stdout, stderr, "agx: unknown provider")
			}
		})
	}
}
