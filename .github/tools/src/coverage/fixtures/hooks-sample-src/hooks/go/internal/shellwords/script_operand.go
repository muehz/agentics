package shellwords

import (
	"regexp"
	"slices"
	"strings"
	"unicode"
)

// A protected verb in a script may take a non-literal operand only when the operand is a variable proven to hold one
// value. Every fact that proves a value comes from the lexer: the one bare assignment, its literal text, the invocation
// index it was found at. The script text is read only to refuse, so a text pattern that misreads bash can only cost a
// proof, never grant one.

// constantValue admits a value that bash hands to a command unchanged whether or not the use is quoted: no blank,
// glob, brace, tilde, quote, expansion or operator character.
var constantValue = regexp.MustCompile(`^[A-Za-z0-9._/:@%+,=-]+$`)

// mktempCall admits only mktemp with an optional -d and one template, which prints a path it has just created. Any
// other option (-u, -p, -t, --tmpdir, --dry-run) or a second command fails the pattern. An unquoted template may not
// hold a character that ends the command. The pattern trusts the name mktemp: a function of that name, in the script
// or in a file it sources, must be refused by the function-shadow mark, whose name set therefore has to keep mktemp.
var mktempCall = regexp.MustCompile("^\\$\\(\\s*mktemp(?:\\s+-d)?(?:\\s+(\"[^\"]*\"|'[^']*'|[^\\s\"'()`;&|<>\\\\]+))?\\s*\\)$")

// tempComponent is the last path component a mktemp template may have: literal, ending in the random part.
var tempComponent = regexp.MustCompile(`^[A-Za-z0-9._-]*XXXXXX$`)

// scriptTempPath stands for a proven mktemp result: a child of the temporary directory, as the path mktemp made is.
const scriptTempPath = "/tmp/agentics-script-temp"

// dynamicNames are set by the shell itself, between any two commands or as the unnamed target of a builtin: read
// and select write REPLY, mapfile writes MAPFILE, getopts writes OPTARG and OPTIND, coproc writes COPROC and
// COPROC_PID, and checkwinsize writes LINES and COLUMNS. None of those writes names the variable in a word.
var dynamicNames = map[string]bool{"_": true, "PWD": true, "OLDPWD": true, "IFS": true, "RANDOM": true, "SRANDOM": true,
	"SECONDS": true, "LINENO": true, "BASHPID": true, "PPID": true, "UID": true, "EUID": true, "HISTCMD": true,
	"EPOCHSECONDS": true, "EPOCHREALTIME": true, "GROUPS": true, "DIRSTACK": true, "PIPESTATUS": true, "FUNCNAME": true,
	"REPLY": true, "MAPFILE": true, "OPTARG": true, "OPTIND": true, "COPROC": true, "COPROC_PID": true, "LINES": true,
	"COLUMNS": true}

// nameWriters are the commands that can assign a variable named by one of their words.
var nameWriters = map[string]bool{"read": true, "mapfile": true, "readarray": true, "getopts": true, "unset": true,
	"export": true, "declare": true, "typeset": true, "readonly": true, "local": true, "let": true, "printf": true,
	"for": true, "select": true, "wait": true}

// enclosingKeywords open or condition a command; one before the assignment may make it run zero or several times.
var enclosingKeywords = map[string]bool{"if": true, "then": true, "elif": true, "else": true, "fi": true, "while": true,
	"until": true, "for": true, "do": true, "done": true, "case": true, "esac": true, "select": true, "function": true,
	"time": true, "coproc": true, "!": true, "[[": true, "]]": true}

type operandProof struct {
	value string // the constant, "" for a temp
	temp  bool
	at    int // the first invocation index parsed after the assignment
}

// operandScope is one expanded script: its invocations from start on, its bare assignments from bare on, the
// invocations it produced itself, its lines, and the number of wrappers its own commands carry.
type operandScope struct {
	start, bare int
	own         []int
	lines       *scriptLines
	sourced     bool
	wrappers    int
}

// operandReference is a whole word that is one plain reference: $NAME, ${NAME}, "$NAME" or "${NAME}".
var operandReference = regexp.MustCompile(`^("?)\$(?:([A-Za-z_][A-Za-z0-9_]*)|\{([A-Za-z_][A-Za-z0-9_]*)\})("?)$`)

