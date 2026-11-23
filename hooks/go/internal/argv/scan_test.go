package argv

import (
	"fmt"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// lit is a literal word whose raw form equals its text.
func lit(text string) Word {
	return Word{Text: text, Raw: text, Literal: true}
}

// expanded is a word whose text depends on a runtime expansion.
func expanded(text string) Word {
	return Word{Text: text, Raw: text}
}

func words(texts ...string) []Word {
	out := make([]Word, len(texts))
	for i, text := range texts {
		out[i] = lit(text)
	}
	return out
}

// joinedValue is the value word that an attached `name=value` form yields.
func joinedValue(source Word, value string) Word {
	source.Text = value
	source.Raw = ""
	return source
}

var coreOptions = []Option{
	{Name: "-v", Arity: ArityFlag},
	{Name: "--verbose", Arity: ArityFlag},
	{Name: "-o", Arity: ArityValue},
	{Name: "--output", Arity: ArityValue},
	{Name: "--color", Arity: ArityJoined},
	{Name: "--git-dir", Arity: ArityValue},
	{Name: "--context", Arity: ArityValue},
	{Name: "--data", Arity: ArityValue},
}

func operand(at int, word Word) Arg {
	return Arg{Role: RoleOperand, At: at, Word: word, ValueAt: -1}
}

func unknown(at int, word Word) Arg {
	return Arg{Role: RoleUnknown, At: at, Word: word, ValueAt: -1}
}

func end(at int, word Word) Arg {
	return Arg{Role: RoleEnd, At: at, Word: word, ValueAt: -1}
}

func flag(at int, word Word, name string) Arg {
	return Arg{Role: RoleOption, At: at, Word: word, Name: name, Exact: true, ValueAt: -1}
}

func separate(at int, word Word, name string, value Word) []Arg {
	return []Arg{
		{Role: RoleOption, At: at, Word: word, Name: name, Exact: true, Value: value, HasValue: true, ValueAt: at + 1},
		{Role: RoleValue, At: at + 1, Word: value, Name: name, ValueAt: -1},
	}
}

func joined(at int, word Word, name, value string) Arg {
	return Arg{Role: RoleOption, At: at, Word: word, Name: name, Exact: true,
		Value: joinedValue(word, value), HasValue: true, Joined: true, ValueAt: -1}
}

func args(parts ...any) []Arg {
	var out []Arg
	for _, part := range parts {
		switch p := part.(type) {
		case Arg:
			out = append(out, p)
		case []Arg:
			out = append(out, p...)
		default:
			panic(fmt.Sprintf("args: unexpected %T", part))
		}
	}
	return out
}

func TestScanCore(t *testing.T) {
	gitDir := expanded("--git-dir=$X")
	context := expanded("--context=$C")
	data := expanded("--data=@$PAYLOAD")
	nonLiteralEnd := expanded("--")
	tests := []struct {
		name        string
		traits      []Trait
		words       []Word
		want        []Arg
		wantProblem *Problem
	}{
		{
			name:  "flag",
			words: words("-v", "x"),
			want:  args(flag(0, lit("-v"), "-v"), operand(1, lit("x"))),
		},
		{
			name:  "value option with a separate value",
			words: words("-o", "f", "x"),
			want:  args(separate(0, lit("-o"), "-o", lit("f")), operand(2, lit("x"))),
		},
		{
			name:  "value joined by equals",
			words: words("--output=f"),
			want:  args(joined(0, lit("--output=f"), "--output", "f")),
		},
		{
			name:  "flag with an equals value",
			words: words("--verbose=yes", "x"),
			want:  args(joined(0, lit("--verbose=yes"), "--verbose", "yes"), operand(1, lit("x"))),
		},
		{
			name:        "value option as the last word",
			words:       words("-v", "-o"),
			want:        args(flag(0, lit("-v"), "-v"), flag(1, lit("-o"), "-o")),
			wantProblem: &Problem{Kind: MissingValue, At: 1, Word: lit("-o")},
		},
		{
			name:        "non-literal option word",
			words:       []Word{expanded("-$X"), lit("-v")},
			want:        args(unknown(0, expanded("-$X")), flag(1, lit("-v"), "-v")),
			wantProblem: &Problem{Kind: NotLiteral, At: 0, Word: expanded("-$X")},
		},
		{
			name:  "non-literal value is reported, not judged",
			words: []Word{lit("-o"), expanded("$F")},
			want:  args(separate(0, lit("-o"), "-o", expanded("$F"))),
		},
		{
			name:        "non-literal operand",
			words:       []Word{lit("-v"), expanded("$F")},
			want:        args(flag(0, lit("-v"), "-v"), operand(1, expanded("$F"))),
			wantProblem: &Problem{Kind: NotLiteral, At: 1, Word: expanded("$F")},
		},
		{
			name:        "unknown dash word is read as a flag and the walk continues",
			words:       words("-z", "-o", "f", "-q"),
			want:        args(unknown(0, lit("-z")), separate(1, lit("-o"), "-o", lit("f")), unknown(3, lit("-q"))),
			wantProblem: &Problem{Kind: UnknownOption, At: 0, Word: lit("-z")},
		},
		{
			name:  "double dash then option-shaped words",
			words: words("-v", "--", "-v", "-o", "--zz"),
			want: args(flag(0, lit("-v"), "-v"), end(1, lit("--")),
				operand(2, lit("-v")), operand(3, lit("-o")), operand(4, lit("--zz"))),
		},
		{
			name:  "options after an operand are still read",
			words: words("x", "-v", "-o", "f"),
			want:  args(operand(0, lit("x")), flag(1, lit("-v"), "-v"), separate(2, lit("-o"), "-o", lit("f"))),
		},
		{
			name:  "empty word is an operand",
			words: words(""),
			want:  args(operand(0, lit(""))),
		},
		{
			name:        "lone dash",
			words:       words("-"),
			want:        args(unknown(0, lit("-"))),
			wantProblem: &Problem{Kind: UnknownOption, At: 0, Word: lit("-")},
		},
		{
			name:        "triple dash",
			words:       words("---"),
			want:        args(unknown(0, lit("---"))),
			wantProblem: &Problem{Kind: UnknownOption, At: 0, Word: lit("---")},
		},
		{
			name:        "double dash with a value",
			words:       words("--=x", "y"),
			want:        args(unknown(0, lit("--=x")), operand(1, lit("y"))),
			wantProblem: &Problem{Kind: UnknownOption, At: 0, Word: lit("--=x")},
		},
		{
			name:        "unknown long option with a value",
			words:       words("--zz=1"),
			want:        args(unknown(0, lit("--zz=1"))),
			wantProblem: &Problem{Kind: UnknownOption, At: 0, Word: lit("--zz=1")},
		},
		{
			name:  "value option takes a following double dash",
			words: words("-o", "--", "x"),
			want:  args(separate(0, lit("-o"), "-o", lit("--")), operand(2, lit("x"))),
		},
		{
			name:  "value option takes a following dash word",
			words: words("-o", "-x"),
			want:  args(separate(0, lit("-o"), "-o", lit("-x"))),
		},
		{
			name:  "joined-only option without equals takes no value",
			words: words("--color", "x", "--color=always"),
			want: args(flag(0, lit("--color"), "--color"), operand(1, lit("x")),
				joined(2, lit("--color=always"), "--color", "always")),
		},
		{
			name:        "non-literal double dash by default",
			words:       []Word{nonLiteralEnd, lit("-v")},
			want:        args(unknown(0, nonLiteralEnd), flag(1, lit("-v"), "-v")),
			wantProblem: &Problem{Kind: NotLiteral, At: 0, Word: nonLiteralEnd},
		},
		{
			name:        "non-literal double dash read by its text",
			traits:      []Trait{ReadNonLiteral},
			words:       []Word{nonLiteralEnd, lit("-v")},
			want:        args(end(0, nonLiteralEnd), operand(1, lit("-v"))),
			wantProblem: &Problem{Kind: NotLiteral, At: 0, Word: nonLiteralEnd},
		},
		{
			name:        "non-literal git dir by default",
			words:       []Word{gitDir, lit("status")},
			want:        args(unknown(0, gitDir), operand(1, lit("status"))),
			wantProblem: &Problem{Kind: NotLiteral, At: 0, Word: gitDir},
		},
		{
			name:        "non-literal git dir read by its text",
			traits:      []Trait{ReadNonLiteral},
			words:       []Word{gitDir, lit("status")},
			want:        args(joined(0, gitDir, "--git-dir", "$X"), operand(1, lit("status"))),
			wantProblem: &Problem{Kind: NotLiteral, At: 0, Word: gitDir},
		},
		{
			name:        "non-literal context by default",
			words:       []Word{context},
			want:        args(unknown(0, context)),
			wantProblem: &Problem{Kind: NotLiteral, At: 0, Word: context},
		},
		{
			name:        "non-literal context read by its text",
			traits:      []Trait{ReadNonLiteral},
			words:       []Word{context},
			want:        args(joined(0, context, "--context", "$C")),
			wantProblem: &Problem{Kind: NotLiteral, At: 0, Word: context},
		},
		{
			name:        "non-literal data by default",
			words:       []Word{data},
			want:        args(unknown(0, data)),
			wantProblem: &Problem{Kind: NotLiteral, At: 0, Word: data},
		},
		{
			name:        "non-literal data read by its text",
			traits:      []Trait{ReadNonLiteral},
			words:       []Word{data},
			want:        args(joined(0, data, "--data", "@$PAYLOAD")),
			wantProblem: &Problem{Kind: NotLiteral, At: 0, Word: data},
		},
		{
			name:        "only the first problem is kept",
			words:       []Word{lit("-z"), expanded("-$Y"), lit("-o")},
			want:        args(unknown(0, lit("-z")), unknown(1, expanded("-$Y")), flag(2, lit("-o"), "-o")),
			wantProblem: &Problem{Kind: UnknownOption, At: 0, Word: lit("-z")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewSpec(coreOptions...).With(tt.traits...).Scan(tt.words)
			if !reflect.DeepEqual(got.Args, tt.want) {
				t.Errorf("Args:\n got %+v\nwant %+v", got.Args, tt.want)
			}
			checkProblem(t, got.Problem, tt.wantProblem)
		})
	}
}

func checkProblem(t *testing.T, got, want *Problem) {
	t.Helper()
	switch {
	case got == nil && want == nil:
	case got == nil || want == nil:
		t.Errorf("Problem = %+v, want %+v", got, want)
	case got.Kind != want.Kind || got.At != want.At || !reflect.DeepEqual(got.Word, want.Word):
		t.Errorf("Problem = {%v %d %+v}, want {%v %d %+v}", got.Kind, got.At, got.Word, want.Kind, want.At, want.Word)
	}
}

func TestScanMethods(t *testing.T) {
	spec := NewSpec(coreOptions...)

	commands := []struct {
		name        string
		words       []Word
		want        []Word
		wantProblem *Problem
	}{
		{
			name:        "problem before the operand",
			words:       words("-z", "x", "-v"),
			want:        words("x", "-v"),
			wantProblem: &Problem{Kind: UnknownOption, At: 0, Word: lit("-z")},
		},
		{
			name:  "problem after the operand",
			words: words("-v", "x", "-z"),
			want:  words("x", "-z"),
		},
		{
			name:        "no operand",
			words:       words("-v", "-z"),
			wantProblem: &Problem{Kind: UnknownOption, At: 1, Word: lit("-z")},
		},
		{
			name:  "operand after double dash",
			words: words("-o", "f", "--", "-x", "y"),
			want:  words("-x", "y"),
		},
	}
	for _, tt := range commands {
		t.Run("Command/"+tt.name, func(t *testing.T) {
			got, problem := spec.Scan(tt.words).Command()
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Command words = %+v, want %+v", got, tt.want)
			}
			checkProblem(t, problem, tt.wantProblem)
		})
	}

	t.Run("End", func(t *testing.T) {
		if got := spec.Scan(words("-v", "x", "--", "y")).End(); got != 2 {
			t.Errorf("End with double dash = %d, want 2", got)
		}
		if got := spec.Scan(words("-v", "x")).End(); got != -1 {
			t.Errorf("End without double dash = %d, want -1", got)
		}
		if got := spec.Scan(words("-o", "--")).End(); got != -1 {
			t.Errorf("End with double dash as a value = %d, want -1", got)
		}
	})

	t.Run("Operands", func(t *testing.T) {
		got := spec.Scan(words("a", "-o", "b", "--", "c")).Operands()
		if want := words("a", "c"); !reflect.DeepEqual(got, want) {
			t.Errorf("Operands = %+v, want %+v", got, want)
		}
	})

	t.Run("Values", func(t *testing.T) {
		got := spec.Scan(words("-o", "a", "-v", "-o=b", "-o")).Values("-o")
		want := []Word{lit("a"), joinedValue(lit("-o=b"), "b")}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("Values = %+v, want %+v", got, want)
		}
		if got := spec.Scan(words("-o", "a")).Values("--output"); len(got) != 0 {
			t.Errorf("Values of an absent option = %+v, want none", got)
		}
	})
}

