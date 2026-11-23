package shellwords

import (
	"context"
	"slices"
	"strings"
)

// wrapperReading is what one walk over a wrapper's words finds: the command it runs and, for env, the changes it
// makes to the environment of that command.
//
// A non-zero refusal is the "not readable" result. It is distinct from a reading with a child: child and
// commandString are then nil, and the reading names no command. The parser's posture for it is that the invocation
// becomes not inspectable, with the refusal as its reason. effects are collected even when the strict reading refuses,
// because command-guardian reads the environment changes of an env line it cannot vouch for.
//
// child is the wrapped command's words, or commandString is the literal command text of a shell's -c or a
// PowerShell's -Command, read in dialect. assignments are env's NAME=value words before the child. complete is false
// once xargs supplies arguments the words do not show. inspectionOnly marks command -v and -V, which run nothing.
// Depth is bounded by the parser, which never reads a wrapper past maxWrapperDepth.
type wrapperReading struct {
	child          []Word
	assignments    []Word
	dialect        Dialect
	commandString  *Word
	complete       bool
	inspectionOnly bool
	effects        []EnvironmentEffect
	refusal        Refusal
}

// dashDashRule is how an option row reads a "--" word.
type dashDashRule int

const (
	// dashDashNone takes "--" as an unknown option, so the row refuses it.
	dashDashNone dashDashRule = iota
	// dashDashAny ends the options at a word whose text is "--", whether it was spelled literally or not.
	dashDashAny
)

// wrapperPrefix is an option spelled with its value in the same word. valueRequired refuses the bare prefix as an
// unknown option, so an attached -i needs a non-empty rest while --input= does not.
type wrapperPrefix struct {
	prefix        string
	valueRequired bool
}

// wrapperRow is a wrapper whose options are one closed list, read before the child. Only literal words match an
// option. A value flag whose next word is missing, empty, or not literal refuses with formWrapperValue; any other word
// starting with "-" refuses with formWrapperOption; running out of words refuses with formNoChild.
type wrapperRow struct {
	flags      []string
	valueFlags []string
	prefixes   []wrapperPrefix
	dashDash   dashDashRule
}

// wrapperReader is a hand-written reading for a wrapper whose grammar is not a row. Only env reads ctx and
// occurrence.
type wrapperReader func(ctx context.Context, exe string, words []Word, dialect Dialect, occurrence int) wrapperReading

// wrapperSpec is one wrapper's grammar: an option row, or a reader when read is set.
type wrapperSpec struct {
	row  wrapperRow
	read wrapperReader
}

// wrappers is the closed roster of commands the parser looks through, keyed by the lower-case executable name.
var wrappers = map[string]wrapperSpec{
	"sh":         {read: readShell},
	"bash":       {read: readShell},
	"pwsh":       {read: readPowerShell},
	"powershell": {read: readPowerShell},
	"env":        {read: readEnv},
	"command":    {read: readCommand},
	"nohup":      {read: readNohup},
	"exec":       {read: readNohup},
	"time":       {read: readTime},
	"xargs":      {read: readXargs},
	"timeout":    {read: readTimeout},
	"nice":       {read: readNice},
	"flock":      {read: readFlock},
	"sudo": {row: wrapperRow{
		flags:      []string{"-E", "-n"},
		valueFlags: []string{"-u", "-g"},
		dashDash:   dashDashAny,
	}},
	"doas": {row: wrapperRow{
		flags:      []string{"-n", "-L"},
		valueFlags: []string{"-u", "-C"},
	}},
	"ionice": {row: wrapperRow{
		flags:      []string{"-t"},
		valueFlags: []string{"-c", "-n", "-p"},
		prefixes:   []wrapperPrefix{{prefix: "--class="}, {prefix: "--classdata="}},
	}},
	"setsid": {row: wrapperRow{
		flags: []string{"-c", "--ctty", "-f", "--fork", "-w", "--wait"},
	}},
	"stdbuf": {row: wrapperRow{
		valueFlags: []string{"-i", "-o", "-e"},
		prefixes: []wrapperPrefix{
			{prefix: "-i", valueRequired: true}, {prefix: "-o", valueRequired: true}, {prefix: "-e", valueRequired: true},
			{prefix: "--input="}, {prefix: "--output="}, {prefix: "--error="},
		},
	}},
}

