package flow

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
)

const (
	phases      = "intent spec decompose execute gate"
	events      = "create challenge approve revoke diverge verify revert demote block unblock pivot waive branch release change skip na enter bind pause done review escape order commit drop note"
	statuses    = "ok failed blocked pending declined cancelled"
	reviewKinds = "plan packet quality integration spec"
	escapeKinds = "plan packet quality integration"
	commitKinds = "fix triage user housekeeping"
	noteTargets = "slices slice-S<n> lane-S<n> coverage policy"

	noCommitShape = "no-commit exit=<n> cmd=<command> out=<excerpt>"
	// A no-commit cell quotes a command and its output, so it is bounded to keep the row readable.
	noCommitLimit = 500
)

var (
	evidencePair      = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`^[a-z-]+=[^ ]+$`) })
	evidenceTimestamp = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`^20[0-9]{2}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$`)
	})
	evidenceRound   = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`^[1-9][0-9]*$`) })
	noCommitExit    = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`^-?[1-9][0-9]*$`) })
	evidenceBlocked = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`^(0|[1-9][0-9]*)$`) })
	noteSliceTarget = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`^slice-S[1-9][0-9]*$`) })
	noteLaneTarget  = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`^lane-S[1-9][0-9]*$`) })
	evidenceSlices  = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`^S[1-9][0-9]*(,S[1-9][0-9]*)*$`) })
	evidenceIssue   = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`^([A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+)?#[1-9][0-9]*$`)
	})
	evidenceCommitID = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`^[0-9a-f]{7,64}$`) })
)

type cellError struct{ field, value, reason string }

// Error describes the invalid cell and why it was rejected.
func (e *cellError) Error() string {
	return fmt.Sprintf("flow: invalid %s: '%s' -- %s", e.field, e.value, e.reason)
}

// ExitCode is the flow invalid-cell status, independent of hook deny codes.
func (e *cellError) ExitCode() int { return 2 }

func invalidCell(field, value, reason string) error {
	return &cellError{field, value, reason}
}

func inSet(value, set string) bool {
	for _, item := range strings.Split(set, " ") {
		if value == item {
			return true
		}
	}
	return false
}

func stripCell(value string) string {
	value = strings.NewReplacer("\t", " ", "\r", " ", "\n", " ").Replace(value)
	return strings.Trim(value, " \t\n\v\f\r")
}

func cleanCell(value string) string {
	value = stripCell(value)
	if value != "" && strings.ContainsRune("=+-@'", rune(value[0])) {
		return "'" + value
	}
	return value
}

func sessionCell(value string) string {
	if value = cleanCell(value); value == "" {
		return "-"
	}
	return value
}

// validateCells returns pre-guard values so callers can resolve auto evidence
// before applying the spreadsheet guard to the stored cells.
func validateCells(values [7]string) ([7]string, error) {
	for i := range values {
		values[i] = stripCell(values[i])
	}
	checks := []struct {
		index              int
		field, set, reason string
	}{
		{0, "phase", phases, "expected one of: " + phases},
		{1, "event", events, "expected one of: " + events},
		{2, "target", "", "a target is required"},
		{3, "status", statuses, "expected one of: " + statuses},
		{4, "why", "", "a one-line reason is required"},
		{5, "evidence", "", "evidence is required; write none when there is nothing to point at"},
		{6, "next", "", "next is required on every row"},
	}
	for _, check := range checks {
		value := values[check.index]
		if (check.set != "" && !inSet(value, check.set)) || (check.set == "" && value == "") {
			return values, invalidCell(check.field, value, check.reason)
		}
	}
	token, _, _ := strings.Cut(values[6], " ")
	if !inSet(token, phases+" done") {
		return values, invalidCell("next", values[6], "the first word must be a phase name or done")
	}
	return values, validateEvidence(values[1], values[2], values[3], values[5])
}
