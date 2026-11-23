package shellwords

import (
	"context"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/muehz/agentics/hooks/go/internal/argv"
	"github.com/muehz/agentics/hooks/go/internal/pathshape"
)

// gitValueOptions are the git options before the subcommand that take the next word as their value. Only the lenient
// walk of lenientGitVerb reads them: the write-target reader uses that walk when ReadGit cannot read a line to its
// verb, so a line such as `git -- checkout .`, which git may still run, keeps its targets.
var gitValueOptions = map[string]bool{"-C": true, "-c": true, "--git-dir": true, "--work-tree": true, "--namespace": true,
	"--exec-path": true, "--super-prefix": true, "--config-env": true}

// stateCommands change the shell that runs the call, or its aliases, functions and traps, in ways the guard cannot
// follow from the text.
var stateCommands = map[string]bool{"alias": true, "unalias": true, "function": true, "source": true, ".": true, "eval": true, "trap": true}

// stateVariables are variables whose value makes the shell run commands of its own before or after the call.
// declarers are the builtins that give a variable or function a value the next shell can inherit.
var declarers = map[string]bool{"export": true, "declare": true, "typeset": true, "local": true, "readonly": true}

var stateVariables = map[string]bool{"PROMPT_COMMAND": true}

// writeRedirects returns the redirects that open a file for writing. A descriptor duplication (2>&1, >&2, >&-) writes
// to no file of its own.
func writeRedirects(redirects []Redirection) []Redirection {
	var out []Redirection
	for _, r := range redirects {
		if !strings.Contains(r.Operator, ">") || isDescriptorDup(r) {
			continue
		}
		out = append(out, r)
	}
	return out
}

func isDescriptorDup(r Redirection) bool {
	if r.Operator != ">&" {
		return false
	}
	text := strings.TrimSuffix(r.Target.Text, "-")
	if text == "" {
		return r.Target.Text == "-"
	}
	for i := 0; i < len(text); i++ {
		if text[i] < '0' || text[i] > '9' {
			return false
		}
	}
	return true
}

// redirectsToFiles reports whether a redirect writes somewhere other than the terminal-like devices.
func redirectsToFiles(redirects []Redirection) bool {
	for _, r := range redirects {
		switch r.Target.Text {
		case "/dev/null", "/dev/stdout", "/dev/stderr":
			continue
		}
		return true
	}
	return false
}

// predecessorProblem reports the first rule an earlier typed invocation breaks, or RuleNone.
func predecessorProblem(inv *Invocation) Rule {
	// exec replaces the shell only when nothing between it and the typed list starts another process.
	if _, replaces := runsInList(*inv, 0); replaces {
		return RulePredecessorState
	}
	if redirectsToFiles(inv.OutputRedirects) {
		return RulePredecessorMayWrite
	}
	if !inv.Inspectable {
		if inv.Reason.Rule() == RuleBudget {
			// A canceled inspection already refuses the enclosing tool call. Let later
			// reads observe their own deadline and keep their own diagnostic.
			return RuleNone
		}
		return RulePredecessorMayWrite
	}
	if reason := predecessorWrapperProblem(inv.Wrappers, inv.Dialect); reason != RuleNone {
		return reason
	}
	if inv.FuncDef && slices.ContainsFunc(inv.Wrappers, func(w Word) bool { return w.Text == "bash" || w.Text == "sh" }) {
		return RuleNone
	}
	words := inv.Words
	for len(words) > 1 && words[0].Literal && words[0].Text == "builtin" && inv.Dialect == POSIX {
		words = words[1:]
		if words[0].Literal && words[0].Text == "--" {
			words = words[1:]
		}
	}
	if len(words) == 0 {
		return RuleNone
	}
	if !words[0].Literal {
		return RulePredecessorMayWrite
	}
	exe := shellName(words[0], inv.Dialect)
	args := words[1:]
	if prefixConstruct(inv.Assignments, inv.EnvironmentResets, exe) != constructNone {
		return RulePredecessorState
	}
	switch {
	case stateCommands[exe]:
		return RulePredecessorState
	case exe == "set" && setChangesState(args), exe == "shopt" && shoptChangesState(args):
		return RulePredecessorState
	case declarers[exe] && declaresState(exe, args):
		return RulePredecessorState
	case nameAssigners[exe] && assignsStateName(exe, args):
		return RulePredecessorState
	case exe == "hash" && hasShortFlag(args, 'p'), exe == "enable" && len(operandWords(args)) > 0:
		return RulePredecessorState
	case exe == "mapfile" || exe == "readarray" || exe == "wait":
		if pinnedConstruct(inv, nil) != constructNone {
			return RulePredecessorState
		}
	}
	if nonWritingPredecessor(exe, args, inv) {
		return RuleNone
	}
	return RulePredecessorMayWrite
}

var (
	literalArrayName  = regexp.MustCompile(`^[A-Za-z_][A-Za-z_0-9]*\+?=$`)
	literalArrayWords = regexp.MustCompile(`^[A-Za-z0-9_./+ \t-]+$`)
	simpleArithmetic  = regexp.MustCompile(`^(?:[A-Za-z_][A-Za-z_0-9]*[ \t]*(?:=|==)[ \t]*)?[0-9]+(?:[ \t]*[-+*/%][ \t]*[0-9]+)*$`)
	// Only ordinary variable and positional expansions count as quoted data.
	quotedArrayVariables = regexp.MustCompile(`^(?:[^$\x60\\]|\\.|[$](?:[A-Za-z_][A-Za-z_0-9]*|[0-9@*#?]|[{][A-Za-z_][A-Za-z_0-9]*(?:\[@\])?[}]))*$`)
)

// The splitter supplies the entire span from the lexical opener. Executable
// substitutions keep the ordinary checks active instead of becoming data.
func provenGroupData(opener byte, body, tail string) bool {
	if opener == '(' {
		var ok bool
		tail, ok = strings.CutPrefix(tail, ")")
		if !ok || !simpleArithmetic.MatchString(strings.TrimSpace(body)) {
			return false
		}
	} else {
		lexed := lexSegment(body, POSIX)
		if lexed.refusal.Refused() || len(lexed.redirects) != 0 {
			return false
		}
		for _, word := range lexed.words {
			quotedData := fullyQuoted(word) && (word.Literal || word.Raw[0] == '"' && quotedArrayVariables.MatchString(word.Raw[1:len(word.Raw)-1]))
			if word.Substituted || textAssignsState(word.Raw) || !quotedData && (!word.Literal || !literalArrayWords.MatchString(word.Raw)) {
				return false
			}
		}
		return true
	}
	tail = strings.TrimLeft(tail, " \t")
	return tail == "" || strings.HasPrefix(tail, "&&") || strings.HasPrefix(tail, "||") || tail[0] == ';' || tail[0] == '\n'
}

// Array syntax follows an unquoted assignment prefix, possibly after other
// assignments or reserved openers in the same command. It is not a callable
// group just because an element's value is a variable reference.
func arrayAssignmentPrefix(prefix string) bool {
	lexed := lexSegment(prefix, POSIX)
	if lexed.refusal.Refused() || len(lexed.redirects) != 0 || len(lexed.words) == 0 {
		return false
	}
	last := len(lexed.words) - 1
	word := lexed.words[last]
	if !literalArrayName.MatchString(word.Raw) {
		return false
	}
	command, _ := stripPrefixes(lexed.words[:last], true)
	name := variableName(word.Raw)
	return len(command) == 0 && namedVariable(name) == constructNone && !stateVariables[name]
}

func predecessorWrapperProblem(wrappers []Word, dialect Dialect) Rule {
	subshell := false
	for _, wrapper := range wrappers {
		if !wrapper.Literal {
			return RulePredecessorMayWrite
		}
		switch shellName(wrapper, dialect) {
		case "bash", "sh":
			subshell = true
		case "command", "time", "env":
		case "exec":
			if !subshell {
				return RulePredecessorState
			}
		default:
			return RulePredecessorMayWrite
		}
	}
	return RuleNone
}