func TestNewSpecPanics(t *testing.T) {
	panics := []struct {
		name    string
		options []Option
	}{
		{name: "empty name", options: []Option{{Name: ""}}},
		{name: "name without a dash", options: []Option{{Name: "v"}}},
		{name: "name containing equals", options: []Option{{Name: "--a=b", Arity: ArityValue}}},
		{name: "duplicate with a different arity", options: []Option{{Name: "-o", Arity: ArityValue}, {Name: "-o", Arity: ArityFlag}}},
	}
	for _, tt := range panics {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Errorf("NewSpec(%+v) did not panic", tt.options)
				}
			}()
			NewSpec(tt.options...)
		})
	}

	t.Run("duplicate with the same arity merges and the first wins", func(t *testing.T) {
		spec := NewSpec(
			Option{Name: "--dry-run", Arity: ArityOptional, Spellings: []string{"client"}},
			Option{Name: "--dry-run", Arity: ArityOptional, Spellings: []string{"server"}},
		)
		want := []Option{{Name: "--dry-run", Arity: ArityOptional, Spellings: []string{"client"}}}
		if !reflect.DeepEqual(spec.options, want) {
			t.Errorf("options = %+v, want %+v", spec.options, want)
		}
	})
}

func TestProblemKindString(t *testing.T) {
	names := map[ProblemKind]string{
		UnknownOption: "UnknownOption",
		MissingValue:  "MissingValue",
		NotLiteral:    "NotLiteral",
	}
	const marker = "MARKER-7f3a"
	for kind, want := range names {
		if got := kind.String(); got != want {
			t.Errorf("%d.String() = %q, want %q", kind, got, want)
		}
		problem := &Problem{Kind: kind, Word: Word{Text: marker, Raw: marker, Literal: true}}
		if got := problem.Kind.String(); strings.Contains(got, marker) {
			t.Errorf("a problem's kind renders its word: %q", got)
		}
		if _, ok := any(problem).(error); ok {
			t.Error("*Problem implements error, so it could be rendered into a message")
		}
		if _, ok := any(problem).(fmt.Stringer); ok {
			t.Error("*Problem implements fmt.Stringer, so it could be rendered into a message")
		}
		if _, ok := any(*problem).(fmt.Stringer); ok {
			t.Error("Problem implements fmt.Stringer, so it could be rendered into a message")
		}
	}
	if got := ProblemKind(200).String(); got != "InvalidProblemKind" {
		t.Errorf("an undeclared kind renders %q, want the fixed name InvalidProblemKind", got)
	}
}

