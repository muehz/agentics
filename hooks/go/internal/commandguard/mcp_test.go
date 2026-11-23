package commandguard

import (
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/settings/settingstest"
	"github.com/muehz/agentics/hooks/go/internal/toolname"
)

// TestMCPGates pins the typed GitHub MCP rules.
//
// These cases are deliberately NOT in tableCases so this focused suite can
// organize the typed GitHub MCP rules by operation and assert them directly
// instead of duplicating the cases in the broader handler corpus.
//
// Every case runs through runGo, the same entry point the pinned bank uses, so
// a rule is exercised with the real dispatch, allowlist and mode handling
// rather than by calling checkMCP directly.
func TestMCPGates(t *testing.T) {
	cases := []tableCase{
		// --- create ---------------------------------------------------------
		// An ABSENT draft field is the live-pull-request case: the server
		// defaults it to false. This is the row the whole rule exists for.
		{name: "create_pr_without_draft", tool: "mcp__github__create_pull_request",
			input: `{"owner":"o","repo":"r","title":"t","head":"h","base":"main"}`,
			want:  "deny"},
		{name: "create_pr_draft_false", tool: "mcp__github__create_pull_request",
			input: `{"owner":"o","repo":"r","title":"t","head":"h","base":"main","draft":false}`,
			want:  "deny"},
		{name: "create_pr_draft_true_passes", tool: "mcp__github__create_pull_request",
			input: `{"owner":"o","repo":"r","title":"t","head":"h","base":"main","draft":true}`,
			want:  "pass"},
		// A string "true" is not a JSON boolean. The gate reads a typed field
		// and must not accept a look-alike.
		{name: "create_pr_draft_string_does_not_vouch", tool: "mcp__github__create_pull_request",
			input: `{"owner":"o","repo":"r","title":"t","head":"h","base":"main","draft":"true"}`,
			want:  "deny"},
		// A sibling field naming the flag must not vouch for the call, the
		// same property the CLI rules hold.
		{name: "create_pr_body_mentioning_draft_does_not_vouch", tool: "mcp__github__create_pull_request",
			input: `{"owner":"o","repo":"r","title":"t","head":"h","base":"main","body":"remember draft: true"}`,
			want:  "deny"},

		// --- update ---------------------------------------------------------
		// An absent draft field changes no draft state, so an ordinary title
		// edit must pass. This is the case that keeps the rule usable.
		{name: "update_pr_title_only_passes", tool: "mcp__github__update_pull_request",
			input: `{"owner":"o","repo":"r","pullNumber":1,"title":"new title"}`,
			want:  "pass"},
		{name: "update_pr_draft_false_denies", tool: "mcp__github__update_pull_request",
			input: `{"owner":"o","repo":"r","pullNumber":1,"draft":false}`,
			want:  "deny"},
		// Going back TO draft is not a publication and must pass.
		{name: "update_pr_draft_true_passes", tool: "mcp__github__update_pull_request",
			input: `{"owner":"o","repo":"r","pullNumber":1,"draft":true}`,
			want:  "pass"},
		{name: "update_pr_draft_state_false_denies", tool: "mcp__github__update_pull_request_draft_state",
			input: `{"owner":"o","repo":"r","pullNumber":1,"draft":false}`,
			want:  "deny"},
		{name: "update_pr_draft_state_true_passes", tool: "mcp__github__update_pull_request_draft_state",
			input: `{"owner":"o","repo":"r","pullNumber":1,"draft":true}`,
			want:  "pass"},

		// --- unconditional gates --------------------------------------------
		{name: "delete_repository_denies", tool: "mcp__github__delete_repository",
			input: `{"owner":"o","repo":"r"}`, want: "deny"},
		{name: "resolve_thread_denies", tool: "mcp__github__resolve_review_thread",
			input: `{"threadID":"PRRT_x"}`, want: "deny"},
		{name: "unresolve_thread_denies", tool: "mcp__github__unresolve_review_thread",
			input: `{"threadID":"PRRT_x"}`, want: "deny"},

		// --- name matching --------------------------------------------------
		// A tool whose name CONTAINS a gated name in another position is a
		// different tool and must not inherit the gate.
		{name: "longer_neighbour_is_not_gated", tool: "mcp__github__create_pull_request_with_copilot",
			input: `{"owner":"o","repo":"r","problem_statement":"fix it"}`, want: "pass"},
		{name: "gated_name_in_namespace_position_is_not_gated", tool: "mcp__delete_repository__ping",
			input: `{"owner":"o"}`, want: "pass"},
		// Hosts spell the namespace differently; every spelling must gate.
		{name: "dot_namespace_gates", tool: "github.delete_repository",
			input: `{"owner":"o","repo":"r","pullNumber":1}`, want: "deny"},
		{name: "slash_namespace_gates", tool: "github/delete_repository",
			input: `{"owner":"o","repo":"r","pullNumber":1}`, want: "deny"},
		{name: "unnamespaced_gates", tool: "delete_repository",
			input: `{"owner":"o","repo":"r","pullNumber":1}`, want: "deny"},

		// --- deliberately ungated -------------------------------------------
		// Recorded as behavior so a future reader sees the decision rather
		// than an omission: a file deletion is a revertible commit.
		{name: "delete_file_is_not_gated", tool: "mcp__github__delete_file",
			input: `{"owner":"o","repo":"r","path":"a.txt","message":"m","branch":"b"}`,
			want:  "pass"},
		{name: "read_tool_is_not_gated", tool: "mcp__github__get_file_contents",
			input: `{"owner":"o","repo":"r","path":"a.txt"}`, want: "pass"},

		// --- the `value` envelope -------------------------------------------
		// Copilot and Codex normalize a tool's arguments into {"value": ...}
		// rather than passing them through. A rule that read the top level
		// alone would find no draft field there — denying every correctly
		// drafted call on two of the three surfaces, and reading a genuine
		// omission and a normalization shape as the same thing.
		{name: "enveloped_create_without_draft_denies", tool: "mcp__github__create_pull_request",
			input: `{"value":{"owner":"o","repo":"r","title":"t","head":"h","base":"main"}}`,
			want:  "deny"},
		{name: "enveloped_create_with_draft_passes", tool: "mcp__github__create_pull_request",
			input: `{"value":{"owner":"o","repo":"r","title":"t","head":"h","base":"main","draft":true}}`,
			want:  "pass"},
		{name: "enveloped_update_draft_false_denies", tool: "mcp__github__update_pull_request",
			input: `{"value":{"owner":"o","repo":"r","pullNumber":1,"draft":false}}`,
			want:  "deny"},
		{name: "enveloped_update_title_only_passes", tool: "mcp__github__update_pull_request",
			input: `{"value":{"owner":"o","repo":"r","pullNumber":1,"title":"new"}}`,
			want:  "pass"},
		// A non-object envelope must not panic or vouch for anything.
		{name: "string_envelope_still_denies", tool: "mcp__github__create_pull_request",
			input: `{"value":"draft: true"}`, want: "deny"},

		// --- mode and allowlist still apply ---------------------------------
		{name: "warn_mode_reports_instead_of_denying", tool: "mcp__github__delete_repository",
			input: `{"owner":"o","repo":"r","pullNumber":1}`, env: map[string]string{"COMMAND_GUARD_MODE": "warn"}, want: "context"},
		{name: "allowlist_skips_the_gate", tool: "mcp__github__delete_repository",
			input: `{"owner":"o","repo":"r","pullNumber":1}`,
			env:   map[string]string{"COMMAND_GUARD_ALLOWLIST": "delete_repository"}, want: "pass"},
		{name: "skip_env_disables_the_gate", tool: "mcp__github__delete_repository",
			input: `{"owner":"o","repo":"r","pullNumber":1}`,
			env:   map[string]string{"SKIP_COMMAND_GUARD": "true"}, want: "pass"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			raw, diagText, _ := runGo(t, c)
			outcome := raw
			kind := outcome.Kind()
			if kind != c.want {
				t.Fatalf("expected %s, got %s (outcome %s, diag %q)", c.want, kind, raw, diagText)
			}
		})
	}
}