// Only a command whose normal effect is known can precede the bytes a script reader inspected.
func nonWritingPredecessor(exe string, args []Word, inv *Invocation) bool {
	switch exe {
	case "git":
		return !gitWritesTree(args, inv.Dialect)
	case "bash":
		return inv.Inspection == BashMetadata
	case "command":
		return len(args) > 0 && args[0].Literal && args[0].Text == "-v"
	case "let":
		for _, a := range args {
			if !a.Literal || !simpleArithmetic.MatchString(a.Text) {
				return false
			}
		}
		return true
	case "env":
		return len(args) == 0
	case "find":
		return readOnlyFind(args)
	case "rg", "sort", "diff", "uniq", "file", "date", "hostname":
		return readOnlyUtility(exe, args)
	case "sort-object":
		return inv.Dialect == PowerShell && readOnlyPowerShellSort(args)
	case "mkdir", "md":
		for _, a := range args {
			if !a.Literal || activeBrace(a) || scriptPatternArg(a, inv.Dialect) || slices.Contains(strings.FieldsFunc(strings.ToLower(a.Text), func(r rune) bool { return r == '/' || r == '\\' }), ".agentics") {
				return false
			}
		}
		return true
	case "cd", "pwd", "true", "false", ":", "test", "[", "[[", "wait", "sleep",
		"echo", "printf", "ls", "dir", "stat", "head", "tail", "cat", "wc", "cut", "tr", "cmp", "grep",
		"which", "type", "dirname", "basename", "realpath", "readlink", "uname", "whoami", "id", "sha256sum", "shasum", "printenv",
		"set", "shopt", "declare", "typeset", "local", "export", "readonly", "read", "mapfile", "readarray", "getopts", "unset", "hash", "enable":
		return true
	case "set-location", "sl", "chdir", "get-location", "gl", "write-output", "write-host", "write-information",
		"get-childitem", "gci", "test-path", "get-item", "gi", "get-content", "gc", "select-string", "sls",
		"get-command", "gcm", "get-date", "get-filehash":
		return inv.Dialect == PowerShell
	}
	return false
}

var psSortSwitches = map[string]bool{"casesensitive": true, "descending": true, "stable": true, "unique": true}

func readOnlyPowerShellSort(args []Word) bool {
	for _, a := range args {
		if !a.Literal {
			return false
		}
	}
	const values = "property inputobject culture top bottom"
	named, _, ok := bindPSOptions("sort-object", args, values+" casesensitive descending stable unique", psSortSwitches)
	if !ok {
		return false
	}
	allowed := strings.Fields(values)
	for name := range named {
		if !slices.Contains(allowed, name) {
			return false
		}
	}
	return true
}

// Option recognition ends at --. Before then, a word expanded by the shell can supply an option.
func readOnlyUtility(exe string, args []Word) bool {
	var flags, values, short, shortValues string
	maxOperands := len(args)
	switch exe {
	case "sort":
		flags = "--reverse --unique --numeric-sort --human-numeric-sort --version-sort --ignore-case --stable --check --merge --zero-terminated"
		values = "--key --field-separator --parallel --buffer-size"
		short = "bdfghinMruVszcCm"
		shortValues = "ktS"
	case "rg":
		flags = "--hidden --files --files-with-matches --files-without-match --line-number --no-line-number --ignore-case --smart-case --case-sensitive --fixed-strings --word-regexp --invert-match --count --count-matches --quiet --no-heading --heading --json --no-ignore --text --no-filename --with-filename --only-matching --pcre2 --multiline --no-config --no-messages"
		values = "--glob --iglob --regexp --type --type-not --type-add --max-count --max-depth --context --before-context --after-context --color --sort --sortr"
		short = "inuSwlLFvqcHhoUPazs"
		shortValues = "geftTmABC"
	case "diff":
		flags = "--unified --brief --recursive --ignore-case --ignore-space-change --ignore-all-space --report-identical-files --new-file --text"
		short = "urqibwsaN"
	case "uniq":
		flags = "--count --repeated --unique --ignore-case --zero-terminated"
		short = "cduiz"
		maxOperands = 1
	case "file":
		flags = "--brief --mime --mime-type --mime-encoding --dereference --no-dereference"
		short = "biLh"
	case "date":
		flags = "--utc --universal --iso-8601 --rfc-email"
		values = "--date --reference"
		short = "uIR"
		shortValues = "dr"
	case "hostname":
		flags = "--short --fqdn --domain --ip-address --all-ip-addresses"
		short = "sfdiI"
		maxOperands = 0
	}
	operands := 0
	afterOptions := false
	longFlags, longValues := strings.Fields(flags), strings.Fields(values)
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !a.Literal || activeBrace(a) || scriptPatternArg(a, POSIX) {
			return false
		}
		if !afterOptions && a.Text == "--" {
			afterOptions = true
			continue
		}
		if afterOptions || !strings.HasPrefix(a.Text, "-") || a.Text == "-" {
			operands++
			if exe == "date" && !strings.HasPrefix(a.Text, "+") {
				return false
			}
			continue
		}
		name, _, joined := strings.Cut(a.Text, "=")
		if !joined && slices.Contains(longFlags, name) {
			continue
		}
		needsValue := false
		if strings.HasPrefix(a.Text, "--") {
			if !slices.Contains(longValues, name) {
				return false
			}
			needsValue = !joined
		} else {
			for j, flag := range a.Text[1:] {
				if strings.ContainsRune(shortValues, flag) {
					needsValue = j+2 == len(a.Text)
					break
				}
				if !strings.ContainsRune(short, flag) {
					return false
				}
			}
		}
		if needsValue {
			i++
			if i == len(args) || !args[i].Literal || activeBrace(args[i]) || scriptPatternArg(args[i], POSIX) {
				return false
			}
		}
	}
	return operands <= maxOperands
}

func activeBrace(w Word) bool {
	return strings.ContainsAny(w.Raw, "{}") && !fullyQuoted(w)
}

func fullyQuoted(w Word) bool {
	raw := w.Raw
	return len(raw) >= 2 && (raw[0] == '"' || raw[0] == '\'') && raw[len(raw)-1] == raw[0] && !strings.ContainsRune(raw[1:len(raw)-1], rune(raw[0]))
}