// readWrapper reads words, whose first word is the wrapper exe, in the parent dialect. occurrence numbers env's
// effects. An exe outside the roster panics, because the parser asks only for a wrapper it has already identified.
func readWrapper(ctx context.Context, exe string, words []Word, dialect Dialect, occurrence int) wrapperReading {
	spec, ok := wrappers[exe]
	if !ok {
		panic("readWrapper called for non-wrapper " + exe)
	}
	if spec.read != nil {
		return spec.read(ctx, exe, words, dialect, occurrence)
	}
	return spec.row.readRow(exe, words, dialect)
}

func (row wrapperRow) readRow(exe string, words []Word, dialect Dialect) wrapperReading {
	i := 1
	for i < len(words) {
		w := words[i].Text
		switch {
		case row.dashDash == dashDashAny && w == "--":
			return wrapperChild(exe, words, i+1, true, dialect, nil)
		case words[i].Literal && slices.Contains(row.flags, w):
			i++
		case words[i].Literal && slices.Contains(row.valueFlags, w):
			if !literalValueAt(words, i+1) {
				return wrapperRefusal(dialect, formWrapperValue, exe, w)
			}
			i += 2
		case words[i].Literal && row.prefixed(w):
			i++
		case strings.HasPrefix(w, "-"):
			return wrapperRefusal(dialect, formWrapperOption, exe, w)
		default:
			return wrapperChild(exe, words, i, true, dialect, nil)
		}
	}
	return wrapperRefusal(dialect, formNoChild, exe)
}

func (row wrapperRow) prefixed(w string) bool {
	for _, p := range row.prefixes {
		if strings.HasPrefix(w, p.prefix) && (!p.valueRequired || len(w) > len(p.prefix)) {
			return true
		}
	}
	return false
}

// literalValueAt reports whether words[i] exists and is a literal, non-empty option value.
func literalValueAt(words []Word, i int) bool {
	return i < len(words) && words[i].Literal && words[i].Text != ""
}

// wrapperRefusal is the not-readable result: no child and no assignments, in the parent dialect, complete.
func wrapperRefusal(dialect Dialect, kind formKind, args ...string) wrapperReading {
	return wrapperReading{dialect: dialect, complete: true, refusal: formRefusal(kind, args...)}
}

// wrapperChild reads the child at words[i], which must exist and be literal.
func wrapperChild(exe string, words []Word, i int, complete bool, dialect Dialect, assignments []Word) wrapperReading {
	r := wrapperReading{assignments: assignments, dialect: dialect, complete: complete}
	switch {
	case i >= len(words):
		r.refusal = formRefusal(formNoChild, exe)
	case !words[i].Literal:
		r.refusal = formRefusal(formChildNotLiteral, exe)
	default:
		r.child = words[i:]
	}
	return r
}

func readShell(_ context.Context, exe string, words []Word, dialect Dialect, _ int) wrapperReading {
	if len(words) < 3 || !words[1].Literal || (words[1].Text != "-c" && words[1].Text != "-lc") {
		return wrapperRefusal(dialect, formShellContract, exe)
	}
	return wrapperReading{dialect: POSIX, commandString: &words[2], complete: true}
}

func readPowerShell(_ context.Context, exe string, words []Word, dialect Dialect, _ int) wrapperReading {
	i := 1
	for i < len(words) && words[i].Literal && (strings.EqualFold(words[i].Text, "-NoProfile") || strings.EqualFold(words[i].Text, "-NonInteractive")) {
		i++
	}
	if i+2 != len(words) || !words[i].Literal || !strings.EqualFold(words[i].Text, "-Command") {
		return wrapperRefusal(dialect, formPowerShellContract, exe)
	}
	return wrapperReading{dialect: PowerShell, commandString: &words[i+1], complete: true}
}

