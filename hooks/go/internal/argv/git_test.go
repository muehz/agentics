package argv

import (
	"reflect"
	"sort"
	"strings"
	"testing"
)

// The git grammar's rows, copied as literals so that a row added to or
// dropped from the private tables fails TestGitGlobalsAreClosed.
var (
	gitValueRowNames  = []string{"-C", "-c", "--git-dir", "--work-tree", "--namespace", "--config-env"}
	gitJoinedRowNames = []string{"--exec-path"}
	gitFlagRowNames   = []string{
		"--literal-pathspecs", "--glob-pathspecs", "--noglob-pathspecs", "--icase-pathspecs",
		"--no-optional-locks", "--bare", "--no-pager", "-P", "--paginate", "-p",
		"--no-replace-objects", "--no-lazy-fetch", "--no-advice",
	}
	gitPrintingNames = []string{
		"--exec-path", "--html-path", "--man-path", "--info-path", "--help", "-h", "--version", "-v",
	}
)

func gitSeparate(name string, at int, option, value Word) GitGlobal {
	return GitGlobal{Name: name, Option: option, At: at, Value: value, HasValue: true, ValueAt: at + 1}
}

func gitAttached(name string, at int, option Word, value string) GitGlobal {
	return GitGlobal{Name: name, Option: option, At: at, Value: joinedValue(option, value), HasValue: true, ValueAt: -1}
}

func gitFlag(name string, at int, option Word) GitGlobal {
	return GitGlobal{Name: name, Option: option, At: at, ValueAt: -1}
}

func gitMissing(name string, at int, option Word) GitGlobal {
	return GitGlobal{Name: name, Option: option, At: at, ValueAt: -1}
}

func gitVerbAt(in []Word, at int, globals ...GitGlobal) Git {
	g := Git{State: GitVerb, Globals: globals, Verb: in[at], VerbAt: at, RestAt: len(in)}
	if at+1 < len(in) {
		g.Args = in[at+1:]
	}
	return g
}

func gitStopped(state GitState, in []Word, restAt int, globals ...GitGlobal) Git {
	g := Git{State: state, Globals: globals, VerbAt: -1, RestAt: restAt}
	if restAt < len(in) {
		g.Rest = in[restAt:]
	}
	return g
}

type gitRow struct {
	name string
	in   []Word
	want Git
}