// Rows copied from the walkers the traits will replace, so that each trait is
// proven against the reading it must keep.
var (
	gitCleanOptions = []Option{
		{Name: "-f", Arity: ArityFlag},
		{Name: "-n", Arity: ArityFlag},
		{Name: "-x", Arity: ArityFlag},
		{Name: "-X", Arity: ArityFlag},
		{Name: "-e", Arity: ArityValue},
	}
	gitConfigOptions = []Option{
		{Name: "--unset", Arity: ArityFlag},
		{Name: "--unset-all", Arity: ArityFlag},
		{Name: "--file", Arity: ArityValue},
		{Name: "--get", Arity: ArityFlag},
		{Name: "--get-all", Arity: ArityFlag},
		{Name: "--get-regexp", Arity: ArityFlag},
	}
	curlOptions = []Option{
		{Name: "-s", Arity: ArityFlag},
		{Name: "-S", Arity: ArityFlag},
		{Name: "-o", Arity: ArityValue},
		{Name: "-d", Arity: ArityValue},
		{Name: "-T", Arity: ArityValue},
	}
	readOptions = []Option{
		{Name: "-p", Arity: ArityValue},
		{Name: "-d", Arity: ArityValue},
		{Name: "-n", Arity: ArityValue},
		{Name: "-N", Arity: ArityValue},
		{Name: "-t", Arity: ArityValue},
		{Name: "-u", Arity: ArityValue},
		{Name: "-i", Arity: ArityValue},
		{Name: "-a", Arity: ArityValue},
	}
	dryRunOptions = []Option{
		{Name: "--dry-run", Arity: ArityOptional, Spellings: []string{"client", "server", "true", "false"}},
	}
	// The old walkers lowered a word with strings.ToLower and looked it up in
	// a lower-case table, so a folded match must agree with that reading and
	// be no looser.
	pulumiOptions = []Option{
		{Name: "--stack", Arity: ArityValue},
		{Name: "--kind", Arity: ArityValue},
	}
	gitGlobalOptions = []Option{
		{Name: "-C", Arity: ArityValue},
		{Name: "-c", Arity: ArityValue},
		{Name: "-P", Arity: ArityFlag},
		{Name: "-p", Arity: ArityFlag},
	}
)

func traitSpecs() map[string]Spec {
	return map[string]Spec{
		"git clean":  NewSpec(gitCleanOptions...).With(Clusters, AnyLetter),
		"git config": NewSpec(gitConfigOptions...).With(Abbreviations),
		"curl":       NewSpec(curlOptions...).With(Clusters, ReadNonLiteral),
		"read":       NewSpec(readOptions...).With(Clusters, AnyLetter),
		"helm":       NewSpec(dryRunOptions...).With(FoldNames),
		"git":        NewSpec(gitGlobalOptions...).With(AttachedShort),
		"hash":       NewSpec(Option{Name: "-p", Arity: ArityValue}).With(Clusters, AnyLetter),
		"ls":         NewSpec().With(Clusters, AnyLetter),
		"fold clusters": NewSpec(
			Option{Name: "-s", Arity: ArityFlag}, Option{Name: "--verbose", Arity: ArityFlag},
		).With(Clusters, FoldNames),
		"cluster abbreviations": NewSpec(
			Option{Name: "-s", Arity: ArityFlag}, Option{Name: "--silent", Arity: ArityFlag},
		).With(Clusters, Abbreviations),
		"cluster joined": NewSpec(
			Option{Name: "-s", Arity: ArityFlag}, Option{Name: "-j", Arity: ArityJoined},
			Option{Name: "-q", Arity: ArityOptional, Spellings: []string{"on"}},
		).With(Clusters),
		"terraform":            NewSpec(Option{Name: "-chdir", Arity: ArityValue}),
		"long file":            NewSpec(Option{Name: "--file", Arity: ArityValue}).With(Abbreviations),
		"pulumi":               NewSpec(pulumiOptions...).With(FoldNames),
		"pulumi abbreviations": NewSpec(pulumiOptions...).With(FoldNames, Abbreviations),
	}
}

