// Package shellwords performs bounded lexical inspection of literal shell commands.
// It never expands variables, executes substitutions, or touches the filesystem.
package shellwords

import (
	"context"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/muehz/agentics/hooks/go/internal/argv"
)

// Dialect selects the quoting, escaping, and operator rules used by Parse.
type Dialect uint8

const (
	// POSIX parses Bourne-style shell syntax with backslash escapes.
	POSIX Dialect = iota
	// PowerShell parses PowerShell syntax with backtick escapes.
	PowerShell
)

// Separator describes the operator that precedes an invocation.
type Separator uint8

const (
	// Start marks the first invocation in a command.
	Start Separator = iota
	// Sequence marks a semicolon or newline.
	Sequence
	// And marks a logical-and operator.
	And
	// Or marks a logical-or operator.
	Or
	// Pipe marks a pipeline operator.
	Pipe
	// Background marks a background operator.
	Background
	// Group marks an invocation in a grouped expression.
	Group
	// Substitution marks an invocation in a command substitution.
	Substitution
)

// Word is a parsed command word with its quote and expansion provenance. The
// argv package owns the type so that readers below this package can name it.
type Word = argv.Word

// Redirection is an input or output redirection attached to an invocation.
type Redirection struct {
	FD       int
	Operator string
	Target   Word
}

// Invocation is one executable context found in a shell command.
type Invocation struct {
	// Inspection retains a Bash inspection operation without inventing a child.
	Inspection InspectionRole `json:",omitempty"`
	// SyntaxOperand is the literal file read by a syntax-only invocation.
	SyntaxOperand *Word `json:",omitempty"`
	// EnvironmentResets retains env wrapper resets in outer-to-inner source order.
	EnvironmentResets []EnvironmentReset
	// Dialect is the syntax of this invocation, including wrapper children.
	Dialect Dialect
	// EnvironmentCall is lexical evidence for a PowerShell static setter.
	EnvironmentCall *PowerShellEnvironmentCall
	Words           []Word
	Wrappers        []Word
	// WrapperIDs identifies wrapper occurrences in Wrappers order, scoped to
	// one Parse result. Shared ancestors retain IDs; identical siblings do not.
	WrapperIDs     []int
	Assignments    []Word
	InputRedirects []Redirection
	// OutputRedirects are the redirects that open a file for writing, including those of the wrapper or script call the
	// invocation runs under: the command writes where its caller sends its output. Descriptor duplications such as 2>&1
	// are left out. Kept out of the encoded form.
	OutputRedirects []Redirection `json:"-"`
	Before          Separator
	Parent          int
	Raw             string
	Inspectable     bool
	// Reason says why the invocation is not inspectable, the zero Refusal when nothing is refused. It encodes as the
	// text it renders, so the encoded form is the string it replaced.
	Reason   Refusal
	Complete bool
	// Script marks an invocation parsed from the text of an inspected script.
	Script *ScriptOrigin `json:"script,omitempty"`
	// PipelineInput marks an invocation inside a grouped command that receives
	// input from a preceding pipeline stage.
	PipelineInput bool
	// PipelineOutput marks an invocation inside a grouped command whose output
	// flows into a following pipeline stage.
	PipelineOutput bool
	// PipelineInputEdge identifies the stream entering this invocation.
	PipelineInputEdge int
	// PipelineOutputEdge identifies the stream leaving this invocation.
	PipelineOutputEdge int
}

// InspectionRole distinguishes nonexecuting metadata from a syntax file read.
type InspectionRole uint8

const (
	// NoInspection marks ordinary executable evidence.
	NoInspection InspectionRole = iota
	// BashMetadata marks an exact Bash help or version request.
	BashMetadata
	// BashSyntaxRead marks a Bash syntax check whose file still needs read policy.
	BashSyntaxRead
)

// EnvironmentReset is one env option that removes inherited variables.
// Occurrence identifies the env wrapper within one Parse result; Order is its
// option word offset. Name retains unresolved expansion evidence for unsets.
type EnvironmentReset struct {
	Occurrence int
	Order      int
	Dialect    Dialect
	All        bool
	Name       Word
}

// EnvironmentEffectKind distinguishes lexical environment writes and resets.
type EnvironmentEffectKind uint8

const (
	// EnvironmentAssignment carries a name=value word.
	EnvironmentAssignment EnvironmentEffectKind = iota
	// EnvironmentUnset carries an unset name with its expansion provenance.
	EnvironmentUnset
	// EnvironmentResetAll removes the inherited environment.
	EnvironmentResetAll
)

// EnvironmentEffect survives even when a wrapper emits no executable child.
// Occurrence identifies an env wrapper within a Parse result; zero identifies
// a shell assignment. Order is the word offset in that source context.
type EnvironmentEffect struct {
	Occurrence int
	Order      int
	Dialect    Dialect
	Kind       EnvironmentEffectKind
	Word       Word
}

// Result contains the invocations found by Parse.
type Result struct {
	Invocations []Invocation
	// EnvironmentEffects retains wrapper effects in source order, before
	// recursively parsed inner effects, independently of child emission.
	EnvironmentEffects []EnvironmentEffect
	// BareAssignments holds the assignments of segments that run no command,
	// such as `NAME=value;`. Such a segment changes a shell variable that a
	// later command inherits when it is exported, so it produces no
	// Invocation but stays visible to guards that read environment writes.
	BareAssignments []Word
	// Scripts records each script read as data during ParseScripts, once per
	// path, so a guard can log what it inspected.
	Scripts []ScriptRecord `json:"scripts,omitempty"`
	// ScriptCalls holds one entry per expanded script call, a nested call before the call that made it, so a guard can
	// judge the arguments each script received.
	ScriptCalls []ScriptCall `json:"scriptCalls,omitempty"`
}

const maxWrapperDepth = 4

// reserveCap bounds the capacity Parse reserves up front. Typical commands
// have fewer invocations and words than this; longer ones grow by append, so
// crafted input cannot inflate the reservation.
const reserveCap = 16

type rawSegment struct {
	raw                string
	before             Separator
	parentSegment      int
	groupID            int
	pipelineInput      bool
	pipelineInputEdge  int
	pipelineOutput     bool
	pipelineOutputEdge int
	// bracedWord marks the first segment after a { that the splitter opened as a group although a character other
	// than a blank follows it. bash takes { as a group only as a word of its own, so there it starts a word that
	// brace expansion may turn into another command: {cd,} sub runs cd sub.
	bracedWord bool
	// funcDef marks a segment whose text was cut off by a ( that opens a function body.
	funcDef bool
	// owner is the segment whose word or command the innermost substitution, backtick or process substitution around this
	// segment belongs to, -1 outside every one. The later commands of a substitution do not name it as a parent.
	owner int
	// afterBackground marks the first segment emitted after a & separator. The separator a segment carries can be
	// overwritten by a group opener that follows the &, as in `x & { y; }`, so before alone does not tell.
	afterBackground bool
	// stderrPipeEdge is the pipeline edge a segment reads through |&. The splitter keeps marking |& as a pipe followed
	// by a background separator, which the other guards rely on; this field records that the segment is in fact the
	// next member of that pipeline. It is set only on a plain command: a group or process substitution after |& gets
	// afterBackground instead.
	stderrPipeEdge int
}

type lexedSegment struct {
	words     []Word
	input     []Redirection
	redirects []Redirection
	// refusal is the last typed-form failure the lexer wrote: an ANSI-C escape it cannot decode, then at the end of the
	// segment an unterminated quote or a redirection without a target, which win when present.
	refusal Refusal
}