// applyOperandRule rewrites each proven operand of a protected verb to a literal word, so the guards judge the path it
// names, and refuses the invocation at the first operand it cannot prove.
//
// A sourced script proves nothing: a function it defines runs whenever its caller calls it, after writes in the caller
// that the sourced text cannot see. A command run by a further shell, such as bash -c, expands the variable in that
// shell, where only an exported value exists.
func (p *parser) applyOperandRule(s operandScope) {
	// The first pass finds the variables protected verbs reference, so a script without such a use costs no proof
	// work at all, and a proof is sought only for a name that is used.
	type use struct {
		index      int
		verb       string
		otherShell bool
	}
	var uses []use
	wanted := map[string]bool{}
	for _, i := range s.own {
		inv := &p.result.Invocations[i]
		verb, protected := protectedVerb(inv)
		if !inv.Inspectable || !protected {
			continue
		}
		otherShell := inv.Dialect != POSIX
		for _, w := range inv.Wrappers[min(s.wrappers, len(inv.Wrappers)):] {
			exe := strings.ToLower(ExecutableName(w.Text))
			otherShell = otherShell || isShellExe(exe) || exe == "pwsh" || exe == "powershell"
		}
		uses = append(uses, use{i, verb, otherShell})
		for _, w := range inv.Words[1:] {
			if m := operandReference.FindStringSubmatch(w.Raw); !w.Literal && m != nil && !otherShell {
				wanted[m[2]+m[3]] = true
			}
		}
	}
	var proofs map[string]operandProof
	if !s.sourced && len(wanted) > 0 {
		proofs = p.operandProofs(s, wanted)
	}
	for _, u := range uses {
		i, verb, otherShell := u.index, u.verb, u.otherShell
		inv := &p.result.Invocations[i]
		// A forked parse abandoned at its deadline may still hold this invocation's words, so they are copied before
		// the first rewrite rather than written in place.
		copied := false
		for k := 1; k < len(inv.Words); k++ {
			if inv.Words[k].Literal {
				continue
			}
			m := operandReference.FindStringSubmatch(inv.Words[k].Raw)
			var proof operandProof
			// The lexer never yields a word with one unmatched quote; m[1] == m[4] is defense in depth.
			ok := m != nil && m[1] == m[4] && !otherShell
			if ok {
				proof, ok = proofs[m[2]+m[3]]
			}
			switch {
			case !ok || i < proof.at:
			case !proof.temp:
				if !copied {
					inv.Words, copied = slices.Clone(inv.Words), true
				}
				inv.Words[k] = Word{Text: proof.value, Raw: proof.value, Literal: true}
				continue
			case verb == "rm" && m[1] == `"`: // unquoted, a temporary directory holding a blank would split
				if !copied {
					inv.Words, copied = slices.Clone(inv.Words), true
				}
				inv.Words[k] = Word{Text: scriptTempPath, Raw: scriptTempPath, Literal: true}
				continue
			}
			inv.Inspectable = false
			inv.Reason = ScriptReason("script-operand-not-literal", inv.Script)
			break
		}
	}
}

// operandProofs returns the wanted variables of the script that hold one known value from their assignment on. The
// facts it compares against are gathered once per script, on the first candidate, so the cost grows with the script
// and not with the product of its assignments and its lines.
func (p *parser) operandProofs(s operandScope, wanted map[string]bool) map[string]operandProof {
	bares := p.result.BareAssignments[s.bare:]
	writes := map[string]int{}
	for _, b := range bares {
		if name := assignmentName(b); wanted[name] {
			writes[name]++
		}
	}
	var facts *operandFacts
	proofs := map[string]operandProof{}
	for i, b := range bares {
		name := assignmentName(b)
		if !wanted[name] {
			continue
		}
		// The lexer records only NAME=value as a bare assignment (NAME+=value arrives as a command word), a claimed
		// assignment's text is never a line of this script, and the lexer never marks a word both literal and
		// substituted; the HasPrefix, bareClaimed and Substituted tests are defense in depth.
		if writes[name] != 1 || p.bareClaimed[s.bare+i] || dynamicName(name) || !strings.HasPrefix(b.Text, name+"=") || s.bare+i >= len(p.bareAt) {
			continue
		}
		proof := operandProof{at: p.bareAt[s.bare+i]}
		value := b.Text[len(name)+1:]
		switch {
		case b.Literal && !b.Substituted && constantValue.MatchString(value):
			proof.value = value
		case b.Substituted && mktempResult(value):
			proof.temp = true
		default:
			continue
		}
		if facts == nil {
			facts = p.gatherOperandFacts(s.start, s.lines.scan)
		}
		if facts.flatTop(b.Raw) && !facts.written[name] {
			proofs[name] = proof
		}
	}
	return proofs
}

