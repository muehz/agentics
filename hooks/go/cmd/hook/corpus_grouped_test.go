package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hosttest"
	"github.com/muehz/agentics/hooks/go/internal/surface"
)

// groupedAgreementExclusions names corpus entries the grouped event cannot
// reproduce: each one exercises a single check's own argv validation, and
// the grouped event takes no per-check argv, so there is nothing to compare.
var groupedAgreementExclusions = map[string]bool{
	"command-guardian/argv-wrong-mode-rejected": true,
	"command-guardian/argv-no-args-rejected":    true,
	"command-guardian/argv-extra-arg-rejected":  true,
	"secrets-guardian/argv-wrong-mode-rejected": true,
}

// groupedAgreementSingles are the single guard checks the grouped pre-tool
// event runs together.
var groupedAgreementSingles = []string{"command-guardian", "release-guardian", "secrets-guardian"}

// TestCorpusGroupedAgreement runs every applicable corpus entry through each
// single guard check and through the grouped pre-tool event with the same
// stdin, environment, and fixture, and requires the grouped event to deny
// exactly when some single check denied, faulted, or rejected the payload.
//
// A single check counts as faulted or rejected when it did not deny and
// wrote non-empty stderr; the grouped event folds a guard's error or fault
// into a deny. That rule has a known limit: a single check that faults
// without writing stderr reads as a pass, and a passing check that writes
// informational stderr reads as a fault. On the current guards no corpus row
// hits either case, because on the single path the renderer drops handler
// stderr for pass and context outcomes, while a fault always has stderr:
// renderError writes the handler's stderr if there is any, otherwise the
// error's own message, otherwise a fallback. If that rendering changes, check this rule
// again.
func TestCorpusGroupedAgreement(t *testing.T) {
	table := loadCorpusTable(t)
	known := map[string]bool{}
	for _, entry := range table.Entries {
		known[entry.Name] = true
	}
	for name := range groupedAgreementExclusions {
		if !known[name] {
			t.Errorf("excluded entry %q is missing from %s", name, corpusEntriesPath)
		}
	}
	for _, surface := range corpusSurfaces {
		t.Run(string(surface), func(t *testing.T) {
			for _, entry := range table.Entries {
				if !entry.appliesTo(surface) || groupedAgreementExclusions[entry.Name] {
					continue
				}
				t.Run(entry.Name, func(t *testing.T) {
					checkGroupedAgreement(t, table, surface, entry)
				})
			}
		})
	}
}

func checkGroupedAgreement(t *testing.T, table corpusTable, surface surface.ID, entry corpusEntry) {
	t.Helper()
	runAs := func(hook string) corpusResult {
		argv := legalArgs(surface, hook)
		if argv == nil {
			t.Fatalf("no legal argv for %s on %s: unknown surface, a hook it does not support, or an unreadable shipped config", hook, surface)
		}
		e := entry
		e.Hook = hook
		e.Argv = argv
		return runCorpusEntry(t, table, surface, e)
	}

	denies := func(r corpusResult) bool {
		return hosttest.Read(surface, r.Exit, []byte(r.Stdout), []byte(r.Stderr)).Decision == hosttest.Deny
	}
	var report strings.Builder
	wantDeny := false
	deadline := false
	for _, hook := range groupedAgreementSingles {
		result := runAs(hook)
		denied := denies(result)
		faulted := !denied && result.Stderr != ""
		if denied || faulted {
			wantDeny = true
		}
		deadline = deadline || hasDeadline(result)
		fmt.Fprintf(&report, "\n  %s: deny=%t fault=%t exit=%d\n    stdout=%q\n    stderr=%q",
			hook, denied, faulted, result.Exit, result.Stdout, result.Stderr)
	}
	grouped := runAs("pre-tool")
	deadline = deadline || hasDeadline(grouped)
	gotDeny := denies(grouped)
	if gotDeny == wantDeny {
		return
	}
	note := ""
	if deadline {
		note = " [inconclusive (load): an output reports deadline exceeded]"
	}
	t.Errorf("%s/%s: grouped deny=%t, single checks deny-or-fault=%t%s%s\n  pre-tool: exit=%d\n    stdout=%q\n    stderr=%q",
		surface, entry.Name, gotDeny, wantDeny, note, report.String(), grouped.Exit, grouped.Stdout, grouped.Stderr)
}

