package shellwords

import (
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"
)

// ScriptRead is one definitive read of a script file. Text is valid UTF-8 without CR or NUL.
// Limit is empty on success and otherwise a named limit such as script-too-large; Detail never holds file content.
type ScriptRead struct {
	// Path is the first path that reached the file: an alias (hardlink, 8.3 name, case variant) returns the first
	// read, which is what lets a path-based cycle check catch aliases. Among concurrent guard goroutines the
	// spelling shown can vary from run to run, while all three guards agree on one read within a request.
	Path   string
	ID     string // stable file identity; equal for hardlink aliases, "" on a refusal or when the filesystem gives none
	Text   string
	SHA256 string
	Bytes  int
	Limit  string
	Detail string
}

// ScriptReader reads a resolved script path as data. Implementations must never execute it.
type ScriptReader interface{ ReadScript(path string) ScriptRead }

// ScriptOptions enables script-call expansion. The zero value disables it, so ParseContext is unchanged.
type ScriptOptions struct {
	Reader  ScriptReader
	Context ExecutionContext // tool-level proof; script calls need AllowsNewProof
	Cwd     string           // proven working directory, "" when unproven
}

// ScriptOrigin marks an invocation parsed from script text. Line is 1-based in the original file, 0 when unknown.
type ScriptOrigin struct {
	Path  string   `json:"path"`
	Line  int      `json:"line,omitempty"`
	Chain []string `json:"chain,omitempty"`
}

// ScriptRecord is the evidence a guard logs for one inspected script. Text is kept in memory only.
type ScriptRecord struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Bytes  int    `json:"bytes"`
	Text   string `json:"-"`
	// ID is the file identity the reader reported, "" when it gave none. It is evidence for the write-target check and
	// is never logged.
	ID string `json:"-"`
}

type scriptState struct {
	opts  ScriptOptions
	chain []string
	// ids is aligned with chain: the file identity of each script in it, "" when the reader gave none.
	ids []string
	// hostGOOS and parseHook are read from their package variables once per ParseScripts call, so a forked parse,
	// which may outlive the call, never reads a variable a test is resetting.
	hostGOOS  string
	parseHook func(child *parser)
	// deadline ends every script parse of one ParseScripts call; it is zero when expansion is disabled.
	deadline time.Time
	// callAssignments are the prefix assignments of the wrappers and the call around the script call being parsed, outermost
	// first. They act on the call, so a typed call is judged by them. wrapperFed is set while a wrapper around the call
	// has its standard input redirected, and callerFed while the script of a typed call that the typed command feeds is
	// being parsed: a script it calls without an input of its own reads the same stream.
	callAssignments []Word
	wrapperFed      bool
	callerFed       bool
}

type scriptForm struct {
	path Word
	args []Word
	// fed is true when the call's own standard input is redirected or piped.
	fed bool
}

var scriptBashOptions = map[string]bool{"--noprofile": true, "--norc": true, "-e": true, "-u": true}

// parseScriptForm classifies words, where words[0] is sh or bash. isScript is false for -c, -lc and a bare shell,
// which keep their existing handling. A non-empty reason is a named limit.
func parseScriptForm(words []Word) (form scriptForm, isScript bool, reason string) {
	i := 1
	for i < len(words) {
		w := words[i]
		switch {
		case !w.Literal:
			return scriptForm{}, true, "script-path-not-literal"
		case w.Text == "-c" || w.Text == "-lc":
			return scriptForm{}, false, ""
		case w.Text == "--":
			i++
			if i >= len(words) {
				return scriptForm{}, false, ""
			}
			return scriptForm{path: words[i], args: words[i+1:]}, true, ""
		case scriptBashOptions[w.Text]:
			i++
		case strings.HasPrefix(w.Text, "-") && w.Text != "-":
			return scriptForm{}, true, "script-form-unsupported-option"
		default:
			return scriptForm{path: w, args: words[i+1:]}, true, ""
		}
	}
	return scriptForm{}, false, ""
}

