package shellwords

import (
	"path"
	"strings"
	"time"
	"unicode"

	"github.com/muehz/agentics/hooks/go/internal/pathshape"
)

// IsAbsoluteShellPath reports whether p names a directory without a working directory to resolve it against, by the
// rule the cwd proof applies: a POSIX absolute path, which includes //server/share, or a Windows drive path (C:/x or
// C:\x), on every host. It is lexical, so a Windows host accepts the /ws spelling the shell uses.
func IsAbsoluteShellPath(p string) bool { return isAbsolutePath(p) }

// isAbsolutePath accepts POSIX absolute paths and Windows drive paths (C:/x or C:\x).
func isAbsolutePath(p string) bool {
	if path.IsAbs(p) {
		return true
	}
	return pathshape.HasDrive(p) && len(p) >= 3 && (p[2] == '/' || p[2] == '\\')
}

// slashPath cleans p with forward slashes. A drive path is cleaned below its drive, so .. cannot climb past the
// drive root.
func slashPath(p string) string {
	if pathshape.HasDrive(p) {
		rest := strings.ReplaceAll(p[2:], "\\", "/")
		if strings.HasPrefix(rest, "/") {
			return p[:2] + path.Clean(rest)
		}
	}
	return path.Clean(p)
}

// cwdPath is a directory being replayed: a root, "/" or a drive such as "C:/", and the components below it.
type cwdPath struct {
	root  string
	parts []string
	// host is the host whose path rules the walk follows, taken from the parser state.
	host pathshape.Host
	// mayFail marks each component added by a cd that may have failed while the call still ran. A failed cd leaves
	// the directory alone, so a later .. over such a component climbs from somewhere else: `cd nope; cd ..` ends one
	// level above the start, not at it. A component that is never climbed over is safe: a call path below it can
	// only be read if the directory exists, and then the cd did not fail.
	mayFail []bool
	// moved is set once a cd has changed the directory. The replay holds a cd's directory by its spelling, while a
	// shell on a host with real symlinks holds the real directory, so a .. taken from a directory a cd named may
	// land elsewhere than the replay says. Only the provider's own directory is known to be real.
	moved bool
}

// startCwdPath starts a directory at the absolute path abs, walked as walk does.
func startCwdPath(host pathshape.Host, abs string, mayFail, rootStays bool) (cwdPath, bool) {
	abs = strings.ReplaceAll(abs, "\\", "/")
	c := cwdPath{root: "/", host: host}
	if pathshape.HasDrive(abs) {
		c.root, abs = abs[:2]+"/", abs[2:]
	}
	ok := c.walk(abs, mayFail, rootStays)
	return c, ok
}

// walk applies the relative path rel and returns false when the directory it lands in cannot be known. rootStays
// says whether .. at a drive root stays there. It does in PowerShell and inside one C:/-spelled operand, but Git Bash
// holds a directory it entered as /c/..., and its /c/.. is the MSYS root, which the guard cannot name. "/" is its own
// parent everywhere.
//
// Off Windows a .. is only followed over components of the provider's own directory, before any cd: a component named
// by a cd, or added earlier in the same walk, may be a symlink, and the shell would climb out of its target.
func (c *cwdPath) walk(rel string, mayFail, rootStays bool) bool {
	floor := len(c.parts)
	for _, part := range strings.Split(strings.ReplaceAll(rel, "\\", "/"), "/") {
		switch part {
		case "", ".":
		case "..":
			n := len(c.parts)
			if n == 0 {
				if c.root != "/" && !rootStays {
					return false
				}
				continue
			}
			if !c.host.Windows() && (c.moved || n > floor) {
				return false
			}
			if c.mayFail[n-1] {
				return false
			}
			c.parts, c.mayFail = c.parts[:n-1], c.mayFail[:n-1]
		default:
			c.parts = append(c.parts, part)
			c.mayFail = append(c.mayFail, mayFail)
		}
	}
	return true
}

func (c cwdPath) String() string { return c.root + strings.Join(c.parts, "/") }

// literalPathWord rejects words the shell would expand: a leading ~, $, `, globs and braces. A ~ inside a word
// (a Windows short name such as RUNNER~1) is literal.
func literalPathWord(w Word) bool {
	return w.Literal && w.Text != "" && !strings.ContainsAny(w.Text, "*?[{}$`") && !strings.HasPrefix(w.Text, "~")
}

