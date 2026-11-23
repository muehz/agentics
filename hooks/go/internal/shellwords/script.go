package shellwords

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/muehz/agentics/hooks/go/internal/pathshape"
)

// ScriptHeader records fixed-size metadata peeked from the beginning of a file so callers can classify binary images
// and shebang lines without retaining unbounded content. All fields are value-comparable.
type ScriptHeader struct {
	// HasNUL reports whether the header peek contains a NUL byte (0x00).
	HasNUL bool
	// BinaryMagic reports whether the header peek starts with an ELF, Mach-O, or PE executable header (requiring at
	// least one NUL byte in the peek for PE).
	BinaryMagic bool
	// Shebang holds the first line when the header peek starts with "#!", with a trailing CRLF (\r\n) or LF (\n)
	// stripped while preserving any lone CR (\r not followed by \n) before capping at 257 bytes so lines exceeding the
	// 256-byte limit remain distinguishable by ClassifyShebang, or "" when the peek does not start with "#!".
	Shebang string
}

// ScriptRead is one definitive read of a script file. Text holds the script text, populated with raw file bytes by
// snapshot readers for dialect-specific encoding checks or pre-populated directly by test doubles.
// Limit is RuleNone on success and otherwise the rule the read refused under, such as RuleTooLarge; Detail never holds
// file content. All fields are value-comparable so cached reads can be compared directly and shared safely across
// concurrent guards.
type ScriptRead struct {
	// Path is the first path that reached the file: an alias (hardlink, 8.3 name, case variant) returns the first
	// read, which is what lets a path-based cycle check catch aliases. Among concurrent guard goroutines the
	// spelling shown can vary from run to run, while all three guards agree on one read within a request.
	Path   string
	ID     string // stable file identity; equal for hardlink aliases, "" on a refusal or when the filesystem gives none
	Text   string
	SHA256 string
	Bytes  int
	Limit  Rule
	Detail string
	// Raw holds the immutable raw file bytes read from disk when Limit is RuleNone and !NotExist && !TooLarge; test
	// readers may leave Raw empty and populate Text directly.
	Raw string
	// NotExist reports that Lstat or file open observed os.ErrNotExist, distinct from other RuleUnreadable failures.
	NotExist bool
	// IsSymlink reports that the target path itself or any traversed parent component is a symlink.
	IsSymlink bool
	// TooLarge reports that the file exceeded the configured read cap, allowing callers to distinguish oversized
	// non-scripts from scripts.
	TooLarge bool
	// Header holds metadata peeked from the first 4 KiB of the opened file, or peeked without reading the full file
	// when TooLarge is true.
	Header ScriptHeader
	// ResolvedHeader holds metadata peeked from the final symlink target when IsSymlink is true and the final target is
	// a regular file.
	ResolvedHeader ScriptHeader
	// HasResolvedHeader reports whether ResolvedHeader was populated for a resolved regular file target.
	HasResolvedHeader bool
}

// ScriptReader reads a resolved script path as data. Implementations must never execute it.
type ScriptReader interface{ ReadScript(path string) ScriptRead }

// ScriptClassifier reads the same cached file as ScriptReader for dispatch
// classification. Callers must use ReadScript before inspecting a script body.
// Classification consumes a file slot without charging the script-byte budget.
type ScriptClassifier interface{ InspectScript(path string) ScriptRead }

// ScriptProber performs a slot-exempt existence check without opening file content or consuming a read slot.
type ScriptProber interface {
	// ProbeExists reports whether path exists without reading its content, along with any refusal rule raised before
	// probing.
	ProbeExists(path string) (bool, Rule)
}

// ScriptOptions enables script-call expansion. The zero value disables it, so ParseContext is unchanged.
type ScriptOptions struct {
	Reader  ScriptReader
	Context ExecutionContext // tool-level proof; script calls need AllowsNewProof
	Cwd     string           // proven working directory, "" when unproven
	// Host is the host whose path rules apply to a script word. The zero value is the running host; a test names
	// another host with pathshape.For.
	Host pathshape.Host
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
	// host is taken from the options, and parseHook from its package variable, once per ParseScripts call. A forked
	// parse copies both, so it follows the host its parse began with and never reads a variable a test is resetting.
	host      pathshape.Host
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
	dialect, caller Dialect
	sourced, direct bool
	read            *ScriptRead
	path            Word
	args            []Word
	// sourceArgsBound is proof that a POSIX source call rebinds positional arguments.
	sourceArgsBound bool
	// fed is true when the call's own standard input is redirected or piped.
	fed bool
}

