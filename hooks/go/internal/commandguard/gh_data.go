package commandguard

import (
	"sort"
	"strings"

	"github.com/muehz/agentics/hooks/go/internal/argv"
	"github.com/muehz/agentics/hooks/go/internal/shellwords"
)

// blankedGhCommand returns command with the literal data words of every
// eligible invocation replaced by spaces of equal byte length, so the gh
// regex rules stop reading prose as an executed command. Invocations and
// their words are located by source position, never by value: removing a
// value would also erase the same text inside an executing context. Any miss
// in an invocation that has data words returns command unchanged, which fails
// closed. This is the gh-only path. Redirected prose, body-file values and git
// grep use bashDataView's full argument proof, shared with the generic
// scanner, before newReadings considers this fallback.
func blankedGhCommand(command string, invocations []shellwords.Invocation, dialect shellwords.Dialect) string {
	// Every gated verb's regex starts with gh; nothing to blank without it.
	if !strings.Contains(strings.ToLower(command), "gh") || !ghDataRelaxable(command, invocations, dialect) {
		return command
	}
	out := []byte(command)
	cursor := 0
	for idx, inv := range invocations {
		if inv.Parent >= 0 {
			// A substitution child sits inside its parent's Raw, which the
			// walk has already passed; its words are never data.
			continue
		}
		at := strings.Index(command[cursor:], inv.Raw)
		if inv.Raw == "" || at < 0 {
			return command
		}
		start := cursor + at
		cursor = start + len(inv.Raw)
		data, echoes := ghDataWords(inv)
		if data == nil {
			continue
		}
		spans, ok := wordSpans(inv.Raw, start, inv.Words)
		if !ok {
			return command
		}
		var dataSpans [][2]int
		for i, span := range spans {
			if data[i] {
				dataSpans = append(dataSpans, span)
			}
		}
		// A command whose stdout carries the data (echo, printf, a search
		// match) is data only while that output reaches nothing that could
		// run it: no redirection anywhere in the command, and every pipeline
		// consumer downstream inert.
		if echoes && (ghDataRedirects(command, dataSpans) || !ghDataOutputInert(invocations, idx)) {
			continue
		}
		for _, span := range dataSpans {
			for j := span[0]; j < span[1]; j++ {
				if out[j] != '\n' {
					out[j] = ' '
				}
			}
		}
	}
	return string(out)
}

// ghDataRedirects reports whether command, outside the given data spans,
// contains '>': any output redirection (2>&1 included) could write the data
// to a file something else then runs, so the relaxation is not granted.
func ghDataRedirects(command string, spans [][2]int) bool {
	next := 0
	for _, span := range spans {
		if strings.Contains(command[next:span[0]], ">") {
			return true
		}
		next = span[1]
	}
	return strings.Contains(command[next:], ">")
}

// ghDataInertConsumers read piped text without running it. "gh" is not here:
// whether a gh consumer is inert depends on its subcommand (see
// ghConsumerInert), since `gh extension exec` and `gh alias set` both hand
// stdin to something else entirely.
var ghDataInertConsumers = map[string]bool{
	"cat": true, "head": true, "tail": true, "wc": true, "sort": true, "uniq": true,
	"grep": true, "rg": true, "less": true, "more": true,
	"out-host": true, "out-null": true, "measure-object": true, "sort-object": true,
	"select-object": true, "select-string": true,
}

// ghDataConsumerVerbs is the vetted set of gh subcommands (as returned by
// ghSubcommand: two lowercase words) that only take stdin as a data sink
// into GitHub's API — an issue/PR/gist body or a release's notes — rather
// than running or otherwise acting on it.
var ghDataConsumerVerbs = map[string]bool{
	"issue comment": true, "issue create": true, "issue edit": true,
	"pr comment": true, "pr create": true, "pr edit": true, "pr review": true,
	"release create": true, "release edit": true, "gist create": true,
}

// ghConsumerInert reports whether a `gh …` pipeline consumer is a vetted
// stdin-to-API data sink: a verb in ghDataConsumerVerbs, or `gh api …` (any
// endpoint). Anything else — `gh extension exec`, `gh alias set`, an
// unrecognized or non-literal subcommand — hands stdin to something other
// than a fixed request field, so it is not inert.
func ghConsumerInert(words []shellwords.Word) bool {
	command, _, _, ok := ghSubcommand(words[1:])
	if !ok {
		return false
	}
	return ghDataConsumerVerbs[command] || strings.HasPrefix(command, "api ")
}