// compoundKeywords open or continue a compound command. A cd that may sit inside one cannot prove cwd.
var compoundKeywords = map[string]bool{"if": true, "then": true, "else": true, "elif": true, "fi": true,
	"while": true, "until": true, "for": true, "do": true, "done": true, "case": true, "esac": true,
	"select": true, "function": true, "{": true, "}": true}

// isCD reports whether exe changes the directory. chdir, sl and Set-Location are PowerShell commands only: bash
// has no such builtins, so there they are ordinary programs and leave the directory alone.
func isCD(exe string, dialect Dialect) bool {
	if exe == "cd" {
		return true
	}
	return dialect == PowerShell && (exe == "chdir" || exe == "set-location" || exe == "sl")
}

// isDirectoryStack reports whether exe pushes or pops the directory stack, which the guard does not follow.
func isDirectoryStack(exe string, dialect Dialect) bool {
	if exe == "pushd" || exe == "popd" {
		return true
	}
	return dialect == PowerShell && (exe == "push-location" || exe == "pop-location")
}

var (
	compoundWords = map[string]bool{"if": true, "then": true, "elif": true, "else": true, "do": true, "while": true,
		"until": true, "for": true, "case": true, "select": true, "function": true}
	directoryChangeWords = map[string]bool{"cd": true, "pushd": true, "popd": true, "chdir": true, "sl": true,
		"set-location": true, "push-location": true, "pop-location": true}
)

// expandingChars are the characters that let the shell turn a word into another one at run time: parameter and
// command substitution, ANSI-C quoting ($'...'), globs and brace expansion. A command word holding one may become cd.
// A } without its { expands nothing.
const expandingChars = "$`*?[{"

// compoundWithDirectoryChange reports whether text holds a compound-command keyword and something that may change the
// directory: a directory-change word of its own, or any character that lets a word expand at run time. The check is
// textual and conservative: quotes and backslashes are dropped first, so c\d and "cd" count, words compare in lower
// case in either dialect, and a word inside a quoted string counts too.
//
// The expansion rule over-refuses on purpose. A cd built at run time, such as $C, c$'d', {cd,} or c? once a file
// named cd exists, cannot be told apart from a harmless variable or glob by its text, and later in a loop body it
// moves the next pass of a call before it. So a list with a compound keyword and any variable, substitution, glob
// or brace proves no working directory, even when none of them could ever be a cd.
func compoundWithDirectoryChange(text string) bool {
	text = strings.Map(func(r rune) rune {
		if r == '\\' || r == '"' || r == '\'' {
			return -1
		}
		return r
	}, text)
	compound, change := false, strings.ContainsAny(text, expandingChars)
	for _, w := range strings.FieldsFunc(text, func(r rune) bool {
		return unicode.IsSpace(r) || strings.ContainsRune(";&|()<>{}`$", r)
	}) {
		w = strings.ToLower(w)
		compound = compound || compoundWords[w]
		change = change || directoryChangeWords[w]
	}
	return compound && change
}

// shellName is the name the shell looks w up by when it is a command word. bash finds builtins by their exact,
// case-sensitive name after quote removal, never with a directory: CD, Command and /usr/bin/cd are other programs.
// PowerShell matches its commands in any case.
func shellName(w Word, dialect Dialect) string {
	if dialect == PowerShell {
		return strings.ToLower(w.Text)
	}
	return w.Text
}

// runsInList reports whether inv, produced by a segment of a list whose own wrappers number depth, ran in that
// list's shell. Extra wrappers decide it. In bash the command builtin runs its command in the same shell, and so
// does the time keyword, which is a keyword only unquoted and unescaped in the run of words that opens a command:
// `time time cd` keeps both, while `"time" cd`, `\time cd` and `command time cd` run an external time. env, sudo,
// nohup, bash, xargs and the rest start another process, so a directory change stays there. unknown is true for
// exec, which replaces the shell and so cannot be followed.
func runsInList(inv Invocation, depth int) (same, unknown bool) {
	opening := true
	for _, w := range inv.Wrappers[min(depth, len(inv.Wrappers)):] {
		name := shellName(w, inv.Dialect)
		switch {
		case strings.EqualFold(ExecutableName(w.Text), "exec"):
			return false, true
		case inv.Dialect != POSIX:
			return false, false // PowerShell has neither a command builtin nor a time keyword
		case name == "time" && w.Raw == "time" && opening:
		case name == "command":
			opening = false
		default:
			return false, false
		}
	}
	return true, false
}