type parser struct {
	ctx               context.Context
	dialect           Dialect
	result            Result
	nextPipelineEdge  int
	nextWrapperID     int
	wrapperIDs        []int
	environmentResets []EnvironmentReset
	nextEnvironmentID int
	environmentDepth  int
	script            scriptState
	// bareClaimed holds the indexes of Result.BareAssignments that an expanded script has already attributed to
	// itself, so the script that called it does not attribute them a second time.
	bareClaimed map[int]bool
	// bareAt is aligned with Result.BareAssignments: the number of invocations recorded when each was found. An
	// invocation at that index or later was parsed after the assignment, including those of later lines. The
	// substitution children of the assignment's own segment are appended after it and so share its index although
	// they run before it; that is harmless for the operand proofs, because a proven constant has no children and a
	// proven temp's only child is its mktemp.
	bareAt []int
	// judged holds the indexes of invocations whose own pinned-name assignments were already judged by the script that
	// produced them. A script that calls them adds its call prefix to their assignments afterwards; that prefix is
	// reported once at the call, not once for each command.
	judged map[int]bool
	// resetBase is the number of environment resets in force where the script being expanded was called; only the
	// resets beyond it belong to the script. Resets outside the script, such as the env -u PATH of a typed
	// `env -u PATH bash a.sh`, are not judged by this layer; a later change to the typed checks owns them.
	resetBase int
	// pendingCalls are the refused stand-ins for call prefixes, waiting for the line of the call in the script that
	// holds it.
	pendingCalls []pendingCall
	// lists holds the command lists being parsed, outermost first: the typed command, then each `-c` string or script
	// text that encloses the segment being parsed. A script call's working directory is replayed from them.
	lists []commandList
	// predecessorScanned and bareScanned count the invocations and bare assignments already judged as predecessors of a
	// typed script call, so each typed call resumes the scan where the last one stopped.
	predecessorScanned, bareScanned int
	// scriptSpellings are the paths script calls asked the reader for, kept for the write-target check.
	scriptSpellings []string
	// expansions counts the script calls that reached expansion, so a segment can tell whether it held one.
	expansions int
	// strayWrite is set when a typed segment that runs no command redirects output to a file, and typedFunction when a
	// typed command defines a function: a script call after either runs in a shell it changed. typedStateText is set
	// when the typed text assigns a pinned or state variable through a default-value expansion or an arithmetic
	// assignment, which leave no assignment word to judge.
	strayWrite, typedFunction, typedStateText bool
}

// newParser returns a parser for one ParseScripts call. The parse hook a test may replace is read here, once, so a
// forked parse never reads it.
func newParser(ctx context.Context, dialect Dialect, opts ScriptOptions) parser {
	p := parser{dialect: dialect, ctx: ctx, script: scriptState{opts: opts, host: opts.Host, parseHook: scriptParseHook}}
	if opts.Reader != nil {
		p.script.deadline = time.Now().Add(scriptCeiling)
	}
	return p
}

// fork returns a copy of p that shares no slice or map with it, so a copy that outlives its deadline can keep writing
// while the caller goes on. ctx and the reader are shared read-only. The cloned invocations still share their inner
// slices and pointers (Words, Assignments, Script and the rest) with the caller's and with every copy abandoned
// earlier, so parser code never writes through them: it replaces a slice or an origin with a new one (the operand
// rewrite clones Words, the pending-call pass makes a new origin, an append to InputRedirects clips first). Every
// reference-typed field of parser must be cloned here; a reflection test enforces it.
func (p *parser) fork() *parser {
	f := *p
	f.result.Invocations = slices.Clone(p.result.Invocations)
	f.result.EnvironmentEffects = slices.Clone(p.result.EnvironmentEffects)
	f.result.BareAssignments = slices.Clone(p.result.BareAssignments)
	f.result.Scripts = slices.Clone(p.result.Scripts)
	f.result.ScriptCalls = slices.Clone(p.result.ScriptCalls)
	f.script.callAssignments = slices.Clone(p.script.callAssignments)
	f.wrapperIDs = slices.Clone(p.wrapperIDs)
	f.environmentResets = slices.Clone(p.environmentResets)
	f.script.chain = slices.Clone(p.script.chain)
	f.script.ids = slices.Clone(p.script.ids)
	f.bareClaimed = maps.Clone(p.bareClaimed)
	f.bareAt = slices.Clone(p.bareAt)
	f.judged = maps.Clone(p.judged)
	f.pendingCalls = slices.Clone(p.pendingCalls)
	f.lists = slices.Clone(p.lists)
	f.scriptSpellings = slices.Clone(p.scriptSpellings)
	for i := range f.lists {
		f.lists[i].segments = slices.Clone(p.lists[i].segments)
	}
	return &f
}

// commandList is one list of commands as one shell runs it: the typed command, a `-c` string or a script's text.
type commandList struct {
	// depth is the number of wrappers of the list itself. A command of the list that has more ran through a wrapper
	// of its own, such as env or bash, and so possibly in another process.
	depth int
	// grouped is true when the text holds ( ), { } or a backtick. The lexer marks only the first segment of a group,
	// substitution or function body, so a later cd inside one looks like a command of the list itself; a cd is never
	// proven while the list or an enclosing one is grouped. This over-refuses by design: such a character anywhere
	// in the text, even in ${X} or a quoted string, is enough once a cd has been seen.
	grouped bool
	// compoundCD is true when the text holds a compound-command keyword and either a directory-change word or a
	// character that lets a word expand to one at run time (see compoundWithDirectoryChange). A cd in another branch,
	// or later in a loop body, is not ordered before a call by the text, so no call in the list or below it is proven.
	compoundCD bool
	// inLoop is true while the segment being parsed sits in a loop body or condition of this list or of a list that
	// encloses it, so a list nested in that segment knows its commands may run again.
	inLoop   bool
	segments []segmentMark
}

// segmentMark is one segment of a list with the separator that the list's own text puts before it. Every segment is
// recorded, including one that leaves no invocation in this shell (a bare assignment, `command -v`, an empty `-c`
// string, a wrapped command): its separator still decides whether an earlier cd was backgrounded, piped or skipped.
type segmentMark struct {
	before Separator
	// first and end bound the invocations the segment produced. end is -1 while the segment is being parsed, which
	// holds for the segment that contains the call being resolved and for each one that started an enclosing list.
	first, end int
	// foreignPrefix is set when the parser skipped a leading word that the shell does not treat as a keyword, such as
	// If, "if" or -x. The shell then runs that word as the command, so a cd after it is not a cd of this shell.
	foreignPrefix bool
	// bracedWord is copied from the raw segment: the segment may be part of one brace-expanded command word, whose
	// expansion the parser never sees.
	bracedWord bool
	// sourcedFile is set when the segment sourced a file into this shell. The sourced text's own list, with its
	// separators, groups and compound commands, is gone by the time a later call is resolved, so its cds cannot be
	// replayed.
	sourcedFile bool
}

// Parse returns the effective invocations in command. Recognized execution
// wrappers are unwrapped only when every option and command token is literal.
func Parse(command string, dialect Dialect) Result {
	return ParseContext(context.Background(), command, dialect)
}

// ParseContext is Parse with cooperative cancellation for static PowerShell
// and Bash inspection proofs. Legacy lexical loops are not yet cooperatively
// cancellable.
func ParseContext(ctx context.Context, command string, dialect Dialect) Result {
	return ParseScripts(ctx, command, dialect, ScriptOptions{})
}

// ParseScripts is ParseContext with script-call expansion enabled when
// opts.Reader is set: a closed-form sh or bash script call is read as data
// and its text parsed as child invocations tagged with their ScriptOrigin.
// The zero ScriptOptions gives exactly the ParseContext result.
//
// Script parses share one deadline, scriptCeiling from the start of the
// call; a script call that would run past it is refused as script-budget.
func ParseScripts(ctx context.Context, command string, dialect Dialect, opts ScriptOptions) Result {
	p := newParser(ctx, dialect, opts)
	p.parseCommand(command, dialect, nil, true, Start, -1, 0)
	if opts.Reader != nil {
		p.checkScriptWrites()
	}
	if len(p.result.Invocations) == 0 {
		// The top-level reservation below must not turn "no invocations"
		// into an empty non-nil slice.
		p.result.Invocations = nil
	}
	if ctx.Err() != nil {
		// Preserve source and executable children, but revoke all literal-call
		// proofs and publish an uninspectable result before any guard can pass.
		for i := range p.result.Invocations {
			inv := &p.result.Invocations[i]
			inv.Inspection, inv.SyntaxOperand = NoInspection, nil
			if inv.EnvironmentCall != nil {
				inv.EnvironmentCall = &PowerShellEnvironmentCall{}
			}
			inv.Inspectable = false
			inv.Reason = cancelRefusal(inv.Dialect)
		}
		if len(p.result.Invocations) == 0 {
			p.result.Invocations = []Invocation{{Raw: command, Dialect: dialect, Reason: cancelRefusal(dialect)}}
		}
	}
	return p.result
}

