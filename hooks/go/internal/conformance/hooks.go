// Package conformance audits agentics's hook engine: the dispatcher's ASCII-only
// requirement, the provider hook configs (RunSchema), the hooks/go module's
// hygiene gates (RunModuleGates), and the committed-binary staleness guard
// (RunFreshness). The committed agx launcher bin/agx.exe goes through the
// same reproducibility gates as the hook binaries, and the freshness area
// also enforces the bin-launchers rule on bin/.
//
// Audit composes those areas and returns a versioned per-area inventory that
// keeps the findings gathered before an operational error. When every area
// completes, it drops the rebuild findings that a stale embedded source digest
// already explains; after an operational error it returns the raw findings.
// Probing live provider CLIs is not done here.
package conformance

import (
	"slices"

	"github.com/muehz/agentics/hooks/go/internal/hookbuild"
)

// AuditArea is one ordered part of a hook audit.
type AuditArea struct {
	Name     string    `json:"name"`
	Failures []Failure `json:"failures"`
	Error    *string   `json:"error"`
}

// Failure is one rule violation found in the audited checkout. An audit area
// reports two different things, and the split between them is load-bearing:
// the failures it returns mean the checkout violates rules, and an error means
// the area could not audit at all (I/O failure, missing toolchain, malformed
// input it cannot classify). Never fold an audit error into a failure or the
// reverse: a guard that errors instead of failing can be silently skipped, and
// one that fails instead of erroring blames the checkout for the auditor's own
// problems. Rule identifiers are stable kebab-case strings, unique within an
// area. The JSON tags are the keys the TypeScript conformance reads.
type Failure struct {
	// Rule is the stable kebab-case identifier of the rule that fired.
	Rule string `json:"rule"`
	// Path is the repo-relative file or directory the violation is in; empty
	// for repo-level rules.
	Path string `json:"path"`
	// Message says what is wrong and, where knowable, how to fix it.
	Message string `json:"message"`
}

// AuditResult is the versioned hook audit inventory.
type AuditResult struct {
	Version int         `json:"version"`
	Areas   []AuditArea `json:"areas"`
}

// AuditOptions adjusts which gates Audit runs.
type AuditOptions struct {
	// SkipTests leaves the module-test and module-coverage-floor gates out of
	// the module area. Every other gate still runs, and the result keeps
	// version 1.
	SkipTests bool
	// SourceOnly rejects staged native executables and verifies the artifacts
	// built in scratch storage independently of local build outputs.
	SourceOnly bool
}

// Audit runs the ordered areas, retaining findings even if an area errors.
func Audit(repoRoot string, options AuditOptions) (AuditResult, error) {
	result := AuditResult{Version: 1}
	var auditErr error
	var all []Failure
	for _, area := range hookAreas {
		entry := AuditArea{Name: area.name, Failures: []Failure{}}
		if auditErr != nil {
			message := "not-run: prior operational error"
			entry.Error = &message
		} else {
			found, err := area.run(repoRoot, options)
			all = append(all, found...)
			entry.Failures = append(entry.Failures, found...)
			if err != nil {
				auditErr = err
				message := err.Error()
				entry.Error = &message
			}
		}
		result.Areas = append(result.Areas, entry)
	}
	if auditErr == nil {
		kept := dedupeStaleRebuild(all, hookbuild.ContractBinaries)
		counts := make(map[Failure]int)
		for _, f := range kept {
			counts[f]++
		}
		for i := range result.Areas {
			result.Areas[i].Failures = slices.DeleteFunc(result.Areas[i].Failures, func(f Failure) bool {
				if counts[f] == 0 {
					return true
				}
				counts[f]--
				return false
			})
		}
	}
	return result, auditErr
}

// runFunc is one audit area. It returns the rule violations found in the
// checkout at repoRoot, or an error when the area could not audit it; see
// Failure for why the two are never interchangeable. options is the audit's
// own, so each area decides which of its gates to leave out.
type runFunc func(repoRoot string, options AuditOptions) ([]Failure, error)

type hookArea struct {
	name string
	run  runFunc
}

// hookAreas is the audit's order: ASCII text, config schema, module gates,
// then freshness. Freshness must see the committed hooks/bin and bin/agx.exe
// before any rebuild overwrites them, and that holds structurally:
// RunModuleGates builds into a temp dir and never touches either. Module gates
// also compare every fresh build against the hooks/SHA256SUMS manifest entry,
// the same artifact RunFreshness checks, so a source/manifest disagreement is
// caught even when hooks/bin holds no committed binary. So the order matters
// only for cheap-first reporting, not for correctness.
var hookAreas = []hookArea{
	{name: "ascii", run: func(root string, _ AuditOptions) ([]Failure, error) { return runASCII(root) }},
	{name: "schema", run: func(root string, _ AuditOptions) ([]Failure, error) { return RunSchema(root) }},
	{name: "module", run: func(root string, options AuditOptions) ([]Failure, error) {
		return runModuleGates(root, options)
	}},
	{name: "freshness", run: func(root string, options AuditOptions) ([]Failure, error) {
		return runFreshness(root, options.SourceOnly)
	}},
}

// dedupeStaleRebuild drops the rebuild-comparison findings a stale embedded
// source digest already explains, per binary — not per rule. A stale digest
// on binary X makes checkBuildContract's rebuild comparison for X fail for
// the identical reason RunFreshness already reported
// (module-rebuild-mismatch, Path "hooks/bin/X"): both point at the same fix
// (make -C hooks/go build-all), so the redundant one is dropped. A finding on
// a binary whose digest is FRESH always stands: the digest cannot explain
// it, so it may be a genuine tamper the maintainer must see before a rebuild
// silently overwrites the evidence. The launcher follows the same rule: both
// findings on it carry Path bin/agx.exe, so its stale stamp drops its own
// rebuild mismatch and nothing else.
//
// module-rebuild-manifest is not per-binary — its Path is always
// hooks/SHA256SUMS — so it is dropped only when EVERY contract binary's
// digest is stale. With even one binary fresh, that binary's own mismatch
// could be the sole cause of the manifest disagreement, so the manifest
// finding stays until every stale binary is accounted for.
func dedupeStaleRebuild(failures []Failure, binaries []string) []Failure {
	stale := make(map[string]bool, len(binaries))
	for _, f := range failures {
		if f.Rule == "freshness-stale-digest" {
			stale[f.Path] = true
		}
	}
	if len(stale) == 0 {
		return failures
	}
	allStale := true
	for _, name := range binaries {
		if !stale["hooks/bin/"+name] {
			allStale = false
			break
		}
	}
	return slices.DeleteFunc(failures, func(f Failure) bool {
		switch f.Rule {
		case "module-rebuild-mismatch":
			return stale[f.Path]
		case "module-rebuild-manifest":
			return allStale
		default:
			return false
		}
	})
}
