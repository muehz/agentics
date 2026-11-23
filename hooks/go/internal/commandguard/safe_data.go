package commandguard

import (
	"os"
	"regexp"
	"slices"
	"strings"

	"github.com/muehz/agentics/hooks/go/internal/argv"
	"github.com/muehz/agentics/hooks/go/internal/shellwords"
	"github.com/muehz/agentics/hooks/go/internal/toolcall"
	"github.com/muehz/agentics/hooks/go/internal/toolname"
)

var plainDataPath = regexp.MustCompile(`^(?:[^$` + "`" + `(){}\\\r\n]|\$[A-Za-z_][A-Za-z0-9_]*|\$\{[A-Za-z_][A-Za-z0-9_]*\})+$`)
var inertPrintfConversion = regexp.MustCompile(`%(?:%|[-+ #0]*[0-9]*(?:\.[0-9]+)?[bcdioxXuUfeEgGaAsq])`)

// bashDataView grants the widened exemption only after every invocation and
// every option is understood, with proven being the whole-command inert proof
// over view, which the caller computes once. One unknown consumer can execute
// earlier output from a pipe or file, so it retains the original command's
// evidence everywhere.
func bashDataView(fields normalizedFields, parsed shellwords.Result, view shellwords.HeredocView, proven bool) (string, bool) {
	if !bashDataDialect(fields.call) || !fields.commandPresent {
		return fields.command, false
	}
	if !proven {
		return fields.command, false
	}
	// Unquoted bodies retain the established generic-only treatment. This
	// view must not suppress that path when it has not removed any data.
	if strings.Contains(fields.command, "<<") && view.Text == fields.command {
		return fields.command, false
	}
	pagerDisabled := emptyGitPager(fields.call)
	out := []byte(view.Text)
	cursor := 0
	for _, inv := range parsed.Invocations {
		// The unchanged introducer/argument bytes locate each invocation in
		// source order. A failed association never falls back to value matching.
		at := strings.Index(view.Text[cursor:], inv.Raw)
		if inv.Raw == "" || at < 0 {
			return fields.command, false
		}
		start := cursor + at
		cursor = start + len(inv.Raw)
		spans, ok := wordSpans(inv.Raw, start, inv.Words)
		if !ok {
			return fields.command, false
		}
		_, data := inertDataInvocation(inv, pagerDisabled)
		for i, span := range spans {
			if !data[i] {
				continue
			}
			for j := span[0]; j < span[1]; j++ {
				if out[j] != '\n' && out[j] != '\r' {
					out[j] = ' '
				}
			}
		}
	}
	return string(out), true
}

// wordSpans locates each of one invocation's words in raw, the invocation's
// source text found at byte offset start, and returns their spans as offsets
// in the text raw was found in, aligned with words. Each word is searched from
// the end of the one before, so a word that occurs twice is placed at its own
// occurrence and the walk is linear in the length of raw. It reports false
// when a word has no source text or is not found, because a lost association
// must never fall back to matching values.
func wordSpans(raw string, start int, words []shellwords.Word) ([][2]int, bool) {
	spans := make([][2]int, len(words))
	cursor := 0
	for i, word := range words {
		at := strings.Index(raw[cursor:], word.Raw)
		if word.Raw == "" || at < 0 {
			return nil, false
		}
		wordStart := start + cursor + at
		cursor += at + len(word.Raw)
		spans[i] = [2]int{wordStart, wordStart + len(word.Raw)}
	}
	return spans, true
}

// bashDataDialect reads the Bash data dialect from the tool call: the strict
// layers must be readable, and the only shell selector, if any, is one key
// spelled exactly shell that names bash. When the strict layers are not
// readable it reports false, so an unreadable envelope earns no data proof.
func bashDataDialect(call toolcall.Call) bool {
	if toolname.Base(call.Name()) != "Bash" {
		return false
	}
	layers, ok := call.ProofLayers()
	if !ok {
		return false
	}
	seen := false
	for _, layer := range layers {
		for key, shell := range layer {
			if !strings.EqualFold(key, "shell") {
				continue
			}
			if seen || key != "shell" {
				return false
			}
			seen = true
			name, ok := shell.(string)
			if !ok || !strings.EqualFold(shellwords.ExecutableName(name), "bash") {
				return false
			}
		}
	}
	return true
}