func (p *parser) parseCommand(command string, dialect Dialect, wrappers []Word, complete bool, inherited Separator, parent int, depth int) {
	if dialect == POSIX && p.script.opts.Reader != nil && len(p.script.chain) == 0 && textAssignsState(command) {
		// Read before the heredoc bodies are cut: bash expands an unquoted body.
		p.typedStateText = true
	}
	if dialect == POSIX {
		command = stripHeredocBodies(command)
	}
	list := len(p.lists)
	p.lists = append(p.lists, commandList{depth: len(wrappers), grouped: strings.ContainsAny(command, "(){}`"),
		compoundCD: p.script.opts.Reader != nil && compoundWithDirectoryChange(command)})
	defer func() { p.lists = p.lists[:list] }()
	segments := splitSegmentsContext(p.ctx, command, dialect, &p.nextPipelineEdge)
	if depth == 0 && p.result.Invocations == nil {
		p.result.Invocations = make([]Invocation, 0, min(len(segments), reserveCap))
	}
	segmentLast := make([]int, len(segments))
	for i := range segmentLast {
		segmentLast[i] = -1
	}
	typed := p.script.opts.Reader != nil && len(p.script.chain) == 0
	var judge *concurrencyJudge
	if typed {
		judge = newConcurrencyJudge(segments, list > 0 && p.lists[list-1].inLoop)
	}
	first := true
	for i, segment := range segments {
		before := segment.before
		if first && inherited != Start {
			before = inherited
		}
		resolvedParent := parent
		if segment.parentSegment >= 0 && segment.parentSegment < len(segmentLast) {
			resolvedParent = segmentLast[segment.parentSegment]
		}
		start := len(p.result.Invocations)
		// The mark keeps the separator of the list's own text: the first segment of a nested list inherits the
		// separator of the command that started it, which belongs to the enclosing list.
		p.lists[list].segments = append(p.lists[list].segments, segmentMark{before: segment.before, first: start, end: -1, bracedWord: segment.bracedWord})
		if segment.funcDef && typed && len(wrappers) == 0 {
			p.typedFunction = true
		}
		expanded := p.expansions
		if judge != nil {
			p.lists[list].inLoop = judge.enter(i)
		}
		p.parseSegment(segment.raw, dialect, wrappers, complete, before, resolvedParent, depth, segment.pipelineInput, segment.pipelineOutput, segment.pipelineInputEdge, segment.pipelineOutputEdge)
		if judge != nil {
			judge.leave(p, i, start, p.expansions > expanded, len(wrappers))
		}
		p.lists[list].segments[len(p.lists[list].segments)-1].end = len(p.result.Invocations)
		if len(p.result.Invocations) > start {
			segmentLast[i] = len(p.result.Invocations) - 1
			first = false
		}
	}
}