// readEnv reads env's options, assignments, and child, and collects the environment effects in the same walk.
//
// The strict reading stops at its first refusal, at "--", or at the child. The effects reading is more lenient,
// because command-guardian must see every change env may make: -u and --unset take the next word whenever there is
// one, and --unset= and an attached -u count whether or not they are literal. Every word the strict reading accepts is
// an effect, and neither "--" nor a child is one, so after a refusal the walk goes on only while the words are effects.
//
// The context is polled once per effect word and once more when the words run out, never after the word that stops
// the effects. Once a poll finds it canceled, polling stops and effects stay nil, while the strict reading, which does
// not depend on the context, still completes.
func readEnv(ctx context.Context, exe string, words []Word, dialect Dialect, occurrence int) wrapperReading {
	var (
		reading     wrapperReading
		decided     bool
		assignments []Word
		effects     []EnvironmentEffect
	)
	live := true
	decide := func(r wrapperReading) { reading, decided = r, true }
	i := 1
	for ; i < len(words); i++ {
		if live && ctx.Err() != nil {
			live, effects = false, nil
		}
		w := words[i]
		if !decided {
			switch {
			case w.Text == "--":
				decide(wrapperChild(exe, words, i+1, true, dialect, assignments))
			case w.Literal && (w.Text == "-i" || w.Text == "--ignore-environment"):
			case w.Literal && (w.Text == "-u" || w.Text == "--unset"):
				if !literalValueAt(words, i+1) {
					decide(wrapperRefusal(dialect, formEnvName, w.Text))
				}
			case w.Literal && strings.HasPrefix(w.Text, "--unset=") && strings.TrimPrefix(w.Text, "--unset=") != "":
			case w.Literal && strings.HasPrefix(w.Text, "-u") && len(w.Text) > 2:
			case strings.HasPrefix(w.Text, "-"):
				decide(wrapperRefusal(dialect, formWrapperOption, "env", w.Text))
			case IsAssignment(w.Text):
				assignments = append(assignments, w)
			default:
				decide(wrapperChild(exe, words, i, true, dialect, assignments))
			}
		}
		if decided && !live {
			break
		}
		e := EnvironmentEffect{Occurrence: occurrence, Order: i, Dialect: dialect}
		switch {
		case w.Literal && (w.Text == "-i" || w.Text == "--ignore-environment"):
			e.Kind = EnvironmentResetAll
		case w.Literal && (w.Text == "-u" || w.Text == "--unset"):
			e.Kind = EnvironmentUnset
			if i+1 < len(words) {
				i++
				e.Word = words[i]
			}
		case strings.HasPrefix(w.Text, "--unset="):
			e.Kind = EnvironmentUnset
			e.Word = w
			e.Word.Text = strings.TrimPrefix(w.Text, "--unset=")
		case strings.HasPrefix(w.Text, "-u") && len(w.Text) > 2:
			e.Kind = EnvironmentUnset
			e.Word = w
			e.Word.Text = w.Text[2:]
		case IsAssignment(w.Text):
			e.Kind = EnvironmentAssignment
			e.Word = w
		default:
			reading.effects = effects
			return reading
		}
		if live {
			effects = append(effects, e)
		}
	}
	if i >= len(words) {
		if live && ctx.Err() != nil {
			live = false
		}
		if !decided {
			decide(wrapperRefusal(dialect, formNoChild, "env"))
		}
	}
	if live {
		reading.effects = effects
	}
	return reading
}

func readCommand(_ context.Context, exe string, words []Word, dialect Dialect, _ int) wrapperReading {
	i := 1
	if i < len(words) && words[i].Literal && (words[i].Text == "-v" || words[i].Text == "-V") {
		return wrapperReading{dialect: dialect, complete: true, inspectionOnly: true}
	}
	if i < len(words) && words[i].Literal && words[i].Text == "--" {
		i++
	}
	if i < len(words) && strings.HasPrefix(words[i].Text, "-") {
		return wrapperRefusal(dialect, formWrapperOption, "command", words[i].Text)
	}
	return wrapperChild(exe, words, i, true, dialect, nil)
}

func readNohup(_ context.Context, exe string, words []Word, dialect Dialect, _ int) wrapperReading {
	i := 1
	if i < len(words) && words[i].Literal && words[i].Text == "--" {
		i++
	}
	if i < len(words) && strings.HasPrefix(words[i].Text, "-") {
		return wrapperRefusal(dialect, formWrapperOption, exe, words[i].Text)
	}
	return wrapperChild(exe, words, i, true, dialect, nil)
}

func readTime(_ context.Context, exe string, words []Word, dialect Dialect, _ int) wrapperReading {
	i := 1
	if i < len(words) && words[i].Literal && words[i].Text == "-p" {
		i++
	}
	if i < len(words) && words[i].Literal && words[i].Text == "--" {
		i++
	}
	if i < len(words) && strings.HasPrefix(words[i].Text, "-") {
		return wrapperRefusal(dialect, formWrapperOption, "time", words[i].Text)
	}
	return wrapperChild(exe, words, i, true, dialect, nil)
}