// cluster is a cluster arg read without a problem.
func cluster(at int, word Word, name, letters string) Arg {
	return Arg{Role: RoleOption, At: at, Word: word, Name: name, Exact: name != "", Letters: letters, ValueAt: -1}
}

func withJoined(arg Arg, value string) Arg {
	arg.Value = joinedValue(arg.Word, value)
	arg.HasValue = true
	arg.Joined = true
	return arg
}

func withSeparate(arg Arg, value Word) []Arg {
	arg.Value = value
	arg.HasValue = true
	arg.ValueAt = arg.At + 1
	return []Arg{arg, {Role: RoleValue, At: arg.At + 1, Word: value, Name: arg.Name, ValueAt: -1}}
}

func inexact(arg Arg) Arg {
	arg.Exact = false
	return arg
}

func unknownLetters(at int, word Word, letters string) Arg {
	arg := unknown(at, word)
	arg.Letters = letters
	return arg
}

func TestScanTraits(t *testing.T) {
	nonLiteralUpload := expanded("-T$F")
	tests := []struct {
		name        string
		spec        string
		words       []Word
		want        []Arg
		wantProblem *Problem
	}{
		// FoldNames
		{
			name:  "fold: exact spelling",
			spec:  "helm",
			words: words("--dry-run=client"),
			want:  args(joined(0, lit("--dry-run=client"), "--dry-run", "client")),
		},
		{
			name:  "fold: another case gives the table's spelling",
			spec:  "helm",
			words: words("--Dry-Run=server"),
			want:  args(inexact(joined(0, lit("--Dry-Run=server"), "--dry-run", "server"))),
		},
		{
			name:        "fold: a longer name is still unknown",
			spec:        "helm",
			words:       words("--dry-runs"),
			want:        args(unknown(0, lit("--dry-runs"))),
			wantProblem: &Problem{Kind: UnknownOption, At: 0, Word: lit("--dry-runs")},
		},
		{
			name:  "fold: a long option read in any case beside clusters",
			spec:  "fold clusters",
			words: words("--VERBOSE"),
			want:  args(inexact(flag(0, lit("--VERBOSE"), "--verbose"))),
		},
		{
			name:        "fold: cluster letters never fold",
			spec:        "fold clusters",
			words:       words("-sS"),
			want:        args(unknownLetters(0, lit("-sS"), "sS")),
			wantProblem: &Problem{Kind: UnknownOption, At: 0, Word: lit("-sS")},
		},
		{
			name:  "fold: ASCII upper case matches",
			spec:  "pulumi",
			words: words("--STACK", "dev"),
			want:  args(withSeparate(inexact(flag(0, lit("--STACK"), "--stack")), lit("dev"))),
		},
		{
			name:        "fold: the long s is not s, as strings.ToLower reads it",
			spec:        "pulumi",
			words:       words("--ſtack", "dev"),
			want:        args(unknown(0, lit("--ſtack")), operand(1, lit("dev"))),
			wantProblem: &Problem{Kind: UnknownOption, At: 0, Word: lit("--ſtack")},
		},
		{
			name:  "fold: the Kelvin sign lowers to k",
			spec:  "pulumi",
			words: words("--Kind", "x"),
			want:  args(withSeparate(inexact(flag(0, lit("--Kind"), "--kind")), lit("x"))),
		},
		{
			name:  "fold: a spelling in upper case",
			spec:  "helm",
			words: words("--dry-run", "SERVER"),
			want:  args(separate(0, lit("--dry-run"), "--dry-run", lit("SERVER"))),
		},
		{
			name:  "fold: a spelling with the long s is not taken",
			spec:  "helm",
			words: words("--dry-run", "ſerver"),
			want:  args(flag(0, lit("--dry-run"), "--dry-run"), operand(1, lit("ſerver"))),
		},
		{
			name:  "fold abbreviation: ASCII upper case prefix",
			spec:  "pulumi abbreviations",
			words: words("--ST", "dev"),
			want:  args(withSeparate(inexact(flag(0, lit("--ST"), "--stack")), lit("dev"))),
		},
		{
			name:        "fold abbreviation: a long s prefix is unknown",
			spec:        "pulumi abbreviations",
			words:       words("--ſt", "dev"),
			want:        args(unknown(0, lit("--ſt")), operand(1, lit("dev"))),
			wantProblem: &Problem{Kind: UnknownOption, At: 0, Word: lit("--ſt")},
		},
		{
			name:        "fold abbreviation: a whole long s name is unknown",
			spec:        "pulumi abbreviations",
			words:       words("--ſtack", "dev"),
			want:        args(unknown(0, lit("--ſtack")), operand(1, lit("dev"))),
			wantProblem: &Problem{Kind: UnknownOption, At: 0, Word: lit("--ſtack")},
		},
		{
			name:  "fold abbreviation: a Kelvin sign prefix",
			spec:  "pulumi abbreviations",
			words: words("--Ki", "x"),
			want:  args(withSeparate(inexact(flag(0, lit("--Ki"), "--kind")), lit("x"))),
		},
		// Abbreviations
		{
			name:  "abbreviation: unique prefix",
			spec:  "git config",
			words: words("--unset-a"),
			want:  args(inexact(flag(0, lit("--unset-a"), "--unset-all"))),
		},
		{
			name:  "abbreviation: an exact row wins over a longer row",
			spec:  "git config",
			words: words("--unset"),
			want:  args(flag(0, lit("--unset"), "--unset")),
		},
		{
			name:  "abbreviation: a value option takes the next word",
			spec:  "git config",
			words: words("--fil", "x"),
			want:  args(withSeparate(inexact(flag(0, lit("--fil"), "--file")), lit("x"))),
		},
		{
			name:  "abbreviation: a value option joined by equals",
			spec:  "git config",
			words: words("--fil=x"),
			want:  args(inexact(joined(0, lit("--fil=x"), "--file", "x"))),
		},
		{
			name:        "abbreviation: ambiguous prefix",
			spec:        "git config",
			words:       words("--un", "x"),
			want:        args(unknown(0, lit("--un")), operand(1, lit("x"))),
			wantProblem: &Problem{Kind: UnknownOption, At: 0, Word: lit("--un")},
		},
		{
			name:        "abbreviation: ambiguous prefix ending in a dash",
			spec:        "git config",
			words:       words("--get-"),
			want:        args(unknown(0, lit("--get-"))),
			wantProblem: &Problem{Kind: UnknownOption, At: 0, Word: lit("--get-")},
		},
		{
			name:        "abbreviation: a word longer than every row",
			spec:        "git config",
			words:       words("--filex"),
			want:        args(unknown(0, lit("--filex"))),
			wantProblem: &Problem{Kind: UnknownOption, At: 0, Word: lit("--filex")},
		},
		{
			name:        "abbreviation: a single-dash word never abbreviates",
			spec:        "git config",
			words:       words("-u"),
			want:        args(unknown(0, lit("-u"))),
			wantProblem: &Problem{Kind: UnknownOption, At: 0, Word: lit("-u")},
		},
		{
			name:  "abbreviation: one long row resolves its prefix",
			spec:  "long file",
			words: words("--f=x"),
			want:  args(inexact(joined(0, lit("--f=x"), "--file", "x"))),
		},
		{
			name:        "abbreviation: triple dash",
			spec:        "long file",
			words:       words("---"),
			want:        args(unknown(0, lit("---"))),
			wantProblem: &Problem{Kind: UnknownOption, At: 0, Word: lit("---")},
		},
		{
			name:        "abbreviation: an empty long name never abbreviates",
			spec:        "long file",
			words:       words("--=x"),
			want:        args(unknown(0, lit("--=x"))),
			wantProblem: &Problem{Kind: UnknownOption, At: 0, Word: lit("--=x")},
		},
		{
			name:        "abbreviation: an unresolved long word is not a cluster",
			spec:        "cluster abbreviations",
			words:       words("--ss", "x"),
			want:        args(unknown(0, lit("--ss")), operand(1, lit("x"))),
			wantProblem: &Problem{Kind: UnknownOption, At: 0, Word: lit("--ss")},
		},
		// Clusters
		{
			name:  "cluster: git clean value letter takes the rest",
			spec:  "git clean",
			words: words("-feconfig"),
			want:  args(withJoined(cluster(0, lit("-feconfig"), "-e", "fe"), "config")),
		},
		{
			name:  "cluster: git clean value letter takes the next word",
			spec:  "git clean",
			words: words("-fe", "x"),
			want:  args(withSeparate(cluster(0, lit("-fe"), "-e", "fe"), lit("x"))),
		},
		{
			name:        "cluster: git clean value letter as the last word",
			spec:        "git clean",
			words:       words("-fe"),
			want:        args(cluster(0, lit("-fe"), "-e", "fe")),
			wantProblem: &Problem{Kind: MissingValue, At: 0, Word: lit("-fe")},
		},
		{
			name:  "cluster: a single-letter row is read as an option",
			spec:  "git clean",
			words: words("-x", "-e", "y"),
			want:  args(flag(0, lit("-x"), "-x"), separate(1, lit("-e"), "-e", lit("y"))),
		},
		{
			name:  "cluster: curl flags then a value letter",
			spec:  "curl",
			words: words("-sSo", "x"),
			want:  args(withSeparate(cluster(0, lit("-sSo"), "-o", "sSo"), lit("x"))),
		},
		{
			name:        "cluster: an unknown letter ends the cluster",
			spec:        "curl",
			words:       words("-sWo", "f"),
			want:        args(unknownLetters(0, lit("-sWo"), "sW"), operand(1, lit("f"))),
			wantProblem: &Problem{Kind: UnknownOption, At: 0, Word: lit("-sWo")},
		},
		{
			name:  "cluster: curl data value keeps its equals",
			spec:  "curl",
			words: words("-dfoo=o"),
			want:  args(withJoined(cluster(0, lit("-dfoo=o"), "-d", "d"), "foo=o")),
		},
		{
			name:  "cluster: curl data never splits at equals",
			spec:  "curl",
			words: words("-d=@payload"),
			want:  args(withJoined(cluster(0, lit("-d=@payload"), "-d", "d"), "=@payload")),
		},
		{
			name:  "cluster: curl upload with an equals value",
			spec:  "curl",
			words: words("-T=x"),
			want:  args(withJoined(cluster(0, lit("-T=x"), "-T", "T"), "=x")),
		},
		{
			name:        "cluster: a non-literal upload is read by its text",
			spec:        "curl",
			words:       []Word{nonLiteralUpload},
			want:        args(withJoined(cluster(0, nonLiteralUpload, "-T", "T"), "$F")),
			wantProblem: &Problem{Kind: NotLiteral, At: 0, Word: nonLiteralUpload},
		},
		{
			name:  "cluster: read value letter takes the rest",
			spec:  "read",
			words: words("-pt", "x"),
			want:  args(withJoined(cluster(0, lit("-pt"), "-p", "p"), "t"), operand(1, lit("x"))),
		},
		{
			name:  "cluster: any letter reads flags that are not rows",
			spec:  "read",
			words: words("-rs", "x"),
			want:  args(cluster(0, lit("-rs"), "", "rs"), operand(1, lit("x"))),
		},
		{
			name:  "cluster: any letter with no rows",
			spec:  "ls",
			words: words("-la"),
			want:  args(cluster(0, lit("-la"), "", "la")),
		},
		{
			name:  "cluster: a long word that is not a row is a cluster",
			spec:  "hash",
			words: words("--p", "x"),
			want:  args(withSeparate(cluster(0, lit("--p"), "-p", "-p"), lit("x"))),
		},
		{
			name:  "cluster: a joined letter takes the rest only",
			spec:  "cluster joined",
			words: words("-sjx", "-sj", "y"),
			want: args(withJoined(cluster(0, lit("-sjx"), "-j", "sj"), "x"),
				cluster(1, lit("-sj"), "-j", "sj"), operand(2, lit("y"))),
		},
		{
			name:  "cluster: an optional letter takes a listed next word",
			spec:  "cluster joined",
			words: words("-sq", "on", "-sq", "off"),
			want: args(withSeparate(cluster(0, lit("-sq"), "-q", "sq"), lit("on")),
				cluster(2, lit("-sq"), "-q", "sq"), operand(3, lit("off"))),
		},
		// AttachedShort
		{
			name:  "attached: a value letter takes the rest",
			spec:  "git",
			words: words("-Cdir", "status"),
			want:  args(joined(0, lit("-Cdir"), "-C", "dir"), operand(1, lit("status"))),
		},
		{
			name:  "attached: a config value keeps its equals",
			spec:  "git",
			words: words("-cuser.name=x"),
			want:  args(joined(0, lit("-cuser.name=x"), "-c", "user.name=x")),
		},
		{
			name:  "attached: a single-dash word never splits at equals",
			spec:  "git",
			words: words("-C=dir"),
			want:  args(joined(0, lit("-C=dir"), "-C", "=dir")),
		},
		{
			name:  "attached: a separate value",
			spec:  "git",
			words: words("-C", "dir", "-P"),
			want:  args(separate(0, lit("-C"), "-C", lit("dir")), flag(2, lit("-P"), "-P")),
		},
		{
			name:        "attached: flag letters do not cluster",
			spec:        "git",
			words:       words("-Pp", "status"),
			want:        args(unknown(0, lit("-Pp")), operand(1, lit("status"))),
			wantProblem: &Problem{Kind: UnknownOption, At: 0, Word: lit("-Pp")},
		},
		{
			name:        "attached: a flag letter takes no attached value",
			spec:        "git",
			words:       words("-P=x"),
			want:        args(unknown(0, lit("-P=x"))),
			wantProblem: &Problem{Kind: UnknownOption, At: 0, Word: lit("-P=x")},
		},
		// Splitting at equals without the short traits
		{
			name:  "a single-dash long row splits at equals",
			spec:  "terraform",
			words: words("-chdir=x"),
			want:  args(joined(0, lit("-chdir=x"), "-chdir", "x")),
		},
		// ArityOptional
		{
			name:  "optional: a listed next word is the value",
			spec:  "helm",
			words: words("--dry-run", "server"),
			want:  args(separate(0, lit("--dry-run"), "--dry-run", lit("server"))),
		},
		{
			name:  "optional: a listed next word in another case",
			spec:  "helm",
			words: words("--dry-run", "FALSE"),
			want:  args(separate(0, lit("--dry-run"), "--dry-run", lit("FALSE"))),
		},
		{
			name:  "optional: an unlisted next word is an operand",
			spec:  "helm",
			words: words("--dry-run", "x"),
			want:  args(flag(0, lit("--dry-run"), "--dry-run"), operand(1, lit("x"))),
		},
		{
			name:        "optional: a following option is not taken",
			spec:        "helm",
			words:       words("--dry-run", "--help"),
			want:        args(flag(0, lit("--dry-run"), "--dry-run"), unknown(1, lit("--help"))),
			wantProblem: &Problem{Kind: UnknownOption, At: 1, Word: lit("--help")},
		},
		{
			name:  "optional: as the last word",
			spec:  "helm",
			words: words("--dry-run"),
			want:  args(flag(0, lit("--dry-run"), "--dry-run")),
		},
		{
			name:  "optional: the equals form always carries a value",
			spec:  "helm",
			words: words("--dry-run=x"),
			want:  args(joined(0, lit("--dry-run=x"), "--dry-run", "x")),
		},
	}
	specs := traitSpecs()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, ok := specs[tt.spec]
			if !ok {
				t.Fatalf("no spec %q", tt.spec)
			}
			got := spec.Scan(tt.words)
			if !reflect.DeepEqual(got.Args, tt.want) {
				t.Errorf("Args:\n got %+v\nwant %+v", got.Args, tt.want)
			}
			checkProblem(t, got.Problem, tt.wantProblem)
		})
	}

	t.Run("Values includes a cluster's value letter", func(t *testing.T) {
		got := specs["curl"].Scan(words("-sSo", "x", "-o", "y")).Values("-o")
		if want := words("x", "y"); !reflect.DeepEqual(got, want) {
			t.Errorf("Values = %+v, want %+v", got, want)
		}
	})

	t.Run("folded duplicates", func(t *testing.T) {
		func() {
			defer func() {
				if recover() == nil {
					t.Error("With(FoldNames) over -L and -l with different arities did not panic")
				}
			}()
			NewSpec(Option{Name: "-L", Arity: ArityValue}, Option{Name: "-l", Arity: ArityFlag}).With(FoldNames)
		}()
		spec := NewSpec(Option{Name: "-L", Arity: ArityValue}, Option{Name: "-l", Arity: ArityValue}).With(FoldNames)
		got := spec.Scan(words("-l", "x")).Args
		if want := args(withSeparate(inexact(flag(0, lit("-l"), "-L")), lit("x"))); !reflect.DeepEqual(got, want) {
			t.Errorf("merged rows: got %+v, want %+v", got, want)
		}
		longS := NewSpec(Option{Name: "--stack", Arity: ArityValue}, Option{Name: "--ſtack", Arity: ArityFlag}).With(FoldNames)
		if got := longS.Scan(words("--ſtack", "x")).Args; !reflect.DeepEqual(got, args(flag(0, lit("--ſtack"), "--ſtack"), operand(1, lit("x")))) {
			t.Errorf("a long s row merged with an s row: got %+v", got)
		}
		plain := NewSpec(Option{Name: "-L", Arity: ArityValue}, Option{Name: "-l", Arity: ArityFlag})
		if got := plain.Scan(words("-l", "x")).Args; !reflect.DeepEqual(got, args(flag(0, lit("-l"), "-l"), operand(1, lit("x")))) {
			t.Errorf("without FoldNames the rows stay apart: got %+v", got)
		}
	})

	// Hostile input must not make the walk allocate per letter or per word
	// beyond the result, or it could exceed a check's deadline.
	t.Run("allocations do not grow with the input", func(t *testing.T) {
		const bound = 3
		long := []Word{lit("-" + strings.Repeat("sS", 2047) + "o"), lit("x")}
		many := make([]Word, 1000)
		for i := range many {
			many[i] = lit([]string{"-sSo", "x", "--zz", "-d=@p", "op"}[i%5])
		}
		folded := make([]Word, 1000)
		for i := range folded {
			folded[i] = lit([]string{"--Dry-Run", "--UNSET-A", "x"}[i%3])
		}
		mixed := append(append([]Option{}, dryRunOptions...), gitConfigOptions...)
		cases := []struct {
			name  string
			spec  Spec
			words []Word
		}{
			{"4096-byte cluster", specs["curl"], long},
			{"1000 words", specs["curl"], many},
			{"1000 folded and abbreviated words", NewSpec(mixed...).With(FoldNames, Abbreviations), folded},
		}
		for _, c := range cases {
			if got := testing.AllocsPerRun(20, func() { c.spec.Scan(c.words) }); got > bound {
				t.Errorf("%s: %v allocations, want at most %d", c.name, got, bound)
			}
		}
	})
}