func (p *parser) parseSegment(raw string, dialect Dialect, wrappers []Word, complete bool, before Separator, parent int, depth int, pipelineInput, pipelineOutput bool, pipelineInputEdge, pipelineOutputEdge int) {
	lexed := lexSegment(raw, dialect)
	words, assignments := stripPrefixes(lexed.words)
	if n := len(p.lists); n > 0 && len(p.lists[n-1].segments) > 0 {
		for _, w := range lexed.words[:len(lexed.words)-len(words)] {
			if !IsAssignment(w.Text) && !isOpenerKeyword(w, dialect) {
				p.lists[n-1].segments[len(p.lists[n-1].segments)-1].foreignPrefix = true
			}
		}
	}
	if len(words) == 0 {
		p.noteStrayWrites(lexed.redirects)
		p.result.BareAssignments = append(p.result.BareAssignments, assignments...)
		for range assignments {
			p.bareAt = append(p.bareAt, len(p.result.Invocations))
		}
		if p.environmentDepth > 0 {
			for i, word := range assignments {
				if p.ctx.Err() != nil {
					break
				}
				p.result.EnvironmentEffects = append(p.result.EnvironmentEffects, EnvironmentEffect{Order: i, Dialect: dialect, Kind: EnvironmentAssignment, Word: word})
			}
		}
		return
	}
	base := Invocation{
		EnvironmentResets:  append([]EnvironmentReset(nil), p.environmentResets...),
		Dialect:            dialect,
		Words:              words,
		Wrappers:           append([]Word(nil), wrappers...),
		WrapperIDs:         append([]int(nil), p.wrapperIDs...),
		Assignments:        append([]Word(nil), assignments...),
		InputRedirects:     append([]Redirection(nil), lexed.input...),
		OutputRedirects:    writeRedirects(lexed.redirects),
		Before:             before,
		Parent:             parent,
		Raw:                raw,
		Inspectable:        true,
		Complete:           complete,
		PipelineInput:      pipelineInput,
		PipelineOutput:     pipelineOutput,
		PipelineInputEdge:  pipelineInputEdge,
		PipelineOutputEdge: pipelineOutputEdge,
	}
	out := base.OutputRedirects
	if dialect == PowerShell {
		base.EnvironmentCall = environmentCallMetadata(p.ctx, raw, words)
	}
	if lexed.refusal.Refused() {
		base.Inspectable = false
		base.Reason = lexed.refusal
		p.result.Invocations = append(p.result.Invocations, base)
		return
	}
	exe := strings.ToLower(ExecutableName(words[0].Text))
	var reading wrapperReading
	if isWrapper(exe) {
		for i, word := range assignments {
			if p.ctx.Err() != nil {
				break
			}
			p.result.EnvironmentEffects = append(p.result.EnvironmentEffects, EnvironmentEffect{Order: i, Dialect: dialect, Kind: EnvironmentAssignment, Word: word})
		}
		// Only a literal env is the env program: only it numbers an occurrence, and only its reading's effects are
		// taken, by the env block below.
		if exe == "env" && words[0].Literal {
			p.nextEnvironmentID++
		}
		reading = readWrapper(p.ctx, exe, words, dialect, p.nextEnvironmentID)
	}
	var ownResets []EnvironmentReset
	if exe == "env" && words[0].Literal {
		previous := p.environmentResets
		effects := reading.effects
		p.result.EnvironmentEffects = append(p.result.EnvironmentEffects, effects...)
		var resets []EnvironmentReset
		for _, effect := range effects {
			if p.ctx.Err() != nil {
				break
			}
			if effect.Kind != EnvironmentAssignment {
				resets = append(resets, EnvironmentReset{Occurrence: effect.Occurrence, Order: effect.Order, Dialect: effect.Dialect, All: effect.Kind == EnvironmentResetAll, Name: effect.Word})
			}
		}
		ownResets = resets
		p.environmentResets = append(append([]EnvironmentReset(nil), previous...), resets...)
		base.EnvironmentResets = append([]EnvironmentReset(nil), p.environmentResets...)
		p.environmentDepth++
		defer func() { p.environmentResets = previous; p.environmentDepth-- }()
	}
	// bash finds the source and . builtins by their exact name only, so /x/source or SOURCE is another program and is
	// kept as an invocation of its own, and so is a source behind a wrapper such as env, which starts a program. A
	// literal source of a .sh file is expanded only inside a script; a typed one keeps its existing handling.
	//
	// Only an operand with a / names a file by path. bash looks a slash-less operand up in PATH before the working
	// directory (sourcepath is on by default), and sh, in POSIX mode, never looks in the working directory, so the
	// file that runs may not be the one next to the caller. Such a source stays an invocation and is labeled
	// script-opaque:source; a backslash is not taken as a separator either.
	if n := len(p.lists); n > 0 && len(p.lists[n-1].segments) > 0 && len(wrappers) == p.lists[n-1].depth &&
		words[0].Literal && (words[0].Text == "source" || words[0].Text == ".") && len(words) >= 2 && words[1].Literal &&
		strings.Contains(words[1].Text, "/") && len(p.script.chain) > 0 && p.script.opts.Reader != nil {
		// The sourced text runs in this shell, but its list is gone once it is parsed, so a later call cannot replay
		// its cds; the mark makes the replay give up after this segment.
		p.lists[n-1].segments[len(p.lists[n-1].segments)-1].sourcedFile = true
		start, callStart := len(p.result.Invocations), len(p.result.ScriptCalls)
		reason := p.expandScript(scriptForm{path: words[1], args: words[2:]}, wrappers, nil, complete, before, parent, depth)
		if reason == RuleNone {
			p.inheritCallContext(start, assignments, lexed.input, writeRedirects(lexed.redirects), pipelineInput, pipelineOutput, pipelineInputEdge, pipelineOutputEdge)
			p.inheritCallOutput(callStart, out)
			p.noteCallPrefix(start, raw, assignments, nil, wrappers, p.wrapperIDs)
			return
		}
		if reason != RuleExtension {
			base.Inspectable, base.Reason = false, ruleRefusal(reason, nil)
			p.result.Invocations = append(p.result.Invocations, base)
			return
		}
		// A source of a file that is not a .sh script stays an invocation and is labeled script-opaque:source.
	}
	if !isWrapper(exe) {
		p.result.Invocations = append(p.result.Invocations, base)
		return
	}
	if depth >= maxWrapperDepth {
		base.Inspectable = false
		base.Reason = formRefusal(formWrapperDepth, exe)
		if p.script.opts.Reader != nil && words[0].Literal && (exe == "sh" || exe == "bash") {
			// A script call past the bound gets the same named reason as a source nest past it, which expandScript
			// checks, unless the tool context already rules out every script call.
			if _, isScript, _ := parseScriptForm(words); isScript {
				base.Reason = ruleRefusal(RuleDepth, nil)
				if !p.script.opts.Context.AllowsNewProof() {
					base.Reason = ruleRefusal(RuleContextUnqualified, nil)
				}
			}
		}
		p.result.Invocations = append(p.result.Invocations, base)
		return
	}
	if dialect == POSIX && words[0].Raw == "bash" && words[0].Text == "bash" && words[0].Literal &&
		len(wrappers) == 0 && len(lexed.redirects) == 0 && complete && p.ctx.Err() == nil {
		if bashMetadataOnly(words) {
			base.Inspection = BashMetadata
		} else if len(words) == 3 && words[1].Literal && words[1].Text == "-n" && words[2].Literal && words[2].Text != "" && words[2].Text != "-" && !strings.HasPrefix(words[2].Text, "-") {
			base.Inspection, base.SyntaxOperand = BashSyntaxRead, &words[2]
		} else if len(words) == 4 && words[1].Literal && words[1].Text == "-n" && words[2].Literal && words[2].Text == "--" && words[3].Literal && words[3].Text != "" && words[3].Text != "-" {
			base.Inspection, base.SyntaxOperand = BashSyntaxRead, &words[3]
		}
		if base.Inspection != NoInspection && bashInspectionSource(p.ctx, raw) {
			p.result.Invocations = append(p.result.Invocations, base)
			return
		}
		base.Inspection, base.SyntaxOperand = NoInspection, nil
	}
	if p.script.opts.Reader != nil && words[0].Literal && (exe == "sh" || exe == "bash") {
		if form, isScript, reason := parseScriptForm(words); isScript {
			start, callStart := len(p.result.Invocations), len(p.result.ScriptCalls)
			// An unqualified tool context outranks every form problem: without
			// it no script call can be proven, whatever its options.
			if !p.script.opts.Context.AllowsNewProof() {
				reason = RuleContextUnqualified
			} else if reason == RuleNone {
				form.fed = p.ownBefore(before) == Pipe || pipelineInput || len(lexed.input) > 0 || rawRedirectsStdin(raw, dialect)
				leave := p.enterCall(assignments, false)
				reason = p.expandScript(form, wrappers, &words[0], complete, before, parent, depth)
				leave()
			}
			if reason != RuleNone {
				base.Inspectable, base.Reason = false, ruleRefusal(reason, nil)
				p.result.Invocations = append(p.result.Invocations, base)
				return
			}
			p.inheritCallContext(start, assignments, lexed.input, writeRedirects(lexed.redirects), pipelineInput, pipelineOutput, pipelineInputEdge, pipelineOutputEdge)
			p.inheritCallOutput(callStart, out)
			p.noteCallPrefix(start, raw, assignments, nil, wrappers, p.wrapperIDs)
			return
		}
	}
	childWords, childAssignments, childDialect, commandString, childComplete, inspectionOnly, reason := reading.child, reading.assignments, reading.dialect, reading.commandString, reading.complete, reading.inspectionOnly, reading.refusal
	if reason.Refused() {
		base.Inspectable = false
		base.Reason = reason
		p.result.Invocations = append(p.result.Invocations, base)
		return
	}
	if inspectionOnly {
		return
	}
	nextWrappers := append(append([]Word(nil), wrappers...), words[0])
	p.nextWrapperID++
	p.wrapperIDs = append(p.wrapperIDs, p.nextWrapperID)
	defer func() { p.wrapperIDs = p.wrapperIDs[:len(p.wrapperIDs)-1] }()
	typedCall := p.script.opts.Reader != nil && len(p.script.chain) == 0
	fed := typedCall && (before == Pipe || pipelineInput || len(lexed.input) > 0 || rawRedirectsStdin(raw, dialect))
	defer p.enterCall(append(slices.Clone(assignments), childAssignments...), fed)()
	if commandString != nil {
		if !commandString.Literal {
			base.Inspectable = false
			base.Reason = formRefusal(formCommandString, exe)
			p.result.Invocations = append(p.result.Invocations, base)
			return
		}
		start, callStart := len(p.result.Invocations), len(p.result.ScriptCalls)
		p.parseCommand(commandString.Text, childDialect, nextWrappers, complete && childComplete, before, parent, depth+1)
		p.inheritCallOutput(callStart, out)
		for i := start; i < len(p.result.Invocations); i++ {
			p.result.Invocations[i].PipelineInput = p.result.Invocations[i].PipelineInput || pipelineInput
			p.result.Invocations[i].PipelineOutput = p.result.Invocations[i].PipelineOutput || pipelineOutput
			if pipelineInputEdge != 0 && p.result.Invocations[i].PipelineInputEdge == 0 {
				p.result.Invocations[i].PipelineInputEdge = pipelineInputEdge
			}
			if pipelineOutputEdge != 0 && p.result.Invocations[i].PipelineOutputEdge == 0 {
				p.result.Invocations[i].PipelineOutputEdge = pipelineOutputEdge
			}
			p.result.Invocations[i].InputRedirects = append(slices.Clip(p.result.Invocations[i].InputRedirects), lexed.input...)
			p.result.Invocations[i].OutputRedirects = append(slices.Clip(p.result.Invocations[i].OutputRedirects), out...)
			p.result.Invocations[i].Assignments = append(append(append([]Word(nil), assignments...), childAssignments...), p.result.Invocations[i].Assignments...)
		}
		p.noteCallPrefix(start, raw, append(append([]Word(nil), assignments...), childAssignments...), ownResets, wrappers, p.wrapperIDs[:len(p.wrapperIDs)-1])
		return
	}
	childRaw := joinRaw(childWords)
	start, callStart := len(p.result.Invocations), len(p.result.ScriptCalls)
	p.parseSegment(childRaw, childDialect, nextWrappers, complete && childComplete, before, parent, depth+1, pipelineInput, pipelineOutput, pipelineInputEdge, pipelineOutputEdge)
	p.inheritCallOutput(callStart, out)
	for i := start; i < len(p.result.Invocations); i++ {
		p.result.Invocations[i].Assignments = append(append(append([]Word(nil), assignments...), childAssignments...), p.result.Invocations[i].Assignments...)
		p.result.Invocations[i].InputRedirects = append(slices.Clip(p.result.Invocations[i].InputRedirects), lexed.input...)
		p.result.Invocations[i].OutputRedirects = append(slices.Clip(p.result.Invocations[i].OutputRedirects), out...)
	}
	p.noteCallPrefix(start, raw, append(append([]Word(nil), assignments...), childAssignments...), ownResets, wrappers, p.wrapperIDs[:len(p.wrapperIDs)-1])
}