// emptyGitPager proves the effective environment disables Git's pager. Git
// gives GIT_PAGER precedence over both core.pager and pager.<command>, and an
// explicitly empty value disables paging. Absence cannot establish that fact.
// Tool overrides take precedence over the inherited environment; malformed or
// competing override containers cannot borrow its empty value. When the strict
// layers are not readable it reports false, so an unreadable envelope earns no
// empty-pager proof.
func emptyGitPager(call toolcall.Call) bool {
	if _, ok := call.ProofLayers(); !ok {
		return false
	}
	overrides := call.Environment()
	if !overrides.Canonical() {
		return false
	}
	values, uncertain := overrides.Values("GIT_PAGER")
	if uncertain {
		return false
	}
	if !overrides.Present() {
		pager, present := os.LookupEnv("GIT_PAGER")
		return present && pager == ""
	}
	// An override container may replace, rather than merge with, the host
	// environment. Without an explicit pager entry, inheritance is unproven.
	// Canonical admits a single container, so values holds at most one entry.
	return len(values) == 1 && values[0] == ""
}

func fullInertInvocationProof(command string, parsed shellwords.Result, pagerDisabled bool) bool {
	if len(parsed.Invocations) == 0 || len(parsed.BareAssignments) != 0 ||
		strings.Contains(command, "$(") || strings.ContainsAny(command, "`#") ||
		strings.Contains(command, "<(") || strings.Contains(command, ">(") || strings.Contains(command, "\\\n") {
		return false
	}
	for _, inv := range parsed.Invocations {
		if !inv.Inspectable || !inv.Complete || len(inv.Wrappers) != 0 || len(inv.Assignments) != 0 || inv.Parent >= 0 ||
			inv.Before == shellwords.Group || inv.Before == shellwords.Substitution || inv.Before == shellwords.Background {
			return false
		}
		for _, word := range inv.Words {
			if hasActiveDataExpansion(word.Raw) {
				return false
			}
		}
		if ok, _ := inertDataInvocation(inv, pagerDisabled); !ok {
			return false
		}
		redirects, ok := inv.OwnRedirects()
		if !ok {
			return false
		}
		for _, redirect := range redirects {
			if redirect.FD > 2 || redirect.Target.Substituted || !plainDataPath.MatchString(redirect.Target.Text) || hasActiveDataExpansion(redirect.Target.Raw) {
				return false
			}
			switch redirect.Operator {
			case "<", "<<", "<<<":
				if redirect.FD > 0 {
					return false
				}
			case ">", ">>":
				if redirect.FD == 0 {
					return false
				}
			default:
				return false
			}
			target := strings.ToLower(strings.ReplaceAll(redirect.Target.Text, "\\", "/"))
			if strings.Contains(target, "/dev/") || strings.Contains(target, "/proc/") {
				return false
			}
		}
	}
	return true
}

// hasActiveDataExpansion checks the original spelling of one parsed word.
// Literal only excludes variable/substitution expansion: an unquoted glob or
// brace expression can still introduce extra argv, including execution options.
// Reject those even after -- rather than claiming the full argv is fixed.
func hasActiveDataExpansion(raw string) bool {
	// HOME and named-user expansion can supply an option-shaped value.
	// Quoting or escaping the leading tilde keeps it literal.
	if strings.HasPrefix(raw, "~") {
		return true
	}
	var quote byte
	ansi := false
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if quote == '\'' {
			if ansi && c == '\\' {
				i++
			} else if c == '\'' {
				quote = 0
			}
			continue
		}
		if c == '\\' {
			i++
			continue
		}
		if quote == '"' {
			if c == '"' {
				quote = 0
			}
			continue
		}
		if c == '$' && i+1 < len(raw) && raw[i+1] == '\'' {
			quote, ansi = '\'', true
			i++
			continue
		}
		if c == '\'' || c == '"' {
			quote = c
			ansi = false
			continue
		}
		// Simple variable paths remain supported for cat/tee/mkdir/cd and
		// ordinary redirections. Other parameter forms already fail their
		// argument proof; their braces must not look like a fixed word here.
		if c == '$' && i+1 < len(raw) && raw[i+1] == '{' {
			if end := strings.IndexByte(raw[i+2:], '}'); end >= 0 {
				end += i + 2
				if plainDataPath.MatchString(raw[i : end+1]) {
					i = end
					continue
				}
			}
		}
		if strings.ContainsRune("*?[]{}", rune(c)) {
			return true
		}
	}
	return false
}