func readOnlyFind(args []Word) bool {
	expression := false
	for i := 0; i < len(args); i++ {
		w := args[i]
		if !expression && w.Text != "" && !strings.ContainsAny(w.Text[:1], "-!()") {
			if w.Literal && !activeBrace(w) && !scriptPatternArg(w, POSIX) {
				continue
			}
			if !w.Literal && fullyQuoted(w) && strings.Contains(w.Text, "/") && !strings.ContainsAny(w.Text[:1], "$`~{") {
				continue
			}
		}
		expression = true
		if !w.Literal || activeBrace(w) {
			return false
		}
		switch w.Text {
		case "-print", "-print0", "-true", "-false", "-empty", "-readable", "-writable", "-executable", "-prune", "-quit", "-depth", "-mount", "-xdev", "-a", "-and", "-o", "-or", "-not", "!", "(", ")":
		case "-name", "-iname", "-path", "-ipath", "-wholename", "-regex", "-iregex", "-type", "-xtype", "-size", "-maxdepth", "-mindepth", "-mtime", "-mmin", "-atime", "-ctime", "-user", "-group", "-perm", "-newer":
			i++
			if i == len(args) || (!args[i].Literal && !fullyQuoted(args[i])) || activeBrace(args[i]) || scriptPatternArg(args[i], POSIX) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// Only a parsed literal global option can establish where Git's subcommand and its operands begin.
func gitWritesTree(args []Word, dialect Dialect) bool {
	g := argv.ReadGit(args)
	if gitGlobalExpands(g, dialect, true) {
		return true
	}
	for _, global := range g.Globals {
		switch global.Name {
		case "-C", "--git-dir", "--work-tree", "--namespace", "--literal-pathspecs", "--glob-pathspecs", "--noglob-pathspecs", "--icase-pathspecs", "--no-optional-locks", "--bare", "--no-pager", "-P", "--no-replace-objects", "--no-lazy-fetch", "--no-advice":
		default:
			return true
		}
	}
	if g.State == argv.GitPrints {
		return len(g.Rest) != 0
	}
	if g.State != argv.GitVerb || !fixedGitWord(g.Verb, dialect) {
		return true
	}
	switch g.Verb.Text {
	case "status", "rev-parse", "ls-files", "check-ignore", "merge-base", "describe", "add", "commit", "fetch":
		return false
	case "diff", "log", "show":
		return !gitReadDiff(g.Args, dialect)
	case "branch", "tag":
		return !gitReadRefs(g.Verb.Text, g.Args, dialect)
	case "config":
		return !gitReadConfig(g.Args, dialect)
	case "restore", "rm":
		return !gitIndexOnly(g.Verb.Text, g.Args, dialect)
	}
	return true
}

func fixedGitWord(w Word, dialect Dialect) bool {
	return w.Literal && !activeBrace(w) && (dialect != POSIX || !scriptPatternArg(w, POSIX))
}

func gitReadDiff(args []Word, dialect Dialect) bool {
	for _, a := range args {
		if !fixedGitWord(a, dialect) {
			return false
		}
		if a.Text == "--" {
			return true
		}
		if !strings.HasPrefix(a.Text, "-") {
			continue
		}
		name, _, joined := strings.Cut(a.Text, "=")
		switch name {
		case "--format", "--pretty", "--color", "--abbrev", "--max-count", "--since", "--until", "--author", "--grep", "--unified":
			if joined {
				continue
			}
		case "--stat", "--shortstat", "--numstat", "--name-only", "--name-status", "--oneline", "--graph", "--all", "--no-color", "--no-patch", "--patch", "--cached", "--staged", "--no-ext-diff", "--no-textconv", "--exit-code", "--quiet", "--check", "--decorate", "--no-decorate", "--no-merges", "--reverse", "--first-parent", "--no-renames":
			if !joined {
				continue
			}
		}
		if len(a.Text) > 1 && (strings.Trim(a.Text[1:], "0123456789") == "" || strings.Trim(a.Text[1:], "pwsuz") == "") {
			continue
		}
		return false
	}
	return true
}

func gitReadRefs(verb string, args []Word, dialect Dialect) bool {
	patterns := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !fixedGitWord(a, dialect) {
			return false
		}
		if a.Text == "--" {
			return patterns || i+1 == len(args)
		}
		if !strings.HasPrefix(a.Text, "-") {
			if !patterns {
				return false
			}
			continue
		}
		name, _, joined := strings.Cut(a.Text, "=")
		switch name {
		case "-l", "--list":
			if joined {
				return false
			}
			patterns = true
		case "-v":
			if joined {
				return false
			}
			patterns = patterns || verb == "tag"
		case "--verify":
			if verb != "tag" || joined {
				return false
			}
			patterns = true
		case "-a", "--all", "-r", "--remotes", "-vv", "--show-current":
			if verb != "branch" || joined {
				return false
			}
		case "--sort", "--format":
			if !joined {
				i++
				if i == len(args) || !fixedGitWord(args[i], dialect) {
					return false
				}
			}
		case "--color", "--column", "--contains", "--no-contains", "--merged", "--no-merged", "--points-at":
		default:
			return false
		}
	}
	return true
}

func gitReadConfig(args []Word, dialect Dialect) bool {
	mode, operands := "", 0
	for _, a := range args {
		if !fixedGitWord(a, dialect) {
			return false
		}
		switch a.Text {
		case "--get", "--get-all", "--list", "-l":
			if mode != "" {
				return false
			}
			mode = a.Text
		case "--global", "--local", "--system", "--worktree", "--show-origin", "--show-scope", "--name-only", "--null", "-z":
		default:
			if strings.HasPrefix(a.Text, "-") {
				return false
			}
			operands++
		}
	}
	switch mode {
	case "--list", "-l":
		return operands == 0
	case "--get", "--get-all":
		return operands == 1 || operands == 2
	}
	return false
}

// gitGlobalExpands reports a global option word of g that is not literal.
func gitGlobalExpands(g argv.Git, dialect Dialect, requireFixedArgv bool) bool {
	for _, global := range g.Globals {
		if !global.Option.Literal || requireFixedArgv &&
			(!fixedGitWord(global.Option, dialect) || global.HasValue && !fixedGitWord(global.Value, dialect)) {
			return true
		}
	}
	return false
}

// curlNamesFromURL reports a curl invocation that saves under a name taken from the URL or the response: -O,
// --remote-name, --remote-name-all.
func curlNamesFromURL(args []Word) bool {
	for _, a := range args {
		if a.Literal && (a.Text == "--remote-name" || a.Text == "--remote-name-all") {
			return true
		}
	}
	return hasShortFlag(args, 'O')
}

// writesUnnamedFiles reports a writer whose words do not list the files it may write: one that applies a patch, runs
// a build or package script, unpacks an archive, checks out git content, or saves a download under a name the URL or
// the server picks. A writer that names its targets (tee, cp, a redirect, sed -i) is left to the write-target check,
// which compares those names with the inspected scripts.
func writesUnnamedFiles(inv *Invocation) bool {
	words := stripBuiltin(inv.Words)
	if len(words) == 0 {
		return false
	}
	exe := strings.ToLower(ExecutableName(words[0].Text))
	args := words[1:]
	switch exe {
	case "patch", "make", "npm", "npx", "pnpm", "yarn", "tar", "unzip":
		return true
	case "go":
		return len(args) > 0 && args[0].Text == "generate"
	case "wget":
		return len(optionValues(args, 'O', "--output-document")) == 0
	case "curl":
		return curlNamesFromURL(args)
	case "git":
		return gitWritesTree(args, inv.Dialect)
	}
	return false
}

// declaresState reports an export, declaration or readonly of a variable that changes which program or startup file
// later commands use, one that runs commands of its own, one whose name is built at run time, or of a function (-f),
// which the next shell inherits. The name counts with or without a value, and a name given to export -n without a
// value is only unexported.
//
// -p does not make export or readonly print-only: bash 5.3 still assigns `export -p NAME=v` and `readonly -p NAME=v`,
// and `export -p NAME` and `export -pf f` still export, so for them -p prints only when no operand follows, and then
// nothing changes anyway. declare, typeset and local with -p print and assign nothing in bash 5.3, but that is not
// assumed of every bash a host may run, so they count as print-only only when every operand is a literal name without
// a value.
//
// For declare, typeset and local, -n makes a name reference: every later assignment to the name, a bare `r=/x`
// included, assigns the variable it refers to, and an unset reference takes its target from the first assignment. The
// text after it does not say which variable that is, so any operand counts. -i likewise: every later assignment to an
// integer variable is evaluated as arithmetic, so `n=BASH_ENV=1` assigns BASH_ENV (bash 5.3).
func declaresState(exe string, args []Word) bool {
	printing, nflag, iflag := false, false, false
	for _, a := range args {
		if a.Literal && len(a.Text) > 1 && a.Text[0] == '-' && !strings.HasPrefix(a.Text, "--") {
			printing = printing || strings.IndexByte(a.Text, 'p') > 0
			nflag = nflag || strings.IndexByte(a.Text, 'n') > 0
			iflag = iflag || strings.IndexByte(a.Text, 'i') > 0
		}
	}
	if printing && exe != "export" && exe != "readonly" && namesOnly(args) {
		return false
	}
	unexport := nflag && exe == "export"
	if (nflag || iflag) && exe != "export" && exe != "readonly" && len(operandWords(args)) > 0 {
		return true
	}
	for _, a := range args {
		if a.Literal && len(a.Text) > 1 && a.Text[0] == '-' {
			if !strings.HasPrefix(a.Text, "--") && strings.IndexByte(a.Text, 'f') > 0 {
				return true
			}
			continue
		}
		if unexport && !strings.Contains(a.Text, "=") {
			continue
		}
		if hiddenName(a) {
			return true
		}
		if name := variableName(a.Text); isPinned(name) || stateVariables[name] {
			return true
		}
	}
	return false
}

// nameAssigners are the builtins that assign or unset a variable named by an operand rather than by a NAME=value word:
// read, mapfile and readarray store input, printf -v stores its output, getopts the option letter, let an arithmetic
// result. PATH is exported, so `read PATH` before a call changes which bash runs it.
var nameAssigners = map[string]bool{"read": true, "mapfile": true, "readarray": true, "printf": true, "getopts": true, "let": true,
	"unset": true}

// assignsStateName reports one of those builtins that may assign a pinned or state variable: a word holds one of
// their names as an identifier, or a word is built at run time and so may name one. The identifier scan also reads
// prompts and format strings, which over-refuses on purpose. printf assigns only with -v, and then only the word after
// it names a variable; a first word built at run time may be -v, so then every word counts.
func assignsStateName(exe string, args []Word) bool {
	words := args
	if exe == "printf" && len(args) > 0 && args[0].Literal {
		words = nil
		for i := 0; i < len(args) && args[i].Literal && strings.HasPrefix(args[i].Text, "-") && args[i].Text != "--"; i++ {
			if args[i].Text == "-v" && i+1 < len(args) {
				i++
				words = append(words, args[i])
			} else if rest, ok := strings.CutPrefix(args[i].Text, "-v"); ok && rest != "" {
				w := args[i]
				w.Text, w.Raw = rest, ""
				words = append(words, w)
			}
		}
	}
	for _, w := range words {
		if !w.Literal {
			return true
		}
		for _, id := range strings.FieldsFunc(w.Text, func(r rune) bool {
			return r != '_' && (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9')
		}) {
			if isPinned(id) || stateVariables[id] {
				return true
			}
		}
	}
	return false
}

// Shell options are judged against allowlists, so an option bash adds later, or a spelling not foreseen here, is
// refused rather than passed. allexport (-a) exports every later assignment, default-value expansion and arithmetic
// assignment; keyword (-k) moves a NAME=value word anywhere in a command into its environment (`bash x.sh BASH_ENV=/x`);
// posix keeps an assignment before a special builtin; physical changes what cd resolves; interactive-comments off makes
// # a word; restricted mode and unknown letters are not modeled. Turning allexport, keyword, posix or physical off is
// harmless, turning interactive-comments on is the default.
var (
	// setLetters are the short options safe in either direction; offLetters are safe only after +.
	setLetters = "euxvfnCBHhETmbpt"
	offLetters = "akP"
	// setNames are the -o names safe in either direction; offNames only after +o, onNames only after -o.
	setNames = map[string]bool{"braceexpand": true, "emacs": true, "errexit": true, "errtrace": true, "functrace": true, "hashall": true,
		"histexpand": true, "history": true, "ignoreeof": true, "monitor": true, "noclobber": true, "noexec": true, "noglob": true,
		"nolog": true, "notify": true, "nounset": true, "onecmd": true, "pipefail": true, "privileged": true, "verbose": true, "vi": true,
		"xtrace": true}
	offNames = map[string]bool{"allexport": true, "keyword": true, "posix": true, "physical": true}
	onNames  = map[string]bool{"interactive-comments": true}
	// shoptOn are the shopt options safe to turn on: glob and history behavior, error handling and the like. shoptOff
	// adds the ones that are only safe to turn off: aliases, a DEBUG-trap mode, source through PATH, cd to a variable's
	// value, and the interactive-only directory helpers.
	shoptOn = map[string]bool{"nullglob": true, "extglob": true, "globstar": true, "dotglob": true, "nocaseglob": true, "nocasematch": true,
		"failglob": true, "lastpipe": true, "inherit_errexit": true, "checkwinsize": true, "cmdhist": true, "histappend": true, "lithist": true,
		"huponexit": true, "globasciiranges": true, "globskipdots": true, "xpg_echo": true, "shift_verbose": true, "progcomp": true,
		"progcomp_alias": true, "promptvars": true, "hostcomplete": true, "complete_fullquote": true, "direxpand": true, "dirspell": true,
		"force_fignore": true, "gnu_errfmt": true, "histreedit": true, "histverify": true, "mailwarn": true, "no_empty_cmd_completion": true,
		"checkjobs": true, "execfail": true, "localvar_inherit": true, "localvar_unset": true, "noexpand_translation": true,
		"varredir_close": true, "patsub_replacement": true, "assoc_expand_once": true, "array_expand_once": true,
		"bash_source_fullpath": true, "interactive_comments": true}
	shoptOff = map[string]bool{"expand_aliases": true, "extdebug": true, "sourcepath": true, "cdable_vars": true, "autocd": true,
		"cdspell": true, "checkhash": true}
)

// setChangesState reports a set whose words are not all on the allowlist: a - or + cluster of safe letters, each o of
// which takes the next word as a safe name (none at the end lists the options). Positional parameters, -, --, a word
// built at run time and anything unknown count.
func setChangesState(args []Word) bool {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !a.Literal || len(a.Text) < 2 || a.Text[0] != '-' && a.Text[0] != '+' || a.Text == "--" {
			return true
		}
		on, names := a.Text[0] == '-', 0
		for _, c := range a.Text[1:] {
			switch {
			case c == 'o':
				names++
			case strings.ContainsRune(setLetters, c), !on && strings.ContainsRune(offLetters, c):
			default:
				return true
			}
		}
		// Each o takes the next word as its name, as bash does.
		for ; names > 0 && i+1 < len(args); names-- {
			i++
			if !safeOptionName(args[i], on) {
				return true
			}
		}
	}
	return false
}

func safeOptionName(w Word, on bool) bool {
	return w.Literal && (setNames[w.Text] || on && onNames[w.Text] || !on && offNames[w.Text])
}

// shoptChangesState reports a shopt that sets or unsets an option off the allowlist. -o selects the set -o names.
// Options end at the first name, as in bash, so a later -o or -s is a name and counts. Printing and querying (-p, -q,
// neither -s nor -u) change nothing.
func shoptChangesState(args []Word) bool {
	set, unset, setNamespace := false, false, false
	i := 0
	for ; i < len(args); i++ {
		a := args[i]
		if !a.Literal {
			return true
		}
		if len(a.Text) < 2 || a.Text[0] != '-' {
			break
		}
		for _, c := range a.Text[1:] {
			switch c {
			case 's':
				set = true
			case 'u':
				unset = true
			case 'o':
				setNamespace = true
			case 'p', 'q':
			default:
				return true
			}
		}
	}
	if set && unset {
		return true
	}
	for _, n := range args[i:] {
		switch {
		case !n.Literal:
			return true
		case !set && !unset:
		case setNamespace:
			if !safeOptionName(n, set) {
				return true
			}
		case set:
			if !shoptOn[n.Text] {
				return true
			}
		default:
			if n.Text == "interactive_comments" || !shoptOn[n.Text] && !shoptOff[n.Text] {
				return true
			}
		}
	}
	return false
}

var (
	// indirectDefault is ${!NAME:=v} or ${!NAME=v}: it assigns the variable whose name NAME holds at run time.
	indirectDefault = regexp.MustCompile(`\$\{!\w+(?:\[[^\]]*\])?:?=`)
	// arithmeticContexts hold arithmetic that may assign: (( )) and $(( )), the old $[ ] form, the operands of a
	// [[ ]] test, and an array subscript.
	arithmeticContexts = []*regexp.Regexp{arithmeticSpan, regexp.MustCompile(`(?s)\$\[(.*?)\]`), conditionSpan,
		regexp.MustCompile(`[A-Za-z_]\w*\[([^\]\n]*)\]`)}
)

// textAssignsState reports a default-value expansion (${NAME:=v}, ${NAME=v}) or an arithmetic assignment of a pinned or
// state variable anywhere in the text, quoted or not, and an indirect default, whose target is built at run time.
// None leaves an assignment word, and under allexport each exports.
func textAssignsState(text string) bool {
	if !strings.Contains(text, "${") && !strings.Contains(text, "((") && !strings.Contains(text, "[") {
		return false
	}
	if indirectDefault.MatchString(text) {
		return true
	}
	stateName := func(name string) bool { return isPinned(name) || stateVariables[name] }
	for _, m := range defaultAssign.FindAllStringSubmatch(text, -1) {
		if stateName(m[1]) {
			return true
		}
	}
	for _, re := range arithmeticContexts {
		for _, m := range re.FindAllStringSubmatch(text, -1) {
			for _, name := range arithmeticAssignments(m[1]) {
				if stateName(name) {
					return true
				}
			}
		}
	}
	return false
}

// namesOnly reports whether every operand is a literal word without a value.
func namesOnly(args []Word) bool {
	for _, w := range operandWords(args) {
		if !w.Literal || strings.Contains(w.Text, "=") {
			return false
		}
	}
	return true
}

// hasShortFlag reports a short-option cluster (-x, -pi, -sSo) that holds the letter c.
//
// It is the script proof's own because of its output-letter test: it finds a writing letter such as curl's -o or
// sed's -i anywhere in a cluster, without knowing which letters take a value, so it calls a command a writer wherever a
// shared option table, which would stop at a value letter, might not.
func hasShortFlag(args []Word, c byte) bool {
	for _, a := range args {
		if strings.HasPrefix(a.Text, "-") && !strings.HasPrefix(a.Text, "--") && strings.IndexByte(a.Text, c) > 0 {
			return true
		}
	}
	return false
}

// lenientGitVerb returns the subcommand word, skipping the options git accepts before it. The write-target reader
// calls it when ReadGit cannot read a line to its verb, or the line prints, so `git -- checkout .` keeps its targets.
func lenientGitVerb(args []Word) (Word, bool) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !a.Literal || !strings.HasPrefix(a.Text, "-") {
			return a, true
		}
		if gitValueOptions[a.Text] {
			i++
		}
	}
	return Word{}, false
}

