package commandguard

import "github.com/muehz/agentics/hooks/go/internal/toolname"

// ---------------------------------------------------------------------------
// Publication and destructive gates on GitHub MCP tools
// ---------------------------------------------------------------------------
//
// One policy, two transports. The CLI publication rules above and the rules
// here open the same pull request, so they carry the same category, the same
// severity and the same posture. Only the sentence naming the fix differs,
// because adding a command-line flag is not an action an MCP caller can take.
//
// The rules below read TYPED FIELDS off the decoded tool input, where the CLI
// rules read command text with regexes. That difference is forced, not a
// preference: an MCP call has no command line. `draft` is a JSON boolean, and
// no regex over the stringified payload can tell a `"draft":false` argument
// from the word draft inside a pull request body.
//
// Every rule keys on the tool name's last namespace component, because the
// namespace is the host's choice and differs between hosts; see package
// toolname for why that is a component comparison and not a suffix test.
//
// These rules also cannot be tripped by prose. They read the tool NAME field
// and one typed argument, never scanned text, so a document that names a
// gated tool — this file, or `hooks/README.md` — passes.

// mcpRule is one typed gate on one MCP tool.
type mcpRule struct {
	tool       string
	rule       rule
	suggestion string

	// denies reports whether this call must be gated. A nil func denies every
	// call of the tool, which is the right shape for a tool whose only
	// behavior is the gated one.
	denies func(layers []map[string]any) bool
}

// draftFlipsToReady reports an explicit demotion of the draft flag.
//
// An ABSENT draft field is not a promotion: the consolidated update tool
// carries draft alongside title, body and state, and gating a call that never
// mentions draft would block an ordinary title edit. This is the opposite of
// the create rule, where the field defaults to false and absence IS a live
// pull request.
func draftFlipsToReady(layers []map[string]any) bool {
	draft, present := boolField(layers, "draft")
	return present && !draft
}

// boolField reads a JSON boolean and reports whether the key held one at all.
//
// It looks at each layer in order, the top level first and then inside
// `value`, because the three surfaces do not agree on where a tool's arguments
// end up. Claude Code hands the provider's tool input through verbatim;
// Copilot and Codex normalize theirs into a `value` envelope carrying the raw
// arguments, promoting only a couple of keys to the top level. A rule that
// read the top level alone would see NO draft field on two of the three
// surfaces — reading absence as a live pull request there, and blocking every
// correctly drafted call. The first layer that holds a JSON boolean wins.
//
// The callers pass the lenient layers, Layers and not ProofLayers, so an
// unreadable envelope still yields a boolean at the root.
func boolField(layers []map[string]any, key string) (value, present bool) {
	for _, layer := range layers {
		if v, ok := layer[key].(bool); ok {
			return v, true
		}
	}
	return false, false
}

// mcpRules are the gated GitHub MCP tools, verified against
// github/github-mcp-server v1.10.1.
//
// Deliberately NOT gated, so the reason survives the next reader:
//
//   - The file-deletion tool removes a file by committing the removal to a
//     branch. The commit is in the history and `git revert` undoes it, which
//     makes it ordinary work rather than a human-only action. Gating it would
//     block routine refactors done over MCP, and a guard that blocks ordinary
//     work gets switched off.
//   - The Copilot pull request tool exists only on the remote server, not in
//     the binary a consumer runs locally, and it hands the task to the
//     Copilot coding agent rather than opening a pull request from the
//     session's own changes.
//   - The merge tool: control of merging is left to the repository's own
//     branch protection and review rules, which this hook does not replace.
//     Where a repository has no such rules, or they exempt the caller (an
//     org admin, for example), this hook does not gate the merge.
//   - The state tools close and reopen rather than publish. Closing is
//     reversible, and unsafe_publication is about what cannot be taken back.
var mcpRules = []mcpRule{
	{
		tool:       "create_pull_request",
		rule:       ruleMCPCreatePullRequest,
		suggestion: mcpCreatePullRequestSuggestion,
		denies: func(layers []map[string]any) bool {
			draft, _ := boolField(layers, "draft")
			return !draft
		},
	},
	{
		tool:       "update_pull_request",
		rule:       ruleMCPUpdatePullRequest,
		suggestion: mcpUpdatePullRequestSuggestion,
		denies:     draftFlipsToReady,
	},
	// The granular tool that does nothing but flip the draft flag. It is a
	// second route to the promotion the rule above gates, so it carries the
	// same gate: without it, a server running the granular pull request tools
	// leaves that rule one tool name away from useless.
	{
		tool:       "update_pull_request_draft_state",
		rule:       ruleMCPDraftState,
		suggestion: mcpDraftStateSuggestion,
		denies:     draftFlipsToReady,
	},
	{
		tool: "delete_repository",
		rule: ruleMCPDeleteRepository,
		suggestion: "Do not delete the repository; deletion destroys its issues, pull requests, and history. " +
			"Tell the user that agents cannot delete repositories and ask them to use the repository settings UI if they still want to delete it.",
	},
	// The MCP-native route to the disposal the generic bank gates on the
	// GraphQL mutation name. That row cannot reach these: the tool names are
	// snake_case and share no substring with the camelCase mutation, so
	// without these rows the gate holds on `gh api` and GraphQL passthrough
	// and opens on a direct tool call. Both directions gate, because
	// resolution state is developer-owned either way.
	{
		tool:       "resolve_review_thread",
		rule:       ruleMCPResolveReviewThread,
		suggestion: reviewThreadSuggestion,
	},
	{
		tool:       "unresolve_review_thread",
		rule:       ruleMCPUnresolveReviewThread,
		suggestion: reviewThreadSuggestion,
	},
}

// checkMCP appends at most one threat for a gated MCP tool call.
//
// The match column carries the tool name AS RECEIVED, namespace included, so
// the diagnostic table and the audit log name the tool the host actually
// offered rather than the bare name the rule matched.
func checkMCP(toolName string, layers []map[string]any, report reportFunc) {
	base := toolname.Base(toolName)
	if base == "" {
		return
	}
	for _, gate := range mcpRules {
		if gate.tool != base {
			continue
		}
		if gate.denies != nil && !gate.denies(layers) {
			return
		}
		report(gate.rule, toolName, gate.suggestion)
		return
	}
}
