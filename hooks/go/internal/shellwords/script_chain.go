package shellwords

import (
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/muehz/agentics/hooks/go/internal/argv"
	"github.com/muehz/agentics/hooks/go/internal/pathshape"
)

// writerCommands are the programs a typed command may run before a script call that can rewrite files the call, or
// the script it reads, depends on. The guard read the script before any predecessor ran, so a predecessor that
// replaces it leaves the call running text nobody inspected.
//
// The set is the script proof's own because it is a closed, documented set: it lists the programs this proof treats as
// writers, which no guard classifies, so it does not derive from the shared command vocabulary, and a name joins it
// only with a test of the call it refuses.
var writerCommands = map[string]bool{"tee": true, "cp": true, "mv": true, "install": true, "ln": true, "dd": true, "rsync": true,
	"tar": true, "unzip": true, "truncate": true, "touch": true, "wget": true, "patch": true, "make": true, "npm": true, "npx": true,
	"pnpm": true, "yarn": true}

// writerGitVerbs are the git subcommands that rewrite the working tree.
var writerGitVerbs = map[string]bool{"checkout": true, "switch": true, "restore": true, "reset": true, "pull": true, "merge": true,
	"rebase": true, "stash": true, "apply": true, "am": true, "read-tree": true, "cherry-pick": true, "revert": true, "clean": true}

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
	words := stripBuiltin(inv.Words)
	if len(words) == 0 {
		return RuleNone
	}
	exe := strings.ToLower(ExecutableName(words[0].Text))
	args := words[1:]
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
		// hash -p binds a command name to any file, and enable loads or turns off builtins.
		return RulePredecessorState
	case writerCommands[exe]:
		return RulePredecessorMayWrite
	case exe == "sed" && (has("-i", "--in-place") || hasShortFlag(args, 'i')):
		return RulePredecessorMayWrite
	case exe == "perl" && hasShortFlag(args, 'i'):
		return RulePredecessorMayWrite
	case (exe == "awk" || exe == "gawk") && has("-i"):
		return RulePredecessorMayWrite
	case exe == "curl" && (has("--output") || hasShortFlag(args, 'o') || curlNamesFromURL(args)):
		return RulePredecessorMayWrite
	case exe == "go" && len(args) > 0 && args[0].Text == "generate":
		return RulePredecessorMayWrite
	case exe == "git" && gitWritesTree(args):
		return RulePredecessorMayWrite
	}
	return RuleNone
}

// gitWritesTree reports a git command, given the words after git, whose subcommand rewrites the working tree,
// read with ReadGit. A line ReadGit cannot read, or one that prints, is a writer when words are left after the walk
// stopped, since git may still run one of them as a writing subcommand; with nothing left it runs no subcommand. A
// global option that is not literal is a writer in every state, because its run-time text may be anything, the verb
// included. The verb is compared exactly, and the index-only test reads the whole argument list.
func gitWritesTree(args []Word) bool {
	g := argv.ReadGit(args)
	if gitGlobalExpands(g) {
		return true
	}
	if g.State != argv.GitVerb {
		return len(g.Rest) > 0
	}
	return (!g.Verb.Literal || writerGitVerbs[g.Verb.Text]) && !gitIndexOnly(g.Verb.Text, args)
}

// gitGlobalExpands reports a global option word of g that is not literal.
func gitGlobalExpands(g argv.Git) bool {
	for _, global := range g.Globals {
		if !global.Option.Literal {
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
		return gitWritesTree(args)
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
		inv := &p.result.Invocations[p.predecessorScanned]
		if inv.Script == nil {
			if reason := predecessorProblem(inv); reason != RuleNone {
				return reason
			}
		}
		p.predecessorScanned++
	}
	for p.bareScanned < len(p.result.BareAssignments) {
		// A bare assignment lasts for the rest of the shell, so a pinned name or IFS set before the call changes what the
		// script runs under, as an export of it does.
		if w := p.result.BareAssignments[p.bareScanned]; stateVariables[variableName(w.Text)] || hiddenName(w) || namedVariable(variableName(w.Text)) != constructNone {
			return RulePredecessorState
		}
		p.bareScanned++
	}
	return RuleNone
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
	host    pathshape.Host
	keys    []string
	set     map[string]bool
	dirs    []string
	dirSet  map[string]bool
	anyPath bool
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
		if !w.Literal && selfReference(text) {
			return true
		}
		tail := text[strings.LastIndexByte(text, '/')+1:]
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
	t := w.Text
	return strings.ContainsAny(t, "*?[") || strings.HasSuffix(t, "/") || strings.HasSuffix(t, "/.") || t == "." || t == ".."
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

// commandWriteTargets returns the words an invocation writes to through its arguments: files it names, and
// directories it may fill with files it does not name. Redirects are separate.
func commandWriteTargets(inv *Invocation) (files, dirs []Word) {
	words := stripBuiltin(inv.Words)
	if len(words) == 0 || !words[0].Literal {
		return nil, nil
	}
	exe := strings.ToLower(ExecutableName(words[0].Text))
	args := words[1:]
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
		return gitWriteTargetsArgv(args)
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
func gitWriteTargetsArgv(args []Word) (files, dirs []Word) {
	g := argv.ReadGit(args)
	var verb Word
	switch {
	case g.State != argv.GitVerb:
		lenient, ok := lenientGitVerb(args)
		if !ok || !lenient.Literal {
			return nil, nil
		}
		verb = lenient
	case gitGlobalExpands(g):
		return nil, nil
	default:
		verb = g.Verb
	}
	switch verb.Text {
	case "checkout", "restore", "rm", "mv":
		if gitIndexOnly(verb.Text, args) {
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
			if writesInspected(in, inv) {
				inv.Inspectable, inv.Reason = false, ruleRefusal(RuleWritesInspected, nil)
			}
			continue
		}
		if writesInspected(in, inv) {
			inv.Inspectable = false
			inv.Reason = ruleRefusal(RuleWritesInspected, inv.Script)
		}
	}
}

func writesInspected(in inspectedPaths, inv *Invocation) bool {
	for _, r := range inv.OutputRedirects {
		if in.hits(r.Target) {
			return true
		}
	}
	files, dirs := commandWriteTargets(inv)
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
func gitIndexOnly(verb string, args []Word) bool {
	has := func(names ...string) bool {
		for _, a := range args {
			for _, n := range names {
				if a.Literal && a.Text == n {
					return true
				}
			}
		}
		return false
	}
	switch verb {
	case "restore":
		return has("--staged", "-S") && !has("--worktree", "-W")
	case "rm":
		return has("--cached")
	}
	return false
}