// dataOptions turns a closed flag list and value list into grammar rows. A
// flag takes no value, and a value option takes the next word or the text
// after its `=`.
func dataOptions(flags, values []string) []argv.Option {
	options := make([]argv.Option, 0, len(flags)+len(values))
	for _, name := range flags {
		options = append(options, argv.Option{Name: name, Arity: argv.ArityFlag})
	}
	for _, name := range values {
		options = append(options, argv.Option{Name: name, Arity: argv.ArityValue})
	}
	return options
}

// valueRows returns the rows of options that take a value, in order.
func valueRows(options []argv.Option) []argv.Option {
	var rows []argv.Option
	for _, option := range options {
		if option.Arity == argv.ArityValue {
			rows = append(rows, option)
		}
	}
	return rows
}

// The data grammars of the inert consumers. The option tables are deliberately
// closed: an unknown option or arity keeps the original evidence. In particular
// no external preprocessor, pager, editor, browser, config override, or printf
// variable assignment is a data consumer.
//
// Search option arities belong to their executable. In particular rg -r and
// -E consume replacement/encoding values, while grep treats both as booleans.
// A --no-config consumed as a value cannot disable inherited preprocessing.
var (
	inertCatOptions   = dataOptions(strings.Fields("-A -b -e -E -n -s -t -T -u -v --show-all --number-nonblank --show-ends --number --squeeze-blank --show-tabs --show-nonprinting"), nil)
	inertTeeOptions   = dataOptions(strings.Fields("-a -i --append --ignore-interrupts"), nil)
	inertMkdirOptions = dataOptions([]string{"-p", "--parents", "-v", "--verbose"}, []string{"-m", "--mode"})
	inertCdOptions    = dataOptions([]string{"-L", "-P", "-e"}, nil)
	inertEchoOptions  = dataOptions([]string{"-n", "-e", "-E"}, nil)
	inertGrepOptions  = dataOptions(
		strings.Fields("-n -i -v -w -x -F -E -G -P -l -L -c -q -s -h -H -r -R -I -a -o -T --line-number --ignore-case --fixed-strings --count --files-with-matches --no-messages"),
		strings.Fields("-e --regexp -f --file -A -B -C --after-context --before-context --context -m --max-count"))
	inertRGOptions = dataOptions(
		strings.Fields("-n -i -v -w -x -F -P -l -L -c -q -s -h -H -I -a -o --line-number --ignore-case --fixed-strings --count --files-with-matches --no-messages --no-heading --hidden --no-ignore --no-config"),
		strings.Fields("-e --regexp -f --file -A -B -C --after-context --before-context --context -m --max-count -g --glob -t --type -T --type-not -r -E"))
	inertGitGrepOptions = dataOptions(
		strings.Fields("-n -i -v -w -F -E -G -P -l -L -c -q -h -H -r -I -a -o --line-number --ignore-case --fixed-strings --count --files-with-matches --no-heading"),
		strings.Fields("-e -f -A -B -C --max-depth --threads"))
	// inertRGConfigOptions are the rows the --no-config pre-scan skips a value
	// for: rg's value options. Only they matter there, because any other word
	// consumes nothing.
	inertRGConfigOptions = valueRows(inertRGOptions)

	inertCatSpec     = argv.NewSpec(inertCatOptions...)
	inertTeeSpec     = argv.NewSpec(inertTeeOptions...)
	inertMkdirSpec   = argv.NewSpec(inertMkdirOptions...)
	inertCdSpec      = argv.NewSpec(inertCdOptions...)
	inertEchoSpec    = argv.NewSpec(inertEchoOptions...)
	inertGrepSpec    = argv.NewSpec(inertGrepOptions...)
	inertRGSpec      = argv.NewSpec(inertRGOptions...)
	inertGitGrepSpec = argv.NewSpec(inertGitGrepOptions...)
	// The pre-scan reads each word's text whether or not it is literal, so a
	// non-literal value option still hides the word after it from the
	// --no-config test; skipping it would let that word count.
	inertRGConfigSpec = argv.NewSpec(inertRGConfigOptions...).With(argv.ReadNonLiteral)
)