func TestReadGit(t *testing.T) {
	var rows []gitRow
	add := func(name string, in []Word, want Git) {
		rows = append(rows, gitRow{name, in, want})
	}

	// Each value row in its separate and in its attached or `=` form, and
	// without its value.
	for _, name := range gitValueRowNames {
		in := words(name, "value", "status")
		add(name+" separate", in, gitVerbAt(in, 2, gitSeparate(name, 0, in[0], in[1])))
		var joinedIn []Word
		if strings.HasPrefix(name, "--") {
			joinedIn = words(name+"=value", "status")
		} else {
			joinedIn = words(name+"value", "status")
		}
		add(name+" attached", joinedIn, gitVerbAt(joinedIn, 1, gitAttached(name, 0, joinedIn[0], "value")))
		missing := words(name)
		add(name+" without its value", missing, gitStopped(GitUnread, missing, 1, gitMissing(name, 0, missing[0])))
	}
	{
		in := words("--exec-path=/usr/lib/git-core", "status")
		add("--exec-path=", in, gitVerbAt(in, 1, gitAttached("--exec-path", 0, in[0], "/usr/lib/git-core")))
	}
	for _, name := range gitFlagRowNames {
		in := words(name, "status")
		add(name, in, gitVerbAt(in, 1, gitFlag(name, 0, in[0])))
	}
	for _, name := range gitPrintingNames {
		in := words(name, "clean", "-fdx")
		add(name+" prints", in, gitStopped(GitPrints, in, 1))
	}

	nonLiteralVerb := []Word{expanded("$VERB"), lit("--hard"), lit("HEAD~1")}
	nonLiteralValue := []Word{lit("-C"), expanded("$DIR"), lit("reset"), lit("--hard"), lit("HEAD~1")}
	nonLiteralDash := []Word{expanded("--$GLOBAL"), lit("clean"), lit("-fdx")}
	nonLiteralDashAlone := []Word{expanded("--$X")}
	nonLiteralAttached := []Word{expanded("-c$X"), lit("status")}
	nonLiteralJoined := []Word{expanded("--git-dir=$GIT_DIR"), lit("clean"), lit("-fdx")}
	nonLiteralFlagValue := []Word{expanded("--bare=$X"), lit("status")}
	emptyVerb := []Word{{Text: "", Raw: `""`, Quoted: true, Literal: true}, lit("status")}
	for _, row := range []gitRow{
		{"empty input", nil, gitStopped(GitUnread, nil, 0)},
		{"unknown option", words("--attr-source", "HEAD", "checkout", "."), gitStopped(GitUnread, words("--attr-source", "HEAD", "checkout", "."), 1)},
		{"bare --exec-path prints", words("--exec-path", "checkout", "."), gitStopped(GitPrints, words("--exec-path", "checkout", "."), 1)},
		{"globals before a printing option", words("--bare", "--help", "clean"), gitStopped(GitPrints, words("--bare", "--help", "clean"), 2, gitFlag("--bare", 0, lit("--bare")))},
		{"non-literal verb", nonLiteralVerb, gitStopped(GitUnread, nonLiteralVerb, 0)},
		{"non-literal option value", nonLiteralValue, gitVerbAt(nonLiteralValue, 2, gitSeparate("-C", 0, nonLiteralValue[0], nonLiteralValue[1]))},
		{"non-literal dash word with words after it", nonLiteralDash, gitStopped(GitUnread, nonLiteralDash, 0)},
		{"non-literal dash word alone", nonLiteralDashAlone, gitStopped(GitUnread, nonLiteralDashAlone, 0)},
		{"non-literal attached config", nonLiteralAttached, gitVerbAt(nonLiteralAttached, 1, gitAttached("-c", 0, nonLiteralAttached[0], "$X"))},
		{"non-literal flag with a value", nonLiteralFlagValue, gitStopped(GitUnread, nonLiteralFlagValue, 0)},
		{"end of options", words("--", "status"), gitStopped(GitUnread, words("--", "status"), 1)},
		{"flag with a value", words("--no-pager=x", "status"), gitStopped(GitUnread, words("--no-pager=x", "status"), 1)},
		{"short flag with a value", words("-P=x", "status"), gitStopped(GitUnread, words("-P=x", "status"), 1)},
		{"globals without a verb", words("--no-pager"), gitStopped(GitUnread, words("--no-pager"), 1, gitFlag("--no-pager", 0, lit("--no-pager")))},
		{"empty verb", emptyVerb, gitVerbAt(emptyVerb, 0)},
		{"attached value starting with =", words("-C=x", "clean", "-fd"), gitVerbAt(words("-C=x", "clean", "-fd"), 1, gitAttached("-C", 0, lit("-C=x"), "=x"))},
		{"printing option with a value", words("--help=x", "status"), gitStopped(GitUnread, words("--help=x", "status"), 1)},
		{"flags never cluster", words("-Pp", "status"), gitStopped(GitUnread, words("-Pp", "status"), 1)},
		{"names match exactly", words("--GIT-DIR=x", "status"), gitStopped(GitUnread, words("--GIT-DIR=x", "status"), 1)},
		{"value option takes a dash word", words("-C", "--bare", "status"), gitVerbAt(words("-C", "--bare", "status"), 2, gitSeparate("-C", 0, lit("-C"), lit("--bare")))},
		{"verb in upper case", words("-p", "STATUS", "-s"), gitVerbAt(words("-p", "STATUS", "-s"), 1, gitFlag("-p", 0, lit("-p")))},

		// TestParsedGitCommandConsumesGlobalOptions in commandguard.
		{"old: directory and pager", words("--no-pager", "-C", "repo", "clean", "-fd"),
			gitVerbAt(words("--no-pager", "-C", "repo", "clean", "-fd"), 3, gitFlag("--no-pager", 0, lit("--no-pager")), gitSeparate("-C", 1, lit("-C"), lit("repo")))},
		{"old: config assignment", words("-c", "core.editor=true", "reset", "--hard", "HEAD~1"),
			gitVerbAt(words("-c", "core.editor=true", "reset", "--hard", "HEAD~1"), 2, gitSeparate("-c", 0, lit("-c"), lit("core.editor=true")))},
		{"old: config environment", words("--config-env=core.editor=GIT_EDITOR", "push", "origin", "main"),
			gitVerbAt(words("--config-env=core.editor=GIT_EDITOR", "push", "origin", "main"), 1, gitAttached("--config-env", 0, lit("--config-env=core.editor=GIT_EDITOR"), "core.editor=GIT_EDITOR"))},
		{"old: short pager", words("-P", "-p", "--no-advice", "push", "origin", "main"),
			gitVerbAt(words("-P", "-p", "--no-advice", "push", "origin", "main"), 3, gitFlag("-P", 0, lit("-P")), gitFlag("-p", 1, lit("-p")), gitFlag("--no-advice", 2, lit("--no-advice")))},
		{"old: dynamic directory value", nonLiteralValue, gitVerbAt(nonLiteralValue, 2, gitSeparate("-C", 0, nonLiteralValue[0], nonLiteralValue[1]))},
		{"old: dynamic joined directory value", nonLiteralJoined, gitVerbAt(nonLiteralJoined, 1, gitAttached("--git-dir", 0, nonLiteralJoined[0], "$GIT_DIR"))},

		// TestParsedGitCommandDoesNotTreatOutputOptionsAsVerbs in commandguard.
		{"old: --help clean", words("--help", "clean"), gitStopped(GitPrints, words("--help", "clean"), 1)},
		{"old: --version clean", words("--version", "clean"), gitStopped(GitPrints, words("--version", "clean"), 1)},
		{"old: --exec-path clean", words("--exec-path", "clean"), gitStopped(GitPrints, words("--exec-path", "clean"), 1)},
		{"old: --html-path clean", words("--html-path", "clean"), gitStopped(GitPrints, words("--html-path", "clean"), 1)},
		{"old: --man-path clean", words("--man-path", "clean"), gitStopped(GitPrints, words("--man-path", "clean"), 1)},
		{"old: --info-path clean", words("--info-path", "clean"), gitStopped(GitPrints, words("--info-path", "clean"), 1)},

		// TestParsedGitCommandDoesNotEvaluateDynamicNames in commandguard.
		{"old: --$GLOBAL clean -fdx", nonLiteralDash, gitStopped(GitUnread, nonLiteralDash, 0)},
		{"old: $VERB --hard HEAD~1", nonLiteralVerb, gitStopped(GitUnread, nonLiteralVerb, 0)},
	} {
		add(row.name, row.in, row.want)
	}

	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			got := ReadGit(row.in)
			want := row.want
			want.words = row.in
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("ReadGit(%s)\n got %#v\nwant %#v", gitText(row.in), got, want)
			}
			wantSub := ""
			if want.State == GitVerb {
				wantSub = strings.ToLower(want.Verb.Text)
			}
			if sub := got.Subcommand(); sub != wantSub {
				t.Fatalf("Subcommand() = %q, want %q", sub, wantSub)
			}
		})
	}
}

