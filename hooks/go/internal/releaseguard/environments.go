package releaseguard

import (
	"errors"
	"os"
	"strings"

	"github.com/muehz/agentics/hooks/go/internal/shellwords"
)

// environmentsVar names the host-set filter that narrows release-guardian to
// deploys whose invocation carries one of the configured markers.
const environmentsVar = "AGX_RELEASE_ENVIRONMENTS"

// environments reads the host-set filter. configured is false when the
// variable is unset, which keeps today's gate-every-deployment behavior.
func environments() (markers []string, configured bool, err error) {
	raw, ok := os.LookupEnv(environmentsVar)
	if !ok {
		return nil, false, nil
	}
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.Trim(entry, " \t")
		if strings.IndexFunc(entry, func(r rune) bool { return r < 0x20 || r == 0x7f }) >= 0 {
			return nil, true, errors.New("release-guardian: " + environmentsVar + " holds a control character")
		}
		if entry != "" {
			markers = append(markers, entry)
		}
	}
	if len(markers) == 0 {
		return nil, true, errors.New("release-guardian: " + environmentsVar + " is set but names no environment")
	}
	return markers, true, nil
}

// environmentRule returns the index of the first marker the invocation
// carries, as a whole word or as the value side of name=value, across its
// wrapper words, assignments, and words. A non-literal word could expand to
// any marker, so it matches as rule -1 (fail closed).
func environmentRule(inv shellwords.Invocation, markers []string) (int, bool) {
	words := append(append(append([]shellwords.Word{}, inv.Wrappers...), inv.Assignments...), inv.Words...)
	for _, w := range words {
		if !w.Literal {
			return -1, true
		}
		value := w.Text
		if _, v, ok := strings.Cut(w.Text, "="); ok {
			value = v
		}
		for i, m := range markers {
			if w.Text == m || value == m {
				return i, true
			}
		}
	}
	return 0, false
}
