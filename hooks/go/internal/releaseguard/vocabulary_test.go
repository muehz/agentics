package releaseguard

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/argv"
)

// unclassifiedDispatchNames returns one problem for each name the guard dispatches on that the lookup does not
// classify. The lookup is a parameter so a test can hand the check a table that has lost one name.
func unclassifiedDispatchNames(lookup func(string) argv.Command) []string {
	var problems []string
	for name := range deployMatchers {
		if !lookup(name).Classified {
			problems = append(problems, fmt.Sprintf("releaseguard deployMatchers: %q is not Classified in argv.Lookup", name))
		}
	}
	sort.Strings(problems)
	return problems
}

func TestDispatchNamesAreClassified(t *testing.T) {
	if len(deployMatchers) == 0 {
		t.Fatal("releaseguard deployMatchers is empty, so the check proves nothing")
	}
	if problems := unclassifiedDispatchNames(argv.Lookup); len(problems) > 0 {
		t.Fatalf("a dispatch name is missing from the vocabulary:\n%s", strings.Join(problems, "\n"))
	}

	names := make([]string, 0, len(deployMatchers))
	for name := range deployMatchers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		t.Run("cleared "+name, func(t *testing.T) {
			cleared := func(candidate string) argv.Command {
				command := argv.Lookup(candidate)
				if candidate == name {
					command.Classified = false
				}
				return command
			}
			problems := unclassifiedDispatchNames(cleared)
			if len(problems) != 1 || !strings.Contains(problems[0], fmt.Sprintf("%q", name)) {
				t.Fatalf("clearing Classified for %q gave %q, want one problem naming it", name, problems)
			}
		})
	}
}
