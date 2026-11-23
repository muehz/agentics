package commandguard

// severityRank orders the blocking tiers. The deny reason names the first
// threat of the highest rank, so a parse limit never fronts a critical rule.
// The four cases are the package's whole severity vocabulary (the rule
// declarations in rules.go and the rows of patterns.go); TestEverySeverityIsNamed
// enumerates every one the package can emit and fails the build the moment a
// fifth string appears here unnamed.
//
// The default is not reachable from any severity this package emits today,
// but it must still fail loud rather than silently rank an unrecognized
// string as harmless: it outranks every named tier, so a mostSevere call
// that ever saw one would surface it as the reason instead of burying it
// under a real critical threat. It does not panic — cmd/hook/main.go
// recovers a panic into a contract.Error, and that outcome renders
// fail-open on some surfaces, so a typo here must never be able to turn a
// deny into a pass at runtime.
func severityRank(severity string) int {
	switch severity {
	case "critical":
		return 3
	case "high":
		return 2
	case "medium":
		return 1
	case severityInfo:
		return 0
	default:
		return 4
	}
}

// mostSevere returns the first threat of the highest severity rank in
// threats, so the deny reason (block mode) and the context message (warn
// mode) name the most severe threat detected, not merely the first one rule
// order happened to report. Ties break by rule order: the first threat at
// the winning rank wins. threats must be non-empty.
func mostSevere(threats []threat) threat {
	best := threats[0]
	for _, t := range threats[1:] {
		if severityRank(t.rule.severity) > severityRank(best.rule.severity) {
			best = t
		}
	}
	return best
}