var scriptBashOptions = map[string]bool{"--noprofile": true, "--norc": true, "-e": true, "-u": true}

// parseScriptForm classifies words, where words[0] is sh or bash. isScript is false for -c, -lc and a bare shell,
// which keep their existing handling. A reason other than RuleNone is a named limit.
func parseScriptForm(words []Word) (form scriptForm, isScript bool, reason Rule) {
	i := 1
	for i < len(words) {
		w := words[i]
		switch {
		case !w.Literal:
			return scriptForm{}, true, RulePathNotLiteral
		case w.Text == "-c" || w.Text == "-lc":
			return scriptForm{}, false, RuleNone
		case w.Text == "--":
			i++
			if i >= len(words) {
				return scriptForm{}, false, RuleNone
			}
			return scriptForm{path: words[i], args: words[i+1:]}, true, RuleNone
		case scriptBashOptions[w.Text]:
			i++
		case strings.HasPrefix(w.Text, "-") && w.Text != "-":
			return scriptForm{}, true, RuleFormUnsupportedOption
		default:
			return scriptForm{path: w, args: words[i+1:]}, true, RuleNone
		}
	}
	return scriptForm{}, false, RuleNone
}

func (p *parser) dispatchFile(inv Invocation, form scriptForm, shell *Word, reason Refusal, depth int) {
	form.caller = inv.Dialect
	form.path.Text = scriptArgumentText(form.path, inv.Dialect)
	start, calls := len(p.result.Invocations), len(p.result.ScriptCalls)
	if !p.script.opts.Context.AllowsNewProof() {
		reason = ruleRefusal(RuleContextUnqualified, nil)
	} else if !reason.Refused() {
		form.fed = p.ownBefore(inv.Before) == Pipe || inv.PipelineInput || len(inv.InputRedirects) > 0 || inv.Dialect == POSIX && rawRedirectsStdin(inv.Raw, POSIX)
		if form.sourced && len(p.lists) > 0 {
			marks := p.lists[len(p.lists)-1].segments
			if len(marks) > 0 {
				marks[len(marks)-1].sourcedFile = true
			}
		}
		leave := p.enterCall(inv.Assignments, false)
		rule := p.expandScript(form, inv.Wrappers, shell, inv.Complete, inv.Before, inv.Parent, depth)
		leave()
		reason = ruleRefusal(rule, inv.Script)
	}
	if reason.Refused() {
		p.refuse(inv, reason)
		return
	}
	p.inheritCallContext(start, inv.Assignments, inv.InputRedirects, inv.OutputRedirects, inv.PipelineInput, inv.PipelineOutput, inv.PipelineInputEdge, inv.PipelineOutputEdge)
	p.inheritCallOutput(calls, inv.OutputRedirects)
	p.noteCallPrefix(start, inv.Raw, inv.Assignments, nil, inv.Wrappers, p.wrapperIDs)
}