// cwdBefore returns the working directory in effect for the script call being parsed, or false when it cannot be
// proven. byCD is true when the failure came from a directory change the guard cannot follow; that is decided even
// when no working directory is known, because an absolute path after such a change still runs in shell state the
// guard cannot vouch for.
//
// The replay walks p.lists, the call's own list and every list that encloses it. A shell starts in the directory
// of the shell that started it, so the cds of an enclosing list count up to the command that started the next
// list, and those of the call's list up to the call. A list's separators are read from its segment marks, never
// from the invocations, because a segment that runs nothing in this shell leaves no invocation behind but still
// separates the commands around it.
//
// A directory the shell may have reached by another route, such as a .. after a cd that may have failed, is not an
// unprovable cd: the replay continues without a directory, so a later unprovable cd still sets byCD.
//
// eval, source and . run text in this shell that the replay does not walk, so each is an unprovable cd for a call
// after it: a sourced file that was expanded leaves a sourcedFile mark on its segment, and any other source, ., or eval
// is caught by its command word, whether it was refused or is still inspectable (a script's children are labeled
// opaque only after the whole script is parsed). A call inside a sourced file is still proven: the source segment is
// the one that started its list. Of the other refused commands, only a refused sh or bash script call is known not to
// be a cd the parser could not take apart, such as `command -p cd sub`; a reason that merely starts with script- is
// not enough.
//
// The replay does not model these, so a proof can still be wrong when one is in play:
//   - CDPATH, inherited from the environment or set by a bare CDPATH= assignment earlier in the list, sends a relative
//     cd somewhere else. Only a CDPATH prefix on the cd itself is refused.
//   - set -P makes cd resolve symlinked components physically, while the replay cleans .. lexically.
//   - A trap on DEBUG runs code, possibly a cd, before every command.
//   - With aliases expanded (shopt -s expand_aliases), any command word may be an alias for cd.
//   - A drive path such as C:/x counts as absolute on every host, though on Linux it is a relative path.
func (p *parser) cwdBefore() (dir cwdPath, ok bool, byCD bool) {
	// A relative event cwd proves nothing: no directory is known, though the replay still runs.
	haveDir := false
	cwd := p.script.opts.Cwd
	if p.dialect != PowerShell {
		cwd = p.script.host.Native(cwd)
	}
	if cwd = slashPath(cwd); isAbsolutePath(cwd) {
		dir, haveDir = startCwdPath(p.script.host, cwd, false, true)
	}
	sawCD, grouped := false, false
	for _, list := range p.lists {
		if list.compoundCD {
			return cwdPath{}, false, true
		}
		grouped = grouped || list.grouped
		depth := list.depth
		// listCD is set once a cd of this list ran. A later & or | then shows that the cd may have run in a
		// background or pipeline subshell; the separator sits on the segment after the cd, whatever that segment is.
		listCD := false
		// condCD is set once a cd ran only if the command before it succeeded. From then on every segment of the list
		// must be reached through the same && chain, or the cd may have been skipped while the call still runs.
		condCD := false
		for j, seg := range list.segments {
			if listCD && (seg.before == Background || seg.before == Pipe) {
				return cwdPath{}, false, true
			}
			if condCD && seg.before != And {
				return cwdPath{}, false, true
			}
			if seg.end < 0 {
				break // the call itself, or the command that started the next list
			}
			if seg.bracedWord {
				// The parser split a brace-expanded command word such as {cd,} into a group, so the command it
				// expands to, possibly a cd, is not among the invocations.
				return cwdPath{}, false, true
			}
			if seg.sourcedFile {
				// The sourced text ran in this shell; its cds sit in a list that is no longer there to replay.
				return cwdPath{}, false, true
			}
			for _, inv := range p.result.Invocations[seg.first:seg.end] {
				if len(inv.Words) == 0 {
					continue
				}
				same, unknown := runsInList(inv, depth)
				if unknown {
					return cwdPath{}, false, true
				}
				if !same {
					continue
				}
				words := inv.Words
				if shellName(words[0], inv.Dialect) == "builtin" && len(words) > 1 {
					words = words[1:]
					// builtin command cd, builtin builtin cd and builtin -- cd all run cd; only a plain cd is followed.
					if n := shellName(words[0], inv.Dialect); n == "command" || n == "builtin" || strings.HasPrefix(n, "-") {
						return cwdPath{}, false, true
					}
				}
				exe := strings.ToLower(ExecutableName(words[0].Text))
				switch {
				case exe == "source" || exe == "." || exe == "eval":
					// Text run in this shell may cd. A literal source that was expanded is caught by its segment mark;
					// this catches the rest, refused or not yet labeled opaque, since the script being parsed is labeled
					// only once all of it is parsed.
					return cwdPath{}, false, true
				case !inv.Inspectable && !refusedScriptCall(&inv):
					// A command the parser could not take apart, such as `command -p cd sub`, may still be a cd of this
					// shell. A refused sh or bash script call is not: it runs in a shell of its own. (A refused source
					// or . was caught above.)
					return cwdPath{}, false, true
				case inv.Dialect == PowerShell && len(words) > 1 && strings.HasPrefix(asciiLower(words[0].Text), "$env:") && strings.HasSuffix(words[1].Text, "="):
					continue
				case !words[0].Literal || strings.ContainsAny(words[0].Text, expandingChars):
					// A command word that expands at run time may be a cd: $C with C=cd, and also words the lexer keeps
					// literal, such as {cd,} or c? and c[d] once a file named cd exists.
					return cwdPath{}, false, true
				case compoundKeywords[exe], isDirectoryStack(exe, inv.Dialect):
					return cwdPath{}, false, true
				case isCD(shellName(words[0], inv.Dialect), inv.Dialect):
					listCD, sawCD = true, true
					if len(inv.Assignments) > 0 || seg.before == Or || seg.before == Pipe || seg.foreignPrefix {
						return cwdPath{}, false, true
					}
					condCD = condCD || seg.before == And
					target, good := cdOperand(words[1:])
					if !good {
						return cwdPath{}, false, true
					}
					if haveDir {
						haveDir = applyCD(&dir, target, mayFailUnnoticed(list.segments[j+1:]), inv.Dialect == PowerShell)
					}
				}
			}
		}
	}
	if sawCD && grouped {
		// A cd inside ( ), $( ), backticks or a function body does not change the directory of the shell that follows,
		// and a call inside a substitution or group runs in a subshell that the flat replay cannot place. The lexer
		// marks neither beyond the first segment, so the text is the only evidence.
		return cwdPath{}, false, true
	}
	return dir, haveDir, false
}