// decodeDataScan consumes the entire argv from start, including option values
// and --. marks contains only proven literal value/operand indices, never
// command heads. It judges every arg rather than only Scan.Problem, which holds just the first word the grammar could not read.
// Any word it cannot prove is a flag, a value, or an operand fails the whole
// line, so an unreadable line is never treated as data.
func decodeDataScan(words []shellwords.Word, start int, spec argv.Spec, prose []string, operandsData, variablePaths bool) (bool, map[int]bool) {
	marks := map[int]bool{}
	if start >= len(words) {
		return true, marks
	}
	ended := false
	for _, arg := range spec.Scan(words[start:]).Args {
		at := start + arg.At
		word := arg.Word
		if !word.Literal && (!variablePaths || word.Substituted || !plainDataPath.MatchString(word.Text)) {
			return false, nil
		}
		role := arg.Role
		if ended {
			// A non-literal `--` that names a plain path ends option reading
			// here, while the grammar, which never reads a non-literal word
			// as `--`, read on. Every later word is an operand.
			role = argv.RoleOperand
		}
		switch role {
		case argv.RoleEnd:
			ended = true
		case argv.RoleOperand:
			if operandsData && word.Literal {
				marks[at] = true
			}
		case argv.RoleValue:
			if !word.Literal {
				return false, nil
			}
			if slices.Contains(prose, arg.Name) {
				marks[at] = true
			}
		case argv.RoleOption:
			if !word.Literal {
				return false, nil
			}
			value := dataValueOption(spec, arg.Name)
			switch {
			case !value && arg.HasValue:
				// A flag spelled with `=` is no row of the closed list.
				return false, nil
			case !value:
			case !arg.HasValue:
				return false, nil
			case arg.Joined:
				if !strings.HasPrefix(arg.Name, "--") {
					return false, nil
				}
				if slices.Contains(prose, arg.Name) {
					marks[at] = true
				}
			}
		case argv.RoleUnknown:
			switch {
			case word.Text == "-":
				// A lone `-` names standard input, an operand.
				if operandsData && word.Literal {
					marks[at] = true
				}
			case !word.Literal && word.Text == "--":
				ended = true
			default:
				return false, nil
			}
		default:
			return false, nil
		}
	}
	return true, marks
}

// dataValueOption reports whether name is a value row of spec. A Scan names
// the row an arg matched but not its arity, and records only its first
// problem, so reading the name alone is how a missing or joined value is told
// apart from a flag.
func dataValueOption(spec argv.Spec, name string) bool {
	problem := spec.Scan([]shellwords.Word{{Text: name, Raw: name, Literal: true}}).Problem
	return problem != nil && problem.Kind == argv.MissingValue
}

// rgNoConfigScan is the rg case's --no-config pre-scan
// over an rg invocation's words. Only a literal --no-config that no value
// option consumed, before the first `--`, counts.
func rgNoConfigScan(words []shellwords.Word) bool {
	if len(words) < 2 {
		return false
	}
	noConfig := false
	for _, arg := range inertRGConfigSpec.Scan(words[1:]).Args {
		switch arg.Role {
		case argv.RoleEnd:
			return noConfig
		case argv.RoleValue:
			continue
		}
		if arg.Word.Literal && arg.Word.Text == "--no-config" {
			noConfig = true
		}
	}
	return noConfig
}