// inheritCallOutput gives the script calls recorded from index start on, the call just expanded and those under it, the
// write redirects of the script call or wrapper they run under, as their invocations get them. A script that runs no
// command leaves no invocation to carry the redirect, and the shell still opens the target.
func (p *parser) inheritCallOutput(start int, output []Redirection) {
	if len(output) == 0 {
		return
	}
	for i := start; i < len(p.result.ScriptCalls); i++ {
		call := &p.result.ScriptCalls[i]
		call.OutputRedirects = append(slices.Clip(call.OutputRedirects), output...)
	}
}

// inheritCallContext gives the children of an expanded script call, from index start on, the pipeline position,
// input redirects and assignment prefixes of the call: the script's commands read and write what the call does.
func (p *parser) inheritCallContext(start int, assignments []Word, input, output []Redirection, pipelineInput, pipelineOutput bool, pipelineInputEdge, pipelineOutputEdge int) {
	for i := start; i < len(p.result.Invocations); i++ {
		inv := &p.result.Invocations[i]
		inv.PipelineInput = inv.PipelineInput || pipelineInput
		inv.PipelineOutput = inv.PipelineOutput || pipelineOutput
		if pipelineInputEdge != 0 && inv.PipelineInputEdge == 0 {
			inv.PipelineInputEdge = pipelineInputEdge
		}
		if pipelineOutputEdge != 0 && inv.PipelineOutputEdge == 0 {
			inv.PipelineOutputEdge = pipelineOutputEdge
		}
		inv.InputRedirects = append(slices.Clip(inv.InputRedirects), input...)
		inv.OutputRedirects = append(slices.Clip(inv.OutputRedirects), output...)
		inv.Assignments = append(append([]Word(nil), assignments...), inv.Assignments...)
	}
}

func bashMetadataOnly(words []Word) bool {
	return len(words) == 2 && words[1].Literal && (words[1].Text == "--version" || words[1].Text == "--help")
}

// bashInspectionSource excludes active pathname/brace/tilde expansion while
// preserving quoted file names. Polling spans whitespace as well as operands.
func bashInspectionSource(ctx context.Context, raw string) bool {
	var quote byte
	nextCheck := 0
	for i := 0; i < len(raw); i++ {
		if i >= nextCheck {
			if ctx.Err() != nil {
				return false
			}
			nextCheck = i + (64 << 10)
		}
		c := raw[i]
		if c == '\\' && quote != '\'' {
			i++
			continue
		}
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			continue
		}
		if c == '\'' || c == '"' {
			quote = c
			continue
		}
		switch c {
		case '*', '?', '[', '{', '}', '~':
			return false
		}
	}
	return quote == 0 && ctx.Err() == nil
}

func stripPrefixes(words []Word) ([]Word, []Word) {
	i := 0
	var assignments []Word
	for i < len(words) {
		text := strings.ToLower(words[i].Text)
		if IsAssignment(words[i].Text) {
			assignments = append(assignments, words[i])
			i++
			continue
		}
		if isSegmentOpener(text) || strings.HasPrefix(text, "-") {
			i++
			continue
		}
		break
	}
	return words[i:], assignments
}

func isSegmentOpener(word string) bool {
	switch word {
	case "if", "then", "elif", "else", "do", "while", "until", "!":
		return true
	default:
		return false
	}
}

// isOpenerKeyword reports whether w, skipped by stripPrefixes, is the keyword the shell reads it as: unquoted, and in
// bash spelled in lower case.
func isOpenerKeyword(w Word, dialect Dialect) bool {
	if w.Raw != w.Text {
		return false
	}
	if dialect == PowerShell {
		return isSegmentOpener(strings.ToLower(w.Text))
	}
	return isSegmentOpener(w.Text)
}

// isWrapper reports a command the parser looks through to the command it runs. The set is the script proof's own
// because it is lexing knowledge: it decides how the parser splits one invocation into the wrapper and the wrapped
// command, which the shared command vocabulary does not describe. parallel and watch are not in it, although the
// vocabulary marks them as running text they are handed. The wrappers table is the roster.
func isWrapper(exe string) bool {
	_, ok := wrappers[exe]
	return ok
}