// predecessorReason judges the typed invocations parsed before the script call being expanded. Commands inside a
// script are never predecessors of a typed call here, and an earlier typed script call keeps proof: its content is
// judged by the checks that run on script content. A predecessor found clean stays clean, so the scan resumes where
// the last one stopped. The text check is not ordered: a default-value or arithmetic assignment of a pinned name
// anywhere in a typed list refuses every call in it, which over-refuses one that only follows the call.
func (p *parser) predecessorReason() Rule {
	if p.typedFunction || p.typedStateText {
		return RulePredecessorState
	}
	if p.strayWrite {
		return RulePredecessorMayWrite
	}
	for p.predecessorScanned < len(p.result.Invocations) {
		inv := p.result.Invocations[p.predecessorScanned]
		if inv.Script == nil {
			inv = p.predecessorInActiveList(inv)
			if reason := predecessorProblem(&inv); reason != RuleNone {
				return reason
			}
		}
		p.predecessorScanned++
	}
	for p.bareScanned < len(p.result.BareAssignments) {
		if p.bareClaimed[p.bareScanned] {
			p.bareScanned++
			continue
		}
		// A bare assignment lasts for the rest of the shell, so a pinned name or IFS set before the call changes what the
		// script runs under, as an export of it does.
		if w := p.result.BareAssignments[p.bareScanned]; stateVariables[variableName(w.Text)] || hiddenName(w) || namedVariable(variableName(w.Text)) != constructNone {
			return RulePredecessorState
		}
		p.bareScanned++
	}
	return RuleNone
}