// operandFacts are what one script's proofs compare against.
type operandFacts struct {
	// lineAt maps each line of the scan text, trimmed, to its index, or to -1 when it occurs more than once.
	lineAt map[string]int
	// clean is the index of the first line that is not clean in the sense of flatTop; every line before it is.
	clean int
	// written holds the names that something other than a bare assignment can write.
	written map[string]bool
}

func (p *parser) gatherOperandFacts(start int, scan string) *operandFacts {
	src := strings.Split(scan, "\n")
	f := &operandFacts{lineAt: make(map[string]int, len(src)), clean: len(src), written: p.writtenNames(start, scan)}
	for i, line := range src {
		key := strings.TrimSpace(line)
		if _, seen := f.lineAt[key]; seen {
			f.lineAt[key] = -1
		} else {
			f.lineAt[key] = i
		}
		if f.clean == len(src) && !cleanLine(line) {
			f.clean = i
		}
	}
	return f
}

func dynamicName(name string) bool {
	return dynamicNames[name] || isPinned(name) || strings.HasPrefix(name, "BASH_") || strings.HasPrefix(name, "COMP_") ||
		strings.HasPrefix(name, "READLINE_")
}

// mktempResult reports whether value is a plain mktemp call whose template, if any, ends in a literal component.
func mktempResult(value string) bool {
	m := mktempCall.FindStringSubmatch(value)
	if m == nil {
		return false
	}
	tmpl := m[1]
	if tmpl == "" {
		return true
	}
	if len(tmpl) >= 2 && (tmpl[0] == '"' || tmpl[0] == '\'') && tmpl[len(tmpl)-1] == tmpl[0] {
		tmpl = tmpl[1 : len(tmpl)-1]
	}
	return !strings.HasPrefix(tmpl, "-") && tempComponent.MatchString(tmpl[strings.LastIndexByte(tmpl, '/')+1:])
}

// wordRun is a maximal run of word characters: a name stands as a whole word in text exactly when it is one of them.
var wordRun = regexp.MustCompile(`\w+`)

// writtenNames returns every name that anything the script runs, or a nested script it runs, can write, apart from
// bare assignments, which the caller counts.
//
// Integer-valued divergence only: an assignment inside an arithmetic context other than (( )), $(( )) and let is not
// detected ($[NAME=5], [[ 1 -eq NAME=5 ]], an array subscript, ${x:NAME=1}, a declare -i variable's value), nor is
// arithmetic text inside a nested script. Bash arithmetic can store only a decimal integer, so the operand the shell
// really passes is then an integer-named relative path, never /, ., .., ~, .git or .env.
func (p *parser) writtenNames(start int, scan string) map[string]bool {
	written := map[string]bool{}
	for _, inv := range p.result.Invocations[start:] {
		for _, a := range inv.Assignments {
			written[assignmentName(a)] = true // a prefix on a special builtin persists in POSIX mode
		}
		words := stripBuiltin(inv.Words)
		if len(words) == 0 {
			continue
		}
		if strings.IndexByte(words[0].Text, '=') > 0 { // NAME+=v, NAME[i]=v
			written[variableName(words[0].Text)] = true
		}
		exe := strings.ToLower(ExecutableName(words[0].Text))
		if !nameWriters[exe] {
			continue
		}
		for _, w := range words[1:] {
			written[variableName(w.Text)] = true
			if strings.HasPrefix(w.Text, "-") {
				// An option cluster can end in the name its last letter takes: read -raNAME, printf -vNAME.
				for j := 1; j < len(w.Text); j++ {
					written[w.Text[j:]] = true
				}
			}
			if exe == "let" {
				for _, name := range wordRun.FindAllString(w.Text, -1) {
					written[name] = true
				}
			}
		}
	}
	for _, re := range []*regexp.Regexp{defaultAssign, descriptorVar} {
		for _, m := range re.FindAllStringSubmatch(scan, -1) {
			written[m[1]] = true
		}
	}
	for _, m := range arithmeticSpan.FindAllStringSubmatch(scan, -1) {
		for _, name := range wordRun.FindAllString(m[1], -1) {
			written[name] = true
		}
	}
	return written
}