// ghDataOutputInert reports whether the stdout of invocations[idx] is unpiped
// or flows only through inert consumers to the end of its pipeline.
func ghDataOutputInert(invocations []shellwords.Invocation, idx int) bool {
	edge := invocations[idx].PipelineOutputEdge
	for edge != 0 {
		consumer := -1
		for j := idx + 1; j < len(invocations); j++ {
			if invocations[j].PipelineInputEdge == edge {
				consumer = j
				break
			}
		}
		if consumer < 0 {
			return false
		}
		words := invocations[consumer].Words
		if len(words) == 0 || !words[0].Literal {
			return false
		}
		head := strings.ToLower(shellwords.ExecutableName(words[0].Text))
		if head == "gh" {
			if !ghConsumerInert(words) {
				return false
			}
		} else if !ghDataInertConsumers[head] {
			return false
		}
		idx, edge = consumer, invocations[consumer].PipelineOutputEdge
	}
	return true
}

// ghDataRelaxable reports whether nothing in the command could re-execute
// data: every invocation is inspectable, unwrapped, headed by a literal word
// that is not an executor, and a POSIX command carries no heredoc.
func ghDataRelaxable(command string, invocations []shellwords.Invocation, dialect shellwords.Dialect) bool {
	if dialect == shellwords.POSIX && strings.Contains(command, "<<") {
		return false
	}
	for _, inv := range invocations {
		if !inv.Inspectable || len(inv.Wrappers) > 0 || len(inv.Words) == 0 || !inv.Words[0].Literal {
			return false
		}
		// A shell or an evaluator runs text it is handed, so a command that
		// invokes one gets no data-position relaxation at all: data an
		// allowlisted command writes can reach it through a variable, a file,
		// or a pipe in the same command (printf -v c "…"; eval "$c").
		if entry := argv.Lookup(strings.ToLower(shellwords.ExecutableName(inv.Words[0].Text))); entry.Shell != argv.NotShell || entry.Evaluator {
			return false
		}
	}
	return true
}

// ghDataWords marks which of inv's words are data positions, or returns nil
// when inv has none or is ineligible: grouped or substituted.
// Invocation.PipelineOutput is true for every pipe producer,
// including a bare top-level "echo … | gh …" that ghDataOutputInert is meant
// to relax, so it is not part of this gate; a grouped producer ("(echo …) |
// …") is still excluded because its sole invocation carries Before == Group.
// Only literal words are marked, so a word carrying a substitution keeps its
// text. echoes reports that the command copies the data to its stdout.
func ghDataWords(inv shellwords.Invocation) (data []bool, echoes bool) {
	if inv.Before == shellwords.Group || inv.Before == shellwords.Substitution {
		return nil, false
	}
	data = make([]bool, len(inv.Words))
	marked := false
	mark := func(i int) {
		if i > 0 && i < len(inv.Words) && inv.Words[i].Literal {
			data[i] = true
			marked = true
		}
	}
	switch strings.ToLower(shellwords.ExecutableName(inv.Words[0].Text)) {
	case "printf":
		if printfWritesVariable(inv.Words) {
			// printf -v NAME writes the formatted result into a shell
			// variable instead of stdout. A later command can still execute
			// that variable's contents, but the output-side gate
			// (ghDataOutputInert) only tracks pipes and redirects — it
			// cannot see a variable read — so this is not a data position
			// at all; leave it ungated.
			break
		}
		echoes = true
		for i := 1; i < len(inv.Words); i++ {
			mark(i)
		}
	case "echo", "write-output", "write-host":
		echoes = true
		for i := 1; i < len(inv.Words); i++ {
			mark(i)
		}
	case "git":
		// The message goes to the commit, not stdout: no output rule.
		markCommitMessage(inv.Words, argv.ReadGit(inv.Words[1:]), mark)
	case "gh":
		// Marking a word that is not prose (-t is --template on some
		// subcommands) only removes text, which can make the regex rules
		// stricter but never hides the invocation's own head or verb. When a
		// blanked flag of the invocation itself would otherwise go unseen,
		// the invocation path (checkInvocationPublication) evaluates every
		// gh invocation independently and still catches it.
		if _, args, _, ok := ghSubcommand(inv.Words[1:]); ok {
			markGhValueFlagsScan(args, len(inv.Words)-len(args), mark)
		}
	case "grep", "rg":
		// A match line can echo the pattern to stdout.
		echoes = true
		markSearchPatternScan(inv.Words, mark)
	case "select-string":
		echoes = true
		markSelectStringPatternScan(inv.Words, mark)
	}
	if !marked {
		return nil, false
	}
	return data, echoes
}