// mayFailUnnoticed reports whether the call can still run after a cd followed by the segments after it, up to the
// call or the command that started the call's shell, fails. Only an unbroken && chain stops it.
func mayFailUnnoticed(after []segmentMark) bool {
	for _, seg := range after {
		if seg.before != And {
			return true
		}
	}
	return false
}

// applyCD moves dir to the literal cd operand and returns false when the result cannot be known.
func applyCD(dir *cwdPath, operand string, mayFail, inPowerShell bool) bool {
	ok := applyCDOperand(dir, operand, mayFail, inPowerShell)
	dir.moved = true
	return ok
}

func applyCDOperand(dir *cwdPath, operand string, mayFail, inPowerShell bool) bool {
	target := dir.host.Native(operand)
	if inPowerShell {
		var rule Rule
		target, rule = shellOperandPath(dir.host, operand, true, *dir, true)
		if rule != RuleNone {
			return false
		}
	}
	if !isAbsolutePath(target) {
		return dir.walk(target, mayFail, inPowerShell)
	}
	// .. inside one C:/-spelled operand stays at the drive root, while a /c/ spelling climbs to the MSYS root.
	next, ok := startCwdPath(dir.host, target, mayFail, inPowerShell || pathshape.HasDrive(operand))
	*dir = next
	return ok
}

// cdOperand returns the one literal, non-option operand of a cd as written.
func cdOperand(operands []Word) (string, bool) {
	if len(operands) != 1 || !literalPathWord(operands[0]) || strings.HasPrefix(operands[0].Text, "-") {
		return "", false
	}
	return operands[0].Text, true
}

