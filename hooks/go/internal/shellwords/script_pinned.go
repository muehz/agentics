package shellwords

import (
	"regexp"
	"strings"

	"github.com/muehz/agentics/hooks/go/internal/argv"
)

// pinnedNames are the variables a script may not assign: they change which program, library, startup file or
// directory every later command uses, so a proof about one command no longer holds for the next. BASH_CMDS and
// BASH_ALIASES rebind a command name to another program or text. Names are matched case-sensitively, as POSIX shells
// do.
var pinnedNames = map[string]bool{"PATH": true, "BASH_ENV": true, "ENV": true, "SHELLOPTS": true, "BASHOPTS": true, "PS4": true,
	"PSModulePath": true, "TMPDIR": true, "HOME": true, "CDPATH": true, "SHELL": true, "NODE_OPTIONS": true, "PYTHONPATH": true,
	"BASH_CMDS": true, "BASH_ALIASES": true}

var pinnedPrefixes = []string{"GIT_", "LD_", "DYLD_", "BASH_FUNC_", "XDG_"}

func isPinned(name string) bool {
	if pinnedNames[name] {
		return true
	}
	for _, p := range pinnedPrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// variableName returns the variable a word names, without a trailing + or an array subscript: NAME for NAME=value,
// NAME+=value and NAME[i]=value, and the whole word when it has no =.
func variableName(text string) string {
	if i := strings.IndexByte(text, '='); i > 0 {
		text = strings.TrimSuffix(text[:i], "+")
	}
	if i := strings.IndexByte(text, '['); i > 0 {
		text = text[:i]
	}
	return text
}

// assignmentName returns NAME for NAME=value and NAME+=value, "" otherwise.
func assignmentName(w Word) string {
	if strings.IndexByte(w.Text, '=') <= 0 {
		return ""
	}
	return variableName(w.Text)
}

// hiddenName reports whether the name a word assigns is built at run time. `read "$n"` and `export "$n=v"` assign
// whichever variable $n holds, and a subscript such as a[$(cmd)] runs its substitution when the name is evaluated.
func hiddenName(w Word) bool {
	text := w.Text
	if i := strings.IndexByte(text, '='); i > 0 {
		text = text[:i]
	}
	return strings.ContainsAny(text, "$`")
}

// stripBuiltin drops each leading builtin, and a -- after it: builtin runs the builtin its next word names.
func stripBuiltin(words []Word) []Word {
	for len(words) > 1 && words[0].Literal && strings.ToLower(ExecutableName(words[0].Text)) == "builtin" {
		words = words[1:]
		if len(words) > 1 && words[0].Literal && words[0].Text == "--" {
			words = words[1:]
		}
	}
	return words
}

// optionScan is the result of reading the words after a builtin's name the way getopts does.
type optionScan struct {
	operands []Word // the words that are not options or option values
	targets  []Word // the values of options that name a variable
	seen     string // the option letters, in order
}

// scanOptionsArgv splits args into options and operands the way a builtin reads them, through the shared option walk:
// each letter of values is a value option that takes the rest of its word as its value, or the next word when nothing
// is left, and any other letter after a dash is a flag. That is why `read -pt x` has the prompt "t" and operand x, and
// `read -aPATH` has the array PATH. The letters in targets are those whose value names a variable. The walk reads a
// cluster letter by letter, a `--` word that is no option as a cluster of its own, and nothing as an option after
// `--`. A dash word that is not literal, and a lone dash, are operands, as are the words after `--`; `--` itself is
// neither.
//
// Every word is readable and nothing is unreadable: the walk's Problem is ignored, because a builtin treats a word
// it cannot place as an operand or a flag, and an option that ends the line has an empty value. A guard that reads
// this result therefore gets no "unreadable" outcome to refuse on.
func scanOptionsArgv(args []Word, values, targets string) optionScan {
	rows := make([]argv.Option, 0, len(values))
	for i := 0; i < len(values); i++ {
		rows = append(rows, argv.Option{Name: "-" + string(values[i]), Arity: argv.ArityValue})
	}
	scan := argv.NewSpec(rows...).With(argv.Clusters, argv.AnyLetter).Scan(args)
	var out optionScan
	for _, arg := range scan.Args {
		switch arg.Role {
		case argv.RoleOperand, argv.RoleUnknown:
			out.operands = append(out.operands, arg.Word)
		case argv.RoleOption:
			// A word that is one whole row carries no cluster letters; its letter is the row's own.
			letters := arg.Letters
			if letters == "" {
				letters = arg.Name[1:]
			}
			for j := 0; j < len(letters); j++ {
				out.seen += string(letters[j])
			}
			// A cluster that ends on a value letter is the only option arg that names a row.
			if arg.Name == "" || strings.IndexByte(targets, arg.Name[1]) < 0 {
				continue
			}
			value := Word{Text: "", Literal: true}
			switch {
			case arg.HasValue && arg.Joined:
				value = Word{Text: arg.Value.Text, Literal: true}
			case arg.HasValue:
				value = arg.Value
			}
			out.targets = append(out.targets, value)
		}
	}
	return out
}

// namedVariable maps a variable name to the construct that assigning it is: IFS persists into every later split, a
// pinned name moves the environment. constructNone means the assignment is harmless.
func namedVariable(name string) construct {
	switch {
	case name == "IFS":
		return constructPersistentIFS
	case isPinned(name):
		return constructPinnedAssignment
	}
	return constructNone
}

// variableConstruct judges the variable a word names: a name built at run time is as bad as a pinned one.
func variableConstruct(w Word) construct {
	if hiddenName(w) {
		return constructPinnedAssignment
	}
	return namedVariable(variableName(w.Text))
}

// prefixConstruct names the construct that a command's prefix assignments and environment resets make, or constructNone. A
// prefix assignment to an unpinned name, and an IFS prefix on read, stay allowed: they last for one command and are
// how scripts split lines. env -i clears PATH and HOME for the command it runs.
func prefixConstruct(assignments []Word, resets []EnvironmentReset, exe string) construct {
	for _, a := range assignments {
		switch name := assignmentName(a); {
		case hiddenName(a):
			return constructPinnedAssignment
		case name == "IFS" && exe != "read":
			return constructIFSPrefix
		case isPinned(name):
			return constructPinnedAssignment
		}
	}
	for _, r := range resets {
		if r.All || isPinned(r.Name.Text) {
			return constructPinnedAssignment
		}
	}
	return constructNone
}

// Options of read and mapfile whose next word is a value. read -a names an array, so its value is a target; mapfile
// -C names a command that runs as the file is read.
const (
	readValues    = "pdnNtuia"
	readTargets   = "a"
	mapfileValues = "dnOsuCc"
)

// pinnedConstruct names the construct on inv that assigns a pinned name or IFS, rebinds a command name, defines an
// alias, or runs a string as a command, or constructNone when there is none. resets are the environment resets that belong to
// the invocation's own script.
func pinnedConstruct(inv *Invocation, resets []EnvironmentReset) construct {
	words := stripBuiltin(inv.Words)
	exe := ""
	if len(words) > 0 {
		exe = strings.ToLower(ExecutableName(words[0].Text))
	}
	if rule := prefixConstruct(inv.Assignments, resets, exe); rule != constructNone {
		return rule
	}
	if len(words) == 0 {
		return constructNone
	}
	args := words[1:]
	// NAME+=value and NAME[i]=value are not assignment words to the lexer, so they arrive as the command word.
	if strings.IndexByte(words[0].Text, '=') > 0 {
		if rule := variableConstruct(words[0]); rule != constructNone {
			return rule
		}
	}
	switch exe {
	case "export", "declare", "typeset", "readonly", "local":
		for _, w := range args {
			if strings.HasPrefix(w.Text, "-") && !strings.Contains(w.Text, "=") {
				continue
			}
			if rule := variableConstruct(w); rule != constructNone {
				return rule
			}
		}
		if exe == "declare" || exe == "typeset" || exe == "local" {
			for _, w := range args {
				if strings.HasPrefix(w.Text, "-") && strings.ContainsRune(w.Text, 'n') {
					return constructNameref
				}
			}
		}
	case "read", "mapfile", "readarray", "unset", "wait":
		var scan optionScan
		switch exe {
		case "read":
			scan = scanOptionsArgv(args, readValues, readTargets)
		case "mapfile", "readarray":
			scan = scanOptionsArgv(args, mapfileValues, "")
			if strings.ContainsRune(scan.seen, 'C') {
				return constructEval
			}
		case "wait":
			scan = scanOptionsArgv(args, "p", "p")
			scan.operands = nil // wait's operands are job and process ids
		default:
			scan = optionScan{operands: args}
		}
		for _, w := range append(scan.operands, scan.targets...) {
			if rule := variableConstruct(w); rule != constructNone {
				return rule
			}
		}
	case "for", "select":
		if len(args) > 0 {
			return variableConstruct(args[0])
		}
	case "getopts":
		if len(args) > 1 {
			return variableConstruct(args[1])
		}
	case "let":
		for _, w := range args {
			if hiddenName(w) {
				return constructPinnedAssignment
			}
			for _, name := range arithmeticAssignments(w.Text) {
				if rule := namedVariable(name); rule != constructNone {
					return rule
				}
			}
		}
	case "hash":
		if scan := scanOptionsArgv(args, "p", ""); strings.ContainsRune(scan.seen, 'p') { // hash NAME only caches the PATH lookup
			return constructPinnedAssignment
		}
	case "enable":
		return constructPinnedAssignment
	case "alias":
		return constructAlias
	case "shopt":
		for _, w := range args {
			if w.Text == "expand_aliases" {
				return constructAlias
			}
		}
	case "coproc":
		return constructCoproc
	case "printf":
		if len(args) > 0 && strings.HasPrefix(args[0].Text, "-v") {
			target := Word{Text: args[0].Text[2:], Literal: args[0].Literal}
			if target.Text == "" && len(args) > 1 {
				target = args[1]
			}
			name := variableName(target.Text)
			if !target.Literal || hiddenName(target) || isPinned(name) || name == "IFS" {
				return constructPrintfV
			}
		}
	}
	return constructNone
}

// arithmeticAssign finds a name assigned inside an arithmetic expression: NAME=, NAME[i]= and the compound forms
// such as += and <<=. The comparisons ==, !=, <= and >= are not assignments.
var arithmeticAssign = regexp.MustCompile(`(?:^|[^\w$])([A-Za-z_]\w*)\s*(?:\[[^\]]*\])?\s*(?:\*\*|<<|>>|[-+*/%&|^])?=(?:[^=]|$)`)

func arithmeticAssignments(expression string) []string {
	if strings.IndexByte(expression, '=') < 0 {
		return nil
	}
	var names []string
	for _, m := range arithmeticAssign.FindAllStringSubmatch(expression, -1) {
		names = append(names, m[1])
	}
	return names
}

// bareAssignmentConstruct names the construct a segment-less assignment makes, or constructNone. A bare assignment lasts for the
// rest of the script, so IFS and pinned names are marked here where the prefix form allows them for one command.
func bareAssignmentConstruct(w Word) construct {
	if hiddenName(w) {
		return constructPinnedAssignment
	}
	return namedVariable(assignmentName(w))
}

var (
	functionName   = `[^\s()<>;&|'"` + "`" + `$\\{}]+`
	funcDefParens  = regexp.MustCompile(`(?:^|[\s;&|(){])(` + functionName + `)\s*\(\s*\)`)
	funcDefKeyword = regexp.MustCompile(`(?:^|[\s;&|(){])function\s+(` + functionName + `)`)
	promptCommand  = regexp.MustCompile(`(?:^|[\s;&|(){])PROMPT_COMMAND\+?=`)
	// A subscript attached to a name, holding a substitution: bash evaluates it as arithmetic and so runs it.
	subscriptSubstitution = regexp.MustCompile("[A-Za-z_][A-Za-z0-9_]*\\[[^\\]\\n]*(?:\\$\\(|`)")
	// (( )) and [[ ]] spans; they may cross lines, which only the whole-text pass sees.
	arithmeticSpan = regexp.MustCompile(`(?s)\(\((.*?)\)\)`)
	conditionSpan  = regexp.MustCompile(`(?s)\[\[(.*?)\]\]`)
	letArguments   = regexp.MustCompile(`\blet\b(.*)`)
	// ${NAME:=value} and ${NAME=value} assign NAME; {NAME}> opens a descriptor and stores its number in NAME.
	defaultAssign = regexp.MustCompile(`\$\{([A-Za-z_]\w*)(?:\[[^\]]*\])?:?=`)
	descriptorVar = regexp.MustCompile(`(?:^|[^$\w])\{([A-Za-z_]\w*)\}[<>]`)
)

// textHit is a construct that the lexer does not surface as an invocation: a function definition lexes to the
// single word of its name, an arithmetic subscript lexes to one non-literal word, and a `((` command, a ${NAME:=v}
// expansion and a {NAME}> redirection leave no assignment word, so the script text is scanned line by line instead.
type textHit struct {
	rule construct
	line int
}

// scanTextConstructs scans the scan text of scriptLines line by line. When the heredoc view and the comment tracker
// are sure of the script it has comments and the bodies of quoted heredocs blanked, so a note that mentions a shadow
// form and a usage example held in a quoted heredoc are not marked. When they are not, it is the script as written and
// no line is skipped, not even one that starts with #: such a line can sit inside a string that closes on it, with code
// after the quote. The scan runs over text rather than lexed words, so it errs toward blocking: a shadow form inside
// a quoted string is marked.
//
// A backslash-newline joins two lines into one, so a line ending in a backslash is also scanned joined with the lines
// it continues into, and its hits go to the line it starts on; the lines are still scanned apart as well, because the
// joining can glue a word onto the one a definition needs to start at. An arithmetic command or expansion and a [[ ]]
// test may also span lines, so those spans are matched across the whole text too. A construct found more than once on
// a line is reported once.
func scanTextConstructs(text string) []textHit {
	var hits []textHit
	seen := map[textHit]bool{}
	adder := func(line int) func(construct) {
		return func(rule construct) {
			if h := (textHit{rule, line}); !seen[h] {
				seen[h] = true
				hits = append(hits, h)
			}
		}
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		scanLine(line, adder(i+1))
	}
	for i := 0; i < len(lines); i++ {
		if !strings.HasSuffix(lines[i], `\`) {
			continue
		}
		first := i
		var joined strings.Builder
		for ; i < len(lines) && strings.HasSuffix(lines[i], `\`); i++ {
			joined.WriteString(strings.TrimSuffix(lines[i], `\`))
		}
		if i < len(lines) {
			joined.WriteString(lines[i])
		}
		scanLine(joined.String(), adder(first+1))
	}
	for _, re := range []*regexp.Regexp{arithmeticSpan, conditionSpan} {
		if !strings.Contains(text, spanOpener(re)) {
			continue
		}
		for _, m := range re.FindAllStringSubmatchIndex(text, -1) {
			span := text[m[2]:m[3]]
			if !strings.Contains(span, "\n") {
				continue
			}
			span = strings.ReplaceAll(strings.ReplaceAll(span, "\\\n", ""), "\n", " ")
			add := adder(strings.Count(text[:m[0]], "\n") + 1)
			if re == arithmeticSpan {
				for _, name := range arithmeticAssignments(span) {
					if rule := namedVariable(name); rule != constructNone {
						add(rule)
					}
				}
			}
			if subscriptSubstitution.MatchString(span) {
				add(constructArithmeticSubstitution)
			}
		}
	}
	return hits
}

// spanOpener is the text a match of a span or let expression cannot do without.
func spanOpener(re *regexp.Regexp) string {
	switch re {
	case conditionSpan:
		return "[["
	case letArguments:
		return "let"
	case arithmeticSpan:
		return "(("
	}
	return "" // an expression not named here is never skipped
}

// scanLine reports through add each construct found on one line of script text. Every expression below needs a literal
// that a line of ordinary commands rarely holds, so a cheap substring test runs first and the expression, which
// backtracks, only runs on a line that can match it. The tests are necessary conditions of the expressions, never
// stricter, so the verdicts do not change.
func scanLine(line string, add func(rule construct)) {
	if strings.TrimSpace(line) == "" {
		return
	}
	// `rm() {`, `rm()` with the brace on the next line, `function rm`, and several definitions on one line.
	for _, d := range [...]struct {
		re       *regexp.Regexp
		possible bool
	}{
		{funcDefParens, strings.IndexByte(line, '(') >= 0 && strings.IndexByte(line, ')') >= 0},
		{funcDefKeyword, strings.Contains(line, "function")},
	} {
		if !d.possible {
			continue
		}
		for _, m := range d.re.FindAllStringSubmatch(line, -1) {
			switch name := strings.ToLower(ExecutableName(m[1])); {
			case argv.Lookup(name).Shadow:
				add(constructFunctionShadow)
			case name == "command_not_found_handle":
				add(constructCommandNotFoundHandle)
			}
		}
	}
	if strings.Contains(line, "PROMPT_COMMAND") && promptCommand.MatchString(line) {
		add(constructPromptCommand)
	}
	for _, d := range [...]struct {
		re       *regexp.Regexp
		possible bool
	}{
		{defaultAssign, strings.Contains(line, "${") && strings.IndexByte(line, '=') >= 0},
		{descriptorVar, strings.IndexByte(line, '{') >= 0 && strings.ContainsAny(line, "<>")},
	} {
		if !d.possible {
			continue
		}
		for _, m := range d.re.FindAllStringSubmatch(line, -1) {
			if rule := namedVariable(m[1]); rule != constructNone {
				add(rule)
			}
		}
	}
	if strings.Contains(line, "((") {
		for _, m := range arithmeticSpan.FindAllStringSubmatch(line, -1) {
			for _, name := range arithmeticAssignments(m[1]) {
				if rule := namedVariable(name); rule != constructNone {
					add(rule)
				}
			}
		}
	}
	if arithmeticSubstitution(line) {
		add(constructArithmeticSubstitution)
	}
}

// arithmeticSubstitution reports whether line evaluates a subscript that holds a substitution as arithmetic: inside
// (( )), in an argument of let, or in a [[ ]] test. A single-bracket test never evaluates its operands that way.
func arithmeticSubstitution(line string) bool {
	for _, re := range []*regexp.Regexp{arithmeticSpan, conditionSpan, letArguments} {
		if !strings.Contains(line, spanOpener(re)) {
			continue
		}
		for _, m := range re.FindAllStringSubmatch(line, -1) {
			if subscriptSubstitution.MatchString(m[1]) {
				return true
			}
		}
	}
	return false
}

// syntheticOpaque builds a refused child for a construct that is not an invocation of its own. Raw stays empty and
// the only word is a fixed label, so a guard that prints either prints nothing of the script.
func (p *parser) syntheticOpaque(rule construct, origin *ScriptOrigin, line int, wrappers []Word, parent int, complete bool) Invocation {
	var o *ScriptOrigin
	if origin != nil {
		o = &ScriptOrigin{Path: origin.Path, Chain: origin.Chain, Line: line}
	}
	return Invocation{Dialect: POSIX, Words: []Word{{Text: "script-construct", Raw: "script-construct", Literal: true}},
		Wrappers: append([]Word(nil), wrappers...), WrapperIDs: append([]int(nil), p.wrapperIDs...),
		Before: Sequence, Parent: parent, Complete: complete, Inspectable: false, Script: o,
		Reason: opaqueRefusal(rule, o)}
}

// markScriptTextConstructs appends the refused children for the constructs of one expanded script that no invocation
// carries: a heredoc whose body the parser may end on another line than bash, the bare assignments recorded since
// bare, each with the line it was found on, and the text hits. Bare assignments already claimed by a script nested
// inside this one belong to that script and are skipped.
func (p *parser) markScriptTextConstructs(lines *scriptLines, bare int, origin *ScriptOrigin, wrappers []Word, parent int, complete bool) {
	if lines.oddHeredoc > 0 {
		p.result.Invocations = append(p.result.Invocations, p.syntheticOpaque(constructHeredocDelimiter, origin, lines.oddHeredoc, wrappers, parent, complete))
	}
	cursor := 0
	for i := bare; i < len(p.result.BareAssignments); i++ {
		if p.bareClaimed[i] {
			continue
		}
		if p.bareClaimed == nil {
			p.bareClaimed = map[int]bool{}
		}
		p.bareClaimed[i] = true
		w := p.result.BareAssignments[i]
		raw := w.Raw
		if raw == "" {
			raw = w.Text
		}
		var line int
		line, cursor = lines.locate(cursor, raw)
		if rule := bareAssignmentConstruct(w); rule != constructNone {
			p.result.Invocations = append(p.result.Invocations, p.syntheticOpaque(rule, origin, line, wrappers, parent, complete))
		}
	}
	for _, hit := range scanTextConstructs(lines.scan) {
		p.result.Invocations = append(p.result.Invocations, p.syntheticOpaque(hit.rule, origin, hit.line, wrappers, parent, complete))
	}
}

// pendingCall is a refused stand-in for the prefix of a script call, waiting for the line of the call.
type pendingCall struct {
	index int
	rule  construct
	raw   string
}

// ownResets returns the environment resets of inv that belong to the script it came from, not to the call that
// started that script.
func (p *parser) ownResets(inv *Invocation) []EnvironmentReset {
	return inv.EnvironmentResets[min(p.resetBase, len(inv.EnvironmentResets)):]
}

// noteCallPrefix reports a pinned or IFS prefix on a call to a nested script once, at the call, instead of once for
// each command of the nested script. The commands from index start on belong to the call; the prefix reaches them
// only after they were judged, so it is judged here, and the stand-in takes the line of the call when the script that
// holds the call has been parsed. A call made from the typed command has no script to hold it, so its prefix is not
// judged by this layer; a later change to the typed checks owns it. wrappers and wrapperIDs are those of the call
// itself, aligned, not the shell wrapper the called script adds to its own commands.
func (p *parser) noteCallPrefix(start int, raw string, assignments []Word, resets []EnvironmentReset, wrappers []Word, wrapperIDs []int) {
	chain := p.script.chain
	if len(chain) == 0 || start >= len(p.result.Invocations) {
		return
	}
	rule := prefixConstruct(assignments, resets, "")
	if rule == constructNone {
		return
	}
	nested := false
	for i := start; i < len(p.result.Invocations); i++ {
		nested = nested || p.result.Invocations[i].Script != nil
	}
	if !nested {
		return
	}
	first := p.result.Invocations[start]
	origin := &ScriptOrigin{Path: chain[len(chain)-1], Chain: append([]string(nil), chain[:len(chain)-1]...)}
	p.result.Invocations = append(p.result.Invocations, Invocation{Dialect: POSIX,
		Words:    []Word{{Text: "script-construct", Raw: "script-construct", Literal: true}},
		Wrappers: append([]Word(nil), wrappers...), WrapperIDs: append([]int(nil), wrapperIDs...),
		Before: Sequence, Parent: first.Parent, Complete: first.Complete, Inspectable: false, Script: origin})
	p.pendingCalls = append(p.pendingCalls, pendingCall{len(p.result.Invocations) - 1, rule, raw})
}

// locatePendingCalls gives the stand-ins made from index start on the line of their call in lines.
func (p *parser) locatePendingCalls(start int, lines *scriptLines) {
	cursor := 0
	kept := p.pendingCalls[:0]
	for _, pc := range p.pendingCalls {
		if pc.index < start {
			kept = append(kept, pc)
			continue
		}
		inv := &p.result.Invocations[pc.index]
		var line int
		line, cursor = lines.locate(cursor, pc.raw)
		// A new origin, not a write through the old one: a forked parse abandoned at its deadline may still hold it.
		inv.Script = &ScriptOrigin{Path: inv.Script.Path, Chain: inv.Script.Chain, Line: line}
		inv.Reason = opaqueRefusal(pc.rule, inv.Script)
	}
	p.pendingCalls = kept
}
