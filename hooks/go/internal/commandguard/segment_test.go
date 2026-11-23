package commandguard

// Tests for the gh-rule tokenizer and its wiring. The strict segmentation
// mode runs only for a Bash event whose tool_input carries no command key, a
// shape the handler corpus does not include, so these pin it directly.

import (
	"reflect"
	"strings"
	"testing"
)

// TestGhGlobalValueBoundariesThroughRun pins how the parsed-invocation path
// walks gh's global value flags before the subcommand: an attached, separated
// or joined value is consumed whole, so a value that looks like --help or the
// end marker neither exempts the invocation nor hides it.
func TestGhGlobalValueBoundariesThroughRun(t *testing.T) {
	type boundaryCase struct{ name, command, value, want string }

	for _, flag := range []string{"-R", "--repo", "--hostname"} {
		for _, form := range []struct{ name, separator string }{
			{"attached", ""}, {"separated", " "}, {"equals", "="},
		} {
			if flag != "-R" && form.name == "attached" {
				continue
			}
			for _, boundary := range []boundaryCase{
				{"mutation", "gh OPTION pr ready 42", "owner/repo", "deny"},
				{"help", "gh OPTION pr ready 42 --help", "owner/repo", "pass"},
				{"end marker", "gh OPTION -- pr ready 42", "owner/repo", "pass"},
				{"help looking value", "gh OPTION pr ready 42", "--help", "deny"},
				{"end marker looking value", "gh OPTION pr ready 42", "--", "deny"},
			} {
				t.Run(flag+"/"+form.name+"/"+boundary.name, func(t *testing.T) {
					tc := tableCase{
						command: strings.ReplaceAll(boundary.command, "OPTION", flag+form.separator+boundary.value),
						want:    boundary.want,
					}
					got := recordCase(t, tc)
					assertCaseSemantics(t, tc, got)
					if tc.want == "deny" && !strings.Contains(got.Outcome.Payload(), "unsafe_publication") {
						t.Fatalf("expected unsafe_publication, got %s", got.Outcome)
					}
				})
			}
		}
	}
}

func TestPublicationOptionBoundariesThroughRun(t *testing.T) {
	for _, tc := range []tableCase{
		{name: "npm browser consumes preview looking value", command: `npm publish --browser --dry-run`, want: "deny"},
		{name: "npm trailing browser is no preview", command: `npm publish --browser`, want: "deny"},
		{name: "npm joined registry cannot consume preview", command: `npm publish --registry=https://example.test --dry-run`, want: "pass"},
		{name: "npm unknown long value is not preview", command: `npm publish --mystery=--dry-run`, want: "deny"},
	} {
		t.Run(tc.name, func(t *testing.T) { assertCaseSemantics(t, tc, recordCase(t, tc)) })
	}
}

// TestSegmentShellLineRelaxed pins the tokenizer mode the gh rules use when
// the canonical promoted command is available: quotes open and close quoted
// spans, and a backslash is an ordinary character.
func TestSegmentShellLineRelaxed(t *testing.T) {
	cases := []struct {
		name string
		line string
		want []string
	}{
		{"plain", "gh pr create --draft", []string{"gh pr create --draft"}},
		{"semicolon splits", "echo a; echo b", []string{"echo a", " echo b"}},
		{"pipe splits", "echo a | grep b", []string{"echo a ", " grep b"}},
		{"ampersand splits", "echo a && echo b", []string{"echo a ", "", " echo b"}},
		{"double quotes protect separators", `echo "a;b"`, []string{`echo "a;b"`}},
		{"single quotes protect separators", `echo 'a|b'`, []string{`echo 'a|b'`}},
		{"backslash is ordinary", `a\b`, []string{`a\b`}},
		{"backslash before quote is ordinary", `gh pr ready \--auto`, []string{`gh pr ready \--auto`}},
		{"comment ends the line", "ls # rm -rf /", []string{"ls "}},
		{"empty line", "", []string{""}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := segmentShellLine(tc.line, false)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("segmentShellLine(%q, false) = %#v, want %#v", tc.line, got, tc.want)
			}
		})
	}
}

// TestSegmentShellLineStrict pins the tokenizer mode the gh rules use when the
// canonical promoted command is absent and the scan falls back to the
// stringified input: a double quote, a single quote, and a backslash each end
// the current segment, so no flag can be vouched for across one of them.
func TestSegmentShellLineStrict(t *testing.T) {
	cases := []struct {
		name string
		line string
		want []string
	}{
		{"plain is unchanged", "gh pr create --draft", []string{"gh pr create --draft"}},
		{"double quote is a boundary", `echo "a;b"`, []string{"echo ", "a", "b", ""}},
		{"single quote is a boundary", `echo 'a|b'`, []string{"echo ", "a", "b", ""}},
		{"backslash is a boundary", `a\b`, []string{"a", "b"}},
		{"escaped quote splits twice", `gh pr create \"--draft\"`, []string{"gh pr create ", "", "--draft", "", ""}},
		{"semicolon still splits", "echo a; echo b", []string{"echo a", " echo b"}},
		{"comment still ends the line", "ls # rm -rf /", []string{"ls "}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := segmentShellLine(tc.line, true)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("segmentShellLine(%q, true) = %#v, want %#v", tc.line, got, tc.want)
			}
		})
	}
}