// expandScript reads the script and parses its text as a child command. It returns a non-empty reason when the
// call must stay blocked; in that case no child was appended.
//
// wrappers are the CALLER's wrappers. shell is the sh or bash word that is added to the children's wrappers
// for a `bash x.sh` call and nil for a `source x.sh` call, which runs in the same shell and so adds no wrapper
// occurrence: WrapperIDs must stay aligned with Wrappers.
//
// The children the script produced directly are labeled by markScriptOpacity before it returns, so a nested
// script's children are final, with their own origin, by the time the enclosing script is labeled.
func (p *parser) expandScript(form scriptForm, wrappers []Word, shell *Word, complete bool, before Separator, parent, depth int) string {
	p.expansions++
	opts := p.script.opts
	if !opts.Context.AllowsNewProof() {
		return "script-context-unqualified"
	}
	if depth >= maxWrapperDepth {
		// A second layer behind the chain bound below, counting wrappers as well as scripts. A source adds no
		// wrapper, so the wrapper-depth check in parseSegment never sees a source nest; each script level counts here
		// against the same bound instead.
		return "script-depth"
	}
	if len(p.script.chain) >= maxScriptDepth {
		return "script-depth"
	}
	childWrappers := wrappers
	if shell != nil {
		childWrappers = append(append([]Word(nil), wrappers...), *shell)
	}
	callDir, callOK, unprovenByCD := p.cwdBefore()
	resolved, reason := resolveScriptPath(p.script.hostGOOS, form.path, callDir, callOK)
	if reason != "" {
		return reason
	}
	if p.inChain(resolved) {
		return "script-cycle"
	}
	if !callOK && unprovenByCD && len(p.script.chain) == 0 {
		// An absolute path needs no cwd, but a directory change the guard cannot follow before a typed call is shell
		// state it cannot vouch for.
		return "script-predecessor-state"
	}
	if len(p.script.chain) == 0 {
		// The prefix of the typed call and of the wrappers around it, and the resets of an env around it, change what
		// every command of the script runs. A script's own calls are judged as script content.
		if prefixConstruct(p.script.callAssignments, p.environmentResets, "") != "" {
			return "script-env-pinned"
		}
		// Typed predecessors only: a script's own commands are judged as script content. This runs before the read, so a
		// script behind a refused predecessor is not read.
		if reason := p.predecessorReason(); reason != "" {
			return reason
		}
	}
	if time.Now().After(p.script.deadline) {
		// Checked before the read, so a call after the deadline neither reads a file nor spends a file slot.
		return "script-budget"
	}
	read := opts.Reader.ReadScript(resolved)
	if read.Limit != "" {
		return read.Limit
	}
	if p.inChain(read.Path) || read.ID != "" && slices.Contains(p.script.ids, read.ID) {
		// The reader may spell the path it read differently from the resolved one, and a hardlink, short name or case
		// variant reaches a file already in the chain under another path; its identity still matches.
		return "script-cycle"
	}
	// The real reader refuses CR; a reader that does not must not hand the lexer text whose line ends it cannot place.
	if strings.Contains(read.Text, "\r") {
		return "script-encoding"
	}
	if reason := prescan(read.Text); reason != "" {
		return reason
	}
	lines := newScriptLines(read.Text)
	consumes, unquoted := scanParams(lines.scan)
	for _, w := range wrappers {
		// xargs appends arguments the guard cannot read.
		if consumes && strings.ToLower(ExecutableName(w.Text)) == "xargs" {
			return "script-caller-argument"
		}
	}
	var args []string
	pattern := false
	for _, a := range form.args {
		switch {
		case a.Literal:
			args = append(args, a.Text)
			// The shell expands an unquoted pattern, so the script receives names the text does not show.
			// A quoted glob is expanded too when the script uses the parameter unquoted.
			pattern = pattern || a.Raw == a.Text && (strings.ContainsAny(a.Text, "*?[{") || strings.HasPrefix(a.Text, "~")) ||
				consumes && unquoted && strings.ContainsAny(a.Text, "*?[")
		case consumes:
			// The caller's argument is a value the guard cannot read, and the script uses its parameters.
			return "script-caller-argument"
		}
	}
	// A typed call is fed by its own pipe or redirect or by a wrapper's; a script it calls without an input of its own
	// reads the stream the caller's script was given.
	fed := form.fed || p.script.wrapperFed
	if len(p.script.chain) > 0 {
		fed = p.script.callerFed && !form.fed
	}
	if fed && stdinReader.MatchString(lines.scan) {
		return "script-stdin-fed"
	}
	callerFed := p.script.callerFed
	p.script.callerFed = fed
	defer func() { p.script.callerFed = callerFed }()
	origin := &ScriptOrigin{Path: read.Path, Chain: append([]string(nil), p.script.chain...)}
	recorded := len(p.result.Scripts)
	spelled := len(p.scriptSpellings)
	p.recordScript(read)
	p.noteSpelling(resolved)
	p.script.chain = append(p.script.chain, read.Path)
	p.script.ids = append(p.script.ids, read.ID)
	defer func() {
		p.script.chain = p.script.chain[:len(p.script.chain)-1]
		p.script.ids = p.script.ids[:len(p.script.ids)-1]
	}()
	if shell != nil {
		p.nextWrapperID++
		p.wrapperIDs = append(p.wrapperIDs, p.nextWrapperID)
		defer func() { p.wrapperIDs = p.wrapperIDs[:len(p.wrapperIDs)-1] }()
	}
	start := len(p.result.Invocations)
	bare := len(p.result.BareAssignments)
	savedBase := p.resetBase
	p.resetBase = len(p.environmentResets)
	defer func() { p.resetBase = savedBase }()
	// The child parse runs in a copy of the parser under the deadline. The deferred pops above stay correct after the
	// copy is adopted, because the copy holds the entries pushed here.
	if !p.parseForked(func(child *parser) {
		child.parseCommand(read.Text, POSIX, childWrappers, complete, before, parent, depth+1)
	}) {
		// A refused script is not one the guard inspected.
		p.result.Scripts = p.result.Scripts[:recorded]
		p.scriptSpellings = p.scriptSpellings[:spelled]
		return "script-budget"
	}
	// Each child gets an origin of its own so it can carry the line it came from. A child that already has one was
	// produced by a nested script and keeps that script's path and line.
	cursor := 0
	var own []int
	for i := start; i < len(p.result.Invocations); i++ {
		inv := &p.result.Invocations[i]
		if inv.Script != nil {
			continue
		}
		var line int
		line, cursor = lines.locate(cursor, inv.Raw)
		inv.Script = &ScriptOrigin{Path: origin.Path, Chain: origin.Chain, Line: line}
		own = append(own, i)
	}
	p.locatePendingCalls(start, lines)
	p.markScriptOpacity(start)
	// Before the operand rule, which would otherwise refuse a git verb taken from a parameter under its own name.
	p.markCallerArguments(own)
	p.applyOperandRule(operandScope{start: start, bare: bare, own: own, lines: lines, sourced: shell == nil, wrappers: len(childWrappers)})
	p.markScriptTextConstructs(lines, bare, origin, childWrappers, parent, complete)
	p.result.ScriptCalls = append(p.result.ScriptCalls, ScriptCall{Path: read.Path, Args: args, ConsumesParams: consumes,
		UnquotedParams: consumes && unquoted, PatternArgs: pattern})
	return ""
}