// ghDataGrammar is the closed data grammar of one group of gh verbs that
// inertGhArgsScan reads.
type ghDataGrammar struct {
	spec argv.Spec
	// presence holds the value rows alone: the presence test reads only
	// whether a content option occurs, and skips each one's value. It reads
	// each word's text whether or not it is literal, as the presence test
	// always has.
	presence argv.Spec
	prose    []string
}

func newGhDataGrammar(flags, values, prose []string) *ghDataGrammar {
	return &ghDataGrammar{
		spec:     argv.NewSpec(dataOptions(flags, values)...),
		presence: argv.NewSpec(dataOptions(nil, values)...).With(argv.ReadNonLiteral),
		prose:    prose,
	}
}

// ghDataGrammars holds one grammar per gh verb, with each group's lists as
// inertGhArgsScan spells them.
var ghDataGrammars = func() map[string]*ghDataGrammar {
	prose := []string{"--body", "-b", "--body-file", "-F", "--title", "-t"}
	values := []string{"--body", "-b", "--body-file", "-F", "--title", "-t", "--repo", "-R"}
	issue := newGhDataGrammar(nil,
		append(append([]string{}, values...), "--assignee", "-a", "--label", "-l", "--milestone", "-m", "--project", "-p"), prose)
	comment := newGhDataGrammar([]string{"--edit-last", "--create-if-none"},
		[]string{"--body", "-b", "--body-file", "-F", "--repo", "-R"}, prose)
	pr := newGhDataGrammar([]string{"--draft", "-d"},
		append(append([]string{}, values...), "--base", "-B", "--head", "-H", "--assignee", "-a", "--label", "-l", "--milestone", "-m", "--project", "-p", "--reviewer", "-r"), prose)
	review := newGhDataGrammar([]string{"--approve", "-a", "--comment", "--request-changes", "-r"},
		[]string{"--body", "-b", "--body-file", "-F", "--repo", "-R", "--commit", "-c"}, prose)
	release := newGhDataGrammar([]string{"--draft", "-d", "--prerelease", "-p", "--verify-tag"},
		[]string{"--notes", "-n", "--notes-file", "-F", "--title", "-t", "--repo", "-R", "--target"},
		[]string{"--notes", "-n", "--notes-file", "-F", "--title", "-t"})
	return map[string]*ghDataGrammar{
		"issue create": issue, "issue edit": issue,
		"issue comment": comment, "pr comment": comment,
		"pr create": pr, "pr edit": pr,
		"pr review":      review,
		"release create": release, "release edit": release,
	}
}()

// ghDataPresentScan is the presence test of the gh data verbs
// over the words after the verb: it names each content option that occurs
// before the first `--`, whether or not it has a value.
func ghDataPresentScan(args []shellwords.Word, presence argv.Spec) map[string]bool {
	present := map[string]bool{}
	for _, arg := range presence.Scan(args).Args {
		switch arg.Role {
		case argv.RoleEnd:
			return present
		case argv.RoleOption:
			present[arg.Name] = true
		}
	}
	return present
}

// inertGhArgsScan reads the gh data verbs. Each verb's flags are independent:
// -n, -t and -b have different meanings on other gh commands. A named file is
// data; its contents are never read here.
func inertGhArgsScan(words []shellwords.Word) (bool, map[int]bool) {
	if len(words) < 3 || !words[1].Literal || !words[2].Literal {
		return false, nil
	}
	verb := words[1].Text + " " + words[2].Text
	grammar, ok := ghDataGrammars[verb]
	if !ok {
		return false, nil
	}
	ok, marks := decodeDataScan(words, 3, grammar.spec, grammar.prose, false, false)
	if !ok {
		return false, nil
	}
	// Creation without explicit content may launch the configured editor.
	// Only syntactic options count; a matching word inside prose cannot pay
	// for the missing title/body/notes field.
	present := ghDataPresentScan(words[3:], grammar.presence)
	body := present["--body"] || present["-b"] || present["--body-file"] || present["-F"]
	if (verb == "issue create" || verb == "pr create") && (!body || (!present["--title"] && !present["-t"])) {
		return false, nil
	}
	if strings.HasSuffix(verb, "comment") && !body {
		return false, nil
	}
	if verb == "pr review" && !body {
		return false, nil
	}
	if verb == "release create" && !present["--notes"] && !present["-n"] && !present["--notes-file"] && !present["-F"] {
		return false, nil
	}
	return true, marks
}