// flatTop reports whether the assignment spelled raw is a line of its own that every run reaches exactly once: the
// only line equal to it, preceded only by clean lines (see cleanLine).
//
// With the earlier lines clean, the first matching line starts a command of its own and so is the assignment itself;
// another copy of the assignment is a second write. The unique-line test stays as defense in depth.
func (f *operandFacts) flatTop(raw string) bool {
	at, ok := f.lineAt[raw]
	return ok && at >= 0 && at <= f.clean
}

// cleanLine reports whether line cannot open a group, function, loop, condition, pipeline, continuation, heredoc or
// multi-line string. Each such construct starts with a byte or keyword refused here, so a miss only costs a proof. The
// trailing backslash test is needed on its own: `echo a \` before an assignment line makes it an argument of echo.
func cleanLine(line string) bool {
	for prev := ""; prev != line; {
		prev, line = line, plainExpansion.ReplaceAllStringFunc(line, func(m string) string { return m[2 : len(m)-1] })
	}
	if strings.ContainsAny(line, "(){}`|&") || strings.Contains(line, "<<") || strings.Contains(line, "$'") ||
		strings.HasSuffix(line, `\`) || !quotesClosed(line) {
		return false
	}
	for _, tok := range strings.FieldsFunc(line, func(r rune) bool { return r == ';' || unicode.IsSpace(r) }) {
		if enclosingKeywords[tok] {
			return false
		}
	}
	return true
}

// plainExpansion is a ${...} parameter expansion holding no brace, quote, backslash or newline, so its closing brace
// is the first one and neither brace can open or close a group. flatTop drops the $ and both braces and keeps the
// inside, so a ( or backtick in it is still refused.
var plainExpansion = regexp.MustCompile(`\$\{[^{}'"\\\n]*\}`)

// quotesClosed reports whether line, read from outside any quote, ends outside one. A # is not taken as a comment:
// bash would stop reading quotes there, so the line is then closed for bash whatever this scan finds.
func quotesClosed(line string) bool {
	var quote byte
	for i := 0; i < len(line); i++ {
		switch c := line[i]; {
		case quote == '\'':
			if c == '\'' {
				quote = 0
			}
		case c == '\\':
			i++
		case quote == '"':
			if c == '"' {
				quote = 0
			}
		case c == '\'' || c == '"':
			quote = c
		}
	}
	return quote == 0
}

// protectedVerb returns the command name and whether a non-literal operand of inv must be proven. A git command whose
// subcommand cannot be read is protected.
//
// chmod and git branch are protected whatever their literal words are. The rule only ever acts on a non-literal word,
// and getopt permutes options, so such a word may itself expand to -R or -D wherever it stands; only a proof that
// rewrites it to its literal value lets the guards judge the recursive or deleting form.
func protectedVerb(inv *Invocation) (string, bool) {
	words := stripBuiltin(inv.Words)
	if len(words) == 0 {
		return "", false
	}
	exe := strings.ToLower(ExecutableName(words[0].Text))
	switch exe {
	case "rm", "dd", "chmod":
		return exe, true
	case "git":
		verb, ok := gitSubcommand(words)
		return exe, !ok || verb == "push" || verb == "branch"
	}
	return exe, false
}

// gitSubcommand skips git's global options the way the git rules do and returns the subcommand.
func gitSubcommand(words []Word) (string, bool) {
	for i := 1; i < len(words); i++ {
		text := words[i].Text
		switch {
		case text == "-C" || text == "-c" || text == "--git-dir" || text == "--work-tree" || text == "--namespace" || text == "--config-env":
			i++
		case len(text) > 2 && (strings.HasPrefix(text, "-C") || strings.HasPrefix(text, "-c")),
			strings.HasPrefix(text, "--git-dir="), strings.HasPrefix(text, "--work-tree="), strings.HasPrefix(text, "--namespace="),
			strings.HasPrefix(text, "--exec-path="), strings.HasPrefix(text, "--config-env="):
		case text == "--literal-pathspecs" || text == "--glob-pathspecs" || text == "--noglob-pathspecs" ||
			text == "--icase-pathspecs" || text == "--no-optional-locks" || text == "--bare" || text == "--no-pager" ||
			text == "-P" || text == "--paginate" || text == "-p" || text == "--no-replace-objects" || text == "--no-lazy-fetch" ||
			text == "--no-advice":
		case strings.HasPrefix(text, "-") || !words[i].Literal:
			return "", false
		default:
			return strings.ToLower(text), true
		}
	}
	return "", false
}