func hasDeadline(r corpusResult) bool {
	return strings.Contains(r.Stdout, "deadline exceeded") || strings.Contains(r.Stderr, "deadline exceeded")
}

// groupedUpdateCommand regenerates the grouped recordings. Its pattern is
// anchored at both ends so it cannot also match other tests whose names
// contain TestBehaviorCorpusGrouped as a substring.
const groupedUpdateCommand = "go test ./cmd/hook -run '^TestBehaviorCorpusGrouped$' -update"

// TestBehaviorCorpusGrouped runs every applicable corpus entry as the grouped
// pre-tool event and compares (exit, stdout, stderr) byte for byte with
// testdata/corpus/recorded/<surface>.pre-tool.json. The entries excluded from
// TestCorpusGroupedAgreement are excluded here for the same reason.
//
// A recording must not capture a timed-out guard: an output that reports
// deadline exceeded is load on the recording host, not behavior, so -update
// fails and leaves that surface's file unwritten. In compare mode such an
// output cannot match a recording and fails as a divergence, labelled as
// load so it is not read as a behavior change.
func TestBehaviorCorpusGrouped(t *testing.T) {
	table := loadCorpusTable(t)
	for _, surface := range corpusSurfaces {
		t.Run(string(surface), func(t *testing.T) {
			argv := legalArgs(surface, "pre-tool")
			if argv == nil {
				t.Fatalf("no legal argv for pre-tool on %s: the shipped config could not be read", surface)
			}
			recordedPath := filepath.Join(corpusRecordedDir, string(surface)+".pre-tool.json")
			var recorded map[string]corpusResult
			if !*update {
				// readRecorded's own message names the single-check command,
				// which would rewrite the wrong set.
				if _, err := os.Stat(recordedPath); err != nil {
					t.Fatalf("no recorded grouped outputs at %s — run: %s", recordedPath, groupedUpdateCommand)
				}
				recorded = readRecorded(t, recordedPath)
			}
			got := map[string]corpusResult{}
			for _, entry := range table.Entries {
				if !entry.appliesTo(surface) || groupedAgreementExclusions[entry.Name] {
					continue
				}
				t.Run(entry.Name, func(t *testing.T) {
					e := entry
					e.Hook = "pre-tool"
					e.Argv = argv
					result := runCorpusEntry(t, table, surface, e)
					got[entry.Name] = result
					if *update {
						if hasDeadline(result) {
							t.Errorf("%s/%s reports deadline exceeded; record on an idle machine\n  exit=%d\n  stdout=%q\n  stderr=%q",
								surface, entry.Name, result.Exit, result.Stdout, result.Stderr)
						}
						return
					}
					want, ok := recorded[entry.Name]
					if !ok {
						t.Fatalf("no recorded grouped output for %s/%s — run: %s", surface, entry.Name, groupedUpdateCommand)
					}
					if result != want {
						note := ""
						if hasDeadline(result) {
							note = " [inconclusive (load): the output reports deadline exceeded]"
						}
						t.Errorf("%s/%s grouped output diverged from the recording%s\n got: exit=%d\n      stdout=%q\n      stderr=%q\nwant: exit=%d\n      stdout=%q\n      stderr=%q",
							surface, entry.Name, note, result.Exit, result.Stdout, result.Stderr, want.Exit, want.Stdout, want.Stderr)
					}
				})
			}
			if *update {
				// Any failed entry, a deadline-exceeded one included, would
				// leave a partial or load-shaped recording behind.
				if t.Failed() {
					t.Errorf("not writing %s: an entry above failed", recordedPath)
					return
				}
				writeRecorded(t, recordedPath, got)
				return
			}
			for name := range recorded {
				if _, live := got[name]; !live {
					t.Errorf("recorded %s/%s has no applicable entry in %s — stale; run: %s", surface, name, corpusEntriesPath, groupedUpdateCommand)
				}
			}
		})
	}
}