// printfWritesVariable reports whether words carries printf's -v flag (as
// `-v NAME` or `-vNAME`) before its first non-flag argument, which sends the
// formatted output to a shell variable rather than stdout.
func printfWritesVariable(words []shellwords.Word) bool {
	for i := 1; i < len(words); i++ {
		t := words[i].Text
		if t == "-v" || (strings.HasPrefix(t, "-v") && len(t) > 2) {
			return true
		}
		if !strings.HasPrefix(t, "-") {
			return false
		}
	}
	return false
}

// commitMessageStart returns the index of the word the gh rules test for
// `commit`: it skips -C, -c, --git-dir, --work-tree and --namespace with their
// values and any other dash word, and stops at the first word that is
// neither. Git's own reading of a separated --config-env value that looks like
// an option differs from this walk (git takes it as the value, the walk reads
// it as an option), so the start stays where the gh rules have always found
// it, never looser and never stricter.
func commitMessageStart(words []shellwords.Word) int {
	i := 1
	for i < len(words) {
		t := words[i].Text
		if t == "-C" || t == "-c" || t == "--git-dir" || t == "--work-tree" || t == "--namespace" {
			i += 2
			continue
		}
		if !strings.HasPrefix(t, "-") {
			break
		}
		i++
	}
	return i
}

// markCommitMessage marks the -m/--message value of `git commit`, with g being
// argv.ReadGit(words[1:]). Flags before the subcommand are skipped with their
// values (-C, -c, …), so a `-c core.editor=…` value is never data. Only a line
// read to its verb is marked; on a line
// the grammar cannot read, or one that only prints, the message stays visible
// to the gh rules. On a readable line the message starts where
// commitMessageStart finds it.
func markCommitMessage(words []shellwords.Word, g argv.Git, mark func(int)) {
	if g.State != argv.GitVerb {
		return
	}
	i := commitMessageStart(words)
	if i >= len(words) || words[i].Text != "commit" {
		return
	}
	for i++; i < len(words); i++ {
		t := words[i].Text
		switch {
		case t == "--":
			return
		case t == "--message":
			mark(i + 1)
			i++
		case strings.HasPrefix(t, "--message="):
			mark(i)
		case t == "-m":
			mark(i + 1)
			i++
		case strings.HasPrefix(t, "-m"):
			mark(i) // -mtext
		case len(t) > 2 && t[0] == '-' && t[len(t)-1] == 'm' && isASCIILetters(t[1:]):
			mark(i + 1) // a boolean cluster ending in m: -am, -sam
			i++
		}
	}
}

// isASCIILetters reports whether s is non-empty and only ASCII letters.
func isASCIILetters(s string) bool {
	for i := 0; i < len(s); i++ {
		if c := s[i] | 0x20; c < 'a' || c > 'z' {
			return false
		}
	}
	return s != ""
}

// valueOptions turns a value-flag table into grammar rows, in name order so
// the rows are the same on every run.
func valueOptions(table map[string]bool) []argv.Option {
	names := make([]string, 0, len(table))
	for name := range table {
		names = append(names, name)
	}
	sort.Strings(names)
	return dataOptions(nil, names)
}