// An already-running shell is the context of this list, not a predecessor
// process. Wrapper IDs distinguish equal spellings in separate invocations.
func (p *parser) predecessorInActiveList(inv Invocation) Invocation {
	if len(p.lists) == 0 {
		return inv
	}
	depth := min(p.lists[len(p.lists)-1].depth, len(p.wrapperIDs), len(inv.WrapperIDs), len(inv.Wrappers))
	shared := 0
	for shared < depth && p.wrapperIDs[shared] != 0 && p.wrapperIDs[shared] == inv.WrapperIDs[shared] {
		shared++
	}
	inv.WrapperIDs, inv.Wrappers = inv.WrapperIDs[shared:], inv.Wrappers[shared:]
	return inv
}

// noteStrayWrites handles write redirects of a segment that runs no command of its own. They belong to a group, a
// subshell, a function body or a process substitution that the splitter closed just before, whose closer leaves no
// invocation to carry them (`{ cat; } > f`, `cat <(true) > f`). They are added to the invocation before, and a typed
// one also raises a flag, because that invocation may already have been judged as a predecessor.
func (p *parser) noteStrayWrites(redirects []Redirection) {
	out := writeRedirects(redirects)
	if len(out) == 0 {
		return
	}
	if len(p.script.chain) == 0 && redirectsToFiles(out) {
		p.strayWrite = true
	}
	if n := len(p.result.Invocations); n > 0 {
		inv := &p.result.Invocations[n-1]
		inv.OutputRedirects = append(slices.Clip(inv.OutputRedirects), out...)
	}
}

// funcDefBefore reports whether the text before a ( in a command is a function name: `name`, `function name`. A
// keyword, an array assignment and an extended glob are followed by ( without defining anything.
func funcDefBefore(raw string) bool {
	t := strings.TrimSpace(raw)
	if t == "" || strings.HasSuffix(t, "=") || strings.ContainsRune("@?*+!", rune(t[len(t)-1])) {
		return false
	}
	fields := strings.Fields(t)
	switch last := fields[len(fields)-1]; {
	case isSegmentOpener(last):
		return false
	case last == "for" || last == "select" || last == "case" || last == "time" || last == "coproc" || last == "in":
		return false
	}
	return true
}

// noteSpelling remembers that path was asked for, once.
func (p *parser) noteSpelling(path string) {
	if !slices.Contains(p.scriptSpellings, path) {
		p.scriptSpellings = append(p.scriptSpellings, path)
	}
}