// leadingParentAfterCD reports a word that starts with .. once a cd has moved the working directory, off Windows.
func leadingParentAfterCD(host pathshape.Host, text string, dir cwdPath) bool {
	if host.Windows() || !dir.moved {
		return false
	}
	for _, part := range strings.Split(text, "/") {
		switch part {
		case "", ".":
		case "..":
			return true
		default:
			return false
		}
	}
	return false
}

func posixShellName(exe string) bool {
	switch exe {
	case "sh", "bash", "dash", "ksh":
		return true
	}
	return false
}

func localPathHead(s string) bool { return strings.ContainsAny(s, "/\\") || pathshape.HasDrive(s) }

func scriptExt(s string) string { return strings.ToLower(path.Ext(strings.ReplaceAll(s, "\\", "/"))) }

func scriptSuffix(s string) bool {
	switch scriptExt(s) {
	case ".sh", ".bash", ".dash", ".ksh", ".ps1", ".zsh", ".fish", ".csh", ".tcsh", ".cmd", ".bat":
		return true
	}
	return false
}

func posixPathAppend(w Word) bool {
	return strings.HasPrefix(w.Text, "PATH+=") && strings.HasPrefix(w.Raw, "PATH+=")
}

func (p *parser) badSearchPath(w Word, dialect Dialect) bool {
	name, value, ok := strings.Cut(w.Text, "=")
	if !ok || (name != "PATH" && !(dialect == POSIX && posixPathAppend(w))) {
		return false
	}
	sep := ":"
	if dialect == PowerShell && p.script.host.Windows() {
		sep = ";"
	}
	if dialect == PowerShell && !w.Literal && strings.EqualFold(value, "$env:PATH") {
		return false
	}
	if dialect == POSIX && strings.HasPrefix(w.Raw, "PATH+=") {
		if value == "" {
			return false
		}
		// A leading separator terminates the existing search list. Without
		// it the first appended characters change an unknown existing entry.
		if !strings.HasPrefix(value, sep) {
			return true
		}
		value = value[len(sep):]
	}
	for _, part := range strings.Split(value, sep) {
		if dialect == POSIX && !w.Literal && !strings.ContainsAny(w.Raw, "'\\\\") && (part == "$PATH" || part == "${PATH}") {
			continue
		}
		if dialect == PowerShell && !w.Literal && strings.EqualFold(part, "$env:PATH") {
			continue
		}
		if !isAbsolutePath(part) || strings.ContainsAny(part, "$\x60()*?[]{}") {
			return true
		}
	}
	return false
}

func (p *parser) refuse(inv Invocation, r Refusal) bool {
	inv.Inspectable, inv.Reason = false, r.locatedAt(inv.Script)
	p.result.Invocations = append(p.result.Invocations, inv)
	return true
}

func (p *parser) resolveModuleCommand(inv *Invocation, meta *psInvocationMeta) Rule {
	raw := inv.Words[0]
	raw.Text = meta.RawCommand
	dir, ok, _ := p.cwdBefore()
	if ok {
		resolved, rule := resolveScriptPath(p.script.host, raw, dir, ok, PowerShell)
		if rule != RuleNone {
			return rule
		}
		suffix, found, rule := p.probeScript(resolved, []string{"", ".ps1", ".bat", ".cmd"})
		if rule != RuleNone {
			return rule
		}
		if found {
			raw.Text += suffix
			inv.Words = append([]Word{raw}, inv.Words[1:]...)
			meta.Opaque = psOpaqueNone
			meta.ModuleStripped = false
		}
	}

	return RuleNone
}