func gitText(in []Word) string {
	texts := make([]string, len(in))
	for i, word := range in {
		texts[i] = word.Text
	}
	return strings.Join(texts, " ")
}

func TestReadGitConfigSpecs(t *testing.T) {
	for _, tc := range []struct {
		name    string
		in      []Word
		specs   []Word
		missing bool
	}{
		{"empty input", nil, nil, false},
		{"separate -c", words("-c", "core.hooksPath=x", "status"), words("core.hooksPath=x"), false},
		{"attached -c", words("-ccore.hooksPath=x", "status"), []Word{joinedValue(lit("-ccore.hooksPath=x"), "core.hooksPath=x")}, false},
		{"attached -c with =", words("-c=x", "status"), []Word{joinedValue(lit("-c=x"), "=x")}, false},
		{"separate --config-env", words("--config-env", "core.hooksPath=HOOKS", "status"), words("core.hooksPath=HOOKS"), false},
		{"joined --config-env", words("--config-env=core.hooksPath=HOOKS", "status"), []Word{joinedValue(lit("--config-env=core.hooksPath=HOOKS"), "core.hooksPath=HOOKS")}, false},
		{"every spec in order", words("-c", "a=1", "--config-env=b=B", "-cc=3", "--config-env", "d=D", "push"),
			[]Word{lit("a=1"), joinedValue(lit("--config-env=b=B"), "b=B"), joinedValue(lit("-cc=3"), "c=3"), lit("d=D")}, false},
		{"non-literal separate key", []Word{lit("-c"), expanded("$KEY=x"), lit("status")}, []Word{expanded("$KEY=x")}, false},
		{"non-literal attached key", []Word{expanded("-c$KEY=x"), lit("status")}, []Word{joinedValue(expanded("-c$KEY=x"), "$KEY=x")}, false},
		{"unknown option before -c", words("--attr-source", "-c", "core.hooksPath=x", "status"), words("core.hooksPath=x"), false},
		{"printing option before -c", words("--help", "-c", "core.hooksPath=x"), words("core.hooksPath=x"), false},
		{"skipped values hide -c", words("-C", "-c", "--git-dir", "-c", "--work-tree", "-c", "--namespace", "-c", "-c", "a=b", "status"), words("a=b"), false},
		{"attached skipped options", words("-Cdir", "--git-dir=-c", "-c", "a=b", "status"), words("a=b"), false},
		{"missing -c value", words("-c"), nil, true},
		{"missing --config-env value after a spec", words("-c", "a=b", "--config-env"), words("a=b"), true},
		{"skipped option at the end", words("-C"), nil, false},
		{"stop at the verb", words("-c", "a=b", "status", "-c", "x=y"), words("a=b"), false},
		{"stop at the first word", words("status", "-c", "x=y"), nil, false},
		{"stop at a non-literal verb", []Word{expanded("$VERB"), lit("-c"), lit("x=y")}, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			specs, missing := ReadGit(tc.in).ConfigSpecs()
			if !reflect.DeepEqual(specs, tc.specs) || missing != tc.missing {
				t.Fatalf("ConfigSpecs(%s) = %#v, %v; want %#v, %v", gitText(tc.in), specs, missing, tc.specs, tc.missing)
			}
		})
	}
}