func inertDataInvocation(inv shellwords.Invocation, pagerDisabled bool) (bool, map[int]bool) {
	words := inv.Words
	if len(words) == 0 || !words[0].Literal {
		return false, nil
	}
	head := strings.ToLower(shellwords.ExecutableName(words[0].Text))
	// A file merely named cat/gh may be an arbitrary script. The widened
	// exemption recognizes ordinary command names, not arbitrary executables.
	if words[0].Text != head {
		return false, nil
	}
	if head == "tee" || head == "cat" {
		for _, word := range words[1:] {
			path := strings.ToLower(strings.ReplaceAll(word.Text, "\\", "/"))
			if strings.Contains(path, "/dev/") || strings.Contains(path, "/proc/") {
				return false, nil
			}
		}
	}
	switch head {
	case "cat":
		return decodeDataScan(words, 1, inertCatSpec, nil, false, true)
	case "tee":
		return decodeDataScan(words, 1, inertTeeSpec, nil, false, true)
	case "mkdir":
		return decodeDataScan(words, 1, inertMkdirSpec, nil, false, true)
	case "cd":
		if len(words) > 3 {
			return false, nil
		}
		return decodeDataScan(words, 1, inertCdSpec, nil, false, true)
	case "echo":
		return decodeDataScan(words, 1, inertEchoSpec, nil, true, false)
	case "printf":
		start := 1
		if start < len(words) && words[start].Text == "--" {
			start++
		}
		if start >= len(words) || !words[start].Literal || strings.HasPrefix(words[start].Text, "-") ||
			strings.Contains(inertPrintfConversion.ReplaceAllString(words[start].Text, ""), "%") {
			return false, nil
		}
		marks := map[int]bool{}
		for i := start; i < len(words); i++ {
			if !words[i].Literal {
				return false, nil
			}
			marks[i] = true
		}
		return true, marks
	case "grep":
		return decodeDataScan(words, 1, inertGrepSpec, []string{"-e", "--regexp"}, true, false)
	case "rg":
		if !rgNoConfigScan(words) {
			return false, nil
		}
		return decodeDataScan(words, 1, inertRGSpec, []string{"-e", "--regexp"}, true, false)
	case "git":
		start, ok := gitGrepArgStart(argv.ReadGit(words[1:]), pagerDisabled)
		if !ok {
			return false, nil
		}
		return decodeDataScan(words, start, inertGitGrepSpec, []string{"-e"}, true, false)
	case "gh":
		return inertGhArgsScan(words)
	}
	return false, nil
}

// gitGrepArgStart reads a `git grep` whose pager is off, over the words
// after `git`. It returns the index, counted in the invocation's words, of the
// first word after `grep`. A line that is not read to its verb, or that has
// any global option other than one literal `--no-pager` or `-P`, is no data
// command, so a pattern on it is never treated as inert.
func gitGrepArgStart(g argv.Git, pagerDisabled bool) (argStart int, ok bool) {
	if g.State != argv.GitVerb || g.Verb.Text != "grep" {
		return 0, false
	}
	switch len(g.Globals) {
	case 0:
	case 1:
		option := g.Globals[0].Option
		if !option.Literal || option.Text != "--no-pager" && option.Text != "-P" {
			return 0, false
		}
		pagerDisabled = true
	default:
		return 0, false
	}
	if !pagerDisabled {
		return 0, false
	}
	return g.VerbAt + 2, true
}