func (p *parser) dispatchLocal(inv Invocation, meta psInvocationMeta, depth int) bool {
	head := inv.Words[0]
	exe := strings.ToLower(ExecutableName(head.Text))

	if inv.Dialect == PowerShell && head.Literal && localPathHead(head.Text) {
		// A local denied extension is classified after path and cwd safety.
		switch scriptExt(head.Text) {
		case ".bat", ".cmd":
			if meta.Opaque == psOpaqueBatchFile {
				meta.Opaque = psOpaqueNone
			}
		case ".zsh", ".fish", ".csh", ".tcsh":
			if meta.Opaque == psOpaqueOtherShell {
				meta.Opaque = psOpaqueNone
			}
		}
	}
	if meta.Opaque != psOpaqueNone {
		if !p.script.opts.Context.AllowsNewProof() && (exe == "pwsh" || exe == "powershell") {
			if _, file, _ := parsePowerShellScriptForm(inv.Words); file {
				return p.refuse(inv, ruleRefusal(RuleContextUnqualified, nil))
			}
		}
		return p.refuse(inv, opaqueRefusal(powerShellConstruct(meta.Opaque), nil))
	}
	source := inv.Dialect == POSIX && (head.Text == "source" || head.Text == ".")
	form := scriptForm{path: head, args: inv.Words[1:], caller: inv.Dialect, sourced: meta.CallKind == psCallDot}
	if source {
		list := len(p.lists) - 1
		if !head.Literal || list < 0 || len(inv.Wrappers) != p.lists[list].depth || len(inv.Words) < 2 || !inv.Words[1].Literal || !strings.Contains(inv.Words[1].Text, "/") {
			return p.refuse(inv, opaqueRefusal(constructSource, nil))
		}
		form.path, form.args, form.sourced = inv.Words[1], inv.Words[2:], true
		// Bash rebinds the arguments when given an explicit list. Other POSIX
		// interpreters may ignore the list and retain the caller's parameters.
		if len(form.args) > 0 && len(inv.Wrappers) > 0 {
			active := inv.Wrappers[len(inv.Wrappers)-1]
			form.sourceArgsBound = active.Literal && active.Text == "bash"
		}
		if strings.HasPrefix(form.path.Text, "/dev/fd/") || strings.HasPrefix(form.path.Text, "/proc/self/fd/") || form.path.Text == "/dev/stdin" {
			return p.refuse(inv, opaqueRefusal(constructSource, nil))
		}
		p.dispatchFile(inv, form, nil, Refusal{}, depth)
		return true
	}
	if inv.Dialect == POSIX && len(p.script.chain) > 0 && strings.Contains(head.Text, "=") && variableConstruct(head) != constructNone {
		return false
	}
	if !head.Literal {
		if inv.Dialect == POSIX || localPathHead(head.Text) || scriptSuffix(head.Text) {
			return p.refuse(inv, opaqueRefusal(constructCommandPositionExpansion, nil))
		}
		return false
	}
	local := localPathHead(head.Text)
	if !local && len(p.script.chain) == 0 {
		unsafe := p.unsafeSearchPath
		for _, group := range [][]Word{inv.Assignments, p.script.callAssignments} {
			for _, w := range group {
				unsafe = unsafe || p.badSearchPath(w, inv.Dialect)
			}
		}
		if unsafe {
			// A script call applies its own prefix and predecessor checks in their established order.
			_, file, _ := shellFileForm(exe, inv.Words)
			if !file {
				return p.refuse(inv, ruleRefusal(RuleEnvPinned, nil))
			}
		}
	}
	if inv.Dialect == POSIX && !local {
		command := stripBuiltin(inv.Words)
		if len(command) > 0 && command[0].Literal && len(command) < len(inv.Words) {
			switch command[0].Text {
			case ".", "source":
				return p.refuse(inv, opaqueRefusal(constructSource, nil))
			case "eval":
				return p.refuse(inv, opaqueRefusal(constructEval, nil))
			}
		}
		if list := len(p.lists) - 1; len(p.script.chain) == 0 && list >= 0 {
			if same, _ := runsInList(inv, p.lists[list].depth); same {
				p.observePOSIXSearchPath(command)
			}
		}
		if head.Text == "eval" {
			return p.refuse(inv, opaqueRefusal(constructEval, nil))
		}
	}
	if inv.Dialect == POSIX {
		switch {
		case exe == "zsh" || exe == "fish" || exe == "csh" || exe == "tcsh":
			return p.refuse(inv, opaqueRefusal(constructOtherShell, nil))
		case exe == "cmd" && cmdTargetsBatchFile(inv.Words[1:]):
			return p.refuse(inv, opaqueRefusal(constructBatchFile, nil))
		}
	}
	if posixShellName(exe) || exe == "pwsh" || exe == "powershell" || exe == "cmd" || meta.ModuleStripped {
		return false
	}
	if !local && !scriptSuffix(head.Text) {
		return false
	}
	ext := scriptExt(head.Text)
	if local && ext == ".exe" && literalPathWord(head) {
		return false
	}
	if !local && ext != ".bat" && ext != ".cmd" {
		return p.refuse(inv, ruleRefusal(RulePathNotLiteral, nil))
	}
	dir, ok, _ := p.cwdBefore()
	resolved, rule := resolveScriptPath(p.script.host, head, dir, ok, inv.Dialect)
	if rule != RuleNone {
		return p.refuse(inv, ruleRefusal(rule, nil))
	}
	if inv.Dialect == PowerShell && ext == "" {
		suffix, found, rule := p.probeScript(resolved, []string{".ps1", ".bat", ".cmd"})
		if rule != RuleNone {
			return p.refuse(inv, ruleRefusal(rule, nil))
		}
		if found {
			ext = suffix
			form.path.Text += suffix
		}
	}
	switch ext {
	case ".bat", ".cmd":
		return p.refuse(inv, opaqueRefusal(constructBatchFile, nil))
	case ".zsh", ".fish", ".tcsh", ".csh":
		return p.refuse(inv, opaqueRefusal(constructOtherShell, nil))
	}
	form.direct = true
	if inv.Dialect == PowerShell && ext == ".ps1" {
		form.dialect = PowerShell
	} else {
		if p.script.deadline.Before(time.Now()) {
			return p.refuse(inv, ruleRefusal(RuleBudget, nil))
		}
		reader := p.script.opts.Reader
		var read ScriptRead
		if classifier, ok := reader.(ScriptClassifier); ok {
			read = classifier.InspectScript(resolved)
		} else {
			read = reader.ReadScript(resolved)
			form.read = &read
		}
		dialect, inspect, reason := p.classifyDirect(read, head.Text, inv.Dialect)
		if reason.Refused() {
			return p.refuse(inv, reason)
		}
		if !inspect {
			return false
		}
		form.dialect = dialect
	}
	var shell *Word
	if !form.sourced {
		shell = &head
	}
	p.dispatchFile(inv, form, shell, Refusal{}, depth)
	return true
}

