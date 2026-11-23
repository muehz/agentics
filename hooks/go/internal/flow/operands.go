package flow

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
)

// manyArguments marks a waive head that takes any number of arguments above its minimum.
const manyArguments = -1

// waiveHead is one entry of the closed list of waive heads: the argument
// counts it takes and whether the head may carry :<check>.
type waiveHead struct {
	name     string
	min, max int
	check    bool
}

// waiveHeads is the closed list of waive heads, in the order of the Operands
// section of flow-state.md, which must list the same heads and counts.
// Arguments are checked for count only, never against the plan, so a waive
// row stays valid when the plan it names changes.
var waiveHeads = []waiveHead{
	{name: "checklist"},
	{name: "proportionality"},
	{name: "coverage"},
	{name: "no-code", max: manyArguments},
	{name: "controller-rule"},
	{name: "independence", min: 1, max: 1},
	{name: "quality-review", min: 1, max: 1},
	{name: "verification", min: 1, max: manyArguments},
	{name: "deepen", min: 1, max: 1},
	{name: "aggregate-review", max: 1},
	{name: "final-review"},
	{name: "union-rule"},
	{name: "stage-1"},
	{name: "verification-preflight"},
	{name: "wave-pauses"},
	{name: "diagnose", min: 1, max: 1},
	{name: "test-first"},
	{name: "delete-untested"},
	{name: "refactor"},
	{name: "root-cause"},
	{name: "prototype-code"},
	{name: "chain"},
	{name: "direction-blockers"},
	{name: "brief-questions"},
	{name: "direction-review"},
	{name: "spec-step", min: 1, max: 1},
	{name: "docs-refresh"},
	{name: "review"},
	{name: "converge"},
	{name: "tests"},
	{name: "work-folder"},
	{name: "security-depth"},
	{name: "finding", min: 2, max: 2},
	{name: "artifact", check: true},
	{name: "migrations", check: true},
	{name: "flags", check: true},
	{name: "rollback", check: true},
	{name: "ci", check: true},
	{name: "on-call", check: true},
	{name: "comms", check: true},
	{name: "window", check: true},
	{name: "rule", min: 1, max: 1},
}

var (
	waiveCheck   = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`^[a-z0-9-]+$`) })
	demoteTarget = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`^wave-[A-Za-z0-9-]+$`) })
)

func lookupWaiveHead(name string) (waiveHead, bool) {
	for _, head := range waiveHeads {
		if head.name == name {
			return head, true
		}
	}
	return waiveHead{}, false
}

// countReason states the argument counts a head takes, or "" when got fits them.
func (h waiveHead) countReason(got int) string {
	if got >= h.min && (h.max == manyArguments || got <= h.max) {
		return ""
	}
	takes := fmt.Sprintf("%d or %d", h.min, h.max)
	switch {
	case h.max == manyArguments:
		takes = fmt.Sprintf("%d or more", h.min)
	case h.min == h.max:
		takes = fmt.Sprint(h.min)
	}
	return fmt.Sprintf("%s takes %s arguments, got %d", h.name, takes, got)
}

// parseWaive checks a waive target against waiveHeads. Its error is the bare
// reason, so a reader can print it beside the row it rejects.
func parseWaive(target string) error {
	word, rest, spaced := strings.Cut(target, " ")
	name, check, hasCheck := strings.Cut(word, ":")
	head, ok := lookupWaiveHead(name)
	if !ok {
		return fmt.Errorf("head %s is not in the Operands list of flow-state.md", name)
	}
	if hasCheck {
		switch {
		case !head.check:
			return errors.New(":<check> is allowed only on a release item")
		case check == "":
			return errors.New("check is empty")
		case !waiveCheck().MatchString(check):
			return fmt.Errorf("check %s is not a-z 0-9 -", check)
		}
	}
	var args []string
	if spaced {
		args = strings.Split(rest, " ")
	}
	for _, arg := range args {
		if arg == "" {
			return errors.New("an argument is empty")
		}
	}
	if reason := head.countReason(len(args)); reason != "" {
		return errors.New(reason)
	}
	return nil
}

// parseDemote checks a demote target names a wave. Its error is the bare
// reason, so a reader can print it beside the row it rejects.
func parseDemote(target string) error {
	if !demoteTarget().MatchString(target) {
		return errors.New("demote names wave-<id>")
	}
	return nil
}