// expandScript reads the script and parses its text as a child command. It returns a rule other than RuleNone when the
// call must stay blocked; in that case no child was appended.
//
// A new shell or direct executable contributes a wrapper occurrence. A sourced
// file runs in the caller's shell and contributes none.
//
// The children the script produced directly are labeled by markScriptOpacity before it returns, so a nested
// script's children are final, with their own origin, by the time the enclosing script is labeled.
func (p *parser) expandScript(form scriptForm, wrappers []Word, shell *Word, complete bool, before Separator, parent, depth int) Rule {
	p.expansions++
	opts := p.script.opts
	if !opts.Context.AllowsNewProof() {
		return RuleContextUnqualified
	}
	if depth >= maxWrapperDepth {
		// A second layer behind the chain bound below, counting wrappers as well as scripts. A source adds no
		// wrapper, so the wrapper-depth check in parseSegment never sees a source nest; each script level counts here
		// against the same bound instead.
		return RuleDepth
	}
	if len(p.script.chain) >= maxScriptDepth {
		return RuleDepth
	}
	childWrappers := wrappers
	if shell != nil {
		childWrappers = append(append([]Word(nil), wrappers...), *shell)
	}
	callDir, callOK, unprovenByCD := p.cwdBefore()
	resolved, reason := resolveScriptPath(p.script.host, form.path, callDir, callOK, form.caller)
	if reason != RuleNone {
		return reason
	}
	if !form.direct && !form.sourced {
		ext := strings.TrimPrefix(scriptExt(form.path.Text), ".")
		if form.dialect == PowerShell && ext != "ps1" || form.dialect == POSIX && !posixShellName(ext) {
			return RuleExtension
		}
	}
	if p.inChain(resolved) {
		return RuleCycle
	}
	if !callOK && unprovenByCD && len(p.script.chain) == 0 {
		// An absolute path needs no cwd, but a directory change the guard cannot follow before a typed call is shell
		// state it cannot vouch for.
		return RulePredecessorState
	}
	if len(p.script.chain) == 0 {
		// The prefix of the typed call and of the wrappers around it, and the resets of an env around it, change what
		// every command of the script runs. A script's own calls are judged as script content.
		if prefixConstruct(p.script.callAssignments, p.environmentResets, "") != constructNone {
			return RuleEnvPinned
		}
		// Typed predecessors only: a script's own commands are judged as script content. This runs before the read, so a
		// script behind a refused predecessor is not read.
		if reason := p.predecessorReason(); reason != RuleNone {
			return reason
		}
	}
	if time.Now().After(p.script.deadline) {
		// Checked before the read, so a call after the deadline neither reads a file nor spends a file slot.
		return RuleBudget
	}
	var read ScriptRead
	if form.read != nil {
		read = *form.read
	} else {
		read = opts.Reader.ReadScript(resolved)
	}
	if read.Limit != RuleNone {
		return read.Limit
	}
	if p.inChain(read.Path) || read.ID != "" && slices.Contains(p.script.ids, read.ID) {
		// The reader may spell the path it read differently from the resolved one, and a hardlink, short name or case
		// variant reaches a file already in the chain under another path; its identity still matches.
		return RuleCycle
	}
	origin := &ScriptOrigin{Path: read.Path, Chain: slices.Clone(p.script.chain)}
	raw := read.Raw
	if raw == "" {
		raw = read.Text
	}
	var lines *scriptLines
	var ps psParseResult
	var consumes, unquoted bool
	var source string
	if form.dialect == PowerShell {
		source, reason = CheckPowerShellEncoding(raw)
		if reason != RuleNone {
			return reason
		}
		if !powerShellSizeWithinBudget(source) {
			return RuleBudget
		}
		ctx, cancel := context.WithDeadline(p.ctx, p.script.deadline)
		ps = parsePowerShell(ctx, source, psModeScript, origin, &p.nextPipelineEdge)
		cancel()
		if ps.Canceled {
			return RuleBudget
		}
		consumes = ps.ConsumesParams
	} else {
		source, reason = CheckPOSIXEncoding(raw)
		if reason != RuleNone {
			return reason
		}
		if reason = prescan(source); reason != RuleNone {
			return reason
		}
		lines = newScriptLines(source)
		consumes, unquoted = scanParams(lines.scan)
		if form.sourced && consumes && !form.sourceArgsBound {
			return RuleCallerArgument
		}
	}
	for _, w := range wrappers {
		// xargs appends arguments the guard cannot read.
		if consumes && strings.ToLower(ExecutableName(w.Text)) == "xargs" {
			return RuleCallerArgument
		}
	}
	var args []string
	pattern := false
	arguments, completeArgs := p.scriptCallerWords(form)
	if consumes && !completeArgs {
		return RuleCallerArgument
	}
	for _, a := range arguments {
		switch {
		case a.Literal:
			args = append(args, scriptArgumentText(a, form.caller))
			// The shell expands an unquoted pattern, so the script receives names the text does not show.
			// A quoted glob is expanded too when the script uses the parameter unquoted.
			pattern = pattern || scriptPatternArg(a, form.caller) || consumes && unquoted && strings.ContainsAny(a.Text, "*?[")
		case consumes:
			// The caller's argument is a value the guard cannot read, and the script uses its parameters.
			return RuleCallerArgument
		}
	}
	// A typed call is fed by its own pipe or redirect or by a wrapper's; a script it calls without an input of its own
	// reads the stream the caller's script was given.
	fed := form.fed || p.script.wrapperFed
	if len(p.script.chain) > 0 {
		fed = p.script.callerFed && !form.fed
	}
	if fed && (lines != nil && stdinReader.MatchString(lines.scan) || form.dialect == PowerShell && psReadsInput(ps, source)) {
		return RuleStdinFed
	}
	callerFed := p.script.callerFed
	p.script.callerFed = fed
	defer func() { p.script.callerFed = callerFed }()
	recorded := len(p.result.Scripts)
	spelled := len(p.scriptSpellings)
	read.Text = source
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
		if form.dialect == PowerShell {
			child.dispatchPowerShell(ps, origin, childWrappers, complete, before, parent, depth+1)
		} else {
			child.parseCommand(source, POSIX, childWrappers, complete, before, parent, depth+1)
		}
	}) {
		// A refused script is not one the guard inspected.
		p.result.Scripts = p.result.Scripts[:recorded]
		p.scriptSpellings = p.scriptSpellings[:spelled]
		return RuleBudget
	}
	// Each child gets an origin of its own so it can carry the line it came from. A child that already has one was
	// produced by a nested script and keeps that script's path and line.
	cursor := 0
	var wrapperLines map[int]int
	var own []int
	for i := start; i < len(p.result.Invocations); i++ {
		inv := &p.result.Invocations[i]
		if inv.Script != nil {
			if inv.Script.Path == origin.Path {
				own = append(own, i)
			}
			continue
		}
		var line int
		if lines != nil {
			if inv.wrapperSource != "" {
				if known, ok := wrapperLines[inv.wrapperSourceID]; ok {
					line = known
				} else {
					line, cursor = lines.locate(cursor, inv.wrapperSource)
					if wrapperLines == nil {
						wrapperLines = map[int]int{}
					}
					wrapperLines[inv.wrapperSourceID] = line
				}
				inv.wrapperSourceID = 0
				inv.wrapperSource = ""
			} else {
				line, cursor = lines.locate(cursor, inv.Raw)
			}
		}
		inv.Script = &ScriptOrigin{Path: origin.Path, Chain: origin.Chain, Line: line}
		own = append(own, i)
	}
	if lines != nil {
		p.locatePendingCalls(start, lines)
	}
	p.markScriptOpacity(start)
	// Before the operand rule, which would otherwise refuse a git verb taken from a parameter under its own name.
	p.markCallerArguments(own)
	p.applyOperandRule(operandScope{start: start, bare: bare, own: own, lines: lines, sourced: shell == nil, wrappers: len(childWrappers)})
	if lines != nil {
		p.markScriptTextConstructs(lines, bare, origin, childWrappers, parent, complete)
	}
	p.result.ScriptCalls = append(p.result.ScriptCalls, ScriptCall{Path: read.Path, Args: args, ConsumesParams: consumes,
		UnquotedParams: consumes && unquoted, PatternArgs: pattern})
	return RuleNone
}

// The PowerShell parser enforces syntax-aware nesting. Bound physical lines
// and whitespace-delimited units before starting it.
func powerShellSizeWithinBudget(source string) bool {
	lines := strings.Count(source, "\n")
	if source != "" && !strings.HasSuffix(source, "\n") {
		lines++
	}
	if lines > maxScriptLines {
		return false
	}
	units := 0
	for range strings.FieldsSeq(source) {
		units++
		if units > maxScriptTokens {
			return false
		}
	}
	return true
}

func psReadsInput(ps psParseResult, source string) bool {
	if powerShellInput.MatchString(source) {
		return true
	}
	for _, inv := range ps.Invocations {
		if len(inv.Words) > 0 && strings.EqualFold(inv.Words[0].Text, "Read-Host") {
			return true
		}
	}
	return false
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

func safeScriptPath(p string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsPrint(r) {
			return r
		}
		return '?'
	}, p)
}
