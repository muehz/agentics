package commandguard

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
	"unicode"

	"github.com/muehz/agentics/hooks/go/internal/toolname"
)

// referenceGhInvocations segments every line for each verb with no byte-pair
// gate and no caching; ghScanner.invocations must return the same segments.
func referenceGhInvocations(toolName string, src scanSource, verb ghVerb) []string {
	if !toolname.IsShell(toolName) {
		return nil
	}
	var lines []string
	if src.promotedCommand {
		lines = strings.Split(joinGhContinuations(src.command), "\n")
	} else {
		found := false
		for _, line := range src.lines {
			if verb.pre.MatchString(line) {
				found = true
				break
			}
		}
		if !found {
			return nil
		}
		lines = src.lines
	}
	var out []string
	for _, line := range lines {
		for _, segment := range segmentShellLine(line, src.strictSegmentation()) {
			if m := verb.extract.FindString(segment); m != "" {
				out = append(out, m)
			}
		}
	}
	return out
}

// ghSeedCommands spell the four verbs through every transform segmentShellLine
// applies: redirects, paren operands, quotes, comments, continuations, case.
var ghSeedCommands = []string{
	"gh pr create --title x",
	"GH PR CREATE --title x",
	"gh >out.txt pr create --title x",
	"gh 2>/dev/null pr create",
	"gh <(echo) pr create",
	"cat <(gh pr create --title x)",
	"echo x; gh pr ready 1",
	"gh pr ready 12 --undo",
	"gh stack submit --auto",
	"gh stack link --open",
	"gh pr create \\\n  --title x",
	"g\\\nh pr create",
	"echo \"gh pr create\"",
	"echo 'gh pr create'",
	"# gh pr create",
	"x # gh pr create",
	"g>x h pr create",
	"g(h) pr create",
	"ſh pr create",
	"echo hi && ls -la",
	"Gh Pr Create",
	"gH pr ready",
	"",
}

// corpusCommands returns every "command" string in the behavior corpus.
func corpusCommands(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile("../../cmd/hook/testdata/corpus/entries.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc any
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	if err := dec.Decode(&doc); err != nil {
		t.Fatal(err)
	}
	var out []string
	var walk func(any)
	walk = func(v any) {
		switch v := v.(type) {
		case map[string]any:
			for k, item := range v {
				if s, ok := item.(string); ok && k == "command" {
					out = append(out, s)
				}
				walk(item)
			}
		case []any:
			for _, item := range v {
				walk(item)
			}
		}
	}
	walk(doc)
	if len(out) == 0 {
		t.Fatal("corpus yielded no commands")
	}
	return out
}

var allGhVerbs = []ghVerb{ghPrCreate, ghPrReady, ghStackSubmit, ghStackLink}

func TestGhScannerMatchesReference(t *testing.T) {
	commands := append(corpusCommands(t), ghSeedCommands...)
	for _, tool := range []string{"Bash", "PowerShell", "Write"} {
		for _, command := range commands {
			sources := []scanSource{
				{promotedCommand: true, command: command},
				{lines: strings.Split(command, "\n")},
			}
			for _, src := range sources {
				g := &ghScanner{toolName: tool, src: src}
				for i, verb := range allGhVerbs {
					got := g.invocations(verb)
					want := referenceGhInvocations(tool, src, verb)
					if !reflect.DeepEqual(got, want) {
						t.Errorf("tool %s promoted=%v verb %d command %q: got %q, want %q",
							tool, src.promotedCommand, i, command, got, want)
					}
				}
			}
		}
	}
}

func TestGhGateIsSound(t *testing.T) {
	// No rune outside ASCII folds into g or h, so a byte-pair check is exact.
	for _, r := range []rune{'g', 'h'} {
		for f := unicode.SimpleFold(r); f != r; f = unicode.SimpleFold(f) {
			if f > unicode.MaxASCII {
				t.Fatalf("rune %U folds into %q; the gh gate must fold it", f, r)
			}
		}
	}
	commands := append(corpusCommands(t), ghSeedCommands...)
	for _, command := range commands {
		for _, line := range strings.Split(joinGhContinuations(command), "\n") {
			for _, strict := range []bool{false, true} {
				for _, segment := range segmentShellLine(line, strict) {
					if lineHasFoldedGH(segment) && !lineHasFoldedGH(line) {
						t.Errorf("segment %q of line %q has a gh pair the line lacks", segment, line)
					}
				}
			}
		}
	}
}