// inChain reports whether path is a script that is already being expanded, which would make the expansion endless:
// a script that sources or calls itself, directly or through others.
func (p *parser) inChain(path string) bool { return slices.Contains(p.script.chain, path) }

func (p *parser) recordScript(read ScriptRead) {
	for _, rec := range p.result.Scripts {
		if rec.Path == read.Path {
			return
		}
	}
	p.result.Scripts = append(p.result.Scripts, ScriptRecord{Path: read.Path, SHA256: read.SHA256, Bytes: read.Bytes, Text: read.Text, ID: read.ID})
}

// label renders a script location for messages: path, line and calling chain, never content.
func (o *ScriptOrigin) label() string {
	s := safeScriptPath(o.Path)
	if o.Line > 0 {
		s = fmt.Sprintf("%s:%d", s, o.Line)
	}
	if len(o.Chain) > 0 {
		via := make([]string, len(o.Chain))
		for i, c := range o.Chain {
			via[i] = safeScriptPath(c)
		}
		s += " (via " + strings.Join(via, " > ") + ")"
	}
	return s
}

// ScriptReason formats a named rule for an invocation parsed from a script: "rule in path:line".
func ScriptReason(rule string, origin *ScriptOrigin) string {
	if origin == nil {
		return rule
	}
	return rule + " in " + origin.label()
}

func safeScriptPath(p string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsPrint(r) {
			return r
		}
		return '?'
	}, p)
}