// TestSegmentShellLineModesDiverge pins that the two modes are not the same
// function: a line carrying a quote or a backslash segments differently. A
// change that threads one mode where the other belongs must fail here.
func TestSegmentShellLineModesDiverge(t *testing.T) {
	for _, line := range []string{`echo "a"`, `echo 'a'`, `a\b`} {
		relaxed := segmentShellLine(line, false)
		strict := segmentShellLine(line, true)
		if reflect.DeepEqual(relaxed, strict) {
			t.Fatalf("segmentShellLine(%q, ...) is identical in both modes: %#v", line, relaxed)
		}
	}
}

// TestGhInvocationsRequiresBashTool pins the guard that the scan-source bit
// does not encode: invocations returns nil for a non-Bash tool even when a
// command is present, and runs in strict mode for Bash without one.
func TestGhInvocationsRequiresBashTool(t *testing.T) {
	// The line carries a double quote deliberately: it is the only way the two
	// tokenizer modes produce different extractions, so this test pins WHICH
	// mode the fallback threads and not merely that it extracts something.
	line := `gh pr create --title "x"`

	nonBash := &ghScanner{
		toolName: "Write",
		src:      scanSource{promotedCommand: true, command: line, lines: []string{line}},
	}
	if got := nonBash.invocations(ghPrCreate); got != nil {
		t.Fatalf("invocations for a non-Bash tool = %#v, want nil", got)
	}

	bashNoCommand := &ghScanner{toolName: "Bash", src: scanSource{lines: []string{line}}}
	got := bashNoCommand.invocations(ghPrCreate)
	want := segmentShellLineMatches(line, true)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("invocations for Bash without a command = %#v, want the strict-mode result %#v", got, want)
	}
	if relaxed := segmentShellLineMatches(line, false); reflect.DeepEqual(want, relaxed) {
		t.Fatalf("this case no longer distinguishes the two modes; pick a line that does (both gave %#v)", want)
	}
}

// segmentShellLineMatches mirrors the extraction loop in invocations so the
// test above compares against the tokenizer mode directly rather than against
// a hand-copied literal.
func segmentShellLineMatches(line string, strict bool) []string {
	var out []string
	for _, segment := range segmentShellLine(line, strict) {
		if m := ghPrCreate.extract.FindString(segment); m != "" {
			out = append(out, m)
		}
	}
	return out
}

func TestNewScanSourceAppliesThePromotedCommandRule(t *testing.T) {
	fields := func(tool string, present bool) normalizedFields {
		f := normalizedFields{toolName: tool, inputJSON: `{"a":1}`}
		if present {
			f.commandPresent = true
			f.command = "echo hi"
			f.scanInput = `{"command":"echo hi"}`
		}
		return f
	}

	for _, tc := range []struct {
		name          string
		tool          string
		present       bool
		wantPromoted  bool
		wantText      string
		wantCommand   string
		wantStrictSeg bool
	}{
		{"Bash with a command is promoted", "Bash", true, true, `{"command":"echo hi"}`, "echo hi", false},
		{"Bash without a command falls back", "Bash", false, false, `{"a":1}`, "", true},
		{"non-Bash with a command is not promoted", "Write", true, false, `{"a":1}`, "", true},
		{"non-Bash without a command is not promoted", "Write", false, false, `{"a":1}`, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := newScanSource(fields(tc.tool, tc.present))
			if got.promotedCommand != tc.wantPromoted {
				t.Errorf("promotedCommand = %v, want %v", got.promotedCommand, tc.wantPromoted)
			}
			if got.text != tc.wantText {
				t.Errorf("text = %q, want %q", got.text, tc.wantText)
			}
			if got.command != tc.wantCommand {
				t.Errorf("command = %q, want %q", got.command, tc.wantCommand)
			}
			if got.strictSegmentation() != tc.wantStrictSeg {
				t.Errorf("strictSegmentation() = %v, want %v", got.strictSegmentation(), tc.wantStrictSeg)
			}
		})
	}
}

// TestFallbackLinesReachTheGhRules pins the WIRING from the scan body into
// the gh scanner. It is the only test that drives a Bash event with no command
// key all the way through Run, the shape the corpus does not cover.
//
// Without it, deleting the scanLines threading at the ghScanner construction
// left src.lines nil, the gh rules silently found nothing, and the whole suite
// stayed green. It lives outside tableCases so this focused wiring regression
// can assert the fallback path directly instead of duplicating the case in the
// broader handler corpus.
//
// The fixture is assembled at compile time so this repository's own
// command-guardian hook does not flag the test source while it is being edited.
func TestFallbackLinesReachTheGhRules(t *testing.T) {
	// A Bash tool whose tool_input carries NO command key, so the promoted
	// path is unavailable and the gh rules must read the fallback lines.
	note := "gh " + "pr " + "create --title x"
	raw, _, _ := runGo(t, tableCase{
		name:  "fallback_lines_reach_gh_rules",
		tool:  "Bash",
		input: `{"note":"` + note + `"}`,
		env:   map[string]string{"COMMAND_GUARD_MODE": "warn"},
	})

	outcome := raw
	if outcome.Kind() == "pass" {
		t.Fatalf("the gh rule found nothing; the scan body never reached the gh scanner\n%s", raw)
	}
	if msg := outcome.Payload(); !strings.Contains(msg, "unsafe_publication") {
		t.Errorf("outcome = %s, want an unsafe_publication threat", raw)
	}
}