// readXargs reads xargs, whose child is never complete: xargs appends arguments read from its input. A refusal before
// the child is read stays complete, as no child was named.
func readXargs(_ context.Context, exe string, words []Word, dialect Dialect, _ int) wrapperReading {
	i := 1
	for i < len(words) {
		w := words[i].Text
		switch {
		case w == "--":
			return wrapperChild(exe, words, i+1, false, dialect, nil)
		case words[i].Literal && (w == "-0" || w == "-r"):
			i++
		case words[i].Literal && (w == "-n" || w == "-P" || w == "-I"):
			if !literalValueAt(words, i+1) {
				return wrapperRefusal(dialect, formWrapperValue, "xargs", w)
			}
			i += 2
		case strings.HasPrefix(w, "-"):
			return wrapperRefusal(dialect, formWrapperOption, "xargs", w)
		default:
			return wrapperChild(exe, words, i, false, dialect, nil)
		}
	}
	return wrapperRefusal(dialect, formXargsChild)
}

// readTimeout reads timeout, whose child follows a literal duration operand.
func readTimeout(_ context.Context, exe string, words []Word, dialect Dialect, _ int) wrapperReading {
	i := 1
	for i < len(words) {
		w := words[i].Text
		switch {
		case words[i].Literal && (w == "--foreground" || w == "--preserve-status" || w == "-v" || w == "--verbose"):
			i++
		case words[i].Literal && (w == "-s" || w == "--signal" || w == "-k" || w == "--kill-after"):
			if !literalValueAt(words, i+1) {
				return wrapperRefusal(dialect, formWrapperValue, "timeout", w)
			}
			i += 2
		case words[i].Literal && (strings.HasPrefix(w, "--signal=") || strings.HasPrefix(w, "--kill-after=")):
			i++
		case strings.HasPrefix(w, "-"):
			return wrapperRefusal(dialect, formWrapperOption, "timeout", w)
		default:
			if !words[i].Literal || i+1 >= len(words) || !looksLikeDuration(w) {
				return wrapperRefusal(dialect, formTimeoutDuration)
			}
			if strings.HasPrefix(words[i+1].Text, "-") {
				return wrapperRefusal(dialect, formTimeoutOption, words[i+1].Text)
			}
			return wrapperChild(exe, words, i+1, true, dialect, nil)
		}
	}
	return wrapperRefusal(dialect, formTimeoutDuration)
}

// readNice reads nice, which also takes the adjustment as an option of its own, -N.
func readNice(_ context.Context, exe string, words []Word, dialect Dialect, _ int) wrapperReading {
	i := 1
	for i < len(words) {
		w := words[i].Text
		switch {
		case words[i].Literal && w == "-n":
			if !literalValueAt(words, i+1) {
				return wrapperRefusal(dialect, formWrapperValue, "nice", w)
			}
			i += 2
		case words[i].Literal && strings.HasPrefix(w, "--adjustment=") && strings.TrimPrefix(w, "--adjustment=") != "":
			i++
		case words[i].Literal && len(w) > 1 && w[0] == '-' && allDigits(w[1:]):
			i++
		case strings.HasPrefix(w, "-"):
			return wrapperRefusal(dialect, formWrapperOption, "nice", w)
		default:
			return wrapperChild(exe, words, i, true, dialect, nil)
		}
	}
	return wrapperRefusal(dialect, formNoChild, "nice")
}

// readFlock reads flock, whose child follows a literal lock-file operand.
func readFlock(_ context.Context, exe string, words []Word, dialect Dialect, _ int) wrapperReading {
	i := 1
	for i < len(words) {
		w := words[i].Text
		switch {
		case words[i].Literal && (w == "-s" || w == "-x" || w == "-e" || w == "-n" || w == "--nb" || w == "-o" || w == "-F" || w == "--verbose"):
			i++
		case words[i].Literal && (w == "-w" || w == "--timeout" || w == "-E"):
			if !literalValueAt(words, i+1) {
				return wrapperRefusal(dialect, formWrapperValue, "flock", w)
			}
			i += 2
		case strings.HasPrefix(w, "-"):
			return wrapperRefusal(dialect, formWrapperOption, "flock", w)
		default:
			if !words[i].Literal || i+1 >= len(words) {
				return wrapperRefusal(dialect, formNoChild, "flock")
			}
			if strings.HasPrefix(words[i+1].Text, "-") {
				return wrapperRefusal(dialect, formFlockOption, words[i+1].Text)
			}
			return wrapperChild(exe, words, i+1, true, dialect, nil)
		}
	}
	return wrapperRefusal(dialect, formNoChild, "flock")
}
