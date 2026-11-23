package flow

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hosttest"
)

// operandsDocRel is flow-state.md relative to the checkout root.
const operandsDocRel = "skills/execute/references/flow-state.md"

// operandsDocPath returns flow-state.md in the checkout the tests run against.
func operandsDocPath(t testing.TB) string {
	t.Helper()
	return filepath.Join(hosttest.Checkout(t), filepath.FromSlash(operandsDocRel))
}

// operandsArguments maps each Arguments cell of the Operands table, with its
// backticks removed, to the counts and check flag of a waiveHead. The doc uses
// only these phrases, so a cell outside the map is a doc the test cannot read
// and fails rather than being guessed at.
var operandsArguments = map[string]waiveHead{
	"none":                              {},
	"0 or 1 group":                      {max: 1},
	"0 or more task ids":                {max: manyArguments},
	"1 group":                           {min: 1, max: 1},
	"1 shape":                           {min: 1, max: 1},
	"1 step name":                       {min: 1, max: 1},
	"1 rule name":                       {min: 1, max: 1},
	"1 branch":                          {min: 1, max: 1},
	"1 or more task ids":                {min: 1, max: manyArguments},
	"2: lane, locator":                  {min: 2, max: 2},
	"none; the head may carry :<check>": {check: true},
}

var operandsDocHead = regexp.MustCompile("`([^`]+)`")

// docOperands reads the table under the ## Operands heading of doc into
// waiveHeads, in table order. A grouped Head cell yields one entry per head.
// A checkout with CRLF line endings reads the same as one with LF.
func docOperands(t *testing.T, doc string) []waiveHead {
	t.Helper()
	_, section, found := strings.Cut(strings.ReplaceAll(doc, "\r\n", "\n"), "\n## Operands\n")
	if !found {
		t.Fatalf("%s has no ## Operands section", operandsDocRel)
	}
	if end := strings.Index(section, "\n## "); end >= 0 {
		section = section[:end]
	}
	var heads []waiveHead
	rows := 0
	for _, line := range strings.Split(section, "\n") {
		if !strings.HasPrefix(line, "|") {
			if rows > 0 {
				break // only the first table is the list
			}
			continue
		}
		rows++
		cells := strings.Split(strings.Trim(line, "|"), "|")
		// A \| inside a cell would split it; the Operands cells hold none.
		if len(cells) != 3 {
			t.Fatalf("Operands row has %d cells, want 3: %s", len(cells), line)
		}
		head, args, emitted := strings.TrimSpace(cells[0]), strings.TrimSpace(cells[1]), strings.TrimSpace(cells[2])
		if rows <= 2 {
			continue // the header row and its delimiter row
		}
		if !strings.HasPrefix(emitted, "`skills/") {
			t.Errorf("Operands row %q: Emitted in %q does not start with the backticked path", head, emitted)
		}
		shape, ok := operandsArguments[strings.ReplaceAll(args, "`", "")]
		if !ok {
			t.Fatalf("Operands row %q: Arguments %q is not one of the table's phrases", head, args)
		}
		names := operandsDocHead.FindAllStringSubmatch(head, -1)
		if len(names) == 0 {
			t.Fatalf("Operands row has no backticked head: %s", line)
		}
		for _, name := range names {
			entry := shape
			entry.name = name[1]
			heads = append(heads, entry)
		}
	}
	return heads
}

// TestOperandsDocParity holds the Operands table of flow-state.md and
// waiveHeads equal, head by head and in order, so neither the list a skill
// reads nor the list flow log enforces can change alone.
func TestOperandsDocParity(t *testing.T) {
	raw, err := os.ReadFile(operandsDocPath(t))
	if err != nil {
		t.Fatal(err)
	}
	doc := docOperands(t, string(raw))
	if slices.Equal(doc, waiveHeads) {
		return
	}
	for i := range max(len(doc), len(waiveHeads)) {
		var inDoc, inGo waiveHead
		if i < len(doc) {
			inDoc = doc[i]
		}
		if i < len(waiveHeads) {
			inGo = waiveHeads[i]
		}
		if inDoc != inGo {
			t.Fatalf("Operands head %d differs: doc %+v, waiveHeads %+v", i+1, inDoc, inGo)
		}
	}
}