// TestMCPSeveritiesAndCategories pins the reason line each gate produces.
//
// The deny reason is built from the rule's category and suggestion, so this
// also proves the severity tier: only critical/high/medium deny, and the
// count prefix names how many blocking rows the payload produced.
func TestMCPSeveritiesAndCategories(t *testing.T) {
	cases := []struct {
		name       string
		tool       string
		input      string
		wantReason string
	}{
		{"create is a high publication threat", "mcp__github__create_pull_request",
			`{"owner":"o","repo":"r","title":"t","head":"h","base":"main"}`,
			"1 blocking threat(s) detected: unsafe_publication - Set draft: true. If the user wants the pull request ready for review, tell them that agents cannot publish it as ready and ask them to mark it ready in the pull request UI or with 'gh pr ready'."},
		{"consolidated draft promotion is a high publication threat", "mcp__github__update_pull_request",
			`{"owner":"o","repo":"r","pullNumber":1,"draft":false}`,
			"1 blocking threat(s) detected: unsafe_publication - Do not set draft to false. Every other field of this tool is ungated. If the user wants the pull request ready for review, tell them that agents cannot publish it as ready and ask them to mark it ready in the pull request UI or with 'gh pr ready'."},
		{"granular draft promotion is a high publication threat", "mcp__github__update_pull_request_draft_state",
			`{"owner":"o","repo":"r","pullNumber":1,"draft":false}`,
			"1 blocking threat(s) detected: unsafe_publication - Leave the pull request's draft state unchanged. If the user wants the pull request ready for review, tell them that agents cannot publish it as ready and ask them to mark it ready in the pull request UI or with 'gh pr ready'."},
		{"repository deletion is a destructive git op", "mcp__github__delete_repository",
			`{"owner":"o","repo":"r"}`,
			"1 blocking threat(s) detected: destructive_git_ops - Do not delete the repository; deletion destroys its issues, pull requests, and history. Tell the user that agents cannot delete repositories and ask them to use the repository settings UI if they still want to delete it."},
		{"thread resolution is finding disposal", "mcp__github__resolve_review_thread",
			`{"threadID":"PRRT_x"}`,
			"1 blocking threat(s) detected: unsafe_finding_disposal - Leave the review thread's resolution state unchanged and draft the reply. Tell the user that agents cannot change review-thread resolution state and ask them to resolve or reopen the thread in the pull request UI, as appropriate."},
		{"thread unresolution is finding disposal", "mcp__github__unresolve_review_thread",
			`{"threadID":"PRRT_x"}`,
			"1 blocking threat(s) detected: unsafe_finding_disposal - Leave the review thread's resolution state unchanged and draft the reply. Tell the user that agents cannot change review-thread resolution state and ask them to resolve or reopen the thread in the pull request UI, as appropriate."},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			raw, _, _ := runGo(t, tableCase{tool: c.tool, input: c.input, want: "deny"})
			outcome := raw
			reason := settingstest.WithoutNote(outcome.Payload())
			if reason != c.wantReason {
				t.Fatalf("reason = %q, want %q", reason, c.wantReason)
			}
		})
	}
}

