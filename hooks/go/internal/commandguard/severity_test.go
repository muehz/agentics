package commandguard

import "testing"

// bespokeReportSeverities are the severity literals passed to report(...)
// outside the generic pattern bank (patterns.go's buildThreatPatterns and
// fallback_patterns.go, which TestEverySeverityIsNamed walks directly
// through threatPatternBank). A source scan of every other report( call
// site in this package (commandguard.go, destructive.go, gh_invocation.go,
// git.go, mcp.go, nuget.go, sql.go, transfers.go) turns up exactly these
// three literals; a fourth one introduced by a future bespoke check will not
// appear here and TestEverySeverityIsNamed will not know to check it, so a
// reviewer adding one must add it to this list by hand.
var bespokeReportSeverities = []string{"critical", "high", severityInfo}

// TestEverySeverityIsNamed is severityRank's loud failure, moved to test
// time (ADR-0004: a switch over a closed set must fail loudly, and a test
// asserts at a seam, not an internal function's return shape — so the
// assertion here is that every severity string this package can ever emit
// is a member of severityRank's named set, not that severityRank returns a
// particular integer). It walks the generic pattern bank row by row and
// checks the small explicit list of bespoke severities gathered above.
func TestEverySeverityIsNamed(t *testing.T) {
	named := map[string]bool{"critical": true, "high": true, "medium": true, severityInfo: true}

	for _, row := range threatPatternBank() {
		if !named[row.severity] {
			t.Errorf("pattern row %s/%s carries unnamed severity %q", row.category, row.re, row.severity)
		}
	}
	for _, severity := range bespokeReportSeverities {
		if !named[severity] {
			t.Errorf("bespoke severity %q is not one of severityRank's named members", severity)
		}
	}
}

// TestSeverityNamesTheReasonEndToEnd (commandguard_test.go) is the seam-level
// coverage for severityRank and mostSevere: it proves, through Run, that a
// deny reason names a critical threat ahead of a high-severity parse-limit
// one rule order reported first. ADR-0004 bars asserting on mostSevere's or
// severityRank's return shape directly, so no such test lives in this file.