func TestScanStop(t *testing.T) {
	core := NewSpec(coreOptions...)
	isText := func(text string) func(Word) bool {
		return func(w Word) bool { return w.Text == text }
	}
	tests := []struct {
		name   string
		words  []Word
		stop   func(Word) bool
		at     int
		want   []Arg
		reason *Problem
	}{
		{name: "stops before the first match", words: words("-v", "x", "-v"), stop: isText("x"), at: 1, want: args(flag(0, lit("-v"), "-v"))},
		{name: "a value never stops", words: words("-o", "x", "x"), stop: isText("x"), at: 2, want: args(separate(0, lit("-o"), "-o", lit("x")))},
		{name: "an option value that is an option never stops", words: words("-o", "-v", "-v"), stop: isText("-v"), at: 2, want: args(separate(0, lit("-o"), "-o", lit("-v")))},
		{name: "the first word may stop", words: words("x", "-v"), stop: isText("x"), at: 0, want: []Arg{}},
		{name: "the end of options may stop", words: words("-v", "--", "x"), stop: isText("--"), at: 1, want: args(flag(0, lit("-v"), "-v"))},
		{name: "nothing after the end stops", words: words("--", "x", "-v"), stop: isText("x"), at: 3, want: args(end(0, lit("--")), operand(1, lit("x")), operand(2, lit("-v")))},
		{name: "no match reads every word", words: words("-v", "y"), stop: isText("x"), at: 2, want: args(flag(0, lit("-v"), "-v"), operand(1, lit("y")))},
		{name: "a problem before the stop is kept", words: words("--zz", "x", "--yy"), stop: isText("x"), at: 1, want: args(unknown(0, lit("--zz"))), reason: &Problem{Kind: UnknownOption, At: 0, Word: lit("--zz")}},
		{name: "a problem at or after the stop is not read", words: words("-v", "x", "--zz"), stop: isText("x"), at: 1, want: args(flag(0, lit("-v"), "-v"))},
		{name: "a stop word that would take a value is not read", words: words("-v", "-o"), stop: isText("-o"), at: 1, want: args(flag(0, lit("-v"), "-v"))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scan, at := core.ScanStop(tt.words, tt.stop)
			if at != tt.at {
				t.Fatalf("stop index = %d, want %d", at, tt.at)
			}
			if !reflect.DeepEqual(scan.Args, tt.want) {
				t.Errorf("args = %+v, want %+v", scan.Args, tt.want)
			}
			checkProblem(t, scan.Problem, tt.reason)
			// The reading before the stop is the prefix of Scan's reading.
			full := core.Scan(tt.words).Args
			k := 0
			for k < len(full) && full[k].At < at {
				k++
			}
			if !reflect.DeepEqual(scan.Args, full[:k]) {
				t.Errorf("args = %+v, want Scan's prefix %+v", scan.Args, full[:k])
			}
		})
	}

	t.Run("the predicate sees only words where an argument starts", func(t *testing.T) {
		var seen []string
		core.ScanStop(words("-v", "-o", "a", "--color=x", "b", "--", "c"), func(w Word) bool {
			seen = append(seen, w.Text)
			return false
		})
		if want := []string{"-v", "-o", "--color=x", "b", "--"}; !reflect.DeepEqual(seen, want) {
			t.Errorf("predicate saw %q, want %q", seen, want)
		}
	})

	// A caller that resumes after each stop must pay for each word once, so a
	// line packed with stop words costs memory linear in its length.
	t.Run("resuming after every stop allocates linearly", func(t *testing.T) {
		const n = 2000
		line := make([]Word, n)
		for i := range line {
			line[i] = lit([]string{"-v", "x"}[i%2])
		}
		stop := isText("x")
		walk := func() {
			for rest := line; len(rest) > 0; {
				_, at := core.ScanStop(rest, stop)
				rest = rest[min(at+1, len(rest)):]
			}
		}
		calls := n/2 + 1
		if got := testing.AllocsPerRun(5, walk); got > float64(calls) {
			t.Errorf("%v allocations for %d stops, want at most one per stop", got, calls)
		}
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		walk()
		runtime.ReadMemStats(&after)
		if got, bound := after.TotalAlloc-before.TotalAlloc, uint64(4*n)*uint64(reflect.TypeFor[Arg]().Size()); got > bound {
			t.Errorf("%d bytes allocated for %d words, want at most %d", got, n, bound)
		}
	})
}