// TestMCPMatchColumnCarriesTheHostName proves the audit log and the diagnostic
// table name the tool the HOST offered, not the bare name the rule matched.
// A reader chasing a block needs the string their host actually printed.
func TestMCPMatchColumnCarriesTheHostName(t *testing.T) {
	const hostName = "mcp__github__delete_repository"
	_, diagText, _ := runGo(t, tableCase{
		tool:  hostName,
		input: `{"owner":"o","repo":"r","pullNumber":1}`,
		want:  "deny",
	})
	if !strings.Contains(diagText, hostName) {
		t.Fatalf("diagnostic table does not name the host tool %q: %q", hostName, diagText)
	}
}

// TestMCPRuleTableIsWellFormed guards the table's own invariants: a duplicate
// tool row would make the second one dead (checkMCP returns at the first
// match), and a severity outside the blocking tiers would silently downgrade a
// gate to advisory.
func TestMCPRuleTableIsWellFormed(t *testing.T) {
	seen := map[string]bool{}
	for _, gate := range mcpRules {
		if gate.tool == "" {
			t.Fatalf("a rule has no tool name")
		}
		if seen[gate.tool] {
			t.Fatalf("duplicate rule for %q: the later row is unreachable", gate.tool)
		}
		seen[gate.tool] = true

		if gate.tool != toolname.Base(gate.tool) {
			t.Fatalf("rule %q is not a bare server-side name; it can never match", gate.tool)
		}
		if gate.rule.severity != "critical" && gate.rule.severity != "high" {
			t.Fatalf("rule %q has severity %q, which does not block", gate.tool, gate.rule.severity)
		}
		if gate.suggestion == "" {
			t.Fatalf("rule %q has no suggestion; a deny with no alternative just gets retried", gate.tool)
		}
	}
}