// writeKey is the one form in which paths are compared: separators as /, the drive path cleaned below its drive, and
// folded to lower case where the file system does not tell case apart.
func writeKey(host pathshape.Host, p string) string {
	p = host.Native(p)
	if host.Windows() {
		p = strings.ReplaceAll(p, `\`, "/")
	}
	if host.FoldsCase() {
		p = strings.ToLower(p)
	}
	return slashPath(p)
}

// inspectedPaths are the paths of every script this parse read, in the compared form: the path the reader reported
// and each spelling the calls asked for, with the directories that hold them.
type inspectedPaths struct {
	powerShell bool
	host       pathshape.Host
	keys       []string
	set        map[string]bool
	dirs       []string
	dirSet     map[string]bool
	anyPath    bool
}

func (p *parser) inspectedPaths() inspectedPaths {
	in := inspectedPaths{host: p.script.host, set: map[string]bool{}, dirSet: map[string]bool{}}
	add := func(s string) {
		k := writeKey(in.host, s)
		if s == "" || in.set[k] {
			return
		}
		in.set[k] = true
		in.keys = append(in.keys, k)
		if d := path.Dir(k); !in.dirSet[d] {
			in.dirSet[d] = true
			in.dirs = append(in.dirs, d)
		}
	}
	for _, rec := range p.result.Scripts {
		add(rec.Path)
	}
	for _, s := range p.scriptSpellings {
		add(s)
	}
	in.anyPath = len(in.keys) > 0
	return in
}

// endsWithPath reports whether some path in list ends with the relative path rel, a whole number of components.
func (in inspectedPaths) endsWithPath(list []string, rel string) bool {
	if in.host.FoldsCase() {
		rel = strings.ToLower(rel)
	}
	for _, k := range list {
		if k == rel || strings.HasSuffix(k, "/"+rel) {
			return true
		}
	}
	return false
}

// cleanRelative returns the relative path text with its leading .. components dropped, "" when nothing is left.
func cleanRelative(text string) string {
	rel := path.Clean(strings.ReplaceAll(text, `\`, "/"))
	for rel == ".." || strings.HasPrefix(rel, "../") {
		rel = strings.TrimPrefix(strings.TrimPrefix(rel, ".."), "/")
	}
	if rel == "." {
		return ""
	}
	return rel
}

// hits reports whether a write to the word may land on an inspected script. It fails closed where the lexical form
// cannot say which file the shell opens:
//   - a spelling the host's path rules call hazardous (see pathshape.Host.MayNameAnotherFile) counts as a match;
//   - a relative path is relative to the directory the script runs in when it writes, which the text alone does not
//     give (the script may cd), so it matches when what remains after its leading .. components is the end of an
//     inspected path;
//   - a word built at run time, or one that starts with ~, matches when it names the script itself ($0,
//     $BASH_SOURCE) or ends in the name of an inspected script after a /; any other such word is allowed, because
//     nothing says where it points.
func (in inspectedPaths) hits(w Word) bool {
	text := w.Text
	if text == "" {
		return false
	}
	if !w.Literal || strings.HasPrefix(text, "~") {
		if in.powerShell && !w.Literal && psSelfFile(text) {
			return true
		}
		if !w.Literal && selfReference(text) {
			return true
		}
		tail := text[strings.LastIndexAny(text, "/\\")+1:]
		return tail != "" && !strings.ContainsAny(tail, expandingChars+"~") && in.endsWithPath(in.keys, tail)
	}
	if in.host.MayNameAnotherFile(text) {
		return true
	}
	if isAbsolutePath(text) {
		return in.set[writeKey(in.host, text)]
	}
	rel := cleanRelative(text)
	return rel != "" && in.endsWithPath(in.keys, rel)
}

// hitsDir reports whether files written into the directory the word names may include an inspected script: the
// directory holds one, or is relative to a directory the text does not give.
func (in inspectedPaths) hitsDir(w Word) bool {
	text := w.Text
	if in.powerShell && in.anyPath && !w.Literal && (strings.EqualFold(text, "$PSScriptRoot") || strings.EqualFold(text, "${PSScriptRoot}")) {
		return true
	}
	if !in.anyPath || !w.Literal || text == "" || strings.HasPrefix(text, "~") {
		return false
	}
	if in.host.MayNameAnotherFile(text) {
		return true
	}
	if isAbsolutePath(text) {
		return in.dirSet[writeKey(in.host, text)]
	}
	rel := cleanRelative(text)
	return rel == "" || in.endsWithPath(in.dirs, rel)
}

// selfToken reports a word that expands to the path of the running script: $0, ${0}, $BASH_SOURCE, ${BASH_SOURCE} and
// ${BASH_SOURCE[i]}.
func selfToken(text string) bool {
	switch text {
	case "$0", "${0}", "$BASH_SOURCE":
		return true
	}
	if rest, ok := strings.CutPrefix(text, "${BASH_SOURCE"); ok {
		if rest == "}" {
			return true
		}
		if strings.HasPrefix(rest, "[") {
			if i := strings.IndexByte(rest, ']'); i > 0 && rest[i+1:] == "}" {
				return true
			}
		}
	}
	return false
}

// selfName reports the parameter name of the running script.
func selfName(name string) bool {
	return name == "0" || name == "BASH_SOURCE" || strings.HasPrefix(name, "BASH_SOURCE[") && strings.HasSuffix(name, "]")
}

// defaultForm splits ${NAME:-default}, ${NAME-default}, ${NAME:=default} and ${NAME=default}.
func defaultForm(text string) (name, def string, ok bool) {
	if !strings.HasPrefix(text, "${") || !strings.HasSuffix(text, "}") {
		return "", "", false
	}
	inner := text[2 : len(text)-1]
	for i := 0; i < len(inner); i++ {
		switch c := inner[i]; c {
		case '[':
			j := strings.IndexByte(inner[i:], ']')
			if j < 0 {
				return "", "", false
			}
			i += j
		case ':':
			if i+1 < len(inner) && (inner[i+1] == '-' || inner[i+1] == '=') {
				return inner[:i], inner[i+2:], true
			}
			return "", "", false
		case '-', '=':
			return inner[:i], inner[i+1:], true
		case '%', '#', '/', '^', ',', '+', '?', '@', '!':
			return "", "", false
		}
	}
	return "", "", false
}

// selfReference reports a word that expands to the path of the running script: the forms selfToken lists, a default
// form whose parameter or default is one, and a word that ends in one (`realpath "$0"`, `basename "$0"`). The
// directory of the script (`dirname "$0"`) and a name built from it (`${0}.bak`) are other files.
func selfReference(text string) bool {
	if selfToken(text) {
		return true
	}
	if name, def, ok := defaultForm(text); ok {
		return selfName(name) || selfReference(def)
	}
	t := strings.TrimRight(text, `)" '`)
	i := strings.LastIndexByte(t, '$')
	if i <= 0 || !selfToken(t[i:]) {
		return false
	}
	return !strings.HasSuffix(strings.TrimRight(t[:i], `" '`), "dirname")
}

// operandWords returns the words that are not options; after -- every word is an operand.
func operandWords(args []Word) []Word {
	var out []Word
	for i, a := range args {
		if a.Literal && a.Text == "--" {
			return append(out, args[i+1:]...)
		}
		if a.Literal && len(a.Text) > 1 && a.Text[0] == '-' {
			continue
		}
		out = append(out, a)
	}
	return out
}

// bulkSource reports a copy source that stands for many files: a glob, a directory with a trailing / or /., or the
// working directory itself.
func bulkSource(w Word) bool {
	return strings.ContainsAny(w.Text, "*?[") || directoryCopySource(w.Text)
}

func directoryCopySource(text string) bool {
	return strings.HasSuffix(text, "/") || strings.HasSuffix(text, "/.") || text == "." || text == ".."
}

// copyDestinations returns what a cp, mv, install, ln or rsync writes: the destination, and for a destination
// directory the file each source becomes in it. -t names the directory and every operand is then a source. bulk holds
// the destination directory when a source stands for many files, which may include an inspected script.
func copyDestinations(args []Word) (files, bulk []Word) {
	var dir *Word
	rest := make([]Word, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a.Literal && a.Text == "-t" && i+1 < len(args):
			i++
			dir = &args[i]
		case a.Literal && strings.HasPrefix(a.Text, "--target-directory="):
			w := a
			w.Text, w.Raw = strings.TrimPrefix(a.Text, "--target-directory="), ""
			dir = &w
		case a.Literal && a.Text == "--target-directory" && i+1 < len(args):
			i++
			dir = &args[i]
		default:
			rest = append(rest, a)
		}
	}
	operands := operandWords(rest)
	var dest *Word
	sources := operands
	if dir != nil {
		dest = dir
	} else if len(operands) >= 2 {
		dest = &operands[len(operands)-1]
		sources = operands[:len(operands)-1]
	}
	if dest == nil {
		return nil, nil
	}
	files = []Word{*dest}
	for _, s := range sources {
		if dest.Literal && s.Literal && s.Text != "" {
			files = append(files, Word{Text: path.Join(dest.Text, path.Base(strings.ReplaceAll(s.Text, `\`, "/"))), Literal: true})
		}
		if bulkSource(s) {
			bulk = []Word{*dest}
		}
	}
	return files, bulk
}

// optionValues returns the values given to an option: --long=value, --long value, -x value, -xvalue, and the letter
// x at the end of or inside a short cluster such as -sSo.
//
// It is the script proof's own because of its output-letter test: it takes the output letter's value wherever the
// letter stands in a cluster, so every file a download or archive command may write is listed, where a shared option
// table that knows the other letters could read the cluster differently and drop a target.
func optionValues(args []Word, short byte, long string) []Word {
	var out []Word
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !a.Literal {
			continue
		}
		switch {
		case a.Text == long && i+1 < len(args):
			i++
			out = append(out, args[i])
		case strings.HasPrefix(a.Text, long+"="):
			w := a
			w.Text, w.Raw = strings.TrimPrefix(a.Text, long+"="), ""
			out = append(out, w)
		case strings.HasPrefix(a.Text, "-") && !strings.HasPrefix(a.Text, "--"):
			if at := strings.IndexByte(a.Text, short); at > 0 {
				if at == len(a.Text)-1 {
					if i+1 < len(args) {
						i++
						out = append(out, args[i])
					}
				} else {
					w := a
					w.Text, w.Raw = a.Text[at+1:], ""
					out = append(out, w)
				}
			}
		}
	}
	return out
}

// urlNames returns the file a download of each operand creates in the working directory: the last component of the
// URL path.
func urlNames(args []Word) []Word {
	var out []Word
	for _, w := range operandWords(args) {
		t := w.Text
		if i := strings.IndexAny(t, "?#"); i >= 0 {
			t = t[:i]
		}
		if name := t[strings.LastIndexByte(t, '/')+1:]; name != "" && !strings.ContainsAny(name, expandingChars) {
			out = append(out, Word{Text: name, Literal: true})
		}
	}
	return out
}

// inPlaceEdit reports sed, perl and awk invocations that rewrite their file operands.
func inPlaceEdit(exe string, args []Word) bool {
	has := func(flags ...string) bool {
		for _, a := range args {
			for _, f := range flags {
				if a.Text == f || strings.HasPrefix(a.Text, f+"=") {
					return true
				}
			}
		}
		return false
	}
	switch exe {
	case "sed":
		return has("-i", "--in-place") || hasShortFlag(args, 'i')
	case "perl":
		return hasShortFlag(args, 'i')
	case "awk", "gawk":
		return has("-i")
	}
	return false
}

// extractsArchive reports a tar invocation that unpacks: an old-style first word holding x, a short cluster holding x,
// or --extract or --get.
func extractsArchive(args []Word) bool {
	if len(args) > 0 && args[0].Literal && !strings.HasPrefix(args[0].Text, "-") && strings.Contains(args[0].Text, "x") {
		return true
	}
	for _, a := range args {
		if a.Literal && (a.Text == "--extract" || a.Text == "--get") {
			return true
		}
	}
	return hasShortFlag(args, 'x')
}

var dotWord = Word{Text: ".", Literal: true}

func psSelfFile(text string) bool {
	switch strings.ToLower(text) {
	case "$pscommandpath", "${pscommandpath}", "$myinvocation.mycommand.path", "$myinvocation.mycommand.definition", "${myinvocation}.mycommand.path", "${myinvocation}.mycommand.definition":
		return true
	}
	return false
}

// The provider binder understands quoted values and switch parameters. Other file-writing
// cmdlets need their own parameter lists because named values can precede positional paths.
var psOutputParameters = map[string]string{
	"export-csv":        "path literalpath inputobject encoding delimiter quotefields usequotes append force noclobber notypeinformation includetypeinformation useculture noheader",
	"export-clixml":     "path literalpath inputobject encoding depth force noclobber",
	"invoke-webrequest": "uri outfile method headers body contenttype credential timeoutsec useragent infile websession passthru usebasicparsing usedefaultcredentials skiphttperrorcheck",
	"invoke-restmethod": "uri outfile method headers body contenttype credential timeoutsec useragent infile websession passthru usebasicparsing usedefaultcredentials skiphttperrorcheck",
	"expand-archive":    "path literalpath destinationpath force passthru",
}

var psOutputSwitches = map[string]bool{
	"notypeinformation": true, "includetypeinformation": true, "useculture": true, "noheader": true,
	"usebasicparsing": true, "usedefaultcredentials": true, "skiphttperrorcheck": true,
}

func bindPSOutput(exe string, args []Word) (map[string]Word, []Word, bool) {
	if params, ok := psOutputParameters[exe]; ok {
		return bindPSOptions(exe, args, params, psOutputSwitches)
	}
	return bindPSProviderOptions(exe, args)
}

// A list is a single parser word. Re-read its elements before comparing paths so a literal
// comma in a quoted filename cannot be confused with two path operands.
func psTargetElements(ctx context.Context, words []Word) (targets []Word, resolved bool) {
	resolved = true
	for _, w := range words {
		if w.Raw == "" || isPSScalarRaw(w.Raw) {
			targets = append(targets, w)
			continue
		}
		if ctx.Err() != nil {
			return targets, false
		}
		parser := psParser{ctx: ctx, mode: psModeTyped, line: 1}
		elements, ok := parser.providerTargets(w)
		targets = append(targets, elements...)
		resolved = resolved && ok
	}
	return targets, resolved
}

func psJoinTarget(dir, name Word) Word {
	return Word{Text: path.Join(strings.ReplaceAll(dir.Text, `\`, "/"), strings.ReplaceAll(name.Text, `\`, "/")), Literal: dir.Literal && name.Literal}
}

func psWriteTargets(ctx context.Context, exe string, args []Word) (files, dirs []Word) {
	for _, a := range args {
		if !a.Literal && strings.HasPrefix(normalizeLeadingDashes(a.Raw), "-") && !isUnquotedFlagHead(a.Raw) {
			// The run-time parameter name decides which following word is its destination.
			return nil, []Word{dotWord}
		}
	}
	named, rest, ok := bindPSOutput(exe, args)
	if !ok {
		// Unresolved parameter binding leaves the output location unproven.
		return nil, []Word{dotWord}
	}
	firstPath := func(names ...string) Word {
		for _, n := range names {
			if a, ok := named[n]; ok {
				return a
			}
		}
		if len(rest) == 0 {
			return Word{}
		}
		a := rest[0]
		rest = rest[1:]
		return a
	}
	switch exe {
	case "copy-item", "move-item":
		dst, hasDest := named["destination"]
		src, literalSources := named["literalpath"]
		hasSrc := literalSources
		if !literalSources {
			src, hasSrc = named["path"]
		}
		if !hasDest && len(rest) > 0 {
			if hasSrc {
				dst, rest = rest[0], rest[1:]
			} else if len(rest) >= 2 {
				dst, rest = rest[len(rest)-1], rest[:len(rest)-1]
			}
		}
		if dst == (Word{}) {
			dst = dotWord
		}
		if hasSrc {
			rest = append(slices.Clone(rest), src)
		}
		destinations, known := psTargetElements(ctx, []Word{dst})
		if !known || len(destinations) != 1 {
			return nil, []Word{dotWord}
		}
		sources, allKnown := psTargetElements(ctx, rest)
		bulk := !allKnown || len(sources) > 1
		for _, a := range args {
			head, value, _ := strings.Cut(normalizeLeadingDashes(a.Raw), ":")
			if isUnquotedFlagHead(a.Raw) && strings.HasPrefix("-recurse", strings.ToLower(head)) && !strings.EqualFold(value, "$false") {
				bulk = true
			}
		}
		files = append(files, destinations...)
		for _, src := range sources {
			src.Text = strings.ReplaceAll(src.Text, `\`, "/")
			bulk = bulk || directoryCopySource(src.Text) || !src.Literal || !literalSources && strings.ContainsAny(src.Text, "*?[")
			if src.Literal && src.Text != "" {
				name := Word{Text: path.Base(src.Text), Literal: true}
				files = append(files, psJoinTarget(destinations[0], name))
			}
		}
		if bulk {
			dirs = append(dirs, destinations...)
		}
	case "set-content", "add-content", "clear-content", "set-item", "out-file", "tee-object", "export-csv", "export-clixml":
		files = append(files, firstPath("literalpath", "filepath", "path"))
	case "new-item":
		parent := firstPath("path", "literalpath")
		if name, hasName := named["name"]; hasName {
			if parent.Text == "" {
				parent = dotWord
			}
			parents, known := psTargetElements(ctx, []Word{parent})
			if !known {
				return nil, []Word{dotWord}
			}
			for _, dir := range parents {
				files = append(files, psJoinTarget(dir, name))
			}
		} else {
			files = append(files, parent)
		}
	case "rename-item":
		src := firstPath("path", "literalpath")
		files = append(files, src, firstPath("newname"))
	case "invoke-webrequest", "invoke-restmethod":
		if out, ok := named["outfile"]; ok {
			files = append(files, out)
		}
	case "expand-archive":
		firstPath("path", "literalpath")
		dst := firstPath("destinationpath")
		if dst == (Word{}) {
			dst = dotWord
		}
		dirs = append(dirs, dst)
	}
	files, fileOK := psTargetElements(ctx, files)
	dirs, dirOK := psTargetElements(ctx, dirs)
	if !fileOK || !dirOK {
		dirs = append(dirs, dotWord)
	}
	return files, dirs
}

// commandWriteTargets returns the words an invocation writes to through its arguments: files it names, and
// directories it may fill with files it does not name. Redirects are separate.
func commandWriteTargets(ctx context.Context, inv *Invocation) (files, dirs []Word) {
	words := stripBuiltin(inv.Words)
	if len(words) == 0 || !words[0].Literal {
		return nil, nil
	}
	exe := strings.ToLower(ExecutableName(words[0].Text))
	args := words[1:]
	if inv.Dialect == PowerShell && (psProviderOptions[exe] != "" || psOutputParameters[exe] != "") {
		return psWriteTargets(ctx, exe, args)
	}
	switch exe {
	case "tee", "truncate", "touch", "patch":
		return operandWords(args), nil
	case "cp", "mv", "install", "rsync", "ln":
		files, bulk := copyDestinations(args)
		return files, bulk
	case "dd":
		for _, a := range args {
			if rest, ok := strings.CutPrefix(a.Text, "of="); ok {
				w := a
				w.Text, w.Raw = rest, ""
				files = append(files, w)
			}
		}
		return files, nil
	case "curl":
		files = optionValues(args, 'o', "--output")
		for _, a := range args {
			if a.Literal && (a.Text == "--remote-name" || a.Text == "--remote-name-all") {
				return append(files, urlNames(args)...), nil
			}
		}
		if hasShortFlag(args, 'O') {
			files = append(files, urlNames(args)...)
		}
		return files, nil
	case "wget":
		if files = optionValues(args, 'O', "--output-document"); len(files) > 0 {
			return files, nil
		}
		return urlNames(args), nil
	case "sed", "perl", "awk", "gawk":
		if inPlaceEdit(exe, args) {
			return operandWords(args), nil
		}
	case "tar":
		if extractsArchive(args) {
			if dirs = optionValues(args, 'C', "--directory"); len(dirs) == 0 {
				dirs = []Word{dotWord}
			}
		}
	case "unzip":
		for _, a := range args {
			if a.Literal && (a.Text == "-l" || a.Text == "-v" || a.Text == "-t" || a.Text == "-p" || a.Text == "-Z") {
				return nil, nil
			}
		}
		for i, a := range args {
			if a.Literal && a.Text == "-d" && i+1 < len(args) {
				dirs = append(dirs, args[i+1])
			} else if a.Literal && strings.HasPrefix(a.Text, "-d") && len(a.Text) > 2 {
				w := a
				w.Text, w.Raw = a.Text[2:], ""
				dirs = append(dirs, w)
			}
		}
		if len(dirs) == 0 {
			dirs = []Word{dotWord}
		}
	case "git":
		return gitWriteTargetsArgv(args, inv.Dialect)
	}
	return files, dirs
}

// gitWriteTargetsArgv returns the files and directories a git command writes through its arguments, given the words
// after git. The caller has already refused a head word that is not literal.
//
// A line ReadGit reads to its verb names its subcommand from that verb, unless a global option is not literal: its
// run-time text may be the verb, so no target can be named and none is listed. A line ReadGit cannot read, or one
// that prints, keeps the lenient walk of lenientGitVerb, because listing nothing would drop the targets of lines such as
// `git -- checkout .` that git may still run. The operands start after the first literal word equal to the verb,
// which can be an option value, so `git -C checkout checkout .` also lists checkout: starting at the verb itself
// would drop a target.
func gitWriteTargetsArgv(args []Word, dialect Dialect) (files, dirs []Word) {
	g := argv.ReadGit(args)
	var verb Word
	switch {
	case g.State != argv.GitVerb:
		lenient, ok := lenientGitVerb(args)
		if !ok || !lenient.Literal {
			return nil, nil
		}
		verb = lenient
	case gitGlobalExpands(g, dialect, false):
		return nil, nil
	default:
		verb = g.Verb
	}
	switch verb.Text {
	case "checkout", "restore", "rm", "mv":
		if g.State == argv.GitVerb && !gitGlobalExpands(g, dialect, true) && gitIndexOnly(verb.Text, g.Args, dialect) {
			return nil, nil
		}
		var after []Word
		for i, a := range args {
			if a.Literal && a.Text == verb.Text {
				after = args[i+1:]
				break
			}
		}
		files = operandWords(after)
		for _, w := range files {
			if w.Literal && (w.Text == "." || strings.HasSuffix(w.Text, "/")) {
				dirs = append(dirs, w)
			}
		}
	}
	return files, dirs
}

// checkScriptWrites refuses a command that writes to an inspected script. The guard judged the text it read before
// anything ran, and bash reads a script as it runs, so a script that edits itself or another inspected script runs text
// no guard saw. Redirect targets of a script call reach the commands of the script, so `bash x.sh >> x.sh` is covered.
// Typed commands are judged too: one that runs at the same time as the call, before a later pass of a loop, or in a
// substitution of the call itself runs after the guard read the script and before or while the shell reads it. That
// also refuses a typed write after the call, which is accepted.
func (p *parser) checkScriptWrites() {
	if len(p.result.Scripts) == 0 {
		return
	}
	in := p.inspectedPaths()
	for i := range p.result.Invocations {
		inv := &p.result.Invocations[i]
		if !inv.Inspectable {
			continue
		}
		if inv.Script == nil {
			if writesInspected(p.ctx, in, inv) {
				inv.Inspectable, inv.Reason = false, ruleRefusal(RuleWritesInspected, nil)
			}
			continue
		}
		if writesInspected(p.ctx, in, inv) {
			inv.Inspectable = false
			inv.Reason = ruleRefusal(RuleWritesInspected, inv.Script)
		}
	}
}

func writesInspected(ctx context.Context, in inspectedPaths, inv *Invocation) bool {
	in.powerShell = inv.Dialect == PowerShell
	for _, r := range inv.OutputRedirects {
		if in.hits(r.Target) {
			return true
		}
	}
	files, dirs := commandWriteTargets(ctx, inv)
	for _, w := range files {
		if in.hits(w) {
			return true
		}
	}
	for _, w := range dirs {
		if in.hitsDir(w) {
			return true
		}
	}
	return false
}

// gitIndexOnly reports a git command that changes only the index: restore --staged without --worktree, rm --cached.
func gitIndexOnly(verb string, args []Word, dialect Dialect) bool {
	index := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !fixedGitWord(a, dialect) {
			return false
		}
		if a.Text == "--" {
			break
		}
		if !strings.HasPrefix(a.Text, "-") {
			continue
		}
		switch {
		case verb == "restore" && (a.Text == "--staged" || a.Text == "-S"):
			index = true
		case verb == "restore" && (a.Text == "-s" || a.Text == "--source"):
			i++
			if i == len(args) || !fixedGitWord(args[i], dialect) {
				return false
			}
		case verb == "restore" && strings.HasPrefix(a.Text, "--source="):
		case verb == "rm" && a.Text == "--cached":
			index = true
		case verb == "rm" && (a.Text == "--ignore-unmatch" || a.Text == "--quiet" || a.Text == "--force" || len(a.Text) > 1 && strings.Trim(a.Text[1:], "rfqn") == ""):
		default:
			return false
		}
	}
	return index
}