func TestScanFlag(t *testing.T) {
	spec := NewSpec(
		Option{Name: "--force", Arity: ArityFlag},
		Option{Name: "--no-force", Arity: ArityFlag},
		Option{Name: "-f", Arity: ArityFlag},
		Option{Name: "-n", Arity: ArityFlag},
		Option{Name: "-o", Arity: ArityValue},
	).With(Clusters)
	helm := NewSpec(dryRunOptions...).With(FoldNames)
	tests := []struct {
		name           string
		spec           Spec
		words          []Word
		flag, negative string
		want           FlagState
	}{
		{name: "absent", spec: spec, words: words("x", "-o", "--force"), flag: "--force", negative: "--no-force", want: FlagAbsent},
		{name: "bare name", spec: spec, words: words("--force", "x"), flag: "--force", negative: "--no-force", want: FlagTrue},
		{name: "bare negative", spec: spec, words: words("--no-force"), flag: "--force", negative: "--no-force", want: FlagFalse},
		{name: "value true", spec: spec, words: words("--force=true"), flag: "--force", negative: "--no-force", want: FlagTrue},
		{name: "value false", spec: spec, words: words("--force=false"), flag: "--force", negative: "--no-force", want: FlagFalse},
		{name: "value in another case without folding", spec: spec, words: words("--force=TRUE"), flag: "--force", negative: "--no-force", want: FlagUnknown},
		{name: "other value", spec: spec, words: words("--force=maybe"), flag: "--force", negative: "--no-force", want: FlagUnknown},
		{name: "negative with a value", spec: spec, words: words("--no-force=true"), flag: "--force", negative: "--no-force", want: FlagUnknown},
		{name: "conflict", spec: spec, words: words("--force", "--no-force"), flag: "--force", negative: "--no-force", want: FlagConflict},
		{name: "unknown before conflict", spec: spec, words: words("--force", "--no-force", "--force=x"), flag: "--force", negative: "--no-force", want: FlagUnknown},
		{name: "a later occurrence does not override", spec: spec, words: words("--no-force", "--force=true"), flag: "--force", negative: "--no-force", want: FlagConflict},
		{name: "empty negative matches nothing", spec: spec, words: words("--no-force"), flag: "--force", want: FlagAbsent},
		{name: "name with no row", spec: spec, words: words("--zz"), flag: "--zz", want: FlagUnknown},
		{name: "cluster letter", spec: spec, words: words("-nf"), flag: "-n", want: FlagTrue},
		{name: "cluster last letter", spec: spec, words: words("-nf"), flag: "-f", want: FlagTrue},
		{name: "cluster negative letter", spec: spec, words: words("-fn"), flag: "--force", negative: "-n", want: FlagFalse},
		{name: "unknown cluster is not read", spec: spec, words: words("-fz"), flag: "-f", want: FlagAbsent},
		{name: "operand after double dash", spec: spec, words: words("--", "--force"), flag: "--force", want: FlagAbsent},
		{name: "optional equals client", spec: helm, words: words("--dry-run=client"), flag: "--dry-run", want: FlagTrue},
		{name: "optional equals server", spec: helm, words: words("--dry-run=server"), flag: "--dry-run", want: FlagTrue},
		{name: "optional separate server", spec: helm, words: words("--dry-run", "server"), flag: "--dry-run", want: FlagTrue},
		{name: "optional leaves the next word", spec: helm, words: words("--dry-run", "--help"), flag: "--dry-run", want: FlagUnknown},
		{name: "optional separate false in another case", spec: helm, words: words("--dry-run", "FALSE"), flag: "--dry-run", want: FlagFalse},
		{name: "optional as the last word", spec: helm, words: words("--dry-run"), flag: "--dry-run", want: FlagTrue},
		{name: "optional with an unlisted value", spec: helm, words: words("--dry-run=none"), flag: "--dry-run", want: FlagUnknown},
		{name: "optional in another case", spec: helm, words: words("--DRY-RUN=TRUE"), flag: "--dry-run", want: FlagTrue},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.spec.Scan(tt.words).Flag(tt.flag, tt.negative); got != tt.want {
				t.Errorf("Flag(%q, %q) = %d, want %d", tt.flag, tt.negative, got, tt.want)
			}
		})
	}
}

