package secretsguard

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/argv"
)

// dispatchTables returns every set of names the guard dispatches on, keyed by the identifier that holds it, with each
// list sorted.
func dispatchTables() map[string][]string {
	tables := map[string][]string{
		"readerCommands": nil,
		"secretCommands": nil,
		"git":            {gitExecutable},
	}
	for name := range readerCommands {
		tables["readerCommands"] = append(tables["readerCommands"], name)
	}
	for name := range secretCommands {
		tables["secretCommands"] = append(tables["secretCommands"], name)
	}
	for _, names := range tables {
		sort.Strings(names)
	}
	return tables
}

// unclassifiedDispatchNames returns one problem for each name the guard dispatches on that the lookup does not
// classify. The lookup is a parameter so a test can hand the check a table that has lost one name.
func unclassifiedDispatchNames(lookup func(string) argv.Command) []string {
	var problems []string
	for table, names := range dispatchTables() {
		for _, name := range names {
			if !lookup(name).Classified {
				problems = append(problems, fmt.Sprintf("secretsguard %s: %q is not Classified in argv.Lookup", table, name))
			}
		}
	}
	sort.Strings(problems)
	return problems
}

func TestDispatchNamesAreClassified(t *testing.T) {
	tables := dispatchTables()
	for table, names := range tables {
		if len(names) == 0 {
			t.Fatalf("secretsguard %s is empty, so the check proves nothing", table)
		}
	}
	if problems := unclassifiedDispatchNames(argv.Lookup); len(problems) > 0 {
		t.Fatalf("a dispatch name is missing from the vocabulary:\n%s", strings.Join(problems, "\n"))
	}

	for table, names := range tables {
		for _, name := range names {
			t.Run("cleared "+table+" "+name, func(t *testing.T) {
				cleared := func(candidate string) argv.Command {
					command := argv.Lookup(candidate)
					if candidate == name {
						command.Classified = false
					}
					return command
				}
				problems := unclassifiedDispatchNames(cleared)
				if len(problems) == 0 {
					t.Fatalf("clearing Classified for %q passed the check", name)
				}
				for _, problem := range problems {
					if !strings.Contains(problem, fmt.Sprintf("%q", name)) {
						t.Fatalf("clearing Classified for %q gave a problem that names another command: %s", name, problem)
					}
				}
			})
		}
	}
}
