package commandguard

import (
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/settings"
)

// ruleCategories is the closed set of threat categories the package reports
// today. A rule outside it is a new category, which hooks/README.md must
// document before a rule may carry it.
var ruleCategories = map[string]bool{
	"destructive_file_ops":    true,
	"database_destruction":    true,
	"permission_abuse":        true,
	"network_exfiltration":    true,
	"system_danger":           true,
	"destructive_git_ops":     true,
	"git_hook_bypass":         true,
	"unsafe_publication":      true,
	"unsafe_finding_disposal": true,
	"command_reformulation":   true,
	"command_parse_limit":     true,
	"hook_settings_edit":      true,
	"registry_destruction":    true,
	"service_disruption":      true,
}

// The parsed rules are every declared rule no bank row carries.
const parsedRuleCount = 37

func TestRuleTableIsClosed(t *testing.T) {
	named := map[string]bool{"critical": true, "high": true, "medium": true, severityInfo: true}
	governable := map[settings.Key]bool{}
	for _, key := range settings.Keys {
		governable[key] = key != settings.RequireDeployRecord
	}

	declared := map[ruleID]rule{}
	for _, r := range declaredRules {
		if r.id == "" {
			t.Errorf("rule %+v has no id", r)
		}
		if _, dup := declared[r.id]; dup {
			t.Errorf("rule id %q is declared twice", r.id)
		}
		declared[r.id] = r
		if !ruleCategories[r.category] {
			t.Errorf("rule %q has category %q outside the closed list", r.id, r.category)
		}
		if !named[r.severity] {
			t.Errorf("rule %q has severity %q that severityRank does not name", r.id, r.severity)
		}
		if r.governed != (r.policy != "") {
			t.Errorf("rule %q: governed = %v with policy %q", r.id, r.governed, r.policy)
		}
		if r.governed && !governable[r.policy] {
			t.Errorf("rule %q is governed by %q, which is not a command-guardian setting", r.id, r.policy)
		}
		if r.category == "command_reformulation" && (!r.governed || r.policy != settings.CommandReformulation) {
			t.Errorf("rule %q is advice but is governed by %q (%v), want %q", r.id, r.policy, r.governed, settings.CommandReformulation)
		}
	}

	rows := threatPatternBank()
	carried := map[ruleID]int{}
	for i, row := range rows {
		r := row.rule
		if prev, dup := carried[r.id]; dup {
			t.Errorf("rows %d and %d share rule %q", prev, i, r.id)
		}
		carried[r.id] = i
		if d, ok := declared[r.id]; !ok || d != r {
			t.Errorf("row %d (%s) carries rule %+v, which declaredRules does not hold", i, row.re, r)
		}
		if row.category != r.category || row.severity != r.severity {
			t.Errorf("row %d (%s) is %s/%s, its rule %q is %s/%s", i, row.re, row.category, row.severity, r.id, r.category, r.severity)
		}
	}
	if parsed := len(declaredRules) - len(carried); parsed != parsedRuleCount {
		t.Errorf("%d declared rules are carried by no bank row, want the %d parsed rules", parsed, parsedRuleCount)
	}

	// The MCP rules are declared by tool; each must match its gate's columns.
	mcpDeclared := map[string]rule{
		"create_pull_request":             ruleMCPCreatePullRequest,
		"update_pull_request":             ruleMCPUpdatePullRequest,
		"update_pull_request_draft_state": ruleMCPDraftState,
		"delete_repository":               ruleMCPDeleteRepository,
		"resolve_review_thread":           ruleMCPResolveReviewThread,
		"unresolve_review_thread":         ruleMCPUnresolveReviewThread,
	}
	if len(mcpDeclared) != len(mcpRules) {
		t.Errorf("%d MCP rules are declared, mcpRules has %d", len(mcpDeclared), len(mcpRules))
	}
	for _, gate := range mcpRules {
		r, ok := mcpDeclared[gate.tool]
		if !ok {
			t.Errorf("MCP tool %q has no declared rule", gate.tool)
			continue
		}
		if gate.rule != r {
			t.Errorf("MCP tool %q carries rule %q, want %q", gate.tool, gate.rule.id, r.id)
		}
	}
}

// TestGovernedRulesAreExactlyTheseRules pins which rules a setting governs, by
// literal id. The other governance tests derive their expectation from the
// declaration they check, so only this one fails when a fixed safety rule gains
// a setting or a governed rule changes its key.
func TestGovernedRulesAreExactlyTheseRules(t *testing.T) {
	want := map[ruleID]settings.Key{
		"targeted-rm-advice":                  settings.CommandReformulation,
		"find-delete-advice":                  settings.CommandReformulation,
		"gh-pr-create":                        settings.DraftPRs,
		"gh-pr-ready":                         settings.DraftPRs,
		"gh-stack-submit":                     settings.DraftPRs,
		"gh-stack-link":                       settings.DraftPRs,
		"npm-publish":                         settings.BlockPackagePublish,
		"nuget-publish":                       settings.BlockPackagePublish,
		"mcp-create-pull-request":             settings.DraftPRs,
		"mcp-update-pull-request":             settings.DraftPRs,
		"mcp-update-pull-request-draft-state": settings.DraftPRs,
		"mcp-resolve-review-thread":           settings.BlockReviewThreadChanges,
		"mcp-unresolve-review-thread":         settings.BlockReviewThreadChanges,
		"bank-npm-publish":                    settings.BlockPackagePublish,
		"bank-resolve-review-thread":          settings.BlockReviewThreadChanges,
		"bank-alert-dismissed-state":          settings.BlockSecurityAlertDismissal,
		"bank-alert-dismissed-reason":         settings.BlockSecurityAlertDismissal,
		"bank-find-exec-grep":                 settings.CommandReformulation,
		"bank-find-exec-cat":                  settings.CommandReformulation,
		"bank-find-exec-wc":                   settings.CommandReformulation,
		"bank-find-exec-head":                 settings.CommandReformulation,
		"bank-find-exec-sed":                  settings.CommandReformulation,
		"bank-find-delete":                    settings.CommandReformulation,
		"bank-recursive-force-rm":             settings.CommandReformulation,
	}
	seen := map[ruleID]bool{}
	for _, r := range declaredRules {
		key, governed := want[r.id]
		seen[r.id] = true
		if r.governed != governed || r.policy != key {
			t.Errorf("rule %q: governed = %v by %q, want governed = %v by %q", r.id, r.governed, r.policy, governed, key)
		}
	}
	for id := range want {
		if !seen[id] {
			t.Errorf("rule %q is pinned as governed but no longer declared", id)
		}
	}
}