// TestGitGlobalsAreClosed compares the private tables with the literal lists
// in both directions, so one wrong row cannot hide a verb unnoticed.
func TestGitGlobalsAreClosed(t *testing.T) {
	want := map[string]Arity{}
	for _, name := range gitValueRowNames {
		want[name] = ArityValue
	}
	for _, name := range gitJoinedRowNames {
		want[name] = ArityJoined
	}
	for _, name := range gitFlagRowNames {
		want[name] = ArityFlag
	}
	got := map[string]Arity{}
	for _, option := range gitGlobalRows {
		if _, dup := got[option.Name]; dup {
			t.Errorf("global row %s appears twice", option.Name)
		}
		got[option.Name] = option.Arity
		if len(option.Spellings) != 0 {
			t.Errorf("global row %s has spellings", option.Name)
		}
	}
	for name, arity := range want {
		if g, ok := got[name]; !ok {
			t.Errorf("global row %s is missing", name)
		} else if g != arity {
			t.Errorf("global row %s has arity %d, want %d", name, g, arity)
		}
	}
	for name := range got {
		if _, ok := want[name]; !ok {
			t.Errorf("global row %s is not in the test's list", name)
		}
	}
	if len(gitFlagRowNames) != 13 || len(gitValueRowNames) != 6 {
		t.Errorf("the test lists %d flags and %d value options, want 13 and 6", len(gitFlagRowNames), len(gitValueRowNames))
	}

	gotPrinting := append([]string(nil), gitPrintingOptions...)
	wantPrinting := append([]string(nil), gitPrintingNames...)
	sort.Strings(gotPrinting)
	sort.Strings(wantPrinting)
	if !reflect.DeepEqual(gotPrinting, wantPrinting) {
		t.Errorf("printing options = %v, want %v", gotPrinting, wantPrinting)
	}
	if len(wantPrinting) != 8 {
		t.Errorf("the test lists %d printing options, want 8", len(wantPrinting))
	}
}

