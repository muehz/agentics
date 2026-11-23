package commandguard

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
		"settingsWriteVerbs":     keysOf(settingsWriteVerbs),
		"settingsDirectoryVerbs": keysOf(settingsDirectoryVerbs),
		"powershellCopyVerbs":    keysOf(powershellCopyVerbs),
		"hookRemoveVerbs":        keysOf(hookRemoveVerbs),
		"hookMoveVerbs":          keysOf(hookMoveVerbs),
		"hookNullVerbs":          keysOf(hookNullVerbs),
		"sqlClients":             keysOf(sqlClients),
		"publicationClients":     keysOf(publicationClients),
		"downloadProducers":      keysOf(downloadProducers),
	}
	for _, names := range tables {
		sort.Strings(names)
	}
	return tables
}

func keysOf[V any](table map[string]V) []string {
	names := make([]string, 0, len(table))
	for name := range table {
		names = append(names, name)
	}
	return names
}

// unclassifiedDispatchNames returns one problem for each name the guard dispatches on that the lookup does not
// classify. The lookup is a parameter so a test can hand the check a table that has lost one name.
func unclassifiedDispatchNames(lookup func(string) argv.Command) []string {
	var problems []string
	for table, names := range dispatchTables() {
		for _, name := range names {
			if !lookup(name).Classified {
				problems = append(problems, fmt.Sprintf("commandguard %s: %q is not Classified in argv.Lookup", table, name))
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
			t.Fatalf("commandguard %s is empty, so the check proves nothing", table)
		}
	}
	if problems := unclassifiedDispatchNames(argv.Lookup); len(problems) > 0 {
		t.Fatalf("a dispatch name is missing from the vocabulary:\n%s", strings.Join(problems, "\n"))
	}

	// A name several tables hold appears in several problems, and every one of them must name it.
	tableNames := make([]string, 0, len(tables))
	for table := range tables {
		tableNames = append(tableNames, table)
	}
	sort.Strings(tableNames)
	cleared := map[string]bool{}
	for _, table := range tableNames {
		for _, name := range tables[table] {
			if cleared[name] {
				continue
			}
			cleared[name] = true
			t.Run("cleared "+table+" "+name, func(t *testing.T) {
				lookup := func(candidate string) argv.Command {
					command := argv.Lookup(candidate)
					if candidate == name {
						command.Classified = false
					}
					return command
				}
				problems := unclassifiedDispatchNames(lookup)
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