// looksLikeDuration reports whether s has the shape of a GNU coreutils
// timeout DURATION: digits, an optional single decimal point, and an
// optional single-letter suffix (s, m, h, or d). It rejects arbitrary
// executables (e.g. "kubectl") that would otherwise slip into the duration
// slot and shift a real executable into a false "child" position.
func looksLikeDuration(s string) bool {
	if s == "" {
		return false
	}
	numPart := s
	switch s[len(s)-1] {
	case 's', 'm', 'h', 'd':
		numPart = s[:len(s)-1]
	}
	if numPart == "" {
		return false
	}
	dot := false
	for i := 0; i < len(numPart); i++ {
		c := numPart[i]
		if c == '.' {
			if dot {
				return false
			}
			dot = true
			continue
		}
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func joinRaw(words []Word) string {
	parts := make([]string, len(words))
	for i, word := range words {
		parts[i] = word.Raw
	}
	return strings.Join(parts, " ")
}

// ExecutableName returns a path-independent, case-preserving executable name.
func ExecutableName(word string) string {
	if slash := strings.LastIndexAny(word, `/\`); slash >= 0 {
		word = word[slash+1:]
	}
	if len(word) > 4 && strings.EqualFold(word[len(word)-4:], ".exe") {
		word = word[:len(word)-4]
	}
	return word
}

// IsAssignment reports whether word is a literal NAME=value command prefix.
func IsAssignment(word string) bool {
	eq := strings.IndexByte(word, '=')
	if eq <= 0 {
		return false
	}
	for i := 0; i < eq; i++ {
		c := word[i]
		letter := c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
		digit := c >= '0' && c <= '9'
		if !letter && !(i > 0 && digit) {
			return false
		}
	}
	return true
}

func lexSegment(segment string, dialect Dialect) lexedSegment {
	escape := byte('\\')
	if dialect == PowerShell {
		escape = '`'
	}
	var out lexedSegment
	out.words = make([]Word, 0, min(strings.Count(segment, " ")+1, reserveCap))
	var text, raw strings.Builder
	started, single, double, quoted, literal := false, false, false, false, true
	substituted := false
	pendingFD, pendingOp := -1, ""
	flush := func() {
		if !started {
			return
		}
		word := Word{Text: text.String(), Raw: raw.String(), Quoted: quoted, Literal: literal, Substituted: substituted}
		if pendingOp != "" {
			out.redirects = append(out.redirects, Redirection{FD: pendingFD, Operator: pendingOp, Target: word})
			if pendingOp == "<" || pendingOp == "<>" {
				out.input = append(out.input, Redirection{FD: pendingFD, Operator: pendingOp, Target: word})
			}
			pendingFD, pendingOp = -1, ""
		} else {
			out.words = append(out.words, word)
		}
		text.Reset()
		raw.Reset()
		started, quoted, literal = false, false, true
		substituted = false
	}
	for i := 0; i < len(segment); i++ {
		c := segment[i]
		if single {
			raw.WriteByte(c)
			if c == '\'' {
				single = false
			} else {
				text.WriteByte(c)
			}
			continue
		}
		if double {
			raw.WriteByte(c)
			switch c {
			case '"':
				double = false
			case escape:
				if i+1 < len(segment) && (dialect == PowerShell || strings.ContainsRune("$`\"\\\n", rune(segment[i+1]))) {
					i++
					raw.WriteByte(segment[i])
					if segment[i] != '\n' {
						text.WriteByte(segment[i])
					}
				} else {
					text.WriteByte(c)
				}
			case '$', '`':
				literal = false
				text.WriteByte(c)
				// A command substitution embedded in this double-quoted word
				// (echo "$(rm -rf /)") can itself carry nested double-quoted
				// text (echo "$(echo "x")"). Copy the whole substitution span
				// verbatim rather than toggling double per quote character it
				// contains, or a nested quote would be mistaken for the close
				// of this word's own outer quote and split the word wrong.
				if c == '$' && i+1 < len(segment) && segment[i+1] == '(' {
					substituted = true
					if end := heredocDollarParenEnd(segment, i); end > i {
						span := segment[i+1 : end]
						text.WriteString(span)
						raw.WriteString(span)
						i = end - 1
					}
				} else if c == '`' {
					substituted = true
					if end := heredocBacktickEnd(segment, i); end > i {
						span := segment[i+1 : end]
						text.WriteString(span)
						raw.WriteString(span)
						i = end - 1
					}
				}
			default:
				text.WriteByte(c)
			}
			continue
		}
		if c == escape {
			raw.WriteByte(c)
			if i+1 < len(segment) {
				i++
				raw.WriteByte(segment[i])
				if segment[i] != '\n' {
					started = true
					text.WriteByte(segment[i])
				}
			} else {
				started = true
			}
			continue
		}
		if dialect == POSIX && c == '$' && i+1 < len(segment) && segment[i+1] == '\'' {
			end := ansiCEnd(segment, i)
			span := segment[i:end]
			decoded, refusal := decodeANSI(span)
			started, quoted = true, true
			text.WriteString(decoded)
			raw.WriteString(span)
			if refusal.Refused() {
				out.refusal = refusal
			}
			i = end - 1
			continue
		}
		switch c {
		case ' ', '\t', '\r', '\n':
			flush()
		case '\'':
			started, single, quoted = true, true, true
			raw.WriteByte(c)
		case '"':
			started, double, quoted = true, true, true
			raw.WriteByte(c)
		case '$', '`':
			started, literal = true, false
			text.WriteByte(c)
			raw.WriteByte(c)
			end := -1
			if c == '$' && i+1 < len(segment) && segment[i+1] == '(' {
				substituted = true
				end = heredocDollarParenEnd(segment, i)
			} else if c == '`' && dialect == POSIX {
				substituted = true
				end = heredocBacktickEnd(segment, i)
			}
			if end > i {
				text.WriteString(segment[i+1 : end])
				raw.WriteString(segment[i+1 : end])
				i = end - 1
			}
		case '<', '>':
			fd := -1
			if started && !quoted {
				if n, err := strconv.Atoi(text.String()); err == nil {
					fd = n
					text.Reset()
					raw.Reset()
					started = false
				} else {
					flush()
				}
			} else {
				flush()
			}
			op := segment[i : i+1]
			if i+1 < len(segment) {
				next := segment[i+1]
				if next == c || c == '<' && (next == '>' || next == '&' || next == '(') || c == '>' && (next == '&' || next == '|') {
					i++
					op = segment[i-1 : i+1]
					if op == "<<" && i+1 < len(segment) && segment[i+1] == '<' {
						i++
						op = segment[i-2 : i+1]
					}
				}
			}
			pendingFD, pendingOp = fd, op
		case '&':
			if i+1 < len(segment) && segment[i+1] == '>' {
				i++
				pendingFD, pendingOp = -1, ">"
				continue
			}
			started = true
			text.WriteByte(c)
			raw.WriteByte(c)
		default:
			started = true
			text.WriteByte(c)
			raw.WriteByte(c)
		}
	}
	flush()
	if single || double {
		out.refusal = formRefusal(formUnterminated)
	} else if pendingOp != "" {
		out.refusal = formRefusal(formRedirection, pendingOp)
	}
	if len(out.words) == 0 {
		out.words = nil
	}
	return out
}

func splitSegments(command string, dialect Dialect, nextPipelineEdge *int) []rawSegment {
	return splitSegmentsContext(context.Background(), command, dialect, nextPipelineEdge)
}

func splitSegmentsContext(ctx context.Context, command string, dialect Dialect, nextPipelineEdge *int) []rawSegment {
	type frame struct {
		closing           byte
		outerDouble       bool
		parentSegment     int
		groupID           int
		pipelineInput     bool
		pipelineInputEdge int
		// substitution marks an interrupted enclosing command.
		// reserved is the index of that word's segment, appended empty when the
		// substitution opened so child segments can name it as their parent
		// before its text is complete. outerText and openAt restore the
		// interrupted text when the substitution closes.
		substitution bool
		reserved     int
		openAt       int
		outerText    string
	}
	var segments []rawSegment
	var b strings.Builder
	inSingle, inDouble := false, false
	parameterDepth := 0
	braceDepth := 0
	before := Start
	parentSegment := -1
	wordStart := true
	onlyWhitespace := true
	nextGroupID := 0
	lastClosedGroupID := 0
	pendingPipelineEdge := 0
	pendingOutputEdge := 0
	pendingFill := -1
	pendingBraced := false
	pendingBackground, pendingStderrEdge := false, 0
	// groupAfterStderrPipe turns a |& whose consumer is a group or process substitution into a background mark, which
	// is the stricter of the two for the guards that read it.
	groupAfterStderrPipe := func() {
		if pendingStderrEdge != 0 {
			pendingStderrEdge, pendingBackground = 0, true
		}
	}
	var frames []frame
	activeGroup := func() (int, bool, int) {
		rootID := 0
		inputEdge := 0
		for i := len(frames) - 1; i >= 0; i-- {
			if frames[i].groupID != 0 {
				rootID = frames[i].groupID
				if frames[i].pipelineInputEdge != 0 && inputEdge == 0 {
					inputEdge = frames[i].pipelineInputEdge
				}
				continue
			}
			if frames[i].closing == ')' || frames[i].closing == '`' {
				return 0, false, 0
			}
		}
		return rootID, inputEdge != 0, inputEdge
	}
	// ownerNow is the segment the innermost substitution or process substitution being read belongs to.
	ownerNow := func() int {
		for i := len(frames) - 1; i >= 0; i-- {
			if frames[i].groupID == 0 {
				if frames[i].substitution {
					return frames[i].reserved
				}
				return frames[i].parentSegment
			}
		}
		return -1
	}
	emit := func(raw string, next Separator, reserve bool) int {
		index := -1
		if reserve || strings.TrimSpace(raw) != "" {
			if pendingFill >= 0 {
				// The rest of a command a substitution interrupted:
				// complete the segment reserved when the substitution opened.
				index = pendingFill
				pendingFill = -1
				segments[index].raw = raw
				if next == Pipe {
					segments[index].pipelineOutputEdge = pendingOutputEdge
				}
			} else {
				groupID, pipelineInput, groupInputEdge := activeGroup()
				inputEdge := groupInputEdge
				outputEdge := 0
				if before == Pipe || next == Pipe {
					if next == Pipe {
						if before == Pipe {
							inputEdge = pendingPipelineEdge
						}
						outputEdge = pendingOutputEdge
					} else {
						inputEdge = pendingPipelineEdge
					}
				} else if inputEdge == 0 {
					inputEdge = pendingPipelineEdge
				}
				segments = append(segments, rawSegment{raw: raw, before: before, parentSegment: parentSegment, groupID: groupID, pipelineInput: pipelineInput, pipelineInputEdge: inputEdge, pipelineOutputEdge: outputEdge, bracedWord: pendingBraced, owner: ownerNow(),
					afterBackground: pendingBackground, stderrPipeEdge: pendingStderrEdge})
				pendingBraced, pendingBackground, pendingStderrEdge = false, false, 0
				index = len(segments) - 1
			}
			if next != Pipe {
				pendingPipelineEdge = 0
				pendingOutputEdge = 0
			}
		} else if next == Sequence && (before == Pipe || before == And || before == Or || before == Background || before == Group || before == Substitution) {
			// A line break after a control operator continues that construct;
			// do not replace its edge while flushing comment/whitespace-only text.
			return index
		}
		if next == And || next == Or || next == Background {
			pendingPipelineEdge, pendingOutputEdge = 0, 0
		}
		if next == Background {
			pendingBackground = true
		}
		before, parentSegment = next, -1
		wordStart = true
		return index
	}
	flush := func(next Separator) int {
		raw := b.String()
		b.Reset()
		return emit(raw, next, false)
	}
	// openSubstitution preserves the enclosing command around a substitution.
	// The enclosing word keeps its segment: it is reserved now, empty, so the
	// children flushed before the word completes can name it as their parent,
	// and its text arrives at the word's own flush. A second substitution in
	// the same word reuses the reservation.
	openSubstitution := func(closing byte, at int) {
		reserved := pendingFill
		if reserved < 0 {
			reserved = emit("", Substitution, true)
		} else {
			before = Substitution
		}
		frames = append(frames, frame{closing: closing, outerDouble: inDouble, parentSegment: reserved, substitution: true, reserved: reserved, openAt: at, outerText: b.String()})
		b.Reset()
		pendingFill = -1
		parentSegment = reserved
		inDouble = false
		wordStart = true
	}
	// closeSubstitution resumes the interrupted word with the
	// substitution text in place, so lexSegment reads it as one non-literal
	// word.
	closeSubstitution := func(last frame, end int) {
		b.WriteString(last.outerText)
		b.WriteString(command[last.openAt : end+1])
		pendingFill = last.reserved
		wordStart = false
	}
	escape := byte('\\')
	if dialect == PowerShell {
		escape = '`'
	}
	for i := 0; i < len(command); i++ {
		c := command[i]
		if b.Len() == 0 {
			onlyWhitespace = true
		}
		atStart := onlyWhitespace
		if !strings.ContainsRune(" \t\r\n", rune(c)) {
			onlyWhitespace = false
		}
		if dialect == PowerShell && !inSingle && !inDouble && c == '[' && atStart {
			if end, _ := literalEnvironmentCall(ctx, command[i:]); end > 0 {
				after := i + end
				if after == len(command) || strings.ContainsRune(" \t\r\n;|&)", rune(command[after])) {
					b.WriteString(command[i:after])
					i = after - 1
					wordStart = false
					continue
				}
			}
		}
		if !inSingle && c == escape && i+1 < len(command) && (command[i+1] == '\n' || command[i+1] == '\r') {
			i++
			if command[i] == '\r' && i+1 < len(command) && command[i+1] == '\n' {
				i++
			}
			continue
		}
		if lastClosedGroupID != 0 && c != ' ' && c != '\t' && c != '\r' && c != '\n' && c != '#' && c != '|' {
			lastClosedGroupID = 0
		}
		if inSingle {
			b.WriteByte(c)
			if c == '\'' {
				inSingle = false
			}
			wordStart = false
			continue
		}
		if dialect == POSIX && !inDouble && c == '$' && i+1 < len(command) && command[i+1] == '\'' {
			end := ansiCEnd(command, i)
			b.WriteString(command[i:end])
			i = end - 1
			wordStart = false
			continue
		}
		if c == escape {
			b.WriteByte(c)
			if i+1 < len(command) {
				i++
				b.WriteByte(command[i])
			}
			wordStart = false
			continue
		}
		if c == '$' && i+1 < len(command) && command[i+1] == '(' {
			openSubstitution(')', i)
			i++
			continue
		}
		if !inDouble && i+1 < len(command) && (c == '<' || c == '>') && command[i+1] == '(' {
			if strings.TrimSpace(b.String()) == "" {
				groupAfterStderrPipe()
			}
			parent := flush(Group)
			frames = append(frames, frame{closing: ')', outerDouble: inDouble, parentSegment: parent})
			parentSegment = parent
			i++
			inDouble = false
			continue
		}
		if c == '$' && i+1 < len(command) && command[i+1] == '{' {
			parameterDepth++
			b.WriteString("${")
			wordStart = false
			i++
			continue
		}
		if c == ')' && !inDouble && len(frames) > 0 && frames[len(frames)-1].closing == ')' {
			flush(Group)
			last := frames[len(frames)-1]
			frames = frames[:len(frames)-1]
			lastClosedGroupID = last.groupID
			if last.pipelineInputEdge != 0 {
				pendingPipelineEdge = 0
			}
			inDouble = last.outerDouble
			if last.substitution {
				closeSubstitution(last, i)
			}
			// A substitution (groupID 0; groups and braces number themselves)
			// closes in the middle of a word: `$(x)#y` is one word in the shell.
			// A hash here is data, not a comment, or the rest of the line
			// would be swallowed and its commands never inspected.
			if last.groupID == 0 {
				wordStart = false
			}
			continue
		}
		if c == '`' && dialect == POSIX {
			if len(frames) > 0 && frames[len(frames)-1].closing == '`' {
				flush(Group)
				last := frames[len(frames)-1]
				frames = frames[:len(frames)-1]
				inDouble = last.outerDouble
				if last.substitution {
					closeSubstitution(last, i)
				}
				// Same word-continuation rule as the `)` close above.
				wordStart = false
			} else {
				openSubstitution('`', i)
			}
			continue
		}
		if c == '"' {
			inDouble = !inDouble
			b.WriteByte(c)
			wordStart = false
			continue
		}
		if !inDouble && c == '\'' {
			inSingle = true
			b.WriteByte(c)
			wordStart = false
			continue
		}
		// An unquoted hash at the start of a word begins a shell comment.
		// Keep the following newline for the normal sequence-boundary path;
		// quoted and escaped hashes remain command data.
		if !inDouble && c == '#' && wordStart {
			for i+1 < len(command) && command[i+1] != '\n' && command[i+1] != '\r' {
				i++
			}
			continue
		}
		if !inDouble && c == '}' && parameterDepth > 0 {
			parameterDepth--
			b.WriteByte(c)
			wordStart = false
			continue
		}
		if !inDouble && c == '(' {
			pipelineInput := before == Pipe
			pipelineInputEdge := pendingPipelineEdge
			if pipelineInput {
				for j := len(segments) - 1; j >= 0; j-- {
					if segments[j].groupID == 0 && segments[j].parentSegment < 0 {
						segments[j].pipelineOutput = true
						segments[j].pipelineOutputEdge = pipelineInputEdge
						break
					}
				}
			}
			defines := funcDefBefore(b.String())
			if strings.TrimSpace(b.String()) == "" {
				groupAfterStderrPipe()
			}
			parent := flush(Group)
			if defines && parent >= 0 {
				segments[parent].funcDef = true
			}
			nextGroupID++
			frames = append(frames, frame{closing: ')', outerDouble: false, parentSegment: parent, groupID: nextGroupID, pipelineInput: pipelineInput, pipelineInputEdge: pipelineInputEdge})
			parentSegment = parent
			continue
		}
		if !inDouble && c == ')' {
			flush(Group)
			if len(frames) > 0 && frames[len(frames)-1].closing == ')' {
				frames = frames[:len(frames)-1]
			}
			continue
		}
		if !inDouble && c == '{' && strings.TrimSpace(b.String()) == "" {
			pipelineInput := before == Pipe
			pipelineInputEdge := pendingPipelineEdge
			if pipelineInput {
				for j := len(segments) - 1; j >= 0; j-- {
					if segments[j].groupID == 0 && segments[j].parentSegment < 0 {
						segments[j].pipelineOutput = true
						segments[j].pipelineOutputEdge = pipelineInputEdge
						break
					}
				}
			}
			groupAfterStderrPipe()
			flush(Group)
			if dialect == POSIX && i+1 < len(command) && !strings.ContainsRune(" \t\r\n", rune(command[i+1])) {
				pendingBraced = true
			}
			nextGroupID++
			frames = append(frames, frame{closing: '}', groupID: nextGroupID, pipelineInput: pipelineInput, pipelineInputEdge: pipelineInputEdge})
			braceDepth++
			continue
		}
		if !inDouble && c == '}' && braceDepth > 0 {
			flush(Group)
			if len(frames) > 0 && frames[len(frames)-1].closing == '}' {
				lastClosedGroupID = frames[len(frames)-1].groupID
				if frames[len(frames)-1].pipelineInputEdge != 0 {
					pendingPipelineEdge = 0
				}
				frames = frames[:len(frames)-1]
			}
			braceDepth--
			continue
		}
		if !inDouble {
			switch c {
			case ' ', '\t':
				b.WriteByte(c)
				wordStart = true
				continue
			case ';', '\r', '\n':
				if c == ';' {
					lastClosedGroupID = 0
				}
				flush(Sequence)
				continue
			case '|':
				if i+1 < len(command) && command[i+1] == '|' {
					i++
					flush(Or)
				} else {
					(*nextPipelineEdge)++
					pendingOutputEdge = *nextPipelineEdge
					producerIndex := flush(Pipe)
					pendingPipelineEdge = pendingOutputEdge
					if lastClosedGroupID != 0 {
						for j := range segments {
							if segments[j].groupID == lastClosedGroupID {
								segments[j].pipelineOutput = true
								segments[j].pipelineOutputEdge = pendingPipelineEdge
							}
						}
						lastClosedGroupID = 0
					} else if producerIndex >= 0 {
						segments[producerIndex].pipelineOutput = true
						segments[producerIndex].pipelineOutputEdge = pendingPipelineEdge
					} else {
						for j := len(segments) - 1; j >= 0; j-- {
							if segments[j].groupID == 0 && segments[j].parentSegment < 0 {
								segments[j].pipelineOutput = true
								segments[j].pipelineOutputEdge = pendingPipelineEdge
								break
							}
						}
					}
					pendingOutputEdge = 0
				}
				continue
			case '&':
				if i+1 < len(command) && command[i+1] == '>' {
					b.WriteString("&>")
					wordStart = false
					i++
					continue
				}
				// Keep fd-duplication redirections (2>&1, >&-) in the
				// surrounding invocation. A redirection is not a command
				// boundary; splitting here would make the prefix look
				// malformed and hide its executable from policy checks.
				if i > 0 && command[i-1] == '>' {
					b.WriteByte(c)
					continue
				}
				if i+1 < len(command) && command[i+1] == '&' {
					i++
					lastClosedGroupID = 0
					flush(And)
				} else if i > 0 && command[i-1] == '|' && pendingPipelineEdge != 0 && strings.TrimSpace(b.String()) == "" {
					// |& pipes standard error too. It stays a Background separator for the other guards, and the next
					// command records the edge it reads.
					edge := pendingPipelineEdge
					flush(Background)
					pendingBackground, pendingStderrEdge = false, edge
				} else {
					flush(Background)
				}
				continue
			}
		}
		wordStart = false
		b.WriteByte(c)
	}
	flush(Sequence)
	// A substitution that never closes leaves its word's segment
	// reserved and empty. Restore the text so the segment lexes as the
	// unterminated word it is and the guards keep failing closed on it.
	for len(frames) > 0 {
		last := frames[len(frames)-1]
		frames = frames[:len(frames)-1]
		if last.substitution && segments[last.reserved].raw == "" {
			segments[last.reserved].raw = last.outerText + command[last.openAt:]
		}
	}
	return segments
}

// stripHeredocBodies removes shell data regions before executable segments
// are split. The command line that declares each heredoc remains inspectable.
func stripHeredocBodies(command string) string {
	// Every parse calls this, and a text without << opens no heredoc, so it skips the extents walk.
	if !strings.Contains(command, "<<") {
		return command
	}
	return readHeredocExtents(command).stripBodies(command)
}

// maskHeredocData preserves only executable substitution spans from an
// unquoted heredoc. The remaining body is data, even when another line in the
// same heredoc contains a substitution that must be inspected.
func maskHeredocData(line string) string {
	masked := []byte(line)
	for i, c := range masked {
		if c != '\n' && c != '\r' {
			masked[i] = ' '
		}
	}
	for i := 0; i < len(line); {
		start, end := -1, -1
		if line[i] == '\\' && i+1 < len(line) {
			i += 2
			continue
		}
		if i+1 < len(line) && line[i] == '$' && line[i+1] == '(' {
			start, end = i, heredocDollarParenEnd(line, i)
		} else if line[i] == '`' {
			start, end = i, heredocBacktickEnd(line, i)
		}
		if start < 0 {
			i++
			continue
		}
		if end <= start {
			end = len(line)
		}
		copy(masked[start:end], line[start:end])
		// A heredoc expands command output into data; it does not execute
		// that output. Retain only a group containing the inner commands.
		if line[start] == '$' {
			masked[start] = ' '
		} else {
			masked[start] = '('
			if line[end-1] == '`' {
				masked[end-1] = ')'
			}
		}
		i = end
	}
	return string(masked)
}

func heredocBacktickEnd(text string, start int) int {
	for i := start + 1; i < len(text); i++ {
		if text[i] == '\\' && i+1 < len(text) {
			i++
			continue
		}
		if text[i] == '`' {
			return i + 1
		}
	}
	return -1
}

func heredocDollarParenEnd(text string, start int) int {
	type frame struct {
		depth              int
		inSingle, inDouble bool
		inBacktick         bool
		wordStart          bool
	}
	frames := []frame{{depth: 1, wordStart: true}}
	for i := start + 2; i < len(text); i++ {
		c := text[i]
		current := &frames[len(frames)-1]
		if current.inSingle {
			if c == '\'' {
				current.inSingle = false
			}
			continue
		}
		if current.inBacktick {
			if c == '\\' && i+1 < len(text) {
				i++
				continue
			}
			if c == '`' {
				current.inBacktick = false
			}
			continue
		}
		if c == '\\' && i+1 < len(text) {
			i++
			current.wordStart = false
			continue
		}
		if current.inDouble {
			if c == '"' {
				current.inDouble = false
			}
			if c == '$' && i+1 < len(text) && text[i+1] == '(' {
				frames = append(frames, frame{depth: 1, wordStart: true})
				i++
			}
			continue
		}
		if c == '$' && i+1 < len(text) && text[i+1] == '\'' {
			i = ansiCEnd(text, i) - 1
			current.wordStart = false
			continue
		}
		// An unquoted hash at the start of a word begins a shell comment
		// that runs to the end of the line; a ')' inside it is comment
		// data, not the substitution's close. This mirrors splitSegments'
		// comment handling so both scanners agree on where a substitution
		// ends, even when the comment body contains a stray ')'.
		if c == '#' && current.wordStart {
			for i+1 < len(text) && text[i+1] != '\n' {
				i++
			}
			current.wordStart = false
			continue
		}
		switch c {
		case ' ', '\t', '\n':
			current.wordStart = true
		case '\'':
			current.inSingle = true
			current.wordStart = false
		case '"':
			current.inDouble = true
			current.wordStart = false
		case '`':
			current.inBacktick = true
			current.wordStart = false
		case '$':
			current.wordStart = false
			if i+1 < len(text) && text[i+1] == '(' {
				frames = append(frames, frame{depth: 1, wordStart: true})
				i++
			}
		case '(':
			current.depth++
			current.wordStart = false
		case ')':
			current.depth--
			if current.depth == 0 {
				frames = frames[:len(frames)-1]
				if len(frames) == 0 {
					return i + 1
				}
				frames[len(frames)-1].wordStart = false
			} else {
				current.wordStart = false
			}
		default:
			current.wordStart = false
		}
	}
	return -1
}