func FuzzReadGit(f *testing.F) {
	for _, seed := range []string{
		"", "status", "-C repo clean -fd", "-Crepo status", "-C=x clean -fd", "-C", "-c",
		"-c core.hooksPath=x commit", "-ccore.hooksPath=x commit", "--config-env=a=B push",
		"--config-env", "--exec-path", "--exec-path=x status", "--help clean", "-- status",
		"--no-pager=x status", "-Pp status", "--GIT-DIR=x status", "$VERB --hard", "--$X",
		"-c$X status", "-C $DIR reset --hard", "--attr-source HEAD checkout .", "--bare -p -P",
		" status", "- status", "--- x", "-C -c -c a=b status",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, line string) {
		input := fuzzWords(line)
		g := ReadGit(input)
		if g.RestAt < 0 || g.RestAt > len(input) {
			t.Fatalf("RestAt = %d for %d words", g.RestAt, len(input))
		}
		if len(g.Rest) != len(input)-g.RestAt {
			t.Fatalf("Rest has %d words, want the %d from RestAt", len(g.Rest), len(input)-g.RestAt)
		}
		for i, word := range g.Rest {
			if word != input[g.RestAt+i] {
				t.Fatalf("Rest[%d] is not input[%d]", i, g.RestAt+i)
			}
		}
		bound := g.RestAt
		switch g.State {
		case GitVerb:
			if g.VerbAt < 0 || g.VerbAt >= len(input) || g.Verb != input[g.VerbAt] {
				t.Fatalf("VerbAt = %d for %d words", g.VerbAt, len(input))
			}
			if len(g.Rest) != 0 {
				t.Fatalf("Rest = %v in the verb state", g.Rest)
			}
			if len(g.Args) != len(input)-g.VerbAt-1 {
				t.Fatalf("Args has %d words, want the %d after the verb", len(g.Args), len(input)-g.VerbAt-1)
			}
			for i, word := range g.Args {
				if word != input[g.VerbAt+1+i] {
					t.Fatalf("Args[%d] is not input[%d]", i, g.VerbAt+1+i)
				}
			}
			bound = g.VerbAt
		case GitPrints, GitUnread:
			if g.VerbAt != -1 || len(g.Args) != 0 || g.Subcommand() != "" {
				t.Fatalf("a line without a verb has VerbAt %d, %d args, subcommand %q", g.VerbAt, len(g.Args), g.Subcommand())
			}
		default:
			t.Fatalf("state %d is not declared", g.State)
		}
		for _, global := range g.Globals {
			if global.At < 0 || global.At >= bound || global.ValueAt >= bound {
				t.Fatalf("global at %d (value at %d) is not below %d", global.At, global.ValueAt, bound)
			}
			if global.Option != input[global.At] {
				t.Fatalf("global at %d does not hold its source word", global.At)
			}
		}
		specs, _ := g.ConfigSpecs()
		if len(specs) > len(input) {
			t.Fatalf("%d specs for %d words", len(specs), len(input))
		}
	})
}