// fuzzWords splits a line at spaces, so empty words stay; a word starting
// with '$' stands for a runtime expansion.
func fuzzWords(line string) []Word {
	texts := strings.Split(line, " ")
	out := make([]Word, len(texts))
	for i, text := range texts {
		out[i] = Word{Text: text, Raw: text, Literal: !strings.HasPrefix(text, "$")}
	}
	return out
}

func FuzzScan(f *testing.F) {
	specs := traitSpecs()
	specs["core"] = NewSpec(coreOptions...)
	names := make([]string, 0, len(specs))
	for name := range specs {
		names = append(names, name)
	}
	sort.Strings(names)
	seeds := []string{
		"-feconfig", "-fe x", "-fe", "--unset-a", "--fil x", "--un", "--get-",
		"-sSo x", "-sWo f", "-dfoo=o", "-d=@payload", "-T=x", "-pt x", "-rs x",
		"--dry-run server", "--dry-run --help", "--dry-run FALSE", "--dry-run",
		"-Cdir status", "-cuser.name=x", "-C=dir", "-Pp", "-P=x", "--p x", "-la",
		"---", "--=x", "-", "", "-- -v", "$X -o", "-o $F", "-sjx -sj y", "-sq on",
	}
	for i := range names {
		for _, seed := range seeds {
			f.Add(uint8(i), seed)
		}
	}
	f.Fuzz(func(t *testing.T, which uint8, line string) {
		input := fuzzWords(line)
		spec := specs[names[int(which)%len(names)]]
		scan := spec.Scan(input)
		// A stop changes nothing before it: its reading is Scan's prefix, and
		// it falls only where an argument starts.
		prefix, at := spec.ScanStop(input, func(w Word) bool { return w.Text == "-v" })
		k := 0
		for k < len(scan.Args) && scan.Args[k].At < at {
			k++
		}
		if !reflect.DeepEqual(prefix.Args, scan.Args[:k]) || at < len(input) && scan.Args[k].Role == RoleValue {
			t.Fatalf("ScanStop stopped at %d with %+v, Scan read %+v", at, prefix.Args, scan.Args)
		}
		if len(scan.Args) != len(input) {
			t.Fatalf("%d args for %d words", len(scan.Args), len(input))
		}
		for i, arg := range scan.Args {
			if arg.At != i {
				t.Fatalf("Args[%d].At = %d", i, arg.At)
			}
			if arg.ValueAt != -1 && arg.ValueAt != arg.At+1 {
				t.Fatalf("Args[%d].ValueAt = %d", i, arg.ValueAt)
			}
		}
		if scan.Problem != nil && (scan.Problem.At < 0 || scan.Problem.At >= len(input)) {
			t.Fatalf("Problem.At = %d for %d words", scan.Problem.At, len(input))
		}
		scan.Flag("--dry-run", "--no-dry-run")
	})
}