func (p *parser) observePOSIXSearchPath(words []Word) {
	for len(words) > 1 && words[0].Literal && words[0].Text == "command" {
		words = stripBuiltin(readCommand(p.ctx, "command", words, POSIX, 0).child)
	}
	if len(words) < 2 || !words[0].Literal {
		return
	}
	switch words[0].Text {
	case "export", "declare", "readonly", "typeset":
		for _, w := range words[1:] {
			p.unsafeSearchPath = p.unsafeSearchPath || p.badSearchPath(w, POSIX)
		}
	case "unset":
		for _, w := range words[1:] {
			p.unsafeSearchPath = p.unsafeSearchPath || w.Text == "PATH"
		}
	}
}

func (p *parser) probeScript(path string, suffixes []string) (string, bool, Rule) {
	prober, ok := p.script.opts.Reader.(ScriptProber)
	if !ok {
		return "", false, RuleUnreadable
	}
	for _, s := range suffixes {
		exists, rule := prober.ProbeExists(path + s)
		if rule != RuleNone || exists {
			return s, exists, rule
		}
	}
	return "", false, RuleNone
}

func (p *parser) classifyDirect(read ScriptRead, name string, caller Dialect) (Dialect, bool, Refusal) {
	deny := func(r Rule) (Dialect, bool, Refusal) { return POSIX, false, ruleRefusal(r, nil) }
	if read.NotExist && !scriptSuffix(name) {
		if len(p.script.chain) == 0 {
			return deny(p.predecessorReason())
		}
		return POSIX, false, Refusal{}
	}
	if read.Limit != RuleNone && !(read.IsSymlink && read.HasResolvedHeader && read.Limit == RuleUnreadable) && !(read.TooLarge && read.Limit == RuleTooLarge) {
		return deny(read.Limit)
	}
	header := read.Header
	if read.IsSymlink {
		header = read.ResolvedHeader
	}
	kind := ClassifyShebang(header.Shebang)
	binary := header.BinaryMagic || caller == POSIX && kind == ShebangNone && header.HasNUL
	if read.IsSymlink {
		if read.HasResolvedHeader && (binary || kind == ShebangNonShell) {
			return POSIX, false, Refusal{}
		}
		return deny(RuleUnreadable)
	}
	if path.Base(strings.ReplaceAll(name, "\\", "/")) == "agx" && read.SHA256 == AgxLauncherSHA256 || binary || kind == ShebangNonShell {
		return POSIX, false, Refusal{}
	}
	switch kind {
	case ShebangBatch:
		return POSIX, false, opaqueRefusal(constructBatchFile, nil)
	case ShebangOtherShell, ShebangUnparseable:
		return POSIX, false, opaqueRefusal(constructOtherShell, nil)
	case ShebangPowerShell, ShebangPOSIX:
		if !safeShellShebang(header.Shebang, kind) {
			return deny(RuleFormUnsupportedOption)
		}
	case ShebangNone:
		if caller == PowerShell && !posixShellName(strings.TrimPrefix(scriptExt(name), ".")) {
			return deny(RuleExtension)
		}
	}
	if read.TooLarge {
		return deny(RuleTooLarge)
	}
	if kind == ShebangPowerShell {
		return PowerShell, true, Refusal{}
	}
	return POSIX, true, Refusal{}
}