// The markers' grammars. Every one reads a non-literal word by its text: a
// non-literal pattern option still suppresses the first-operand mark, and a
// non-literal `--` still ends the walk, so neither marks a word whose role
// rests on an expansion as data.
var (
	// ghProseValueOptions are gh's seven prose-carrying options.
	ghProseValueOptions = dataOptions(nil, []string{"--body", "-b", "--title", "-t", "--notes", "-n", "--subject"})
	ghProseValueSpec    = argv.NewSpec(ghProseValueOptions...).With(argv.ReadNonLiteral)

	// searchPatternOptions are the explicit pattern options of grep and rg.
	// AttachedShort reads `-epat` and `-e=pat` as -e with an attached value,
	// as a `-e` prefix test does.
	searchPatternOptions = dataOptions(nil, []string{"-e", "--regexp"})
	searchPatternSpec    = argv.NewSpec(searchPatternOptions...).With(argv.AttachedShort, argv.ReadNonLiteral)
	// grepValueOptions consume the next word, so it is not the pattern.
	grepValueOptions = valueOptions(map[string]bool{
		"-A": true, "-B": true, "-C": true, "-m": true, "-f": true, "-d": true, "-D": true,
		"-g": true, "-t": true, "-T": true, "-j": true, "-M": true,
		"--max-count": true, "--glob": true, "--type": true, "--type-not": true, "--file": true,
		"--context": true, "--before-context": true, "--after-context": true,
	})
	grepValueSpec = argv.NewSpec(grepValueOptions...).With(argv.ReadNonLiteral)

	// Select-String binds its parameters in any letter case.
	selectStringPatternOptions = dataOptions(nil, []string{"-Pattern"})
	selectStringPatternSpec    = argv.NewSpec(selectStringPatternOptions...).With(argv.FoldNames, argv.ReadNonLiteral)
	// selectStringValueOptions consume the next word, so it is not the pattern.
	selectStringValueOptions = valueOptions(map[string]bool{
		"-path": true, "-literalpath": true, "-encoding": true, "-context": true,
		"-include": true, "-exclude": true, "-inputobject": true,
	})
	selectStringValueSpec = argv.NewSpec(selectStringValueOptions...).With(argv.FoldNames, argv.ReadNonLiteral)
)

// markGhValueFlagsScan marks the values of gh's prose-carrying flags among the
// words after the subcommand; offset maps args back to the invocation. Only a
// long option marks a value joined with `=`; `-b=x` stays visible.
func markGhValueFlagsScan(args []shellwords.Word, offset int, mark func(int)) {
	for _, arg := range ghProseValueSpec.Scan(args).Args {
		switch arg.Role {
		case argv.RoleEnd:
			return
		case argv.RoleOption:
			if !arg.Joined {
				mark(offset + arg.At + 1)
			} else if strings.HasPrefix(arg.Name, "--") {
				mark(offset + arg.At)
			}
		}
	}
}

// markSearchPatternScan marks the pattern of grep or rg: every explicit
// pattern before `--`, else the first operand once the value options
// are skipped. A pattern option with no value still counts as explicit, so
// the first operand is then never marked.
func markSearchPatternScan(words []shellwords.Word, mark func(int)) {
	if len(words) == 0 {
		return
	}
	explicit := false
explicitPatterns:
	for _, arg := range searchPatternSpec.Scan(words[1:]).Args {
		at := 1 + arg.At
		switch arg.Role {
		case argv.RoleEnd:
			break explicitPatterns
		case argv.RoleOption:
			explicit = true
			if arg.Joined {
				mark(at)
			} else {
				mark(at + 1)
			}
		}
	}
	if explicit {
		return
	}
	for _, arg := range grepValueSpec.Scan(words[1:]).Args {
		at := 1 + arg.At
		switch arg.Role {
		case argv.RoleEnd:
			mark(at + 1)
			return
		case argv.RoleOperand:
			mark(at)
			return
		}
	}
}

// markSelectStringPatternScan marks Select-String's pattern: the value of the
// first -Pattern, else the first positional argument once
// the value parameters are skipped. PowerShell gives `--` no meaning here, so
// each pass reads on after it as after any other dash word.
func markSelectStringPatternScan(words []shellwords.Word, mark func(int)) {
	if len(words) == 0 {
		return
	}
	found := false
	scanPastEnd(selectStringPatternSpec, words, 1, func(arg argv.Arg, at int) bool {
		if arg.Role == argv.RoleOption && !arg.Joined {
			mark(at + 1)
			found = true
			return true
		}
		return false
	})
	if found {
		return
	}
	scanPastEnd(selectStringValueSpec, words, 1, func(arg argv.Arg, at int) bool {
		if arg.Role == argv.RoleOperand {
			mark(at)
			return true
		}
		return false
	})
}

// scanPastEnd scans words from start and hands each arg, with its index in
// words, to visit until visit returns true. A `--` ends nothing: the words
// after it are scanned again as a fresh line.
func scanPastEnd(spec argv.Spec, words []shellwords.Word, start int, visit func(arg argv.Arg, at int) bool) {
	for start < len(words) {
		next := len(words)
		for _, arg := range spec.Scan(words[start:]).Args {
			if arg.Role == argv.RoleEnd {
				next = start + arg.At + 1
				break
			}
			if visit(arg, start+arg.At) {
				return
			}
		}
		start = next
	}
}