func safeShellShebang(line string, kind ShebangKind) bool {
	words := strings.Fields(strings.TrimPrefix(line, "#!"))
	i := 1
	if strings.EqualFold(ExecutableName(words[0]), "env") {
		for ; i < len(words); i++ {
			w := words[i]
			switch {
			case w == "-S" || w == "-v" || w == "--":
				continue
			case w == "-i":
				return false
			case w == "-u":
				i++
				if i >= len(words) || isPSPinnedEnv(words[i]) || namedVariable(words[i]) != constructNone {
					return false
				}
				continue
			case strings.Contains(w, "="):
				name, _, _ := strings.Cut(w, "=")
				if isPSPinnedEnv(name) || namedVariable(name) != constructNone {
					return false
				}
				continue
			}
			break
		}
		i++
	}
	if kind == ShebangPOSIX {
		for _, w := range words[i:] {
			if !scriptBashOptions[w] {
				return false
			}
		}
		return true
	}
	args := []Word{{Text: "pwsh", Literal: true}}
	for _, w := range words[i:] {
		args = append(args, Word{Text: w, Raw: w, Literal: true})
	}
	args = append(args, Word{Text: "-File", Raw: "-File", Literal: true})
	return powerShellEntry(args) == len(args)-1
}

// shellOperandPath applies the caller's path syntax before any lexical
// cleanup. Windows PowerShell resolves a rooted path on the current drive.
func shellOperandPath(host pathshape.Host, operand string, powershell bool, dir cwdPath, dirOK bool) (string, Rule) {
	text := operand
	if powershell && (host.Windows() || !strings.HasPrefix(text, "\\")) {
		text = strings.ReplaceAll(text, "\\", "/")
	}
	if pathshape.HasDrive(text) && !isAbsolutePath(text) || host.ChangedByCleaning(text) {
		return "", RulePathNotLiteral
	}
	if powershell && host.Windows() {
		if strings.HasPrefix(text, "/") {
			if !dirOK || !pathshape.HasDrive(dir.root) {
				return "", RuleCwdUnproven
			}
			text = dir.root[:2] + text
		}
		return text, RuleNone
	}
	return host.Native(text), RuleNone
}

// resolveScriptPath checks the spelling before resolving it against the caller's proven working directory.
func resolveScriptPath(host pathshape.Host, w Word, dir cwdPath, dirOK bool, dialect Dialect) (string, Rule) {
	if !literalPathWord(w) {
		return "", RulePathNotLiteral
	}
	text, reason := shellOperandPath(host, w.Text, dialect == PowerShell, dir, dirOK)
	if reason != RuleNone {
		return "", reason
	}
	if leadingParentAfterCD(host, text, dir) {
		return "", RulePathNotLiteral
	}
	if isAbsolutePath(text) {
		return slashPath(text), RuleNone
	}
	// PowerShell stays at a drive root, while Git Bash may climb to its MSYS root.
	if !dirOK || !dir.walk(text, false, dialect == PowerShell) {
		return "", RuleCwdUnproven
	}
	return dir.String(), RuleNone
}
