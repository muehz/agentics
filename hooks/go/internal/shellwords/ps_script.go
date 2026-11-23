package shellwords

import (
	"context"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/muehz/agentics/hooks/go/internal/argv"
)

type psParseMode uint8

const (
	psModeTyped psParseMode = iota
	psModeScript
)

type psOpaqueKind uint8

const (
	psOpaqueNone psOpaqueKind = iota
	psOpaqueEval
	psOpaqueDotNetCall
	psOpaqueDynamicCall
	psOpaqueCommandExpansion
	psOpaqueTrap
	psOpaqueAlias
	psOpaqueFunctionShadow
	psOpaquePinnedAssignment
	psOpaqueSource
	psOpaqueOtherShell
	psOpaqueBatchFile
	psOpaqueStdinToShell
	psOpaqueDynamicC
	psOpaqueNestingDepth
	psOpaqueUnsupportedSyntax
)

type psCallKind uint8

const (
	psCallNone psCallKind = iota
	psCallAmpersand
	psCallDot
)

type psInvocationMeta struct {
	CallKind       psCallKind
	ModuleStripped bool
	RawCommand     string
	Opaque         psOpaqueKind
	Line           int
}

type psEnvMutation struct {
	Name         string
	LiteralName  bool
	Value        string
	LiteralValue bool
	Line         int
	Pinned       bool
	At           int // Invocation count when the mutation is recorded.
}

// psOpacity keeps both the earliest diagnostic and the earliest structural
// refusal. Resolving a module-qualified word to a file can revoke a command
// diagnostic, but does not affect the enclosing script's syntax.
type psOpacity struct {
	Opaque         psOpaqueKind
	OpaqueLine     int
	Structural     psOpaqueKind
	StructuralLine int
}

type psParseResult struct {
	psOpacity
	Invocations     []Invocation
	Meta            []psInvocationMeta
	BareAssignments []Word
	BareAt          []int // Invocation counts aligned with BareAssignments.
	EnvMutations    []psEnvMutation
	ConsumesParams  bool
	UnquotedParams  bool
	Canceled        bool
}

var psPinnedEnvNames = map[string]bool{
	"path":         true,
	"psmodulepath": true,
	"pathext":      true,
	"comspec":      true,
	"home":         true,
	"userprofile":  true,
	"tmpdir":       true,
	"temp":         true,
	"tmp":          true,
	"node_options": true,
	"pythonpath":   true,
	"bash_env":     true,
	"env":          true,
	"shell":        true,
	"shellopts":    true,
	"bashopts":     true,
	"ps4":          true,
	"cdpath":       true,
	"bash_cmds":    true,
	"bash_aliases": true,
}

var psPinnedEnvPrefixes = [...]string{
	"git_",
	"ld_",
	"dyld_",
	"xdg_",
	"bash_func_",
}

var psLocationShadowNames = map[string]bool{
	"set-location":  true,
	"sl":            true,
	"chdir":         true,
	"push-location": true,
	"pop-location":  true,
}

var psOtherShellExes = map[string]bool{
	"zsh":  true,
	"fish": true,
	"csh":  true,
	"tcsh": true,
}

var psLOLBinExes = map[string]bool{
	"wscript":  true,
	"cscript":  true,
	"mshta":    true,
	"rundll32": true,
	"regsvr32": true,
}

var psBinaryDashOps = map[string]bool{
	"-and":          true,
	"-or":           true,
	"-xor":          true,
	"-band":         true,
	"-bor":          true,
	"-bxor":         true,
	"-shl":          true,
	"-shr":          true,
	"-f":            true,
	"-join":         true,
	"-split":        true,
	"-isplit":       true,
	"-csplit":       true,
	"-replace":      true,
	"-ireplace":     true,
	"-creplace":     true,
	"-is":           true,
	"-isnot":        true,
	"-as":           true,
	"-eq":           true,
	"-ieq":          true,
	"-ceq":          true,
	"-ne":           true,
	"-ine":          true,
	"-cne":          true,
	"-gt":           true,
	"-igt":          true,
	"-cgt":          true,
	"-ge":           true,
	"-ige":          true,
	"-cge":          true,
	"-lt":           true,
	"-ilt":          true,
	"-clt":          true,
	"-le":           true,
	"-ile":          true,
	"-cle":          true,
	"-like":         true,
	"-ilike":        true,
	"-clike":        true,
	"-notlike":      true,
	"-inotlike":     true,
	"-cnotlike":     true,
	"-match":        true,
	"-imatch":       true,
	"-cmatch":       true,
	"-notmatch":     true,
	"-inotmatch":    true,
	"-cnotmatch":    true,
	"-contains":     true,
	"-icontains":    true,
	"-ccontains":    true,
	"-notcontains":  true,
	"-inotcontains": true,
	"-cnotcontains": true,
	"-in":           true,
	"-iin":          true,
	"-cin":          true,
	"-notin":        true,
	"-inotin":       true,
	"-cnotin":       true,
}

func isPSPinnedEnv(name string) bool {
	lower := asciiLower(name)
	if psPinnedEnvNames[lower] {
		return true
	}
	for _, p := range psPinnedEnvPrefixes {
		if strings.HasPrefix(lower, p) {
			return true
		}
	}
	return false
}

func isPSFunctionShadow(rawName string) bool {
	s := asciiLower(strings.TrimSpace(rawName))
	for _, scope := range []string{"global:", "script:", "local:", "private:"} {
		if strings.HasPrefix(s, scope) {
			s = s[len(scope):]
			break
		}
	}
	if strings.HasPrefix(s, "function:") {
		return true
	}
	if stripped, ok := stripBuiltinPSModulePrefix(s); ok {
		s = asciiLower(stripped)
	}
	preAlias := asciiLower(ExecutableName(s))
	canonical := preAlias
	if target, ok := psCanonicalAliases[preAlias]; ok {
		canonical = target
	}
	return argv.Lookup(preAlias).Shadow ||
		argv.Lookup(canonical).Shadow ||
		psLocationShadowNames[preAlias] ||
		psLocationShadowNames[canonical]
}

type psDeferredSpan struct {
	start     int
	end       int
	line      int
	kind      Separator
	stmtList  bool
	hash      bool
	synthRead *Word
}

type psParser struct {
	ctx               context.Context
	src               string
	mode              psParseMode
	origin            *ScriptOrigin
	nextPipeEdge      *int
	pos               int
	line              int
	steps             int
	depth             int
	inChildParamScope bool
	activeNamespaces  []string
	res               psParseResult
}

func parsePowerShell(ctx context.Context, src string, mode psParseMode, origin *ScriptOrigin, nextPipeEdge *int) psParseResult {
	if ctx == nil {
		ctx = context.Background()
	}
	localEdge := 0
	if nextPipeEdge == nil {
		nextPipeEdge = &localEdge
	}
	p := psParser{
		ctx:          ctx,
		src:          src,
		mode:         mode,
		origin:       origin,
		nextPipeEdge: nextPipeEdge,
		line:         1,
	}
	if ctx.Err() != nil {
		p.res.Canceled = true
		p.finalizeCanceled()
		return p.res
	}
	p.parseStatementList(0, -1, Start)
	if ctx.Err() != nil {
		p.res.Canceled = true
	}
	if p.res.Canceled {
		p.finalizeCanceled()
	}
	return p.res
}

func (p *psParser) finalizeCanceled() {
	for i := range p.res.Invocations {
		inv := &p.res.Invocations[i]
		inv.Inspection, inv.SyntaxOperand = NoInspection, nil
		if inv.EnvironmentCall != nil {
			inv.EnvironmentCall = &PowerShellEnvironmentCall{}
		}
		inv.Inspectable = false
		inv.Reason = cancelRefusal(PowerShell)
	}
	if len(p.res.Invocations) == 0 {
		p.res.Invocations = []Invocation{{
			Raw:         p.src,
			Dialect:     PowerShell,
			Inspectable: false,
			Reason:      cancelRefusal(PowerShell),
		}}
		p.res.Meta = []psInvocationMeta{{Line: 1}}
	}
}

func (p *psParser) step() bool {
	p.steps++
	if p.steps&255 == 0 && p.ctx.Err() != nil {
		p.res.Canceled = true
		return false
	}
	return !p.res.Canceled
}

func (p *psParser) enterDepth(line int) bool {
	if p.depth >= maxScriptNesting {
		p.noteOpaque(psOpaqueNestingDepth, line)
		return false
	}
	p.depth++
	return true
}

func (p *psParser) leaveDepth() {
	p.depth--
}

func (p *psParser) noteOpaque(kind psOpaqueKind, line int) {
	if kind == psOpaqueNone {
		return
	}
	if line <= 0 {
		line = p.line
	}
	if p.res.Structural == psOpaqueNone || line < p.res.StructuralLine {
		p.res.Structural, p.res.StructuralLine = kind, line
	}
	p.noteCommandOpaque(kind, line)
}

func (p *psParser) noteCommandOpaque(kind psOpaqueKind, line int) {
	if p.res.Opaque == psOpaqueNone || line < p.res.OpaqueLine {
		p.res.Opaque = kind
		p.res.OpaqueLine = line
	}
}

func (p *psParser) noteScriptOnlyOpaque(kind psOpaqueKind, line int) psOpaqueKind {
	if p.mode == psModeScript {
		p.noteOpaque(kind, line)
		return kind
	}
	return psOpaqueNone
}

func (p *psParser) makeOrigin(line int) *ScriptOrigin {
	if p.origin == nil {
		return nil
	}
	return &ScriptOrigin{
		Path:  p.origin.Path,
		Chain: append([]string(nil), p.origin.Chain...),
		Line:  line,
	}
}

func (p *psParser) appendInvocation(inv Invocation, meta psInvocationMeta) int {
	if meta.Line <= 0 {
		meta.Line = p.line
	}
	inv.Dialect = PowerShell
	inv.Inspectable = true
	inv.Reason = Refusal{}
	inv.Complete = true
	inv.Script = p.makeOrigin(meta.Line)
	if meta.Opaque != psOpaqueNone {
		p.noteCommandOpaque(meta.Opaque, meta.Line)
	}
	idx := len(p.res.Invocations)
	p.res.Invocations = append(p.res.Invocations, inv)
	p.res.Meta = append(p.res.Meta, meta)
	return idx
}

func (p *psParser) emitSyntheticGetContent(pathWord Word, line int, parent int, before Separator) {
	raw := "get-content -LiteralPath " + pathWord.Raw
	p.appendInvocation(Invocation{
		Words: []Word{
			{Text: "get-content", Raw: "get-content", Literal: true},
			{Text: "-LiteralPath", Raw: "-LiteralPath", Literal: true},
			pathWord,
		},
		Before: before,
		Parent: parent,
		Raw:    raw,
	}, psInvocationMeta{
		RawCommand: "get-content",
		Line:       line,
	})
}

func (p *psParser) atLineStart(pos int) bool {
	for i := pos - 1; i >= 0; i-- {
		if p.src[i] == '\n' || p.src[i] == '\r' {
			return true
		}
		if p.src[i] != ' ' && p.src[i] != '\t' {
			return false
		}
	}
	return true
}

func (p *psParser) checkRequiresDirective(comment string, line int) {
	rest := strings.TrimSpace(comment[1:])
	if len(rest) < len("requires") || !strings.EqualFold(rest[:len("requires")], "requires") {
		return
	}
	tail := rest[len("requires"):]
	if tail == "" || (tail[0] != ' ' && tail[0] != '\t') {
		return
	}
	fields := strings.Fields(tail)
	for i := 0; i < len(fields); i++ {
		f := asciiLower(fields[i])
		switch {
		case strings.HasPrefix("-assembly", f) && len(f) >= 2,
			strings.HasPrefix("-pssnapin", f) && len(f) >= 4:
			p.noteScriptOnlyOpaque(psOpaqueDotNetCall, line)
		case strings.HasPrefix("-modules", f) && len(f) >= 2:
			if i+1 >= len(fields) {
				p.noteOpaque(psOpaqueDynamicCall, line)
				return
			}
			i++
			modSpec := fields[i]
			if strings.ContainsAny(modSpec, `/\:@{}$'"*?`) || strings.HasPrefix(modSpec, ".") {
				p.noteOpaque(psOpaqueDynamicCall, line)
			}
			for _, part := range strings.Split(modSpec, ",") {
				part = strings.TrimSpace(part)
				lowerPart := asciiLower(part)
				if strings.HasSuffix(lowerPart, ".ps1") || strings.HasSuffix(lowerPart, ".psm1") ||
					strings.HasSuffix(lowerPart, ".psd1") || strings.HasSuffix(lowerPart, ".dll") {
					p.noteOpaque(psOpaqueDynamicCall, line)
				}
			}
		}
	}
}

func (p *psParser) skipTrivia(allowNewlines bool) {
	for p.pos < len(p.src) {
		if !p.step() {
			return
		}
		c := p.src[p.pos]
		switch c {
		case ' ', '\t', '\v', '\f':
			p.pos++
		case '`':
			if p.pos+1 < len(p.src) && p.src[p.pos+1] == '\n' {
				p.pos += 2
				p.line++
				continue
			}
			if p.pos+2 < len(p.src) && p.src[p.pos+1] == '\r' && p.src[p.pos+2] == '\n' {
				p.pos += 3
				p.line++
				continue
			}
			if p.pos+1 < len(p.src) && p.src[p.pos+1] == '\r' {
				p.pos += 2
				p.line++
				continue
			}
			return
		case '#':
			start := p.pos
			atStart := p.atLineStart(start)
			commentLine := p.line
			for p.pos < len(p.src) && p.src[p.pos] != '\n' && p.src[p.pos] != '\r' {
				p.pos++
			}
			if atStart {
				p.checkRequiresDirective(p.src[start:p.pos], commentLine)
			}
		case '<':
			if p.pos+1 < len(p.src) && p.src[p.pos+1] == '#' {
				openLine := p.line
				p.pos += 2
				closed := false
				for p.pos < len(p.src) {
					if !p.step() {
						return
					}
					if p.src[p.pos] == '\n' {
						p.line++
						p.pos++
						continue
					}
					if p.src[p.pos] == '\r' {
						if p.pos+1 >= len(p.src) || p.src[p.pos+1] != '\n' {
							p.line++
						}
						p.pos++
						continue
					}
					if p.src[p.pos] == '#' && p.pos+1 < len(p.src) && p.src[p.pos+1] == '>' {
						p.pos += 2
						closed = true
						break
					}
					p.pos++
				}
				if !closed {
					p.noteOpaque(psOpaqueUnsupportedSyntax, openLine)
					return
				}
				continue
			}
			return
		case '\r':
			if !allowNewlines {
				return
			}
			p.pos++
			if p.pos < len(p.src) && p.src[p.pos] == '\n' {
				p.pos++
			}
			p.line++
		case '\n':
			if !allowNewlines {
				return
			}
			p.pos++
			p.line++
		default:
			return
		}
	}
}

func (p *psParser) runDeferredSpan(sp psDeferredSpan, parent int) {
	if sp.synthRead != nil {
		p.emitSyntheticGetContent(*sp.synthRead, sp.line, parent, sp.kind)
		return
	}
	p.runSubParser(sp.start, sp.end, sp.line, parent, sp.kind, sp.stmtList, sp.hash)
}

func (p *psParser) runSubParser(start, end, startLine int, parent int, before Separator, stmtList bool, hash bool) {
	if !p.enterDepth(startLine) {
		return
	}
	defer p.leaveDepth()
	savedSrc, savedPos, savedLine := p.src, p.pos, p.line
	p.src = savedSrc[:end]
	p.pos = start
	p.line = startLine
	if hash {
		p.parseHashtableEntries(parent)
	} else if stmtList {
		p.parseStatementList(0, parent, before)
	} else {
		p.skipTrivia(true)
		if p.pos < len(p.src) {
			p.parseExpression(parent)
			p.skipTrivia(true)
			if p.pos < len(p.src) {
				p.noteOpaque(psOpaqueUnsupportedSyntax, p.line)
			}
		}
	}
	p.src, p.pos, p.line = savedSrc, savedPos, savedLine
}

// decodePSBacktick decodes one PowerShell backtick escape starting at idx (where p.src[idx] == '`').
func (p *psParser) decodePSBacktick(text *strings.Builder) (literalOK bool, ok bool) {
	openLine := p.line
	p.pos++ // skip '`'
	if p.pos >= len(p.src) {
		p.noteOpaque(psOpaqueUnsupportedSyntax, openLine)
		return false, false
	}
	c := p.src[p.pos]
	switch c {
	case '\n':
		p.pos++
		p.line++
		return true, true
	case '\r':
		p.pos++
		if p.pos < len(p.src) && p.src[p.pos] == '\n' {
			p.pos++
		}
		p.line++
		return true, true
	case '0':
		p.pos++
		text.WriteByte(0)
		return true, true
	case 'a':
		p.pos++
		text.WriteByte('\a')
		return true, true
	case 'b':
		p.pos++
		text.WriteByte('\b')
		return true, true
	case 'f':
		p.pos++
		text.WriteByte('\f')
		return true, true
	case 'n':
		p.pos++
		text.WriteByte('\n')
		return true, true
	case 'r':
		p.pos++
		text.WriteByte('\r')
		return true, true
	case 't':
		p.pos++
		text.WriteByte('\t')
		return true, true
	case 'v':
		p.pos++
		text.WriteByte('\v')
		return true, true
	case 'u':
		if p.pos+1 < len(p.src) && p.src[p.pos+1] == '{' {
			closeBrace := strings.IndexByte(p.src[p.pos+2:], '}')
			if closeBrace < 1 || closeBrace > 6 {
				p.noteOpaque(psOpaqueUnsupportedSyntax, openLine)
				p.pos++
				return false, false
			}
			hexStr := p.src[p.pos+2 : p.pos+2+closeBrace]
			val, err := strconv.ParseUint(hexStr, 16, 32)
			if err != nil || !utf8.ValidRune(rune(val)) {
				p.noteOpaque(psOpaqueUnsupportedSyntax, openLine)
				p.pos += 2 + closeBrace + 1
				return false, false
			}
			text.WriteRune(rune(val))
			p.pos += 2 + closeBrace + 1
			return true, true
		}
		p.pos++
		text.WriteByte('u')
		return true, true
	default:
		// Preserves literal character escapes including `$, `", `', ``, and
		// Windows PowerShell character escapes such as i`ex -> "iex".
		r, w := utf8.DecodeRuneInString(p.src[p.pos:])
		text.WriteRune(r)
		p.pos += w
		return true, true
	}
}

func classifyProviderVariable(rawVarName string) (fsPath string, isFS bool, isOpaqueProvider bool) {
	name := strings.TrimSpace(rawVarName)
	lower := asciiLower(name)
	for _, known := range []string{
		"global:", "script:", "local:", "private:", "using:", "workflow:", "variable:",
		"env:", "function:", "filter:", "alias:",
	} {
		if strings.HasPrefix(lower, known) {
			return "", false, false
		}
	}
	if strings.HasPrefix(lower, "filesystem::") {
		rest := strings.TrimSpace(name[len("filesystem::"):])
		return rest, rest != "", true
	}
	if strings.Contains(lower, "::") {
		return "", false, true
	}
	if colon := strings.IndexByte(name, ':'); colon >= 0 {
		if colon == 1 && isASCIILetter(name[0]) && len(name) > 2 {
			return name, true, false
		}
		return "", false, true
	}
	return "", false, false
}

func (p *psParser) observeVariableName(rawVarName string, line int) {
	name := strings.TrimSpace(rawVarName)
	lower := asciiLower(name)
	stripped := lower
	childLocal := false
	for _, scope := range []string{"global:", "script:", "local:", "private:", "using:", "workflow:", "variable:"} {
		if strings.HasPrefix(stripped, scope) {
			if scope == "local:" || scope == "private:" {
				childLocal = true
			}
			stripped = stripped[len(scope):]
			break
		}
	}
	if stripped == "executioncontext" || strings.HasPrefix(stripped, "executioncontext.") {
		p.noteOpaque(psOpaqueEval, line)
	}
	if (!p.inChildParamScope || (!childLocal && (strings.HasPrefix(lower, "script:") || strings.HasPrefix(lower, "global:")))) &&
		(stripped == "args" || stripped == "psboundparameters") {
		p.res.ConsumesParams = true
	}
}

func (p *psParser) observeVariableRead(rawVarName string, line int, parent int, deferred *[]psDeferredSpan) {
	p.observeVariableName(rawVarName, line)
	fsPath, isFS, isOpaque := classifyProviderVariable(rawVarName)
	if isOpaque {
		p.noteScriptOnlyOpaque(psOpaqueDynamicCall, line)
	}
	if isFS {
		w := Word{
			Text:    fsPath,
			Raw:     fsPath,
			Literal: !strings.ContainsAny(fsPath, "$`"),
		}
		if deferred != nil {
			*deferred = append(*deferred, psDeferredSpan{
				line:      line,
				kind:      Group,
				synthRead: &w,
			})
		} else {
			p.emitSyntheticGetContent(w, line, parent, Group)
		}
	}
}

// scanBalancedSpan scans from openAt (where p.src[openAt] is '(' or '{' or '[')
// to the matching close byte, advancing p.pos past closeByte and tracking p.line.
func (p *psParser) scanBalancedSpan(closeByte byte) (innerStart, innerEnd, startLine int, ok bool) {
	openLine := p.line
	p.pos++ // skip opener
	innerStart = p.pos
	startLine = p.line
	type delimFrame struct {
		close byte
	}
	stack := []delimFrame{{close: closeByte}}
	for p.pos < len(p.src) {
		if !p.step() {
			return innerStart, p.pos, startLine, false
		}
		c := p.src[p.pos]
		if c == stack[len(stack)-1].close {
			stack = stack[:len(stack)-1]
			if len(stack) == 0 {
				innerEnd = p.pos
				p.pos++
				return innerStart, innerEnd, startLine, true
			}
			p.pos++
			continue
		}
		switch c {
		case '\n':
			p.line++
			p.pos++
		case '\r':
			if p.pos+1 >= len(p.src) || p.src[p.pos+1] != '\n' {
				p.line++
			}
			p.pos++
		case '`':
			p.pos++
			if p.pos < len(p.src) {
				if p.src[p.pos] == '\n' {
					p.line++
				} else if p.src[p.pos] == '\r' && (p.pos+1 >= len(p.src) || p.src[p.pos+1] != '\n') {
					p.line++
				}
				p.pos++
			} else {
				p.noteOpaque(psOpaqueUnsupportedSyntax, openLine)
				return innerStart, p.pos, startLine, false
			}
		case '#':
			for p.pos < len(p.src) && p.src[p.pos] != '\n' && p.src[p.pos] != '\r' {
				p.pos++
			}
		case '<':
			if p.pos+1 < len(p.src) && p.src[p.pos+1] == '#' {
				if !p.skipBlockCommentFromHere() {
					return innerStart, p.pos, startLine, false
				}
				continue
			}
			p.pos++
		case '@':
			if p.pos+1 < len(p.src) && (p.src[p.pos+1] == '\'' || p.src[p.pos+1] == '"') {
				if _, _, ok := p.scanHereStringRawOnly(); !ok {
					return innerStart, p.pos, startLine, false
				}
				continue
			}
			p.pos++
		case '\'':
			if !p.skipSingleQuotedRawOnly() {
				return innerStart, p.pos, startLine, false
			}
		case '"':
			if !p.skipDoubleQuotedRawOnly() {
				return innerStart, p.pos, startLine, false
			}
		case '(':
			stack = append(stack, delimFrame{close: ')'})
			p.pos++
		case '{':
			stack = append(stack, delimFrame{close: '}'})
			p.pos++
		case '[':
			stack = append(stack, delimFrame{close: ']'})
			p.pos++
		case ')', '}', ']':
			// Mismatched closer before current frame's close.
			p.noteOpaque(psOpaqueUnsupportedSyntax, p.line)
			return innerStart, p.pos, startLine, false
		default:
			p.pos++
		}
	}
	p.noteOpaque(psOpaqueUnsupportedSyntax, openLine)
	return innerStart, p.pos, startLine, false
}

func (p *psParser) skipBlockCommentFromHere() bool {
	openLine := p.line
	p.pos += 2
	for p.pos < len(p.src) {
		if !p.step() {
			return false
		}
		if p.src[p.pos] == '\n' {
			p.line++
			p.pos++
			continue
		}
		if p.src[p.pos] == '\r' {
			if p.pos+1 >= len(p.src) || p.src[p.pos+1] != '\n' {
				p.line++
			}
			p.pos++
			continue
		}
		if p.src[p.pos] == '#' && p.pos+1 < len(p.src) && p.src[p.pos+1] == '>' {
			p.pos += 2
			return true
		}
		p.pos++
	}
	p.noteOpaque(psOpaqueUnsupportedSyntax, openLine)
	return false
}

func (p *psParser) skipSingleQuotedRawOnly() bool {
	openLine := p.line
	p.pos++
	for p.pos < len(p.src) {
		if !p.step() {
			return false
		}
		c := p.src[p.pos]
		if c == '\n' {
			p.line++
			p.pos++
			continue
		}
		if c == '\r' {
			if p.pos+1 >= len(p.src) || p.src[p.pos+1] != '\n' {
				p.line++
			}
			p.pos++
			continue
		}
		if c == '\'' {
			p.pos++
			if p.pos < len(p.src) && p.src[p.pos] == '\'' {
				p.pos++
				continue
			}
			return true
		}
		p.pos++
	}
	p.noteOpaque(psOpaqueUnsupportedSyntax, openLine)
	return false
}

func (p *psParser) skipDoubleQuotedRawOnly() bool {
	openLine := p.line
	p.pos++
	for p.pos < len(p.src) {
		if !p.step() {
			return false
		}
		c := p.src[p.pos]
		switch c {
		case '\n':
			p.line++
			p.pos++
		case '\r':
			if p.pos+1 >= len(p.src) || p.src[p.pos+1] != '\n' {
				p.line++
			}
			p.pos++
		case '`':
			p.pos++
			if p.pos >= len(p.src) {
				p.noteOpaque(psOpaqueUnsupportedSyntax, openLine)
				return false
			}
			if p.src[p.pos] == '\n' {
				p.line++
			} else if p.src[p.pos] == '\r' && (p.pos+1 >= len(p.src) || p.src[p.pos+1] != '\n') {
				p.line++
			}
			p.pos++
		case '$':
			if p.pos+1 < len(p.src) && p.src[p.pos+1] == '(' {
				p.pos++ // at '('
				if _, _, _, ok := p.scanBalancedSpan(')'); !ok {
					return false
				}
				continue
			}
			p.pos++
		case '"':
			p.pos++
			if p.pos < len(p.src) && p.src[p.pos] == '"' {
				p.pos++
				continue
			}
			return true
		default:
			p.pos++
		}
	}
	p.noteOpaque(psOpaqueUnsupportedSyntax, openLine)
	return false
}

func (p *psParser) scanHereStringRawOnly() (bodyStart, bodyEnd int, ok bool) {
	openLine := p.line
	quote := p.src[p.pos+1]
	p.pos += 2
	if p.pos < len(p.src) && p.src[p.pos] == '\r' && p.pos+1 < len(p.src) && p.src[p.pos+1] == '\n' {
		p.pos += 2
		p.line++
	} else if p.pos < len(p.src) && p.src[p.pos] == '\n' {
		p.pos++
		p.line++
	} else {
		p.noteOpaque(psOpaqueUnsupportedSyntax, openLine)
		return p.pos, p.pos, false
	}
	bodyStart = p.pos
	atCol0 := true
	for p.pos < len(p.src) {
		if !p.step() {
			return bodyStart, p.pos, false
		}
		if atCol0 && p.src[p.pos] == quote && p.pos+1 < len(p.src) && p.src[p.pos+1] == '@' {
			bodyEnd = p.pos
			if bodyEnd > bodyStart && p.src[bodyEnd-1] == '\n' {
				bodyEnd--
				if bodyEnd > bodyStart && p.src[bodyEnd-1] == '\r' {
					bodyEnd--
				}
			}
			p.pos += 2
			return bodyStart, bodyEnd, true
		}
		if quote == '"' && p.src[p.pos] == '`' && p.pos+1 < len(p.src) {
			p.pos++
			if p.src[p.pos] == '\n' {
				p.line++
				atCol0 = true
			} else if p.src[p.pos] == '\r' && (p.pos+1 >= len(p.src) || p.src[p.pos+1] != '\n') {
				p.line++
				atCol0 = true
			} else {
				atCol0 = false
			}
			p.pos++
			continue
		}
		if quote == '"' && p.src[p.pos] == '$' && p.pos+1 < len(p.src) && p.src[p.pos+1] == '(' {
			atCol0 = false
			p.pos++ // at '('
			if _, _, _, good := p.scanBalancedSpan(')'); !good {
				return bodyStart, p.pos, false
			}
			continue
		}
		c := p.src[p.pos]
		p.pos++
		if c == '\n' {
			p.line++
			atCol0 = true
		} else if c == '\r' {
			if p.pos >= len(p.src) || p.src[p.pos] != '\n' {
				p.line++
				atCol0 = true
			}
		} else {
			atCol0 = false
		}
	}
	p.noteOpaque(psOpaqueUnsupportedSyntax, openLine)
	return bodyStart, p.pos, false
}

func (p *psParser) scanSingleQuotedString() (text, raw string, ok bool) {
	start := p.pos
	openLine := p.line
	p.pos++
	var b strings.Builder
	for p.pos < len(p.src) {
		if !p.step() {
			return "", p.src[start:p.pos], false
		}
		c := p.src[p.pos]
		if c == '\n' {
			p.line++
			b.WriteByte(c)
			p.pos++
			continue
		}
		if c == '\r' {
			if p.pos+1 >= len(p.src) || p.src[p.pos+1] != '\n' {
				p.line++
			}
			b.WriteByte(c)
			p.pos++
			continue
		}
		if c == '\'' {
			p.pos++
			if p.pos < len(p.src) && p.src[p.pos] == '\'' {
				b.WriteByte('\'')
				p.pos++
				continue
			}
			return b.String(), p.src[start:p.pos], true
		}
		b.WriteByte(c)
		p.pos++
	}
	p.noteOpaque(psOpaqueUnsupportedSyntax, openLine)
	return b.String(), p.src[start:p.pos], false
}

func (p *psParser) scanDoubleQuotedString(parent int, deferred *[]psDeferredSpan) (text, raw string, literal, substituted, ok bool) {
	start := p.pos
	openLine := p.line
	p.pos++
	literal = true
	var b strings.Builder
	for p.pos < len(p.src) {
		if !p.step() {
			return b.String(), p.src[start:p.pos], false, substituted, false
		}
		c := p.src[p.pos]
		switch c {
		case '\n':
			p.line++
			b.WriteByte(c)
			p.pos++
		case '\r':
			if p.pos+1 >= len(p.src) || p.src[p.pos+1] != '\n' {
				p.line++
			}
			b.WriteByte(c)
			p.pos++
		case '`':
			litOK, escOK := p.decodePSBacktick(&b)
			if !litOK {
				literal = false
			}
			if !escOK {
				return b.String(), p.src[start:p.pos], false, substituted, false
			}
		case '$':
			if p.pos+1 < len(p.src) && p.src[p.pos+1] == '(' {
				literal = false
				substituted = true
				subRawStart := p.pos
				p.pos++ // at '('
				inStart, inEnd, subLine, good := p.scanBalancedSpan(')')
				b.WriteString(p.src[subRawStart:p.pos])
				if deferred != nil {
					*deferred = append(*deferred, psDeferredSpan{
						start: inStart, end: inEnd, line: subLine, kind: Substitution, stmtList: true,
					})
				} else {
					p.runSubParser(inStart, inEnd, subLine, parent, Substitution, true, false)
				}
				if !good {
					return b.String(), p.src[start:p.pos], false, substituted, false
				}
				continue
			}
			if p.pos+1 < len(p.src) && (p.src[p.pos+1] == '"' || p.src[p.pos+1] == ' ' || p.src[p.pos+1] == '\t' || p.src[p.pos+1] == '\r' || p.src[p.pos+1] == '\n') {
				// Bare '$' before space or closing quote is literal in PowerShell.
				b.WriteByte('$')
				p.pos++
				continue
			}
			literal = false
			varStart := p.pos
			varName := p.scanVariableToken()
			p.observeVariableRead(varName, p.line, parent, deferred)
			b.WriteString(p.src[varStart:p.pos])
		case '"':
			p.pos++
			if p.pos < len(p.src) && p.src[p.pos] == '"' {
				b.WriteByte('"')
				p.pos++
				continue
			}
			return b.String(), p.src[start:p.pos], literal, substituted, true
		default:
			b.WriteByte(c)
			p.pos++
		}
	}
	p.noteOpaque(psOpaqueUnsupportedSyntax, openLine)
	return b.String(), p.src[start:p.pos], false, substituted, false
}

func (p *psParser) scanHereString(parent int, deferred *[]psDeferredSpan) (text, raw string, literal, substituted, ok bool) {
	start := p.pos
	openLine := p.line
	quote := p.src[p.pos+1]
	p.pos += 2
	if p.pos < len(p.src) && p.src[p.pos] == '\r' && p.pos+1 < len(p.src) && p.src[p.pos+1] == '\n' {
		p.pos += 2
		p.line++
	} else if p.pos < len(p.src) && p.src[p.pos] == '\n' {
		p.pos++
		p.line++
	} else {
		p.noteOpaque(psOpaqueUnsupportedSyntax, openLine)
		return "", p.src[start:p.pos], false, false, false
	}
	literal = true
	var b strings.Builder
	atCol0 := true
	for p.pos < len(p.src) {
		if !p.step() {
			return b.String(), p.src[start:p.pos], false, substituted, false
		}
		if atCol0 && p.src[p.pos] == quote && p.pos+1 < len(p.src) && p.src[p.pos+1] == '@' {
			p.pos += 2
			out := b.String()
			if strings.HasSuffix(out, "\r\n") {
				out = out[:len(out)-2]
			} else if strings.HasSuffix(out, "\n") {
				out = out[:len(out)-1]
			}
			return out, p.src[start:p.pos], literal, substituted, true
		}
		if quote == '\'' {
			c := p.src[p.pos]
			b.WriteByte(c)
			p.pos++
			if c == '\n' {
				p.line++
				atCol0 = true
			} else if c == '\r' {
				if p.pos >= len(p.src) || p.src[p.pos] != '\n' {
					p.line++
					atCol0 = true
				}
			} else {
				atCol0 = false
			}
			continue
		}
		// Double-quoted here-string.
		c := p.src[p.pos]
		switch c {
		case '`':
			atCol0 = false
			litOK, escOK := p.decodePSBacktick(&b)
			if !litOK {
				literal = false
			}
			if !escOK {
				return b.String(), p.src[start:p.pos], false, substituted, false
			}
		case '$':
			atCol0 = false
			if p.pos+1 < len(p.src) && p.src[p.pos+1] == '(' {
				literal = false
				substituted = true
				subRawStart := p.pos
				p.pos++ // at '('
				inStart, inEnd, subLine, good := p.scanBalancedSpan(')')
				b.WriteString(p.src[subRawStart:p.pos])
				if deferred != nil {
					*deferred = append(*deferred, psDeferredSpan{
						start: inStart, end: inEnd, line: subLine, kind: Substitution, stmtList: true,
					})
				} else {
					p.runSubParser(inStart, inEnd, subLine, parent, Substitution, true, false)
				}
				if !good {
					return b.String(), p.src[start:p.pos], false, substituted, false
				}
				continue
			}
			if p.pos+1 < len(p.src) && (p.src[p.pos+1] == ' ' || p.src[p.pos+1] == '\t' || p.src[p.pos+1] == '\r' || p.src[p.pos+1] == '\n') {
				b.WriteByte('$')
				p.pos++
				continue
			}
			literal = false
			varStart := p.pos
			varName := p.scanVariableToken()
			p.observeVariableRead(varName, p.line, parent, deferred)
			b.WriteString(p.src[varStart:p.pos])
		case '\n':
			p.line++
			atCol0 = true
			b.WriteByte(c)
			p.pos++
		case '\r':
			if p.pos+1 >= len(p.src) || p.src[p.pos+1] != '\n' {
				p.line++
				atCol0 = true
			}
			b.WriteByte(c)
			p.pos++
		default:
			atCol0 = false
			b.WriteByte(c)
			p.pos++
		}
	}
	p.noteOpaque(psOpaqueUnsupportedSyntax, openLine)
	return b.String(), p.src[start:p.pos], false, substituted, false
}

// scanVariableToken consumes a $var or ${var} token starting at p.pos (where p.src[p.pos] == '$')
// and returns the variable name without the leading '$' or braces.
func (p *psParser) scanVariableToken() string {
	openLine := p.line
	p.pos++ // skip '$'
	if p.pos >= len(p.src) {
		return ""
	}
	if p.src[p.pos] == '{' {
		p.pos++
		start := p.pos
		for p.pos < len(p.src) {
			if p.src[p.pos] == '`' && p.pos+1 < len(p.src) {
				p.pos += 2
				continue
			}
			if p.src[p.pos] == '}' {
				name := p.src[start:p.pos]
				p.pos++
				return name
			}
			if p.src[p.pos] == '\n' {
				p.line++
			}
			p.pos++
		}
		p.noteOpaque(psOpaqueUnsupportedSyntax, openLine)
		return p.src[start:p.pos]
	}
	if p.src[p.pos] == '?' || p.src[p.pos] == '^' || p.src[p.pos] == '$' {
		ch := p.src[p.pos : p.pos+1]
		p.pos++
		return ch
	}
	start := p.pos
	for p.pos < len(p.src) {
		r, w := utf8.DecodeRuneInString(p.src[p.pos:])
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == ':' || r == '?' {
			p.pos += w
			continue
		}
		break
	}
	return p.src[start:p.pos]
}

func isKeywordBoundary(src string, idx int) bool {
	if idx >= len(src) {
		return true
	}
	switch src[idx] {
	case ' ', '\t', '\r', '\n', ';', '(', '{', '}', ')', '#':
		return true
	case '<':
		return idx+1 < len(src) && src[idx+1] == '#'
	}
	return false
}

func (p *psParser) peekKeyword() string {
	i := p.pos
	for i < len(p.src) && (isASCIILetter(p.src[i]) || p.src[i] == '_') {
		i++
	}
	if i == p.pos || !isKeywordBoundary(p.src, i) {
		return ""
	}
	return asciiLower(p.src[p.pos:i])
}

func (p *psParser) peekCharAfterTrivia(allowNewlines bool) byte {
	savedPos, savedLine, savedSteps := p.pos, p.line, p.steps
	savedOpaque := p.res.psOpacity
	p.skipTrivia(allowNewlines)
	var b byte
	if p.pos < len(p.src) {
		b = p.src[p.pos]
	}
	p.pos, p.line, p.steps = savedPos, savedLine, savedSteps
	p.res.psOpacity = savedOpaque
	return b
}

func (p *psParser) parseStatementList(stopByte byte, parent int, firstBefore Separator) {
	first := true
	for p.pos < len(p.src) {
		if !p.step() {
			return
		}
		p.skipTrivia(true)
		if p.pos >= len(p.src) {
			break
		}
		c := p.src[p.pos]
		if stopByte != 0 && c == stopByte {
			return
		}
		if c == ';' {
			p.pos++
			first = false
			continue
		}
		if c == ')' || c == '}' || c == ']' {
			p.noteOpaque(psOpaqueUnsupportedSyntax, p.line)
			p.pos++
			continue
		}
		before := Sequence
		if first {
			before = firstBefore
		}
		prevPos := p.pos
		p.parseStatement(parent, before)
		if p.pos == prevPos && !p.res.Canceled {
			p.noteOpaque(psOpaqueUnsupportedSyntax, p.line)
			p.pos++
		}
		first = false
	}
	if stopByte != 0 && !p.res.Canceled {
		p.noteOpaque(psOpaqueUnsupportedSyntax, p.line)
	}
}

func (p *psParser) parseStatement(parent int, before Separator) {
	// Optional :label before loop or switch.
	if p.pos < len(p.src) && p.src[p.pos] == ':' && p.pos+1 < len(p.src) && isASCIILetter(p.src[p.pos+1]) {
		savedPos := p.pos
		p.pos++
		for p.pos < len(p.src) && (isASCIILetter(p.src[p.pos]) || (p.src[p.pos] >= '0' && p.src[p.pos] <= '9') || p.src[p.pos] == '_' || p.src[p.pos] == '-') {
			p.pos++
		}
		p.skipTrivia(true)
		kw := p.peekKeyword()
		if kw != "for" && kw != "foreach" && kw != "while" && kw != "do" && kw != "switch" {
			p.pos = savedPos
		}
	}

	// Leading [CmdletBinding()] / [OutputType(...)] attributes before param(...) or function/filter.
	if p.pos < len(p.src) && p.src[p.pos] == '[' && p.looksLikeAttributedParamOrFunction() {
		p.parseAttributedParamOrFunction(parent)
		return
	}

	switch kw := p.peekKeyword(); kw {
	case "using":
		p.parseUsingStatement()
		return
	case "param":
		if p.keywordFollowedBy('(') {
			p.pos += len("param")
			p.skipTrivia(true)
			p.parseParamBlock(parent)
			return
		}
	case "function", "filter":
		p.parseFunctionDef(kw, parent)
		return
	case "if":
		if p.keywordFollowedBy('(') {
			p.parseIfStatement(parent)
			return
		}
	case "switch":
		if p.looksLikeSwitchStatement() {
			p.parseSwitchStatement(parent, before)
			return
		}
	case "foreach":
		if p.keywordFollowedBy('(') {
			p.parseForeachStatement(parent)
			return
		}
		if p.looksLikeForeachParallel() {
			p.noteOpaque(psOpaqueUnsupportedSyntax, p.line)
			p.consumeToStatementEnd()
			return
		}
	case "for":
		if p.keywordFollowedBy('(') {
			p.parseForStatement(parent)
			return
		}
	case "while":
		if p.keywordFollowedBy('(') {
			p.parseWhileStatement(parent)
			return
		}
	case "do":
		if p.keywordFollowedBy('{') {
			p.parseDoStatement(parent)
			return
		}
	case "try":
		if p.keywordFollowedBy('{') {
			p.parseTryStatement(parent)
			return
		}
	case "trap":
		p.parseTrapStatement(parent)
		return
	case "begin", "process", "end", "clean":
		if p.keywordFollowedBy('{') {
			p.pos += len(kw)
			p.skipTrivia(true)
			p.parseBracedBlock(parent, Group)
			return
		}
	case "dynamicparam":
		if p.keywordFollowedBy('{') {
			p.noteScriptOnlyOpaque(psOpaqueDynamicCall, p.line)
			p.pos += len(kw)
			p.skipTrivia(true)
			p.parseBracedBlock(parent, Group)
			return
		}
	case "return", "throw", "exit":
		p.pos += len(kw)
		p.skipTrivia(false)
		if !p.atStatementTerminator() {
			p.parseAssignmentOrPipeline(parent, Group)
		}
		return
	case "break", "continue":
		p.pos += len(kw)
		p.skipTrivia(false)
		if !p.atStatementTerminator() {
			if isASCIILetter(p.src[p.pos]) || p.src[p.pos] == '_' {
				for p.pos < len(p.src) {
					c := p.src[p.pos]
					if !isASCIILetter(c) && (c < '0' || c > '9') && c != '_' && c != '-' {
						break
					}
					p.pos++
				}
			} else {
				p.parseExpression(parent)
			}
		}
		return
	case "enum":
		p.parseEnumStatement()
		return
	case "class", "workflow":
		p.noteOpaque(psOpaqueUnsupportedSyntax, p.line)
		p.pos += len(kw)
		p.skipToNextStatementOrBraceBlock(parent)
		return
	}

	p.parseAssignmentOrPipeline(parent, before)
}

func (p *psParser) atStatementTerminator() bool {
	if p.pos >= len(p.src) {
		return true
	}
	switch p.src[p.pos] {
	case ';', '\r', '\n', '}', ')':
		return true
	}
	return false
}

func (p *psParser) consumeToStatementEnd() {
	for p.pos < len(p.src) && !p.atStatementTerminator() {
		p.pos++
	}
}

func (p *psParser) skipToNextStatementOrBraceBlock(parent int) {
	for p.pos < len(p.src) && !p.atStatementTerminator() {
		if p.src[p.pos] == '{' {
			p.parseBracedBlock(parent, Group)
			return
		}
		p.pos++
	}
}

func (p *psParser) keywordFollowedBy(ch byte) bool {
	kw := p.peekKeyword()
	if kw == "" {
		return false
	}
	savedPos, savedLine, savedSteps := p.pos, p.line, p.steps
	savedOpaque := p.res.psOpacity
	p.pos += len(kw)
	p.skipTrivia(true)
	ok := p.pos < len(p.src) && p.src[p.pos] == ch
	p.pos, p.line, p.steps = savedPos, savedLine, savedSteps
	p.res.psOpacity = savedOpaque
	return ok
}

func (p *psParser) looksLikeForeachParallel() bool {
	savedPos, savedLine := p.pos, p.line
	defer func() { p.pos, p.line = savedPos, savedLine }()
	p.pos += len("foreach")
	p.skipTrivia(false)
	return p.pos < len(p.src) && strings.HasPrefix(asciiLower(p.src[p.pos:]), "-parallel")
}

func (p *psParser) looksLikeAttributedParamOrFunction() bool {
	savedPos, savedLine, savedSteps := p.pos, p.line, p.steps
	savedOpaque := p.res.psOpacity
	defer func() {
		p.pos, p.line, p.steps = savedPos, savedLine, savedSteps
		p.res.psOpacity = savedOpaque
	}()
	count := 0
	for p.pos < len(p.src) && p.src[p.pos] == '[' {
		if _, _, _, ok := p.scanBalancedSpan(']'); !ok {
			return false
		}
		count++
		p.skipTrivia(true)
	}
	if count == 0 {
		return false
	}
	kw := p.peekKeyword()
	if kw == "function" || kw == "filter" {
		return true
	}
	return kw == "param" && p.keywordFollowedBy('(')
}

func (p *psParser) parseAttributedParamOrFunction(parent int) {
	for p.pos < len(p.src) && p.src[p.pos] == '[' {
		p.parseOneParamBracket(parent)
		p.skipTrivia(true)
	}
	kw := p.peekKeyword()
	if kw == "param" {
		p.pos += len("param")
		p.skipTrivia(true)
		p.parseParamBlock(parent)
	} else if kw == "function" || kw == "filter" {
		p.parseFunctionDef(kw, parent)
	}
}

func (p *psParser) parseOneParamBracket(parent int) {
	attrLine := p.line
	inStart, inEnd, subLine, ok := p.scanBalancedSpan(']')
	if !ok {
		return
	}
	inner := strings.TrimSpace(p.src[inStart:inEnd])
	if inner == "" {
		p.noteOpaque(psOpaqueUnsupportedSyntax, attrLine)
		return
	}
	if openParen := strings.IndexByte(inner, '('); openParen >= 0 {
		if inner[len(inner)-1] != ')' {
			p.noteOpaque(psOpaqueUnsupportedSyntax, attrLine)
			return
		}
		attrName := strings.TrimSpace(inner[:openParen])
		kind := checkPSParamAttribute(attrName, true, p.activeNamespaces)
		if kind != psOpaqueNone {
			p.noteScriptOnlyOpaque(kind, attrLine)
		}
		argStart := inStart + strings.IndexByte(p.src[inStart:inEnd], '(') + 1
		argEnd := inStart + strings.LastIndexByte(p.src[inStart:inEnd], ')')
		if argEnd > argStart {
			p.parseAttributeArgsSpan(argStart, argEnd, subLine, parent)
		}
		return
	}
	kind := checkPSParamAttribute(inner, false, p.activeNamespaces)
	if kind != psOpaqueNone {
		p.noteScriptOnlyOpaque(kind, attrLine)
	}
}

func (p *psParser) parseAttributeArgsSpan(start, end, startLine int, parent int) {
	_, spans := p.extractCommaExprArgs(start, end, startLine)
	for _, sp := range spans {
		p.parseOneAttributeArgSpan(sp.start, sp.end, sp.line, parent)
	}
}

func (p *psParser) parseOneAttributeArgSpan(start, end, startLine int, parent int) {
	if !p.enterDepth(startLine) {
		return
	}
	defer p.leaveDepth()
	savedSrc, savedPos, savedLine := p.src, p.pos, p.line
	p.src = savedSrc[:end]
	p.pos = start
	p.line = startLine
	defer func() { p.src, p.pos, p.line = savedSrc, savedPos, savedLine }()

	p.skipTrivia(true)
	if p.pos >= len(p.src) {
		return
	}
	if isASCIILetter(p.src[p.pos]) || p.src[p.pos] == '_' {
		idStart, idLine := p.pos, p.line
		for p.pos < len(p.src) && (isASCIILetter(p.src[p.pos]) || (p.src[p.pos] >= '0' && p.src[p.pos] <= '9') || p.src[p.pos] == '_') {
			p.pos++
		}
		p.skipTrivia(true)
		if p.pos >= len(p.src) {
			// Shorthand boolean attribute property flag such as [Parameter(Mandatory)].
			return
		}
		if p.src[p.pos] == '=' && (p.pos+1 >= len(p.src) || p.src[p.pos+1] != '=') {
			p.pos++
			p.skipTrivia(true)
			if p.pos >= len(p.src) {
				p.noteOpaque(psOpaqueUnsupportedSyntax, p.line)
				return
			}
		} else {
			p.pos, p.line = idStart, idLine
		}
	}
	p.parseExpression(parent)
	p.skipTrivia(true)
	if p.pos < len(p.src) {
		p.noteOpaque(psOpaqueUnsupportedSyntax, p.line)
	}
}

func (p *psParser) parseParamBlock(parent int) {
	if p.pos >= len(p.src) || p.src[p.pos] != '(' {
		p.noteOpaque(psOpaqueUnsupportedSyntax, p.line)
		return
	}
	inStart, inEnd, startLine, ok := p.scanBalancedSpan(')')
	if !ok {
		return
	}
	if !p.enterDepth(startLine) {
		return
	}
	defer p.leaveDepth()

	savedSrc, savedPos, savedLine := p.src, p.pos, p.line
	p.src = savedSrc[:inEnd]
	p.pos = inStart
	p.line = startLine
	defer func() { p.src, p.pos, p.line = savedSrc, savedPos, savedLine }()

	sawParam := false
	for p.pos < len(p.src) {
		if !p.step() {
			return
		}
		p.skipTrivia(true)
		if p.pos >= len(p.src) {
			break
		}
		if p.src[p.pos] == ',' {
			p.pos++
			continue
		}
		for p.pos < len(p.src) && p.src[p.pos] == '[' {
			p.parseOneParamBracket(parent)
			p.skipTrivia(true)
		}
		if p.pos >= len(p.src) {
			break
		}
		if p.src[p.pos] != '$' {
			p.noteOpaque(psOpaqueUnsupportedSyntax, p.line)
			return
		}
		varLine := p.line
		varName := p.scanVariableToken()
		p.observeVariableName(varName, varLine)
		sawParam = true
		p.recordNonConstantWrite(varName)
		p.skipTrivia(true)
		if p.pos < len(p.src) && p.src[p.pos] == '=' {
			p.pos++
			p.skipTrivia(true)
			p.parseExpressionStopAtComma(parent)
		}
		p.skipTrivia(true)
		if p.pos < len(p.src) && p.src[p.pos] == ',' {
			p.pos++
		}
	}
	if sawParam && !p.inChildParamScope {
		p.res.ConsumesParams = true
	}
}

func (p *psParser) parseUsingStatement() {
	line := p.line
	p.pos += len("using")
	p.skipTrivia(false)
	kindStart := p.pos
	for p.pos < len(p.src) && isASCIILetter(p.src[p.pos]) {
		p.pos++
	}
	sub := asciiLower(p.src[kindStart:p.pos])
	p.skipTrivia(false)
	valStart := p.pos
	for p.pos < len(p.src) && !p.atStatementTerminator() && p.src[p.pos] != '#' {
		if p.src[p.pos] == '<' && p.pos+1 < len(p.src) && p.src[p.pos+1] == '#' {
			break
		}
		p.pos++
	}
	val := strings.TrimSpace(p.src[valStart:p.pos])
	p.skipTrivia(false)
	if val == "" {
		p.noteOpaque(psOpaqueUnsupportedSyntax, line)
		return
	}
	switch sub {
	case "module":
		p.noteOpaque(psOpaqueDynamicCall, line)
	case "assembly":
		p.noteScriptOnlyOpaque(psOpaqueDotNetCall, line)
	case "namespace":
		nsLower := asciiLower(val)
		if psAllowedUsingNamespaces[nsLower] {
			p.activeNamespaces = append(p.activeNamespaces, nsLower)
		} else {
			p.noteScriptOnlyOpaque(psOpaqueDotNetCall, line)
		}
	default:
		p.noteOpaque(psOpaqueUnsupportedSyntax, line)
	}
}

func (p *psParser) parseFunctionDef(kw string, parent int) {
	line := p.line
	p.pos += len(kw)
	p.skipTrivia(false)
	if p.pos >= len(p.src) {
		p.noteOpaque(psOpaqueUnsupportedSyntax, line)
		return
	}
	var name string
	if p.src[p.pos] == '\'' {
		var ok bool
		name, _, ok = p.scanSingleQuotedString()
		if !ok {
			return
		}
	} else if p.src[p.pos] == '"' {
		var lit, ok bool
		name, _, lit, _, ok = p.scanDoubleQuotedString(parent, nil)
		if !ok {
			return
		}
		if !lit {
			p.noteScriptOnlyOpaque(psOpaqueFunctionShadow, line)
		}
	} else {
		word, deferred, ok := p.scanSingleArgUnit()
		for _, span := range deferred {
			p.runDeferredSpan(span, parent)
		}
		if !ok {
			return
		}
		if !word.Literal {
			p.noteScriptOnlyOpaque(psOpaqueFunctionShadow, line)
		}
		name = word.Text
	}
	if name == "" {
		p.noteOpaque(psOpaqueUnsupportedSyntax, line)
		return
	}
	if isPSFunctionShadow(name) {
		p.noteScriptOnlyOpaque(psOpaqueFunctionShadow, line)
	}

	prevScope := p.inChildParamScope
	p.inChildParamScope = true
	defer func() { p.inChildParamScope = prevScope }()

	p.skipTrivia(true)
	if p.pos < len(p.src) && p.src[p.pos] == '(' {
		p.parseParamBlock(parent)
		p.skipTrivia(true)
	}
	if p.pos >= len(p.src) || p.src[p.pos] != '{' {
		p.noteOpaque(psOpaqueUnsupportedSyntax, line)
		return
	}
	p.parseBracedBlock(parent, Group)
}

func (p *psParser) parseBracedBlock(parent int, before Separator) {
	if p.pos >= len(p.src) || p.src[p.pos] != '{' {
		p.noteOpaque(psOpaqueUnsupportedSyntax, p.line)
		return
	}
	inStart, inEnd, startLine, ok := p.scanBalancedSpan('}')
	if !ok {
		return
	}
	p.runSubParser(inStart, inEnd, startLine, parent, before, true, false)
}

func (p *psParser) parseParenStatements(parent int, before Separator) {
	if p.pos >= len(p.src) || p.src[p.pos] != '(' {
		p.noteOpaque(psOpaqueUnsupportedSyntax, p.line)
		return
	}
	inStart, inEnd, startLine, ok := p.scanBalancedSpan(')')
	if !ok {
		return
	}
	p.runSubParser(inStart, inEnd, startLine, parent, before, true, false)
}

func (p *psParser) parseIfStatement(parent int) {
	p.pos += len("if")
	p.skipTrivia(true)
	p.parseParenStatements(parent, Group)
	p.skipTrivia(true)
	p.parseBracedBlock(parent, Group)
	for {
		savedPos, savedLine := p.pos, p.line
		p.skipTrivia(true)
		kw := p.peekKeyword()
		if kw == "elseif" && p.keywordFollowedBy('(') {
			p.pos += len("elseif")
			p.skipTrivia(true)
			p.parseParenStatements(parent, Group)
			p.skipTrivia(true)
			p.parseBracedBlock(parent, Group)
			continue
		}
		if kw == "else" && p.keywordFollowedBy('{') {
			p.pos += len("else")
			p.skipTrivia(true)
			p.parseBracedBlock(parent, Group)
			return
		}
		p.pos, p.line = savedPos, savedLine
		return
	}
}

func (p *psParser) looksLikeSwitchStatement() bool {
	savedPos, savedLine, savedSteps := p.pos, p.line, p.steps
	savedOpaque := p.res.psOpacity
	defer func() {
		p.pos, p.line, p.steps = savedPos, savedLine, savedSteps
		p.res.psOpacity = savedOpaque
	}()
	p.pos += len("switch")
	p.skipTrivia(true)
	for p.pos < len(p.src) {
		r, _ := utf8.DecodeRuneInString(p.src[p.pos:])
		if !isPSDashRune(r) {
			break
		}
		start := p.pos
		for p.pos < len(p.src) && !strings.ContainsRune(" \t\r\n({", rune(p.src[p.pos])) {
			p.pos++
		}
		flag := asciiLower(normalizeLeadingDashes(p.src[start:p.pos]))
		p.skipTrivia(true)
		if len(flag) >= 2 && strings.HasPrefix("-file", strings.SplitN(flag, ":", 2)[0]) {
			if !strings.Contains(flag, ":") {
				// Skip file operand token.
				if p.pos < len(p.src) && p.src[p.pos] == '\'' {
					p.skipSingleQuotedRawOnly()
				} else if p.pos < len(p.src) && p.src[p.pos] == '"' {
					p.skipDoubleQuotedRawOnly()
				} else {
					for p.pos < len(p.src) && !strings.ContainsRune(" \t\r\n{", rune(p.src[p.pos])) {
						p.pos++
					}
				}
				p.skipTrivia(true)
			}
			return p.pos < len(p.src) && p.src[p.pos] == '{'
		}
	}
	return p.pos < len(p.src) && p.src[p.pos] == '('
}

func (p *psParser) parseSwitchStatement(parent int, before Separator) {
	line := p.line
	p.pos += len("switch")
	p.skipTrivia(true)
	hasFile := false
	for p.pos < len(p.src) {
		r, _ := utf8.DecodeRuneInString(p.src[p.pos:])
		if !isPSDashRune(r) {
			break
		}
		word, deferred, ok := p.scanArgumentElement(false)
		for _, sp := range deferred {
			p.runDeferredSpan(sp, parent)
		}
		if !ok {
			return
		}
		flagHead, inlineVal, hasColon := strings.Cut(normalizeLeadingDashes(word.Text), ":")
		_, inlineRaw, _ := strings.Cut(word.Raw, ":")
		flagLower := asciiLower(flagHead)
		switch {
		case len(flagLower) >= 2 && strings.HasPrefix("-regex", flagLower),
			len(flagLower) >= 2 && strings.HasPrefix("-wildcard", flagLower),
			len(flagLower) >= 2 && strings.HasPrefix("-exact", flagLower),
			len(flagLower) >= 2 && strings.HasPrefix("-casesensitive", flagLower):
			if hasColon {
				p.noteOpaque(psOpaqueUnsupportedSyntax, line)
				return
			}
		case len(flagLower) >= 2 && strings.HasPrefix("-file", flagLower):
			if hasFile {
				p.noteOpaque(psOpaqueUnsupportedSyntax, line)
				return
			}
			hasFile = true
			var fileWord Word
			if hasColon {
				if inlineRaw == "" {
					p.noteOpaque(psOpaqueUnsupportedSyntax, line)
					return
				}
				fileWord = Word{Text: inlineVal, Raw: inlineRaw, Literal: word.Literal}
			} else {
				p.skipTrivia(true)
				var fDef []psDeferredSpan
				var fOK bool
				fileWord, fDef, fOK = p.scanArgumentElement(false)
				if !fOK || fileWord.Raw == "" {
					p.noteOpaque(psOpaqueUnsupportedSyntax, line)
					return
				}
				for _, sp := range fDef {
					p.runDeferredSpan(sp, parent)
				}
			}
			p.emitSyntheticGetContent(fileWord, line, parent, before)
		default:
			p.noteOpaque(psOpaqueUnsupportedSyntax, line)
			return
		}
		p.skipTrivia(true)
	}
	if !hasFile {
		if p.pos >= len(p.src) || p.src[p.pos] != '(' {
			p.noteOpaque(psOpaqueUnsupportedSyntax, line)
			return
		}
		p.parseParenStatements(parent, Group)
		p.skipTrivia(true)
	}
	if p.pos >= len(p.src) || p.src[p.pos] != '{' {
		p.noteOpaque(psOpaqueUnsupportedSyntax, line)
		return
	}
	inStart, inEnd, startLine, ok := p.scanBalancedSpan('}')
	if !ok {
		return
	}
	p.parseSwitchArms(inStart, inEnd, startLine, parent)
}

func (p *psParser) parseSwitchArms(start, end, startLine int, parent int) {
	if !p.enterDepth(startLine) {
		return
	}
	defer p.leaveDepth()
	savedSrc, savedPos, savedLine := p.src, p.pos, p.line
	p.src = savedSrc[:end]
	p.pos = start
	p.line = startLine
	defer func() { p.src, p.pos, p.line = savedSrc, savedPos, savedLine }()

	for p.pos < len(p.src) {
		if !p.step() {
			return
		}
		p.skipTrivia(true)
		if p.pos >= len(p.src) {
			break
		}
		if p.src[p.pos] == ';' {
			p.pos++
			continue
		}
		// Switch clause condition: scriptblock, parenthesized expression, string, or bare pattern word.
		switch p.src[p.pos] {
		case '{':
			p.parseBracedBlock(parent, Group)
		case '(':
			p.parseParenStatements(parent, Group)
		case '\'':
			p.scanSingleQuotedString()
		case '"':
			p.scanDoubleQuotedString(parent, nil)
		case '@':
			if p.pos+1 < len(p.src) && (p.src[p.pos+1] == '\'' || p.src[p.pos+1] == '"') {
				p.scanHereString(parent, nil)
			} else {
				p.parsePrimaryExpr(parent)
			}
		case '$':
			p.parsePrimaryExpr(parent)
		default:
			for p.pos < len(p.src) && !strings.ContainsRune(" \t\r\n{;", rune(p.src[p.pos])) {
				p.pos++
			}
		}
		p.skipTrivia(true)
		if p.pos >= len(p.src) || p.src[p.pos] != '{' {
			p.noteOpaque(psOpaqueUnsupportedSyntax, p.line)
			return
		}
		p.parseBracedBlock(parent, Group)
	}
}

func (p *psParser) parseForeachStatement(parent int) {
	line := p.line
	p.pos += len("foreach")
	p.skipTrivia(true)
	inStart, inEnd, startLine, ok := p.scanBalancedSpan(')')
	if !ok {
		return
	}
	p.parseForeachHeader(inStart, inEnd, startLine, parent)
	p.skipTrivia(true)
	if p.pos >= len(p.src) || p.src[p.pos] != '{' {
		p.noteOpaque(psOpaqueUnsupportedSyntax, line)
		return
	}
	p.parseBracedBlock(parent, Group)
}

func (p *psParser) parseForeachHeader(start, end, startLine int, parent int) {
	if !p.enterDepth(startLine) {
		return
	}
	defer p.leaveDepth()
	savedSrc, savedPos, savedLine := p.src, p.pos, p.line
	p.src = savedSrc[:end]
	p.pos = start
	p.line = startLine
	defer func() { p.src, p.pos, p.line = savedSrc, savedPos, savedLine }()

	p.skipTrivia(true)
	for p.pos < len(p.src) && p.src[p.pos] == '[' {
		p.validateTypeCastBracket()
		p.skipTrivia(true)
	}
	if p.pos >= len(p.src) || p.src[p.pos] != '$' {
		p.noteOpaque(psOpaqueUnsupportedSyntax, p.line)
		return
	}
	varLine := p.line
	varName := p.scanVariableToken()
	p.observeVariableName(varName, varLine)
	p.recordForeachVarWrite(varName, varLine)
	p.skipTrivia(true)
	if p.peekKeyword() != "in" {
		p.noteOpaque(psOpaqueUnsupportedSyntax, p.line)
		return
	}
	p.pos += len("in")
	p.skipTrivia(true)
	p.parseStatementList(0, parent, Group)
}

func (p *psParser) recordForeachVarWrite(varName string, line int) {
	lower := asciiLower(varName)
	if strings.HasPrefix(lower, "env:") {
		envName := strings.TrimPrefix(varName[4:], `\`)
		lit := envName != "" && !strings.ContainsAny(envName, "$`*?[]")
		pinned := !lit || isPSPinnedEnv(envName)
		p.recordEnvMutation(psEnvMutation{
			Name: envName, LiteralName: lit, Line: line, Pinned: pinned,
		})
		if pinned {
			p.noteScriptOnlyOpaque(psOpaquePinnedAssignment, line)
		}
		return
	}
	if strings.HasPrefix(lower, "function:") {
		p.noteScriptOnlyOpaque(psOpaqueFunctionShadow, line)
		return
	}
	if strings.HasPrefix(lower, "alias:") {
		p.noteScriptOnlyOpaque(psOpaqueAlias, line)
		return
	}
	p.recordNonConstantWrite(varName)
}

func (p *psParser) parseForStatement(parent int) {
	line := p.line
	p.pos += len("for")
	p.skipTrivia(true)
	inStart, inEnd, startLine, ok := p.scanBalancedSpan(')')
	if !ok {
		return
	}
	p.runSubParser(inStart, inEnd, startLine, parent, Group, true, false)
	p.skipTrivia(true)
	if p.pos >= len(p.src) || p.src[p.pos] != '{' {
		p.noteOpaque(psOpaqueUnsupportedSyntax, line)
		return
	}
	p.parseBracedBlock(parent, Group)
}

func (p *psParser) parseWhileStatement(parent int) {
	line := p.line
	p.pos += len("while")
	p.skipTrivia(true)
	p.parseParenStatements(parent, Group)
	p.skipTrivia(true)
	if p.pos >= len(p.src) || p.src[p.pos] != '{' {
		p.noteOpaque(psOpaqueUnsupportedSyntax, line)
		return
	}
	p.parseBracedBlock(parent, Group)
}

func (p *psParser) parseDoStatement(parent int) {
	line := p.line
	p.pos += len("do")
	p.skipTrivia(true)
	p.parseBracedBlock(parent, Group)
	p.skipTrivia(true)
	kw := p.peekKeyword()
	if (kw == "while" || kw == "until") && p.keywordFollowedBy('(') {
		p.pos += len(kw)
		p.skipTrivia(true)
		p.parseParenStatements(parent, Group)
		return
	}
	p.noteOpaque(psOpaqueUnsupportedSyntax, line)
}

func (p *psParser) parseTryStatement(parent int) {
	line := p.line
	p.pos += len("try")
	p.skipTrivia(true)
	p.parseBracedBlock(parent, Group)
	sawHandler := false
	for {
		savedPos, savedLine := p.pos, p.line
		p.skipTrivia(true)
		kw := p.peekKeyword()
		if kw != "catch" {
			p.pos, p.line = savedPos, savedLine
			break
		}
		sawHandler = true
		p.pos += len("catch")
		p.skipTrivia(true)
		for p.pos < len(p.src) && p.src[p.pos] == '[' {
			p.validateTypeCastBracket()
			p.skipTrivia(true)
			if p.pos < len(p.src) && p.src[p.pos] == ',' {
				p.pos++
				p.skipTrivia(true)
			}
		}
		if p.pos >= len(p.src) || p.src[p.pos] != '{' {
			p.noteOpaque(psOpaqueUnsupportedSyntax, p.line)
			return
		}
		p.parseBracedBlock(parent, Group)
	}
	savedPos, savedLine := p.pos, p.line
	p.skipTrivia(true)
	if p.peekKeyword() == "finally" && p.keywordFollowedBy('{') {
		sawHandler = true
		p.pos += len("finally")
		p.skipTrivia(true)
		p.parseBracedBlock(parent, Group)
	} else {
		p.pos, p.line = savedPos, savedLine
	}
	if !sawHandler {
		p.noteOpaque(psOpaqueUnsupportedSyntax, line)
	}
}

func (p *psParser) parseTrapStatement(parent int) {
	line := p.line
	p.noteScriptOnlyOpaque(psOpaqueTrap, line)
	p.pos += len("trap")
	p.skipTrivia(true)
	if p.pos < len(p.src) && p.src[p.pos] == '[' {
		p.validateTypeCastBracket()
		p.skipTrivia(true)
	}
	if p.pos >= len(p.src) || p.src[p.pos] != '{' {
		p.noteOpaque(psOpaqueUnsupportedSyntax, line)
		return
	}
	p.parseBracedBlock(parent, Group)
}

func (p *psParser) parseEnumStatement() {
	line := p.line
	p.pos += len("enum")
	p.skipTrivia(false)
	start := p.pos
	for p.pos < len(p.src) && (isASCIILetter(p.src[p.pos]) || (p.src[p.pos] >= '0' && p.src[p.pos] <= '9') || p.src[p.pos] == '_') {
		p.pos++
	}
	if p.pos == start {
		p.noteOpaque(psOpaqueUnsupportedSyntax, line)
		return
	}
	p.skipTrivia(true)
	if p.pos >= len(p.src) || p.src[p.pos] != '{' {
		p.noteOpaque(psOpaqueUnsupportedSyntax, line)
		return
	}
	p.scanBalancedSpan('}')
}

func (p *psParser) validateTypeCastBracket() (typeRaw string, ok bool) {
	line := p.line
	inStart, inEnd, _, spanOK := p.scanBalancedSpan(']')
	if !spanOK {
		return "", false
	}
	typeRaw = strings.TrimSpace(p.src[inStart:inEnd])
	if _, _, valid := resolvePSDotNetType(typeRaw, p.activeNamespaces); !valid {
		p.noteScriptOnlyOpaque(psOpaqueDotNetCall, line)
		return typeRaw, false
	}
	return typeRaw, true
}

type psLHSInfo struct {
	op           string
	afterOpPos   int
	afterOpLine  int
	simpleVar    string // without '$'; includes scope/drive if present
	hasTypeCast  bool
	castType     string
	multiVars    []string
	isStaticProp bool
	staticType   string
	staticMember string
	castSpans    []psDeferredSpan
	operandSpans []psDeferredSpan
}

// tryScanAssignmentLHS checks whether p.pos begins a PowerShell assignment statement.
func (p *psParser) tryScanAssignmentLHS() (psLHSInfo, bool) {
	if p.pos >= len(p.src) {
		return psLHSInfo{}, false
	}
	c := p.src[p.pos]
	if c != '$' && c != '[' {
		return psLHSInfo{}, false
	}
	savedPos, savedLine, savedSteps := p.pos, p.line, p.steps
	savedOpaque := p.res.psOpacity
	savedConsumes := p.res.ConsumesParams
	restore := func() {
		p.pos, p.line, p.steps = savedPos, savedLine, savedSteps
		p.res.psOpacity = savedOpaque
		p.res.ConsumesParams = savedConsumes
	}

	var info psLHSInfo
	// Check static property assignment [Type]::Member = ... first when followed by ::
	if c == '[' {
		bStart, bEnd, _, ok := p.scanBalancedSpan(']')
		if !ok {
			restore()
			return psLHSInfo{}, false
		}
		if p.pos+1 < len(p.src) && p.src[p.pos] == ':' && p.src[p.pos+1] == ':' {
			p.pos += 2
			mStart := p.pos
			for p.pos < len(p.src) && (isASCIILetter(p.src[p.pos]) || (p.src[p.pos] >= '0' && p.src[p.pos] <= '9') || p.src[p.pos] == '_') {
				p.pos++
			}
			if p.pos == mStart {
				restore()
				return psLHSInfo{}, false
			}
			member := p.src[mStart:p.pos]
			p.skipTrivia(false)
			op, okOp := p.matchAssignOperator()
			if !okOp {
				restore()
				return psLHSInfo{}, false
			}
			info.op = op
			info.isStaticProp = true
			info.staticType = p.src[bStart:bEnd]
			info.staticMember = member
			info.afterOpPos = p.pos
			info.afterOpLine = p.line
			restore()
			return info, true
		}
		// Reset to parse typed variable(s) [Type]$var = ...
		p.pos, p.line = savedPos, savedLine
	}

	scanOneTarget := func() (varName string, hasCast bool, castT string, pureVar bool, ok bool) {
		for p.pos < len(p.src) && p.src[p.pos] == '[' {
			bStart, bEnd, bLine, bOK := p.scanBalancedSpan(']')
			if !bOK {
				return "", false, "", false, false
			}
			hasCast = true
			castT = strings.TrimSpace(p.src[bStart:bEnd])
			info.castSpans = append(info.castSpans, psDeferredSpan{start: bStart, end: bEnd, line: bLine})
			p.skipTrivia(false)
		}
		if p.pos >= len(p.src) || p.src[p.pos] != '$' {
			return "", false, "", false, false
		}
		if p.pos+1 < len(p.src) && p.src[p.pos+1] == '(' {
			return "", false, "", false, false
		}
		varName = p.scanVariableToken()
		if varName == "" {
			return "", false, "", false, false
		}
		pureVar = true
		for p.pos < len(p.src) {
			if p.src[p.pos] == '.' && (p.pos+1 >= len(p.src) || p.src[p.pos+1] != '.') {
				pureVar = false
				p.pos++
				if p.pos < len(p.src) && p.src[p.pos] == '\'' {
					if !p.skipSingleQuotedRawOnly() {
						return "", false, "", false, false
					}
				} else if p.pos < len(p.src) && p.src[p.pos] == '"' {
					start, line := p.pos, p.line
					if !p.skipDoubleQuotedRawOnly() {
						return "", false, "", false, false
					}
					info.operandSpans = append(info.operandSpans, psDeferredSpan{start: start, end: p.pos, line: line, kind: Group})
				} else {
					mStart := p.pos
					for p.pos < len(p.src) && (isASCIILetter(p.src[p.pos]) || (p.src[p.pos] >= '0' && p.src[p.pos] <= '9') || p.src[p.pos] == '_') {
						p.pos++
					}
					if p.pos == mStart {
						return "", false, "", false, false
					}
					if p.pos < len(p.src) && p.src[p.pos] == '(' {
						return "", false, "", false, false
					}
				}
				continue
			}
			if p.src[p.pos] == '[' {
				pureVar = false
				inS, inE, sL, sOK := p.scanBalancedSpan(']')
				if !sOK {
					return "", false, "", false, false
				}
				info.operandSpans = append(info.operandSpans, psDeferredSpan{
					start: inS, end: inE, line: sL, kind: Group, stmtList: false,
				})
				continue
			}
			break
		}
		return varName, hasCast, castT, pureVar, true
	}

	vName, hasCast, castT, pureVar, ok := scanOneTarget()
	if !ok {
		restore()
		return psLHSInfo{}, false
	}
	p.skipTrivia(false)
	if p.pos < len(p.src) && p.src[p.pos] == ',' {
		info.multiVars = append(info.multiVars, vName)
		for p.pos < len(p.src) && p.src[p.pos] == ',' {
			p.pos++
			p.skipTrivia(true)
			nv, _, _, _, nOK := scanOneTarget()
			if !nOK {
				restore()
				return psLHSInfo{}, false
			}
			info.multiVars = append(info.multiVars, nv)
			p.skipTrivia(false)
		}
	} else if pureVar {
		info.simpleVar = vName
		info.hasTypeCast = hasCast
		info.castType = castT
	} else {
		// Property or indexed assignment on variable: $obj.Prop = ... or $arr[$i] = ...
		info.multiVars = []string{vName}
		info.hasTypeCast = hasCast
		info.castType = castT
	}

	op, okOp := p.matchAssignOperator()
	if !okOp {
		restore()
		return psLHSInfo{}, false
	}
	info.op = op
	info.afterOpPos = p.pos
	info.afterOpLine = p.line
	restore()
	return info, true
}

func (p *psParser) matchAssignOperator() (string, bool) {
	if p.pos >= len(p.src) {
		return "", false
	}
	for _, two := range []string{"+=", "-=", "*=", "/=", "%="} {
		if strings.HasPrefix(p.src[p.pos:], two) {
			p.pos += 2
			return two, true
		}
	}
	if p.src[p.pos] == '=' {
		if p.pos+1 < len(p.src) && (p.src[p.pos+1] == '=' || p.src[p.pos+1] == '>') {
			return "", false
		}
		p.pos++
		return "=", true
	}
	return "", false
}

func stripPSScope(varName string) (stripped string, scoped bool) {
	lower := asciiLower(varName)
	for _, sc := range []string{"global:", "script:", "local:", "private:"} {
		if strings.HasPrefix(lower, sc) {
			return varName[len(sc):], true
		}
	}
	return varName, false
}

func (p *psParser) recordNonConstantWrite(varName string) {
	clean, _ := stripPSScope(varName)
	if clean == "" || strings.IndexByte(clean, ':') >= 0 {
		return
	}
	p.recordBareAssignment(Word{
		Text:    clean + "=",
		Raw:     "$" + varName,
		Literal: false,
	})
}

func (p *psParser) recordBareAssignment(word Word) {
	p.res.BareAssignments = append(p.res.BareAssignments, word)
	p.res.BareAt = append(p.res.BareAt, len(p.res.Invocations))
}

func (p *psParser) recordEnvMutation(mutation psEnvMutation) {
	mutation.At = len(p.res.Invocations)
	p.res.EnvMutations = append(p.res.EnvMutations, mutation)
}

func (p *psParser) parseAssignmentOrPipeline(parent int, before Separator) {
	stmtStart := p.pos
	stmtLine := p.line
	info, isAssign := p.tryScanAssignmentLHS()
	if !isAssign {
		p.parsePipelineChain(parent, before)
		return
	}
	if !p.enterDepth(stmtLine) {
		p.consumeToStatementEnd()
		return
	}
	defer p.leaveDepth()

	// Replay the parsed targets once the assignment operator confirms their role.
	lhsEndPos := info.afterOpPos - len(info.op)
	lhsRaw := strings.TrimSpace(p.src[stmtStart:lhsEndPos])
	if info.isStaticProp {
		p.pos = info.afterOpPos
		p.line = info.afterOpLine
		ev := evalPSDotNetStatic(info.staticType, info.staticMember, false, true, nil, p.activeNamespaces)
		if ev.opaque != psOpaqueNone {
			if ev.bothModes {
				p.noteOpaque(ev.opaque, stmtLine)
			} else {
				p.noteScriptOnlyOpaque(ev.opaque, stmtLine)
			}
		}
		p.skipTrivia(true)
		if p.atStatementTerminator() {
			p.noteOpaque(psOpaqueUnsupportedSyntax, stmtLine)
			return
		}
		p.parseAssignmentOrPipeline(parent, Group)
		return
	}

	for _, cast := range info.castSpans {
		if _, _, ok := resolvePSDotNetType(p.src[cast.start:cast.end], p.activeNamespaces); !ok {
			p.noteScriptOnlyOpaque(psOpaqueDotNetCall, cast.line)
		}
	}
	if info.simpleVar != "" {
		p.observeVariableName(info.simpleVar, stmtLine)
	}
	for _, name := range info.multiVars {
		p.observeVariableName(name, stmtLine)
	}
	for _, sp := range info.operandSpans {
		p.runDeferredSpan(sp, parent)
	}
	p.pos = info.afterOpPos
	p.line = info.afterOpLine
	p.skipTrivia(true)
	if p.atStatementTerminator() {
		p.noteOpaque(psOpaqueUnsupportedSyntax, stmtLine)
		return
	}
	rhsStart := p.pos

	// Check assignment-wrapped [Environment]::SetEnvironmentVariable(...)
	if info.op == "=" && info.simpleVar != "" && !strings.HasPrefix(asciiLower(info.simpleVar), "env:") {
		if p.looksLikeDirectEnvSetterAt(rhsStart) {
			p.parseAssignedEnvironmentSetter(stmtStart, stmtLine, lhsRaw, info.simpleVar, parent, before)
			return
		}
	}

	// Check $env:NAME = <rhs> / ${env:NAME} = <rhs>
	if info.simpleVar != "" && strings.HasPrefix(asciiLower(info.simpleVar), "env:") {
		p.parseEnvDriveAssignment(stmtStart, stmtLine, lhsRaw, info, parent, before)
		return
	}

	// Check $function:... = <rhs>, $alias:... = <rhs>, and provider-qualified ${c:...} = <rhs>.
	if info.simpleVar != "" && strings.HasPrefix(asciiLower(info.simpleVar), "function:") {
		p.noteScriptOnlyOpaque(psOpaqueFunctionShadow, stmtLine)
		p.parseAssignmentOrPipeline(parent, Group)
		return
	}
	if info.simpleVar != "" && strings.HasPrefix(asciiLower(info.simpleVar), "alias:") {
		p.noteScriptOnlyOpaque(psOpaqueAlias, stmtLine)
		p.parseAssignmentOrPipeline(parent, Group)
		return
	}
	if info.simpleVar != "" {
		if fsPath, isFS, isOpaque := classifyProviderVariable(info.simpleVar); isFS || isOpaque {
			if isOpaque {
				p.noteScriptOnlyOpaque(psOpaqueDynamicCall, stmtLine)
			}
			if isFS {
				p.noteScriptOnlyOpaque(psOpaqueDynamicCall, stmtLine)
				tWord := Word{Text: fsPath, Raw: fsPath, Literal: !strings.ContainsAny(fsPath, "$`")}
				p.appendInvocation(Invocation{
					Words: []Word{
						{Text: "out-file", Raw: "out-file", Literal: true},
						{Text: "-FilePath", Raw: "-FilePath", Literal: true},
						tWord,
					},
					OutputRedirects: []Redirection{{FD: 1, Operator: ">", Target: tWord}},
					Before:          before,
					Parent:          parent,
					Raw:             lhsRaw,
				}, psInvocationMeta{
					RawCommand: "out-file",
					Line:       stmtLine,
				})
			}
			p.parseAssignmentOrPipeline(parent, Group)
			return
		}
	}

	// Ordinary variable or property/multi-variable assignment.
	rhsWord, isSimpleScalar := p.tryPeekSimpleScalarRHS()
	p.parseAssignmentOrPipeline(parent, Group)
	stmtRaw := strings.TrimSpace(p.src[stmtStart:p.pos])

	if len(info.multiVars) > 0 {
		for _, v := range info.multiVars {
			p.recordForeachVarWrite(v, stmtLine)
		}
		return
	}
	cleanVar, scoped := stripPSScope(info.simpleVar)
	if cleanVar == "" {
		return
	}
	castAllowsProof := !info.hasTypeCast || strings.EqualFold(info.castType, "string") || strings.EqualFold(info.castType, "system.string")
	if info.op == "=" && !scoped && castAllowsProof && isSimpleScalar {
		p.recordBareAssignment(Word{
			Text:        cleanVar + "=" + rhsWord.Text,
			Raw:         stmtRaw,
			Quoted:      rhsWord.Quoted,
			Literal:     rhsWord.Literal,
			Substituted: rhsWord.Substituted,
		})
	} else {
		p.recordBareAssignment(Word{
			Text:    cleanVar + "=",
			Raw:     stmtRaw,
			Literal: false,
		})
	}
}

func (p *psParser) tryPeekSimpleScalarRHS() (Word, bool) {
	savedPos, savedLine, savedSteps := p.pos, p.line, p.steps
	savedOpaque := p.res.psOpacity
	savedConsumes := p.res.ConsumesParams
	defer func() {
		p.pos, p.line, p.steps = savedPos, savedLine, savedSteps
		p.res.psOpacity = savedOpaque
		p.res.ConsumesParams = savedConsumes
	}()
	if p.pos >= len(p.src) {
		return Word{}, false
	}
	var w Word
	switch p.src[p.pos] {
	case '\'':
		txt, raw, ok := p.scanSingleQuotedString()
		if !ok {
			return Word{}, false
		}
		w = Word{Text: txt, Raw: raw, Quoted: true, Literal: true}
	case '"':
		var deferred []psDeferredSpan
		txt, raw, lit, sub, ok := p.scanDoubleQuotedString(-1, &deferred)
		if !ok {
			return Word{}, false
		}
		w = Word{Text: txt, Raw: raw, Quoted: true, Literal: lit, Substituted: sub}
	case '$':
		if p.pos+1 < len(p.src) && p.src[p.pos+1] == '(' {
			rawStart := p.pos
			p.pos++
			if _, _, _, ok := p.scanBalancedSpan(')'); !ok {
				return Word{}, false
			}
			raw := p.src[rawStart:p.pos]
			w = Word{Text: raw, Raw: raw, Literal: false, Substituted: true}
		} else {
			return Word{}, false
		}
	default:
		if p.src[p.pos] >= '0' && p.src[p.pos] <= '9' {
			start := p.pos
			for p.pos < len(p.src) && (p.src[p.pos] >= '0' && p.src[p.pos] <= '9' || p.src[p.pos] == '.') {
				p.pos++
			}
			raw := p.src[start:p.pos]
			w = Word{Text: raw, Raw: raw, Literal: true}
		} else {
			return Word{}, false
		}
	}
	p.skipTrivia(false)
	if !p.atStatementTerminator() {
		return Word{}, false
	}
	return w, true
}

func (p *psParser) looksLikeDirectEnvSetterAt(pos int) bool {
	if pos >= len(p.src) || p.src[pos] != '[' {
		return false
	}
	return environmentMethodEnd(p.src[pos:]) > 0
}

func (p *psParser) parseAssignedEnvironmentSetter(stmtStart, stmtLine int, lhsRaw, simpleVar string, parent int, before Separator) {
	rhsStart := p.pos
	// Advance across [Environment]::SetEnvironmentVariable(...)
	closeBr := strings.IndexByte(p.src[p.pos:], ']')
	p.pos += closeBr + 1
	p.pos += 2 // "::"
	for p.pos < len(p.src) && isASCIILetter(p.src[p.pos]) {
		p.pos++
	}
	p.skipTrivia(false)
	var args []psDotNetArg
	var argSpans []psDeferredSpan
	if p.pos < len(p.src) && p.src[p.pos] == '(' {
		args, argSpans = p.scanDotNetCallArgs()
	}
	p.skipTrivia(false)
	stmtRaw := strings.TrimSpace(p.src[stmtStart:p.pos])
	rhsRaw := strings.TrimSpace(p.src[rhsStart:p.pos])
	p.recordNonConstantWrite(simpleVar)

	var words []Word
	isTightSimple := !strings.ContainsAny(stmtRaw[:len(lhsRaw)+1], " \t\r\n:") &&
		strings.HasPrefix(stmtRaw, lhsRaw+"=") &&
		isSimpleDollarIdent(lhsRaw)
	if isTightSimple {
		words = []Word{{Text: stmtRaw, Raw: stmtRaw, Literal: false}}
	} else {
		words = []Word{
			{Text: lhsRaw, Raw: lhsRaw, Literal: false},
			{Text: "=", Raw: "=", Literal: true},
			{Text: rhsRaw, Raw: rhsRaw, Literal: false},
		}
	}
	envMeta := environmentCallMetadata(p.ctx, stmtRaw, words)
	opaque := p.checkAndRecordEnvSetterArgs(args, stmtLine)
	idx := p.appendInvocation(Invocation{
		EnvironmentCall: envMeta,
		Words:           words,
		Before:          before,
		Parent:          parent,
		Raw:             stmtRaw,
	}, psInvocationMeta{
		RawCommand: rhsRaw,
		Opaque:     opaque,
		Line:       stmtLine,
	})
	for _, sp := range argSpans {
		p.runDeferredSpan(sp, idx)
	}
}

func isSimpleDollarIdent(s string) bool {
	if len(s) < 2 || s[0] != '$' {
		return false
	}
	for i := 1; i < len(s); i++ {
		c := s[i]
		if !(isASCIILetter(c) || (c >= '0' && c <= '9') || c == '_') {
			return false
		}
	}
	return true
}

func (p *psParser) parseEnvDriveAssignment(stmtStart, stmtLine int, lhsRaw string, info psLHSInfo, parent int, before Separator) {
	rhsStart := p.pos
	rhsScalar, isScalar := p.tryPeekSimpleScalarRHS()
	// Defer RHS sub-invocations until after the $env: invocation is emitted.
	rhsEndPos, rhsEndLine, rhsDeferred := p.scanSubStatementSpan()
	rhsRaw := strings.TrimSpace(p.src[rhsStart:rhsEndPos])
	stmtRaw := strings.TrimSpace(p.src[stmtStart:rhsEndPos])
	p.pos, p.line = rhsEndPos, rhsEndLine

	envName := strings.TrimPrefix(info.simpleVar[4:], `\`)
	litName := envName != "" && !strings.ContainsAny(envName, "$`*?[]")
	pinned := !litName || isPSPinnedEnv(envName)

	val := rhsRaw
	litVal := false
	if info.op == "=" {
		if isScalar && rhsScalar.Literal && isPSScalarRaw(rhsScalar.Raw) {
			val = rhsScalar.Text
			litVal = true
		} else if strings.EqualFold(rhsRaw, "$null") {
			val = ""
			litVal = true
		}
	}
	p.recordEnvMutation(psEnvMutation{
		Name:         envName,
		LiteralName:  litName,
		Value:        val,
		LiteralValue: litVal,
		Line:         stmtLine,
		Pinned:       pinned,
	})
	var opaque psOpaqueKind
	if pinned && p.mode == psModeScript {
		opaque = psOpaquePinnedAssignment
	}
	words := []Word{
		{Text: lhsRaw, Raw: lhsRaw, Literal: false},
		{Text: info.op, Raw: info.op, Literal: true},
	}
	if rhsRaw != "" {
		if isScalar {
			words = append(words, rhsScalar)
		} else {
			words = append(words, Word{Text: rhsRaw, Raw: rhsRaw, Literal: false})
		}
	}
	idx := p.appendInvocation(Invocation{
		Words:  words,
		Before: before,
		Parent: parent,
		Raw:    stmtRaw,
	}, psInvocationMeta{
		RawCommand: lhsRaw,
		Opaque:     opaque,
		Line:       stmtLine,
	})
	p.runSubParser(rhsDeferred.start, rhsDeferred.end, rhsDeferred.line, idx, Group, true, false)
}

func (p *psParser) scanSubStatementSpan() (endPos, endLine int, span psDeferredSpan) {
	span = psDeferredSpan{start: p.pos, line: p.line, kind: Group, stmtList: true}
	depthParen, depthBrace, depthBracket := 0, 0, 0
	for p.pos < len(p.src) {
		if !p.step() {
			break
		}
		if depthParen == 0 && depthBrace == 0 && depthBracket == 0 && p.atStatementTerminator() {
			break
		}
		c := p.src[p.pos]
		switch c {
		case '\n':
			p.line++
			p.pos++
		case '\r':
			if p.pos+1 >= len(p.src) || p.src[p.pos+1] != '\n' {
				p.line++
			}
			p.pos++
		case '`':
			p.pos++
			if p.pos < len(p.src) {
				if p.src[p.pos] == '\n' {
					p.line++
					p.pos++
					continue
				}
				if p.src[p.pos] == '\r' {
					p.pos++
					if p.pos < len(p.src) && p.src[p.pos] == '\n' {
						p.pos++
					}
					p.line++
					continue
				}
				p.pos++
			} else {
				p.noteOpaque(psOpaqueUnsupportedSyntax, p.line)
			}
		case '#':
			for p.pos < len(p.src) && p.src[p.pos] != '\n' && p.src[p.pos] != '\r' {
				p.pos++
			}
		case '<':
			if p.pos+1 < len(p.src) && p.src[p.pos+1] == '#' {
				p.skipBlockCommentFromHere()
				continue
			}
			p.pos++
		case '@':
			if p.pos+1 < len(p.src) && (p.src[p.pos+1] == '\'' || p.src[p.pos+1] == '"') {
				p.scanHereStringRawOnly()
				continue
			}
			p.pos++
		case '\'':
			p.skipSingleQuotedRawOnly()
		case '"':
			p.skipDoubleQuotedRawOnly()
		case '(':
			depthParen++
			p.pos++
		case ')':
			if depthParen > 0 {
				depthParen--
				p.pos++
			} else {
				span.end = p.pos
				return p.pos, p.line, span
			}
		case '{':
			depthBrace++
			p.pos++
		case '}':
			if depthBrace > 0 {
				depthBrace--
				p.pos++
			} else {
				span.end = p.pos
				return p.pos, p.line, span
			}
		case '[':
			depthBracket++
			p.pos++
		case ']':
			if depthBracket > 0 {
				depthBracket--
			}
			p.pos++
		default:
			p.pos++
		}
	}
	span.end = p.pos
	return p.pos, p.line, span
}

func (p *psParser) parsePipelineChain(parent int, before Separator) {
	curBefore := before
	for p.pos < len(p.src) {
		if !p.step() {
			return
		}
		p.parsePipeline(parent, curBefore)
		p.skipTrivia(false)
		if p.pos+1 < len(p.src) && p.src[p.pos] == '&' && p.src[p.pos+1] == '&' {
			p.pos += 2
			p.skipTrivia(true)
			if p.pos >= len(p.src) || p.atStatementTerminator() {
				p.noteOpaque(psOpaqueUnsupportedSyntax, p.line)
				return
			}
			curBefore = And
			continue
		}
		if p.pos+1 < len(p.src) && p.src[p.pos] == '|' && p.src[p.pos+1] == '|' {
			p.pos += 2
			p.skipTrivia(true)
			if p.pos >= len(p.src) || p.atStatementTerminator() {
				p.noteOpaque(psOpaqueUnsupportedSyntax, p.line)
				return
			}
			curBefore = Or
			continue
		}
		if p.pos < len(p.src) && p.src[p.pos] == '&' {
			p.pos++
			p.skipTrivia(false)
			if p.atStatementTerminator() {
				return
			}
			curBefore = Background
			continue
		}
		return
	}
}

func (p *psParser) parsePipeline(parent int, before Separator) {
	if p.pos < len(p.src) && p.src[p.pos] == '|' && (p.pos+1 >= len(p.src) || p.src[p.pos+1] != '|') {
		p.noteOpaque(psOpaqueUnsupportedSyntax, p.line)
		p.pos++
		return
	}
	inEdge := 0
	curBefore := before
	for p.pos < len(p.src) {
		if !p.step() {
			return
		}
		stageStartInv := len(p.res.Invocations)
		directIdx := p.parsePipelineStage(parent, curBefore, inEdge)
		p.skipTrivia(false)
		if p.pos < len(p.src) && p.src[p.pos] == '|' && (p.pos+1 >= len(p.src) || p.src[p.pos+1] != '|') {
			pipeLine := p.line
			p.pos++
			p.skipTrivia(true)
			if p.pos >= len(p.src) || p.atStatementTerminator() || (p.src[p.pos] == '|' && (p.pos+1 >= len(p.src) || p.src[p.pos+1] != '|')) {
				p.noteOpaque(psOpaqueUnsupportedSyntax, pipeLine)
				return
			}
			*p.nextPipeEdge++
			edge := *p.nextPipeEdge
			if directIdx >= 0 {
				p.res.Invocations[directIdx].PipelineOutput = true
				p.res.Invocations[directIdx].PipelineOutputEdge = edge
			} else {
				for i := stageStartInv; i < len(p.res.Invocations); i++ {
					p.res.Invocations[i].PipelineOutput = true
					if p.res.Invocations[i].PipelineOutputEdge == 0 {
						p.res.Invocations[i].PipelineOutputEdge = edge
					}
				}
			}
			inEdge = edge
			curBefore = Pipe
			continue
		}
		return
	}
}

func (p *psParser) isExpressionStageStart() bool {
	if p.pos >= len(p.src) {
		return false
	}
	c := p.src[p.pos]
	switch c {
	case '$', '\'', '"', '(', '[', '{':
		return true
	case '@':
		return true
	case '!':
		return true
	case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		return true
	case '+':
		return true
	case '.':
		return p.pos+1 < len(p.src) && p.src[p.pos+1] >= '0' && p.src[p.pos+1] <= '9'
	case '-':
		if p.pos+1 < len(p.src) {
			next := p.src[p.pos+1]
			if next == '-' {
				if p.pos+2 < len(p.src) && p.src[p.pos+2] == '%' {
					return false
				}
				return p.pos+2 < len(p.src) && (p.src[p.pos+2] == '$' || p.src[p.pos+2] == '(' || p.src[p.pos+2] == '[')
			}
			if next >= '0' && next <= '9' || next == '$' || next == '(' || next == '[' {
				return true
			}
			if next == '.' {
				return p.pos+2 < len(p.src) && p.src[p.pos+2] >= '0' && p.src[p.pos+2] <= '9'
			}
		}
		kw := p.peekDashOperator()
		switch kw {
		case "-not", "-cnot", "-inot", "-bnot", "-split", "-csplit", "-isplit", "-join":
			return true
		}
		return false
	}
	return false
}

func (p *psParser) peekDashOperator() string {
	if p.pos >= len(p.src) {
		return ""
	}
	r, w := utf8.DecodeRuneInString(p.src[p.pos:])
	if !isPSDashRune(r) {
		return ""
	}
	i := p.pos + w
	for i < len(p.src) && isASCIILetter(p.src[i]) {
		i++
	}
	if i == p.pos+w {
		return ""
	}
	if i < len(p.src) && !strings.ContainsRune(" \t\r\n([$'\"@+-!", rune(p.src[i])) {
		return ""
	}
	return "-" + asciiLower(p.src[p.pos+w:i])
}

func (p *psParser) parsePipelineStage(parent int, before Separator, inEdge int) int {
	p.skipTrivia(false)
	if p.pos >= len(p.src) || p.atStatementTerminator() {
		return -1
	}

	// Call operator '&' or dot-source operator '.'
	if p.src[p.pos] == '&' && (p.pos+1 >= len(p.src) || (p.src[p.pos+1] != '&' && p.src[p.pos+1] != '>')) {
		return p.parseCommandStage(psCallAmpersand, parent, before, inEdge)
	}
	if p.src[p.pos] == '.' && p.isDotSourceOperator() {
		return p.parseCommandStage(psCallDot, parent, before, inEdge)
	}

	if p.isExpressionStageStart() {
		return p.parseExpressionStage(parent, before, inEdge)
	}
	return p.parseCommandStage(psCallNone, parent, before, inEdge)
}

func (p *psParser) isDotSourceOperator() bool {
	if p.pos+1 >= len(p.src) {
		return true // bare '.' at EOF -> syntax error handled in parseCommandStage
	}
	next := p.src[p.pos+1]
	switch next {
	case ' ', '\t', '{', '\'', '"', '$', '(', '@', ';', '\r', '\n', '|':
		return true
	}
	return false
}

func (p *psParser) parseExpressionStage(parent int, before Separator, inEdge int) int {
	stageStart := p.pos
	stageLine := p.line
	startInv := len(p.res.Invocations)

	// Direct or parenthesized [Environment]::SetEnvironmentVariable(...) stage.
	if envIdx, handled := p.tryParseStandaloneEnvSetterStage(parent, before, inEdge); handled {
		return envIdx
	}

	p.parseExpression(parent)
	p.skipTrivia(false)
	inRedirs, outRedirs, redirDeferred := p.parseStageRedirections()
	for _, sp := range redirDeferred {
		p.runDeferredSpan(sp, parent)
	}
	p.skipTrivia(false)
	if !p.atPipelineStageBoundary() {
		p.noteOpaque(psOpaqueUnsupportedSyntax, p.line)
		for p.pos < len(p.src) && !p.atPipelineStageBoundary() {
			if p.src[p.pos] == '$' && p.pos+1 < len(p.src) && p.src[p.pos+1] == '(' {
				p.pos++
				inS, inE, sL, ok := p.scanBalancedSpan(')')
				if ok {
					p.runSubParser(inS, inE, sL, parent, Substitution, true, false)
				}
				continue
			}
			p.pos++
		}
	}

	if inEdge != 0 {
		for i := startInv; i < len(p.res.Invocations); i++ {
			p.res.Invocations[i].PipelineInput = true
			if p.res.Invocations[i].PipelineInputEdge == 0 {
				p.res.Invocations[i].PipelineInputEdge = inEdge
			}
		}
	}

	if len(outRedirs) > 0 {
		stageRaw := strings.TrimSpace(p.src[stageStart:p.pos])
		idx := p.appendInvocation(Invocation{
			Words: []Word{
				{Text: "out-file", Raw: "out-file", Literal: true},
				{Text: "-FilePath", Raw: "-FilePath", Literal: true},
				outRedirs[0].Target,
			},
			InputRedirects:    inRedirs,
			OutputRedirects:   outRedirs,
			Before:            before,
			Parent:            parent,
			Raw:               stageRaw,
			PipelineInput:     inEdge != 0,
			PipelineInputEdge: inEdge,
		}, psInvocationMeta{
			RawCommand: "out-file",
			Line:       stageLine,
		})
		for _, r := range outRedirs {
			p.checkProviderDriveTarget(r.Target, stageLine, idx)
		}
		return idx
	}
	return -1
}

func (p *psParser) tryParseStandaloneEnvSetterStage(parent int, before Separator, inEdge int) (int, bool) {
	savedPos, savedLine, savedSteps := p.pos, p.line, p.steps
	savedOpaque := p.res.psOpacity
	restore := func() {
		p.pos, p.line, p.steps = savedPos, savedLine, savedSteps
		p.res.psOpacity = savedOpaque
	}

	stageStart := p.pos
	stageLine := p.line
	// Scan the stage (either [Environment]::SetEnvironmentVariable(...) or grouped (...))
	parenWraps := 0
	for p.pos < len(p.src) && p.src[p.pos] == '(' {
		if parenWraps == maxScriptNesting {
			restore()
			return -1, false
		}
		parenWraps++
		p.pos++
		p.skipTrivia(true)
	}
	callStart := p.pos
	if environmentMethodEnd(p.src[callStart:]) == 0 {
		restore()
		return -1, false
	}
	closeBr := strings.IndexByte(p.src[p.pos:], ']')
	if closeBr < 0 {
		restore()
		return -1, false
	}
	p.pos += closeBr + 1 + 2 // past ']::'
	for p.pos < len(p.src) && isASCIILetter(p.src[p.pos]) {
		p.pos++
	}
	p.skipTrivia(false)
	if p.pos >= len(p.src) || p.src[p.pos] != '(' {
		restore()
		return -1, false
	}
	args, argSpans := p.scanDotNetCallArgs()
	callEnd := p.pos
	for k := 0; k < parenWraps; k++ {
		p.skipTrivia(true)
		if p.pos >= len(p.src) || p.src[p.pos] != ')' {
			restore()
			return -1, false
		}
		p.pos++
	}
	p.skipTrivia(false)
	if !p.atPipelineStageBoundary() {
		restore()
		return -1, false
	}
	stageRaw := strings.TrimSpace(p.src[stageStart:p.pos])
	unwrappedCallRaw := strings.TrimSpace(p.src[callStart:callEnd])
	words := []Word{{Text: unwrappedCallRaw, Raw: unwrappedCallRaw, Literal: false}}
	envMeta := environmentCallMetadata(p.ctx, unwrappedCallRaw, words)
	opaque := p.checkAndRecordEnvSetterArgs(args, stageLine)
	idx := p.appendInvocation(Invocation{
		EnvironmentCall:   envMeta,
		Words:             words,
		Before:            before,
		Parent:            parent,
		Raw:               stageRaw,
		PipelineInput:     inEdge != 0,
		PipelineInputEdge: inEdge,
	}, psInvocationMeta{
		RawCommand: unwrappedCallRaw,
		Opaque:     opaque,
		Line:       stageLine,
	})
	for _, sp := range argSpans {
		p.runDeferredSpan(sp, idx)
	}
	return idx, true
}

func (p *psParser) checkAndRecordEnvSetterArgs(args []psDotNetArg, line int) psOpaqueKind {
	if len(args) < 2 || len(args) > 3 {
		p.recordEnvMutation(psEnvMutation{
			LiteralName: false, Line: line, Pinned: true,
		})
		return p.noteScriptOnlyOpaque(psOpaqueDotNetCall, line)
	}
	if len(args) == 3 {
		a2 := args[2].word
		if !a2.Literal || !isPSScalarRaw(a2.Raw) || !strings.EqualFold(a2.Text, "Process") {
			p.recordEnvMutation(psEnvMutation{
				Name: args[0].word.Text, LiteralName: false, Line: line, Pinned: true,
			})
			return p.noteScriptOnlyOpaque(psOpaqueDotNetCall, line)
		}
	}
	a0 := args[0].word
	litName := a0.Literal && isPSScalarRaw(a0.Raw) && a0.Text != ""
	pinned := !litName || isPSPinnedEnv(a0.Text)

	a1 := args[1].word
	val := args[1].raw
	litVal := false
	if a1.Literal && isPSScalarRaw(a1.Raw) {
		val = a1.Text
		litVal = true
	} else if strings.EqualFold(strings.TrimSpace(args[1].raw), "$null") {
		val = ""
		litVal = true
	}
	p.recordEnvMutation(psEnvMutation{
		Name:         a0.Text,
		LiteralName:  litName,
		Value:        val,
		LiteralValue: litVal,
		Line:         line,
		Pinned:       pinned,
	})
	if pinned {
		return p.noteScriptOnlyOpaque(psOpaquePinnedAssignment, line)
	}
	return psOpaqueNone
}

func (p *psParser) atPipelineStageBoundary() bool {
	if p.pos >= len(p.src) {
		return true
	}
	switch p.src[p.pos] {
	case ';', '\r', '\n', '|', '&', '}', ')':
		return true
	}
	return false
}

func (p *psParser) parseExpression(parent int) (isFileInfoCtor bool) {
	isFileInfoCtor = p.parseUnaryAndPrimary(parent)
	for p.pos < len(p.src) {
		if !p.step() {
			return false
		}
		p.skipTrivia(false)
		if p.pos >= len(p.src) {
			break
		}
		// Binary operators (allowing natural line continuation after the operator).
		if p.matchBinaryOperator() {
			isFileInfoCtor = false
			p.skipTrivia(true)
			p.parseUnaryAndPrimary(parent)
			continue
		}
		// PowerShell 7 ternary: cond ? trueExpr : falseExpr
		if p.src[p.pos] == '?' && (p.pos+1 >= len(p.src) || (p.src[p.pos+1] != '?' && p.src[p.pos+1] != '.')) {
			isFileInfoCtor = false
			p.pos++
			p.skipTrivia(true)
			p.parseExpressionUntilColon(parent)
			p.skipTrivia(true)
			if p.pos < len(p.src) && p.src[p.pos] == ':' {
				p.pos++
				p.skipTrivia(true)
				p.parseUnaryAndPrimary(parent)
			} else {
				p.noteOpaque(psOpaqueUnsupportedSyntax, p.line)
			}
			continue
		}
		break
	}
	return isFileInfoCtor
}

func (p *psParser) parseExpressionStopAtComma(parent int) {
	p.parseUnaryAndPrimary(parent)
	for p.pos < len(p.src) {
		if !p.step() {
			return
		}
		p.skipTrivia(false)
		if p.pos >= len(p.src) || p.src[p.pos] == ',' {
			return
		}
		if p.matchBinaryOperatorNoComma() {
			p.skipTrivia(true)
			p.parseUnaryAndPrimary(parent)
			continue
		}
		return
	}
}

func (p *psParser) parseExpressionUntilColon(parent int) {
	p.parseUnaryAndPrimary(parent)
	for p.pos < len(p.src) {
		if !p.step() {
			return
		}
		p.skipTrivia(false)
		if p.pos >= len(p.src) || p.src[p.pos] == ':' {
			return
		}
		if p.matchBinaryOperator() {
			p.skipTrivia(true)
			p.parseUnaryAndPrimary(parent)
			continue
		}
		return
	}
}

func (p *psParser) matchBinaryOperator() bool {
	if p.pos < len(p.src) && p.src[p.pos] == ',' {
		p.pos++
		return true
	}
	return p.matchBinaryOperatorNoComma()
}

func (p *psParser) matchBinaryOperatorNoComma() bool {
	if p.pos >= len(p.src) {
		return false
	}
	c := p.src[p.pos]
	switch c {
	case '+', '*', '/', '%':
		// Ensure not +=, *=, /=, %= or ++.
		if p.pos+1 < len(p.src) && (p.src[p.pos+1] == '=' || (c == '+' && p.src[p.pos+1] == '+')) {
			return false
		}
		p.pos++
		return true
	case '.':
		if p.pos+1 < len(p.src) && p.src[p.pos+1] == '.' {
			p.pos += 2
			return true
		}
		return false
	case '?':
		if p.pos+1 < len(p.src) && p.src[p.pos+1] == '?' {
			p.pos += 2
			return true
		}
		return false
	}
	r, w := utf8.DecodeRuneInString(p.src[p.pos:])
	if isPSDashRune(r) {
		// Binary '-' or '-op'.
		if p.pos+w < len(p.src) && (p.src[p.pos+w] == '=' || p.src[p.pos+w] == '-') {
			return false
		}
		i := p.pos + w
		for i < len(p.src) && isASCIILetter(p.src[i]) {
			i++
		}
		if i > p.pos+w {
			op := "-" + asciiLower(p.src[p.pos+w:i])
			if psBinaryDashOps[op] && (i >= len(p.src) || strings.ContainsRune(" \t\r\n([$'\"@+-!0123456789", rune(p.src[i]))) {
				p.pos = i
				return true
			}
			return false
		}
		// Plain arithmetic subtraction '-' when not a redirection or flag.
		p.pos += w
		return true
	}
	return false
}

func (p *psParser) parseUnaryAndPrimary(parent int) (isFileInfoCtor bool) {
	p.skipTrivia(false)
	for p.pos < len(p.src) {
		if !p.step() {
			return false
		}
		c := p.src[p.pos]
		if c == '!' {
			p.pos++
			p.skipTrivia(true)
			continue
		}
		if (c == '+' || c == '-') && p.pos+1 < len(p.src) && p.src[p.pos+1] == c {
			p.pos += 2
			p.skipTrivia(false)
			if p.pos < len(p.src) && p.src[p.pos] == '$' {
				vn := p.peekVarAtPos()
				if vn != "" {
					p.recordForeachVarWrite(vn, p.line)
				}
			}
			continue
		}
		if c == '+' || (c == '-' && p.peekDashOperator() == "") {
			p.pos++
			p.skipTrivia(false)
			continue
		}
		if dop := p.peekDashOperator(); dop == "-not" || dop == "-cnot" || dop == "-inot" || dop == "-bnot" || dop == "-split" || dop == "-csplit" || dop == "-isplit" || dop == "-join" {
			_, w := utf8.DecodeRuneInString(p.src[p.pos:])
			p.pos += w + len(dop) - 1
			p.skipTrivia(true)
			continue
		}
		// Check [Type] cast when NOT followed by '::' and followed by a castable operand.
		if c == '[' && p.isTypeCastPrefix() {
			p.validateTypeCastBracket()
			p.skipTrivia(false)
			continue
		}
		break
	}
	isFileInfoCtor = p.parsePrimaryExpr(parent)
	return p.parsePostfixChain(parent, isFileInfoCtor)
}

func (p *psParser) peekVarAtPos() string {
	savedPos, savedLine := p.pos, p.line
	defer func() { p.pos, p.line = savedPos, savedLine }()
	return p.scanVariableToken()
}

func (p *psParser) isTypeCastPrefix() bool {
	savedPos, savedLine, savedSteps := p.pos, p.line, p.steps
	savedOpaque := p.res.psOpacity
	defer func() {
		p.pos, p.line, p.steps = savedPos, savedLine, savedSteps
		p.res.psOpacity = savedOpaque
	}()
	if _, _, _, ok := p.scanBalancedSpan(']'); !ok {
		return false
	}
	if p.pos+1 < len(p.src) && p.src[p.pos] == ':' && p.src[p.pos+1] == ':' {
		return false
	}
	p.skipTrivia(false)
	if p.pos >= len(p.src) {
		return false
	}
	switch p.src[p.pos] {
	case '$', '\'', '"', '(', '@', '[', '{', '+', '!':
		return true
	case '-':
		// Cast before negative number -[int]-5, NOT binary operator '[string] -eq $t'.
		return p.peekDashOperator() == ""
	}
	return p.src[p.pos] >= '0' && p.src[p.pos] <= '9'
}

func (p *psParser) parsePrimaryExpr(parent int) (isFileInfoCtor bool) {
	if p.pos >= len(p.src) {
		return false
	}
	line := p.line
	switch p.src[p.pos] {
	case '\'':
		p.scanSingleQuotedString()
		return false
	case '"':
		p.scanDoubleQuotedString(parent, nil)
		return false
	case '@':
		if p.pos+1 < len(p.src) {
			switch p.src[p.pos+1] {
			case '\'', '"':
				p.scanHereString(parent, nil)
				return false
			case '(':
				p.pos++ // at '('
				inStart, inEnd, sLine, ok := p.scanBalancedSpan(')')
				if ok {
					p.runSubParser(inStart, inEnd, sLine, parent, Group, true, false)
				}
				return false
			case '{':
				p.pos++ // at '{'
				inStart, inEnd, sLine, ok := p.scanBalancedSpan('}')
				if ok {
					p.runSubParser(inStart, inEnd, sLine, parent, Group, false, true)
				}
				return false
			}
		}
		p.noteOpaque(psOpaqueUnsupportedSyntax, line)
		p.pos++
		return false
	case '$':
		if p.pos+1 < len(p.src) && p.src[p.pos+1] == '(' {
			p.pos++ // at '('
			inStart, inEnd, sLine, ok := p.scanBalancedSpan(')')
			if ok {
				p.runSubParser(inStart, inEnd, sLine, parent, Substitution, true, false)
			}
			return false
		}
		vn := p.scanVariableToken()
		p.observeVariableRead(vn, line, parent, nil)
		if p.pos+1 < len(p.src) && ((p.src[p.pos] == '+' && p.src[p.pos+1] == '+') || (p.src[p.pos] == '-' && p.src[p.pos+1] == '-')) {
			p.recordForeachVarWrite(vn, line)
		}
		return false
	case '(':
		inStart, inEnd, sLine, ok := p.scanBalancedSpan(')')
		if !ok {
			return false
		}
		inner := strings.TrimSpace(p.src[inStart:inEnd])
		fiCtor := isFileInfoCtorRaw(inner, p.activeNamespaces)
		p.runSubParser(inStart, inEnd, sLine, parent, Group, true, false)
		return fiCtor
	case '{':
		prevScope := p.inChildParamScope
		p.inChildParamScope = true
		p.parseBracedBlock(parent, Group)
		p.inChildParamScope = prevScope
		return false
	case '[':
		return p.parseBracketStaticOrTypeExpr(parent)
	default:
		if (p.src[p.pos] >= '0' && p.src[p.pos] <= '9') || p.src[p.pos] == '.' {
			p.scanNumberLiteral()
			return false
		}
		p.noteOpaque(psOpaqueUnsupportedSyntax, line)
		p.pos++
		return false
	}
}

func isFileInfoCtorRaw(inner string, activeNamespaces []string) bool {
	inner = unwrapOuterParensTrim(inner)
	if strings.HasPrefix(inner, "[") {
		if closeIdx := strings.IndexByte(inner, ']'); closeIdx > 1 {
			typePart := inner[1:closeIdx]
			rest := strings.TrimSpace(inner[closeIdx+1:])
			if canonical, _, ok := resolvePSDotNetType(typePart, activeNamespaces); ok && canonical == "system.io.fileinfo" {
				if strings.HasPrefix(asciiLower(rest), "::new") {
					tail := strings.TrimSpace(rest[5:])
					return strings.HasPrefix(tail, "(") && strings.HasSuffix(tail, ")")
				}
			}
		}
		return false
	}
	lower := asciiLower(inner)
	if !strings.HasPrefix(lower, "new-object") || len(inner) <= len("new-object") {
		return false
	}
	if inner[len("new-object")] != ' ' && inner[len("new-object")] != '\t' {
		return false
	}
	rest := strings.TrimSpace(inner[len("new-object"):])
	if strings.HasPrefix(asciiLower(rest), "-typename") {
		rest = strings.TrimSpace(rest[len("-typename"):])
		rest = strings.TrimPrefix(rest, ":")
		rest = strings.TrimSpace(rest)
	}
	end := 0
	for end < len(rest) && !strings.ContainsRune(" \t\r\n(", rune(rest[end])) {
		end++
	}
	if end == 0 {
		return false
	}
	typeTok := strings.Trim(rest[:end], `"'`)
	canonical, _, ok := resolvePSDotNetType(typeTok, activeNamespaces)
	return ok && canonical == "system.io.fileinfo"
}

func (p *psParser) scanNumberLiteral() {
	if p.pos+1 < len(p.src) && p.src[p.pos] == '0' && (p.src[p.pos+1] == 'x' || p.src[p.pos+1] == 'X') {
		p.pos += 2
		for p.pos < len(p.src) && ((p.src[p.pos] >= '0' && p.src[p.pos] <= '9') || (p.src[p.pos] >= 'a' && p.src[p.pos] <= 'f') || (p.src[p.pos] >= 'A' && p.src[p.pos] <= 'F') || p.src[p.pos] == '_' || isASCIILetter(p.src[p.pos])) {
			p.pos++
		}
		return
	}
	for p.pos < len(p.src) && (p.src[p.pos] >= '0' && p.src[p.pos] <= '9' || p.src[p.pos] == '_') {
		p.pos++
	}
	if p.pos < len(p.src) && p.src[p.pos] == '.' && (p.pos+1 >= len(p.src) || p.src[p.pos+1] != '.') {
		p.pos++
		for p.pos < len(p.src) && (p.src[p.pos] >= '0' && p.src[p.pos] <= '9') {
			p.pos++
		}
	}
	if p.pos < len(p.src) && (p.src[p.pos] == 'e' || p.src[p.pos] == 'E') {
		p.pos++
		if p.pos < len(p.src) && (p.src[p.pos] == '+' || p.src[p.pos] == '-') {
			p.pos++
		}
		for p.pos < len(p.src) && (p.src[p.pos] >= '0' && p.src[p.pos] <= '9') {
			p.pos++
		}
	}
	for p.pos < len(p.src) && isASCIILetter(p.src[p.pos]) {
		p.pos++
	}
}

func (p *psParser) parseBracketStaticOrTypeExpr(parent int) (isFileInfoCtor bool) {
	exprStart := p.pos
	line := p.line
	inStart, inEnd, _, ok := p.scanBalancedSpan(']')
	if !ok {
		return false
	}
	typeRaw := strings.TrimSpace(p.src[inStart:inEnd])
	if p.pos+1 < len(p.src) && p.src[p.pos] == ':' && p.src[p.pos+1] == ':' {
		p.pos += 2
		memberName, litMember := p.scanMemberName(parent)
		if !litMember {
			p.noteScriptOnlyOpaque(psOpaqueDotNetCall, line)
			if p.pos < len(p.src) && p.src[p.pos] == '(' {
				_, spans := p.scanDotNetCallArgs()
				for _, sp := range spans {
					p.runDeferredSpan(sp, parent)
				}
			}
			return false
		}
		savedBeforeParen := p.pos
		p.skipTrivia(false)
		hasParens := p.pos < len(p.src) && p.src[p.pos] == '('
		var args []psDotNetArg
		var argSpans []psDeferredSpan
		if hasParens {
			args, argSpans = p.scanDotNetCallArgs()
		} else {
			p.pos = savedBeforeParen
		}
		callRaw := strings.TrimSpace(p.src[exprStart:p.pos])
		ev := evalPSDotNetStatic(typeRaw, memberName, hasParens, false, args, p.activeNamespaces)
		if ev.isEnvSetter {
			words := []Word{{Text: callRaw, Raw: callRaw, Literal: false}}
			envMeta := environmentCallMetadata(p.ctx, callRaw, words)
			opaque := p.checkAndRecordEnvSetterArgs(args, line)
			idx := p.appendInvocation(Invocation{
				EnvironmentCall: envMeta,
				Words:           words,
				Before:          Group,
				Parent:          parent,
				Raw:             callRaw,
			}, psInvocationMeta{
				RawCommand: callRaw,
				Opaque:     opaque,
				Line:       line,
			})
			for _, sp := range argSpans {
				p.runDeferredSpan(sp, idx)
			}
			return false
		}
		if ev.opaque != psOpaqueNone {
			if ev.bothModes {
				p.noteOpaque(ev.opaque, line)
			} else {
				p.noteScriptOnlyOpaque(ev.opaque, line)
			}
		}
		if ev.synthRead != nil {
			p.emitSyntheticGetContent(*ev.synthRead, line, parent, Group)
		}
		for _, sp := range argSpans {
			p.runDeferredSpan(sp, parent)
		}
		return ev.isFileInfoCtor
	}

	// Standalone [Type] literal (e.g. on RHS of -is [Type] / -as [Type] / [OutputType([Type])]).
	if _, _, valid := resolvePSDotNetType(typeRaw, p.activeNamespaces); !valid {
		p.noteScriptOnlyOpaque(psOpaqueDotNetCall, line)
	}
	return false
}

func (p *psParser) scanMemberName(parent int) (name string, literal bool) {
	if p.pos >= len(p.src) {
		p.noteOpaque(psOpaqueUnsupportedSyntax, p.line)
		return "", false
	}
	switch p.src[p.pos] {
	case '\'':
		txt, _, ok := p.scanSingleQuotedString()
		return txt, ok
	case '"':
		var def []psDeferredSpan
		txt, _, lit, _, ok := p.scanDoubleQuotedString(-1, &def)
		for _, sp := range def {
			p.runDeferredSpan(sp, parent)
		}
		return txt, ok && lit
	case '$':
		if p.pos+1 < len(p.src) && p.src[p.pos+1] == '(' {
			p.pos++
			inS, inE, sL, ok := p.scanBalancedSpan(')')
			if ok {
				p.runSubParser(inS, inE, sL, parent, Substitution, true, false)
			}
			return "", false
		}
		vn := p.scanVariableToken()
		p.observeVariableName(vn, p.line)
		return "", false
	case '(':
		p.parseParenStatements(parent, Group)
		return "", false
	default:
		var b strings.Builder
		for p.pos < len(p.src) {
			c := p.src[p.pos]
			if c == '`' {
				_, ok := p.decodePSBacktick(&b)
				if !ok {
					return b.String(), false
				}
				continue
			}
			if isASCIILetter(c) || (c >= '0' && c <= '9') || c == '_' {
				b.WriteByte(c)
				p.pos++
				continue
			}
			break
		}
		if b.Len() == 0 {
			p.noteOpaque(psOpaqueUnsupportedSyntax, p.line)
			return "", false
		}
		return b.String(), true
	}
}

func (p *psParser) scanDotNetCallArgs() ([]psDotNetArg, []psDeferredSpan) {
	inStart, inEnd, startLine, ok := p.scanBalancedSpan(')')
	if !ok {
		return nil, nil
	}
	return p.extractCommaExprArgs(inStart, inEnd, startLine)
}

func (p *psParser) extractCommaExprArgs(start, end, startLine int) ([]psDotNetArg, []psDeferredSpan) {
	var args []psDotNetArg
	var spans []psDeferredSpan
	if strings.TrimSpace(p.src[start:end]) == "" {
		return nil, nil
	}
	depth := 0
	inSingle, inDouble := false, false
	curStart := start
	curLine := startLine
	line := startLine
	flushArg := func(argEnd int) {
		raw := strings.TrimSpace(p.src[curStart:argEnd])
		w := p.exprArgWord(curStart, argEnd, curLine)
		args = append(args, psDotNetArg{word: w, raw: raw})
		spans = append(spans, psDeferredSpan{
			start: curStart, end: argEnd, line: curLine, kind: Group, stmtList: false,
		})
	}
	for i := start; i < end; i++ {
		c := p.src[i]
		if c == '\n' {
			line++
		}
		if inSingle {
			if c == '\'' {
				if i+1 < end && p.src[i+1] == '\'' {
					i++
					continue
				}
				inSingle = false
			}
			continue
		}
		if inDouble {
			if c == '`' && i+1 < end {
				i++
				if p.src[i] == '\n' {
					line++
				}
				continue
			}
			if c == '"' {
				if i+1 < end && p.src[i+1] == '"' {
					i++
					continue
				}
				inDouble = false
			}
			continue
		}
		switch c {
		case '`':
			if i+1 < end {
				i++
				if p.src[i] == '\n' {
					line++
				}
			}
		case '\'':
			inSingle = true
		case '"':
			inDouble = true
		case '(', '{', '[':
			depth++
		case ')', '}', ']':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				flushArg(i)
				curStart = i + 1
				curLine = line
			}
		}
	}
	flushArg(end)
	return args, spans
}

func (p *psParser) exprArgWord(start, end, startLine int) Word {
	raw := strings.TrimSpace(p.src[start:end])
	if raw == "" {
		return Word{}
	}
	savedSrc, savedPos, savedLine := p.src, p.pos, p.line
	savedOpaque := p.res.psOpacity
	savedConsumes := p.res.ConsumesParams
	defer func() {
		p.src, p.pos, p.line = savedSrc, savedPos, savedLine
		p.res.psOpacity = savedOpaque
		p.res.ConsumesParams = savedConsumes
	}()
	p.src = savedSrc[:end]
	p.pos = start
	p.line = startLine
	p.skipTrivia(true)
	if w, ok := p.tryPeekSimpleScalarRHS(); ok {
		return w
	}
	return Word{Text: raw, Raw: raw, Literal: false}
}

func (p *psParser) parseCommaExprListSpan(start, end, startLine int, parent int) {
	_, spans := p.extractCommaExprArgs(start, end, startLine)
	for _, sp := range spans {
		p.runDeferredSpan(sp, parent)
	}
}

func (p *psParser) parsePostfixChain(parent int, isFileInfoCtor bool) bool {
	for p.pos < len(p.src) {
		if !p.step() {
			return false
		}
		// Postfix ++ / --
		if p.pos+1 < len(p.src) && ((p.src[p.pos] == '+' && p.src[p.pos+1] == '+') || (p.src[p.pos] == '-' && p.src[p.pos+1] == '-')) {
			p.pos += 2
			isFileInfoCtor = false
			continue
		}
		// Indexing [expr] when attached without leading whitespace.
		if p.src[p.pos] == '[' {
			inS, inE, sL, ok := p.scanBalancedSpan(']')
			if ok {
				p.parseCommaExprListSpan(inS, inE, sL, parent)
			}
			isFileInfoCtor = false
			continue
		}
		// Member access .Member or ?.Member (not range '..').
		isNullCond := p.pos+1 < len(p.src) && p.src[p.pos] == '?' && p.src[p.pos+1] == '.'
		isDot := p.src[p.pos] == '.' && (p.pos+1 >= len(p.src) || p.src[p.pos+1] != '.')
		if isNullCond || isDot {
			mLine := p.line
			if isNullCond {
				p.pos += 2
			} else {
				p.pos++
			}
			p.skipTrivia(true)
			mName, litM := p.scanMemberName(parent)
			if p.pos < len(p.src) && p.src[p.pos] == '(' {
				mLower := asciiLower(mName)
				if !litM {
					p.noteScriptOnlyOpaque(psOpaqueDotNetCall, mLine)
				} else if psDangerousInstanceMethods[mLower] || (isFileInfoCtor && mLower == "replace") {
					p.noteScriptOnlyOpaque(psOpaqueDotNetCall, mLine)
				}
				_, argSpans := p.scanDotNetCallArgs()
				for _, sp := range argSpans {
					p.runDeferredSpan(sp, parent)
				}
			}
			isFileInfoCtor = false
			continue
		}
		break
	}
	return isFileInfoCtor
}

func (p *psParser) parseHashtableEntries(parent int) {
	for p.pos < len(p.src) {
		if !p.step() {
			return
		}
		p.skipTrivia(true)
		if p.pos >= len(p.src) {
			return
		}
		if p.src[p.pos] == ';' {
			p.pos++
			continue
		}
		// Key: string, parenthesized expr, variable, or bare key token.
		switch p.src[p.pos] {
		case '\'':
			p.scanSingleQuotedString()
		case '"':
			p.scanDoubleQuotedString(parent, nil)
		case '(':
			p.parseParenStatements(parent, Group)
		case '$':
			p.parsePrimaryExpr(parent)
		default:
			kStart := p.pos
			for p.pos < len(p.src) && !strings.ContainsRune(" \t\r\n=;{}", rune(p.src[p.pos])) {
				p.pos++
			}
			if p.pos == kStart {
				p.noteOpaque(psOpaqueUnsupportedSyntax, p.line)
				p.pos++
				continue
			}
		}
		p.skipTrivia(true)
		if p.pos >= len(p.src) || p.src[p.pos] != '=' {
			p.noteOpaque(psOpaqueUnsupportedSyntax, p.line)
			return
		}
		p.pos++
		p.skipTrivia(true)
		p.parseAssignmentOrPipeline(parent, Group)
	}
}

func (p *psParser) parseCommandStage(callKind psCallKind, parent int, before Separator, inEdge int) int {
	stageStart := p.pos
	stageLine := p.line
	if callKind != psCallNone {
		p.pos++ // skip '&' or '.'
		p.skipTrivia(false)
		if p.pos >= len(p.src) || p.atPipelineStageBoundary() {
			p.noteOpaque(psOpaqueUnsupportedSyntax, stageLine)
			return -1
		}
		// & { ... } or . { ... } scriptblock invocation.
		if p.src[p.pos] == '{' {
			return p.parseInvokedScriptBlock(callKind, stageStart, stageLine, parent, before, inEdge)
		}
	}

	var (
		words        []Word
		inRedirs     []Redirection
		outRedirs    []Redirection
		deferred     []psDeferredSpan
		sawSplatting bool
	)

	// Scan command word (Words[0]) first.
	headWord, headDef, headOK := p.scanArgumentElement(false)
	deferred = append(deferred, headDef...)
	if !headOK || headWord.Raw == "" {
		p.noteOpaque(psOpaqueUnsupportedSyntax, stageLine)
		return -1
	}
	words = append(words, headWord)

	// Scan remaining arguments and redirections in this pipeline stage.
	for p.pos < len(p.src) {
		if !p.step() {
			return -1
		}
		p.skipTrivia(false)
		if p.pos >= len(p.src) || p.atPipelineStageBoundary() {
			break
		}
		// Check redirection operator.
		if redir, isIn, isFileOut, rDef, matched := p.tryScanOneRedirection(); matched {
			deferred = append(deferred, rDef...)
			if isIn {
				inRedirs = append(inRedirs, redir)
			} else if isFileOut {
				outRedirs = append(outRedirs, redir)
			}
			continue
		}
		// Check command splatting @var (not @(...), @{...}, @'...', @"...").
		if p.src[p.pos] == '@' && p.pos+1 < len(p.src) &&
			p.src[p.pos+1] != '(' && p.src[p.pos+1] != '{' && p.src[p.pos+1] != '\'' && p.src[p.pos+1] != '"' {
			sawSplatting = true
		}
		argWord, argDef, argOK := p.scanArgumentElement(true)
		deferred = append(deferred, argDef...)
		if !argOK {
			break
		}
		if isStopParsingToken(argWord) {
			p.noteOpaque(psOpaqueUnsupportedSyntax, p.line)
			p.consumeToStatementEnd()
			break
		}
		if argWord.Raw != "" {
			words = append(words, argWord)
		}
	}

	stageRaw := strings.TrimSpace(p.src[stageStart:p.pos])
	rawCmd, modStripped := normalizePSWords(words)
	opaque, synthRead := p.classifyCommandInvocation(words, callKind, rawCmd, modStripped, sawSplatting, before, inEdge != 0, len(inRedirs) > 0, stageLine)

	idx := p.appendInvocation(Invocation{
		Words:             words,
		InputRedirects:    inRedirs,
		OutputRedirects:   outRedirs,
		Before:            before,
		Parent:            parent,
		Raw:               stageRaw,
		PipelineInput:     inEdge != 0,
		PipelineInputEdge: inEdge,
	}, psInvocationMeta{
		CallKind:       callKind,
		ModuleStripped: modStripped,
		RawCommand:     rawCmd,
		Opaque:         opaque,
		Line:           stageLine,
	})

	for _, r := range outRedirs {
		p.checkProviderDriveTarget(r.Target, stageLine, idx)
	}
	p.checkCommandProviderDrives(words, rawCmd, modStripped, stageLine, idx)

	if synthRead != nil {
		p.emitSyntheticGetContent(*synthRead, stageLine, idx, Group)
	}

	for _, sp := range deferred {
		p.runDeferredSpan(sp, idx)
	}
	return idx
}

func isStopParsingToken(w Word) bool {
	if w.Quoted || w.Raw == "" {
		return false
	}
	norm := normalizeLeadingDashes(w.Raw)
	return norm == "--%"
}

func (p *psParser) parseInvokedScriptBlock(callKind psCallKind, stageStart, stageLine int, parent int, before Separator, inEdge int) int {
	blockInStart, blockInEnd, blockLine, ok := p.scanBalancedSpan('}')
	if !ok {
		return -1
	}
	var (
		inRedirs  []Redirection
		outRedirs []Redirection
		deferred  []psDeferredSpan
	)
	for p.pos < len(p.src) {
		if !p.step() {
			return -1
		}
		p.skipTrivia(false)
		if p.pos >= len(p.src) || p.atPipelineStageBoundary() {
			break
		}
		if redir, isIn, isFileOut, rDef, matched := p.tryScanOneRedirection(); matched {
			deferred = append(deferred, rDef...)
			if isIn {
				inRedirs = append(inRedirs, redir)
			} else if isFileOut {
				outRedirs = append(outRedirs, redir)
			}
			continue
		}
		argW, argDef, argOK := p.scanArgumentElement(true)
		deferred = append(deferred, argDef...)
		if !argOK {
			break
		}
		if isStopParsingToken(argW) {
			p.noteOpaque(psOpaqueUnsupportedSyntax, p.line)
			p.consumeToStatementEnd()
			break
		}
	}
	stageRaw := strings.TrimSpace(p.src[stageStart:p.pos])

	prevScope := p.inChildParamScope
	if callKind == psCallAmpersand {
		p.inChildParamScope = true
	}
	startInv := len(p.res.Invocations)
	p.runSubParser(blockInStart, blockInEnd, blockLine, parent, before, true, false)
	p.inChildParamScope = prevScope

	for _, sp := range deferred {
		p.runDeferredSpan(sp, parent)
	}

	for i := startInv; i < len(p.res.Invocations); i++ {
		inv := &p.res.Invocations[i]
		if inEdge != 0 {
			inv.PipelineInput = true
			if inv.PipelineInputEdge == 0 {
				inv.PipelineInputEdge = inEdge
			}
		}
		if len(inRedirs) > 0 {
			inv.InputRedirects = append(append([]Redirection(nil), inv.InputRedirects...), inRedirs...)
		}
		if len(outRedirs) > 0 {
			inv.OutputRedirects = append(append([]Redirection(nil), inv.OutputRedirects...), outRedirs...)
		}
	}
	if len(p.res.Invocations) == startInv && len(outRedirs) > 0 {
		idx := p.appendInvocation(Invocation{
			Words: []Word{
				{Text: "out-file", Raw: "out-file", Literal: true},
				{Text: "-FilePath", Raw: "-FilePath", Literal: true},
				outRedirs[0].Target,
			},
			InputRedirects:    inRedirs,
			OutputRedirects:   outRedirs,
			Before:            before,
			Parent:            parent,
			Raw:               stageRaw,
			PipelineInput:     inEdge != 0,
			PipelineInputEdge: inEdge,
		}, psInvocationMeta{
			RawCommand: "out-file",
			Line:       stageLine,
		})
		for _, r := range outRedirs {
			p.checkProviderDriveTarget(r.Target, stageLine, idx)
		}
		return idx
	}
	for _, r := range outRedirs {
		p.checkProviderDriveTarget(r.Target, stageLine, startInv)
	}
	return -1
}

// scanArgumentElement scans one PowerShell CommandElement (including comma-separated
// lists when allowCommaList is true, @(...), @{...}, {...}, (...).Prop, and -Flag:Value).
func (p *psParser) scanArgumentElement(allowCommaList bool) (Word, []psDeferredSpan, bool) {
	elemStart := p.pos
	firstWord, deferred, ok := p.scanSingleArgUnit()
	if !ok {
		return firstWord, deferred, false
	}
	if !allowCommaList {
		return firstWord, deferred, true
	}

	// Comma-separated argument list: e.g. cache,other or '--wait', '--diff'
	allLiteral := firstWord.Literal
	var textParts []string
	hasComma := false
	for {
		savedPos, savedLine := p.pos, p.line
		p.skipTrivia(false)
		if p.pos >= len(p.src) || p.src[p.pos] != ',' {
			p.pos, p.line = savedPos, savedLine
			break
		}
		if !hasComma {
			hasComma = true
			textParts = append(textParts, firstWord.Text)
		}
		commaLine := p.line
		p.pos++ // skip ','
		p.skipTrivia(true)
		if p.pos >= len(p.src) || p.atPipelineStageBoundary() {
			p.noteOpaque(psOpaqueUnsupportedSyntax, commaLine)
			return Word{Raw: p.src[elemStart:p.pos], Literal: false}, deferred, false
		}
		nextWord, nextDef, nextOK := p.scanSingleArgUnit()
		deferred = append(deferred, nextDef...)
		if !nextOK {
			return Word{Raw: p.src[elemStart:p.pos], Literal: false}, deferred, false
		}
		allLiteral = allLiteral && nextWord.Literal
		textParts = append(textParts, nextWord.Text)
	}
	if hasComma {
		return Word{
			Text:    strings.Join(textParts, ","),
			Raw:     p.src[elemStart:p.pos],
			Quoted:  false,
			Literal: allLiteral,
		}, deferred, true
	}
	return firstWord, deferred, true
}

func (p *psParser) scanSingleArgUnit() (Word, []psDeferredSpan, bool) {
	if p.pos >= len(p.src) {
		return Word{}, nil, false
	}
	start := p.pos
	var deferred []psDeferredSpan

	switch p.src[p.pos] {
	case '{':
		inS, inE, sL, ok := p.scanBalancedSpan('}')
		deferred = append(deferred, psDeferredSpan{
			start: inS, end: inE, line: sL, kind: Group, stmtList: true,
		})
		raw := p.src[start:p.pos]
		return Word{Text: raw, Raw: raw, Quoted: false, Literal: false}, deferred, ok

	case '(':
		inS, inE, sL, ok := p.scanBalancedSpan(')')
		deferred = append(deferred, psDeferredSpan{
			start: inS, end: inE, line: sL, kind: Group, stmtList: true,
		})
		if ok {
			fiCtor := isFileInfoCtorRaw(p.src[inS:inE], p.activeNamespaces)
			p.consumeAttachedExprPostfix(&deferred, fiCtor)
		}
		raw := p.src[start:p.pos]
		return Word{Text: raw, Raw: raw, Quoted: false, Literal: false}, deferred, ok

	case '[':
		if w, ok, matched := p.tryScanLeadingBracketArg(&deferred); matched {
			return w, deferred, ok
		}

	case '@':
		if p.pos+1 < len(p.src) {
			switch p.src[p.pos+1] {
			case '\'', '"':
				txt, raw, lit, sub, ok := p.scanHereString(-1, &deferred)
				return Word{Text: txt, Raw: raw, Quoted: true, Literal: lit, Substituted: sub}, deferred, ok
			case '(':
				p.pos++ // at '('
				inS, inE, sL, ok := p.scanBalancedSpan(')')
				deferred = append(deferred, psDeferredSpan{
					start: inS, end: inE, line: sL, kind: Group, stmtList: true,
				})
				allLit := ok && p.isPureLiteralArraySpan(inS, inE, sL)
				if ok && p.consumeAttachedExprPostfix(&deferred, false) {
					allLit = false
				}
				raw := p.src[start:p.pos]
				return Word{Text: raw, Raw: raw, Quoted: false, Literal: allLit}, deferred, ok
			case '{':
				p.pos++ // at '{'
				inS, inE, sL, ok := p.scanBalancedSpan('}')
				deferred = append(deferred, psDeferredSpan{
					start: inS, end: inE, line: sL, kind: Group, hash: true,
				})
				raw := p.src[start:p.pos]
				return Word{Text: raw, Raw: raw, Quoted: false, Literal: false}, deferred, ok
			default:
				// Splatting @varName
				p.pos++
				varStart := p.pos
				for p.pos < len(p.src) {
					r, w := utf8.DecodeRuneInString(p.src[p.pos:])
					if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == ':' {
						p.pos += w
						continue
					}
					break
				}
				vName := p.src[varStart:p.pos]
				p.observeVariableName(vName, p.line)
				raw := p.src[start:p.pos]
				return Word{Text: raw, Raw: raw, Quoted: false, Literal: false}, deferred, true
			}
		}
	}

	// Composite word token (adjacent bare chars, quotes, variables, $(...), brackets, and -Flag:<value>).
	var text strings.Builder
	literal := true
	quoted := false
	substituted := false
	for p.pos < len(p.src) {
		c := p.src[p.pos]
		if c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == ';' || c == '|' || c == '&' ||
			c == '(' || c == ')' || c == '{' || c == '}' || c == '<' || c == '>' || c == ',' {
			break
		}
		switch c {
		case '\'':
			quoted = true
			sTxt, _, ok := p.scanSingleQuotedString()
			text.WriteString(sTxt)
			if !ok {
				return Word{Text: text.String(), Raw: p.src[start:p.pos], Quoted: quoted, Literal: false}, deferred, false
			}
		case '"':
			quoted = true
			dTxt, _, dLit, dSub, ok := p.scanDoubleQuotedString(-1, &deferred)
			text.WriteString(dTxt)
			if !dLit {
				literal = false
			}
			if dSub {
				substituted = true
			}
			if !ok {
				return Word{Text: text.String(), Raw: p.src[start:p.pos], Quoted: quoted, Literal: false, Substituted: substituted}, deferred, false
			}
		case '`':
			// Check line continuation at word end vs escaped char.
			if p.pos+1 < len(p.src) && (p.src[p.pos+1] == '\n' || p.src[p.pos+1] == '\r') {
				raw := p.src[start:p.pos]
				if isUnquotedFlagHead(raw) {
					quoted = false
				}
				return Word{Text: text.String(), Raw: raw, Quoted: quoted, Literal: literal, Substituted: substituted}, deferred, true
			}
			litOK, escOK := p.decodePSBacktick(&text)
			if !litOK {
				literal = false
			}
			if !escOK {
				return Word{Text: text.String(), Raw: p.src[start:p.pos], Quoted: quoted, Literal: false}, deferred, false
			}
		case '$':
			literal = false
			if p.pos+1 < len(p.src) && p.src[p.pos+1] == '(' {
				substituted = true
				subStart := p.pos
				p.pos++ // at '('
				inS, inE, sL, ok := p.scanBalancedSpan(')')
				deferred = append(deferred, psDeferredSpan{
					start: inS, end: inE, line: sL, kind: Substitution, stmtList: true,
				})
				text.WriteString(p.src[subStart:p.pos])
				if !ok {
					return Word{Text: text.String(), Raw: p.src[start:p.pos], Quoted: quoted, Literal: false, Substituted: true}, deferred, false
				}
				continue
			}
			varRawStart := p.pos
			vName := p.scanVariableToken()
			p.observeVariableRead(vName, p.line, -1, &deferred)
			if varRawStart == start && p.pos < len(p.src) &&
				(p.src[p.pos] == '[' || (p.src[p.pos] == '.' && (p.pos+1 >= len(p.src) || p.src[p.pos+1] != '.'))) {
				p.consumeAttachedExprPostfix(&deferred, false)
			}
			text.WriteString(p.src[varRawStart:p.pos])
		case ':':
			text.WriteByte(':')
			p.pos++
			// When '-Flag:' ends an unquoted flag head and is immediately followed
			// by an expression/comma-list value (-ArgumentList:@('a','b'), -Verb:'runas', -Flag:($x)),
			// attach that argument value into the same parameter word.
			curRaw := p.src[start:p.pos]
			if isUnquotedFlagHead(curRaw[:len(curRaw)-1]) && p.pos < len(p.src) &&
				!strings.ContainsRune(" \t\r\n;|&)}", rune(p.src[p.pos])) {
				valW, valDef, valOK := p.scanArgumentElement(true)
				deferred = append(deferred, valDef...)
				text.WriteString(valW.Text)
				if !valW.Literal {
					literal = false
				}
				if valW.Substituted {
					substituted = true
				}
				return Word{
					Text:        text.String(),
					Raw:         p.src[start:p.pos],
					Quoted:      false,
					Literal:     literal,
					Substituted: substituted,
				}, deferred, valOK
			}
		default:
			r, w := utf8.DecodeRuneInString(p.src[p.pos:])
			text.WriteRune(r)
			p.pos += w
		}
	}

	raw := p.src[start:p.pos]
	if isUnquotedFlagHead(raw) {
		quoted = false
	}
	return Word{
		Text:        text.String(),
		Raw:         raw,
		Quoted:      quoted,
		Literal:     literal,
		Substituted: substituted,
	}, deferred, true
}

func (p *psParser) tryScanLeadingBracketArg(deferred *[]psDeferredSpan) (Word, bool, bool) {
	savedPos, savedLine, savedSteps := p.pos, p.line, p.steps
	savedOpaque := p.res.psOpacity
	restore := func() {
		p.pos, p.line, p.steps = savedPos, savedLine, savedSteps
		p.res.psOpacity = savedOpaque
	}

	bracketCount := 0
	for p.pos < len(p.src) && p.src[p.pos] == '[' {
		if _, _, _, ok := p.scanBalancedSpan(']'); !ok {
			restore()
			return Word{}, false, false
		}
		bracketCount++
	}
	if bracketCount == 0 || p.pos >= len(p.src) {
		restore()
		return Word{}, false, false
	}

	// Static .NET member access or call: [Type]::Member or [Type]::Method(...)
	if bracketCount == 1 && p.pos+1 < len(p.src) && p.src[p.pos] == ':' && p.src[p.pos+1] == ':' {
		p.pos += 2
		if !p.skipRawMemberAndOptionalParens() {
			restore()
			return Word{}, false, false
		}
		for p.pos < len(p.src) {
			if p.src[p.pos] == '[' {
				if _, _, _, ok := p.scanBalancedSpan(']'); !ok {
					break
				}
				continue
			}
			if p.src[p.pos] == '.' && (p.pos+1 >= len(p.src) || p.src[p.pos+1] != '.') {
				p.pos++
				if !p.skipRawMemberAndOptionalParens() {
					break
				}
				continue
			}
			break
		}
		exprEnd := p.pos
		if exprEnd == len(p.src) || strings.ContainsRune(" \t\r\n;|&)},<>", rune(p.src[exprEnd])) {
			*deferred = append(*deferred, psDeferredSpan{
				start: savedPos, end: exprEnd, line: savedLine, kind: Group, stmtList: false,
			})
			raw := p.src[savedPos:exprEnd]
			return Word{Text: raw, Raw: raw, Quoted: false, Literal: false}, true, true
		}
		restore()
		return Word{}, false, false
	}

	// Chained or single type cast in command argument position: [Type]$x, [Type](expr), [Type]'str'
	switch p.src[p.pos] {
	case '$', '(', '\'', '"', '@':
		restore()
		for p.pos < len(p.src) && p.src[p.pos] == '[' {
			p.validateTypeCastBracket()
		}
		targetW, targetDef, targetOK := p.scanSingleArgUnit()
		*deferred = append(*deferred, targetDef...)
		raw := p.src[savedPos:p.pos]
		return Word{Text: raw, Raw: raw, Quoted: false, Literal: false, Substituted: targetW.Substituted}, targetOK, true
	}

	restore()
	return Word{}, false, false
}

func (p *psParser) skipRawMemberAndOptionalParens() bool {
	if p.pos >= len(p.src) {
		return false
	}
	switch p.src[p.pos] {
	case '\'':
		if !p.skipSingleQuotedRawOnly() {
			return false
		}
	case '"':
		if !p.skipDoubleQuotedRawOnly() {
			return false
		}
	case '(':
		if _, _, _, ok := p.scanBalancedSpan(')'); !ok {
			return false
		}
	case '$':
		if p.pos+1 < len(p.src) && p.src[p.pos+1] == '(' {
			p.pos++
			if _, _, _, ok := p.scanBalancedSpan(')'); !ok {
				return false
			}
		} else {
			p.scanVariableToken()
		}
	default:
		mStart := p.pos
		for p.pos < len(p.src) && (isASCIILetter(p.src[p.pos]) || (p.src[p.pos] >= '0' && p.src[p.pos] <= '9') || p.src[p.pos] == '_' || p.src[p.pos] == '`') {
			if p.src[p.pos] == '`' && p.pos+1 < len(p.src) {
				p.pos += 2
				continue
			}
			p.pos++
		}
		if p.pos == mStart {
			return false
		}
	}
	if p.pos < len(p.src) && p.src[p.pos] == '(' {
		if _, _, _, ok := p.scanBalancedSpan(')'); !ok {
			return false
		}
	}
	return true
}

func (p *psParser) consumeAttachedExprPostfix(deferred *[]psDeferredSpan, isFileInfoCtor bool) bool {
	consumed := false
	for p.pos < len(p.src) {
		if p.src[p.pos] == '[' {
			consumed = true
			isFileInfoCtor = false
			inS, inE, sL, ok := p.scanBalancedSpan(']')
			*deferred = append(*deferred, psDeferredSpan{
				start: inS, end: inE, line: sL, kind: Group, stmtList: false,
			})
			if !ok {
				return true
			}
			continue
		}
		if p.src[p.pos] == '.' && (p.pos+1 >= len(p.src) || p.src[p.pos+1] != '.') {
			consumed = true
			mLine := p.line
			p.pos++
			if p.pos < len(p.src) && p.src[p.pos] == '\'' {
				mName, _, _ := p.scanSingleQuotedString()
				if p.pos < len(p.src) && p.src[p.pos] == '(' {
					mLower := asciiLower(mName)
					if psDangerousInstanceMethods[mLower] || (isFileInfoCtor && mLower == "replace") {
						p.noteScriptOnlyOpaque(psOpaqueDotNetCall, mLine)
					}
					_, sp := p.scanDotNetCallArgs()
					*deferred = append(*deferred, sp...)
				}
				isFileInfoCtor = false
				continue
			}
			if p.pos < len(p.src) && p.src[p.pos] == '"' {
				mName, _, lit, _, _ := p.scanDoubleQuotedString(-1, deferred)
				if p.pos < len(p.src) && p.src[p.pos] == '(' {
					mLower := asciiLower(mName)
					if !lit || psDangerousInstanceMethods[mLower] || (isFileInfoCtor && mLower == "replace") {
						p.noteScriptOnlyOpaque(psOpaqueDotNetCall, mLine)
					}
					_, sp := p.scanDotNetCallArgs()
					*deferred = append(*deferred, sp...)
				}
				isFileInfoCtor = false
				continue
			}
			mStart := p.pos
			for p.pos < len(p.src) && (isASCIILetter(p.src[p.pos]) || (p.src[p.pos] >= '0' && p.src[p.pos] <= '9') || p.src[p.pos] == '_') {
				p.pos++
			}
			mName := p.src[mStart:p.pos]
			if mName == "" && p.pos < len(p.src) && (p.src[p.pos] == '$' || p.src[p.pos] == '(') {
				if p.src[p.pos] == '$' {
					vn := p.scanVariableToken()
					p.observeVariableRead(vn, mLine, -1, deferred)
				} else {
					inS, inE, sL, _ := p.scanBalancedSpan(')')
					*deferred = append(*deferred, psDeferredSpan{
						start: inS, end: inE, line: sL, kind: Group, stmtList: true,
					})
				}
				if p.pos < len(p.src) && p.src[p.pos] == '(' {
					p.noteScriptOnlyOpaque(psOpaqueDotNetCall, mLine)
					_, sp := p.scanDotNetCallArgs()
					*deferred = append(*deferred, sp...)
				}
				isFileInfoCtor = false
				continue
			}
			if p.pos < len(p.src) && p.src[p.pos] == '(' {
				mLower := asciiLower(mName)
				if psDangerousInstanceMethods[mLower] || (isFileInfoCtor && mLower == "replace") {
					p.noteScriptOnlyOpaque(psOpaqueDotNetCall, mLine)
				}
				_, sp := p.scanDotNetCallArgs()
				*deferred = append(*deferred, sp...)
			}
			isFileInfoCtor = false
			continue
		}
		break
	}
	return consumed
}

func (p *psParser) isPureLiteralArraySpan(start, end, startLine int) bool {
	savedSrc, savedPos, savedLine := p.src, p.pos, p.line
	savedOpaque := p.res.psOpacity
	savedConsumes := p.res.ConsumesParams
	defer func() {
		p.src, p.pos, p.line = savedSrc, savedPos, savedLine
		p.res.psOpacity = savedOpaque
		p.res.ConsumesParams = savedConsumes
	}()
	p.src = savedSrc[:end]
	p.pos = start
	p.line = startLine
	for p.pos < len(p.src) {
		p.skipTrivia(true)
		if p.pos >= len(p.src) {
			return true
		}
		c := p.src[p.pos]
		if c == ',' || c == ';' {
			p.pos++
			continue
		}
		switch c {
		case '\'':
			if _, _, ok := p.scanSingleQuotedString(); !ok {
				return false
			}
		case '"':
			var def []psDeferredSpan
			_, _, lit, _, ok := p.scanDoubleQuotedString(-1, &def)
			if !ok || !lit || len(def) > 0 {
				return false
			}
		default:
			if c >= '0' && c <= '9' {
				p.scanNumberLiteral()
			} else {
				return false
			}
		}
		p.skipTrivia(true)
		if p.pos < len(p.src) && p.src[p.pos] != ',' && p.src[p.pos] != ';' {
			return false
		}
	}
	return true
}

func (p *psParser) parseStageRedirections() (inRedirs, outRedirs []Redirection, deferred []psDeferredSpan) {
	for p.pos < len(p.src) {
		p.skipTrivia(false)
		redir, isIn, isFileOut, rDef, matched := p.tryScanOneRedirection()
		if !matched {
			return inRedirs, outRedirs, deferred
		}
		deferred = append(deferred, rDef...)
		if isIn {
			inRedirs = append(inRedirs, redir)
		} else if isFileOut {
			outRedirs = append(outRedirs, redir)
		}
	}
	return inRedirs, outRedirs, deferred
}

func (p *psParser) tryScanOneRedirection() (redir Redirection, isIn, isFileOut bool, deferred []psDeferredSpan, matched bool) {
	if p.pos >= len(p.src) {
		return Redirection{}, false, false, nil, false
	}
	line := p.line
	i := p.pos
	fd := -1
	if (p.src[i] >= '1' && p.src[i] <= '6') && i+1 < len(p.src) && p.src[i+1] == '>' {
		fd = int(p.src[i] - '0')
		i++
	} else if p.src[i] == '*' && i+1 < len(p.src) && p.src[i+1] == '>' {
		fd = -1
		i++
	}
	if i >= len(p.src) || (p.src[i] != '>' && p.src[i] != '<') {
		return Redirection{}, false, false, nil, false
	}
	if p.src[i] == '<' {
		if i+1 < len(p.src) && p.src[i+1] == '#' {
			return Redirection{}, false, false, nil, false
		}
		p.pos = i + 1
		p.skipTrivia(false)
		if p.pos >= len(p.src) || p.atPipelineStageBoundary() {
			p.noteOpaque(psOpaqueUnsupportedSyntax, line)
			return Redirection{}, false, false, nil, true
		}
		target, tDef, ok := p.scanArgumentElement(false)
		if !ok || target.Raw == "" {
			p.noteOpaque(psOpaqueUnsupportedSyntax, line)
			return Redirection{}, false, false, tDef, true
		}
		return Redirection{FD: -1, Operator: "<", Target: target}, true, false, tDef, true
	}

	// '>' or '>>' or '>&1'
	op := ">"
	i++
	if i < len(p.src) && p.src[i] == '>' {
		op = ">>"
		i++
	} else if i < len(p.src) && p.src[i] == '&' {
		op = ">&"
		i++
		if i < len(p.src) && p.src[i] >= '1' && p.src[i] <= '6' {
			stream := p.src[i : i+1]
			p.pos = i + 1
			return Redirection{FD: fd, Operator: op, Target: Word{Text: stream, Raw: stream, Literal: true}}, false, false, nil, true
		}
		p.pos = i
		p.noteOpaque(psOpaqueUnsupportedSyntax, line)
		return Redirection{}, false, false, nil, true
	}
	p.pos = i
	p.skipTrivia(false)
	if p.pos >= len(p.src) || p.atPipelineStageBoundary() {
		p.noteOpaque(psOpaqueUnsupportedSyntax, line)
		return Redirection{}, false, false, nil, true
	}
	target, tDef, ok := p.scanArgumentElement(false)
	if !ok || target.Raw == "" {
		p.noteOpaque(psOpaqueUnsupportedSyntax, line)
		return Redirection{}, false, false, tDef, true
	}
	// Redirecting to exact unquoted $null discards output without opening a file.
	if !target.Quoted && strings.EqualFold(target.Raw, "$null") {
		return Redirection{FD: fd, Operator: op, Target: target}, false, false, tDef, true
	}
	return Redirection{FD: fd, Operator: op, Target: target}, false, true, tDef, true
}

func (p *psParser) classifyCommandInvocation(words []Word, callKind psCallKind, rawCmd string, modStripped bool, sawSplatting bool, before Separator, pipeIn, hasInRedir bool, line int) (psOpaqueKind, *Word) {
	if len(words) == 0 || !words[0].Literal {
		return psOpaqueCommandExpansion, nil
	}

	// PowerShell '.' dot-source precedence: bare slash-less, drive-less target -> psOpaqueSource.
	if callKind == psCallDot {
		hasDrive := len(rawCmd) >= 2 && isASCIILetter(rawCmd[0]) && rawCmd[1] == ':'
		if !strings.ContainsAny(rawCmd, `/\`) && !hasDrive {
			return psOpaqueSource, nil
		}
	}

	// Direct extension checks on rawCmd.
	trimmedCmd := strings.TrimRight(rawCmd, ". ")
	lowerTrimmed := asciiLower(trimmedCmd)
	if strings.HasSuffix(lowerTrimmed, ".bat") || strings.HasSuffix(lowerTrimmed, ".cmd") {
		return psOpaqueBatchFile, nil
	}
	if strings.HasSuffix(lowerTrimmed, ".zsh") || strings.HasSuffix(lowerTrimmed, ".fish") ||
		strings.HasSuffix(lowerTrimmed, ".csh") || strings.HasSuffix(lowerTrimmed, ".tcsh") {
		return psOpaqueOtherShell, nil
	}

	rawExeLower := asciiLower(ExecutableName(rawCmd))
	canonLower := asciiLower(ExecutableName(words[0].Text))
	isCmdletShape := !hasExeSuffix(rawCmd) && (!strings.ContainsAny(rawCmd, `/\`) || modStripped)

	// Evaluators (both modes).
	if isCmdletShape {
		switch canonLower {
		case "invoke-expression", "invoke-command", "add-type", "new-module":
			return psOpaqueEval, nil
		}
	}

	// Unsupported shells (both modes; dash and ksh remain psOpaqueNone).
	if psOtherShellExes[rawExeLower] {
		return psOpaqueOtherShell, nil
	}

	// Standalone Windows script/DLL launchers (both modes).
	if psLOLBinExes[rawExeLower] {
		return psOpaqueDynamicCall, nil
	}

	// cmd / cmd.exe batch vs non-batch.
	if rawExeLower == "cmd" && !modStripped {
		if cmdTargetsBatchFile(words[1:]) {
			return psOpaqueBatchFile, nil
		}
		if p.mode == psModeScript {
			return psOpaqueDynamicCall, nil
		}
	}

	// Import-Module / ipmo (both modes).
	if isCmdletShape && canonLower == "import-module" {
		if !isSafePSImportModule(words) {
			return psOpaqueDynamicCall, nil
		}
	}

	// Start-Process / saps / start.
	if isCmdletShape && canonLower == "start-process" {
		if p.mode == psModeScript || !isTypedStartProcessAllowed(words) {
			return psOpaqueDynamicCall, nil
		}
	}

	// Alias cmdlets (.ps1 only).
	if isCmdletShape && p.mode == psModeScript {
		switch canonLower {
		case "set-alias", "new-alias", "remove-alias", "import-alias":
			return psOpaqueAlias, nil
		}
	}

	// Splatting @var (.ps1 only).
	if sawSplatting && p.mode == psModeScript {
		return psOpaqueDynamicCall, nil
	}

	// New-Object (.NET allowlist in .ps1; StreamReader synthetic read in both modes).
	if isCmdletShape && canonLower == "new-object" {
		noOpaque, synthRead := p.evalNewObjectInvocation(words)
		if noOpaque != psOpaqueNone && p.mode == psModeScript {
			return noOpaque, synthRead
		}
		return psOpaqueNone, synthRead
	}

	// Shell wrapper checks: stdin-to-shell, dynamic-c, nesting-depth.
	fed := before == Pipe || pipeIn || hasInRedir
	if shellOpaque := p.classifyShellAndWrapperOpaque(words, rawExeLower, modStripped, fed); shellOpaque != psOpaqueNone {
		return shellOpaque, nil
	}

	return psOpaqueNone, nil
}

func cmdTargetsBatchFile(args []Word) bool {
	for _, w := range args {
		for _, tok := range strings.FieldsFunc(w.Text, func(r rune) bool {
			return unicode.IsSpace(r) || r == '"' || r == '\'' || r == ';' || r == '&' || r == '|'
		}) {
			t := asciiLower(strings.TrimRight(tok, ". "))
			if strings.HasSuffix(t, ".bat") || strings.HasSuffix(t, ".cmd") {
				return true
			}
		}
	}
	return false
}

func isSafePSImportModule(words []Word) bool {
	if len(words) < 2 {
		return false
	}
	sawModule := false
	checkModWord := func(w Word, rawPart string, textPart string) bool {
		if !w.Literal || rawPart == "" || textPart == "" || hasUnquotedBrace(rawPart) || strings.HasPrefix(rawPart, "@(") {
			return false
		}
		for _, elem := range strings.Split(textPart, ",") {
			elem = strings.TrimSpace(elem)
			if elem == "" || strings.HasPrefix(elem, ".") || strings.HasPrefix(elem, "-") ||
				strings.ContainsAny(elem, `/\:*?[`) {
				return false
			}
			lower := asciiLower(elem)
			for _, badExt := range []string{".ps1", ".psm1", ".psd1", ".ps1xml", ".cdxml", ".dll", ".exe"} {
				if strings.HasSuffix(lower, badExt) {
					return false
				}
			}
		}
		return true
	}

	for i := 1; i < len(words); i++ {
		w := words[i]
		if isUnquotedFlagHead(w.Raw) {
			flagHead, valText, hasColon := strings.Cut(normalizeLeadingDashes(w.Text), ":")
			_, valRaw, _ := strings.Cut(w.Raw, ":")
			fl := asciiLower(flagHead)
			switch {
			case len(fl) >= 3 && strings.HasPrefix("-name", fl):
				if hasColon {
					if !checkModWord(w, valRaw, valText) {
						return false
					}
				} else {
					if i+1 >= len(words) {
						return false
					}
					i++
					if !checkModWord(words[i], words[i].Raw, words[i].Text) {
						return false
					}
				}
				sawModule = true
			case (len(fl) >= 3 && strings.HasPrefix("-force", fl)) ||
				(len(fl) >= 3 && strings.HasPrefix("-global", fl)) ||
				(len(fl) >= 3 && strings.HasPrefix("-passthru", fl)) ||
				(len(fl) >= 2 && strings.HasPrefix("-disablenamechecking", fl)) ||
				(len(fl) >= 4 && strings.HasPrefix("-noclobber", fl)) ||
				fl == "-verbose" || fl == "-whatif":
				if hasColon && !isAllowedSwitchBoolRaw(valRaw) {
					return false
				}
			case fl == "-scope" || fl == "-version" || fl == "-minimumversion" || fl == "-maximumversion" ||
				fl == "-requiredversion" || fl == "-erroraction" || fl == "-ea":
				if hasColon {
					if !w.Literal || valRaw == "" {
						return false
					}
				} else {
					if i+1 >= len(words) || !words[i+1].Literal {
						return false
					}
					i++
				}
			default:
				return false
			}
			continue
		}
		if !checkModWord(w, w.Raw, w.Text) {
			return false
		}
		sawModule = true
	}
	return sawModule
}

func (p *psParser) evalNewObjectInvocation(words []Word) (psOpaqueKind, *Word) {
	var (
		hasType  bool
		typeWord Word
		hasArgs  bool
		argWord  Word
	)
	posIdx := 0
	for i := 1; i < len(words); i++ {
		w := words[i]
		if isUnquotedFlagHead(w.Raw) {
			flagHead, valText, hasColon := strings.Cut(normalizeLeadingDashes(w.Text), ":")
			_, valRaw, _ := strings.Cut(w.Raw, ":")
			fl := asciiLower(flagHead)
			switch {
			case len(fl) >= 2 && strings.HasPrefix("-comobject", fl),
				len(fl) >= 3 && strings.HasPrefix("-strict", fl):
				return psOpaqueDotNetCall, nil
			case len(fl) >= 2 && strings.HasPrefix("-typename", fl):
				if hasType {
					return psOpaqueDotNetCall, nil
				}
				if hasColon {
					typeWord = Word{Text: valText, Raw: valRaw, Literal: w.Literal}
				} else {
					if i+1 >= len(words) {
						return psOpaqueDotNetCall, nil
					}
					i++
					typeWord = words[i]
				}
				hasType = true
			case len(fl) >= 2 && strings.HasPrefix("-argumentlist", fl) || fl == "-args":
				if hasArgs {
					return psOpaqueDotNetCall, nil
				}
				if hasColon {
					argWord = Word{Text: valText, Raw: valRaw, Literal: w.Literal}
				} else {
					if i+1 >= len(words) {
						return psOpaqueDotNetCall, nil
					}
					i++
					argWord = words[i]
				}
				hasArgs = true
			case len(fl) >= 3 && strings.HasPrefix("-property", fl):
				if !hasColon {
					if i+1 >= len(words) {
						return psOpaqueDotNetCall, nil
					}
					i++
				}
			default:
				return psOpaqueDotNetCall, nil
			}
			continue
		}
		if posIdx == 0 && !hasType {
			typeWord = w
			hasType = true
			posIdx++
		} else if !hasArgs {
			argWord = w
			hasArgs = true
			posIdx++
		} else {
			posIdx++
		}
	}
	if !hasType || !typeWord.Literal || !isPSScalarRaw(typeWord.Raw) || typeWord.Text == "" {
		return psOpaqueDotNetCall, nil
	}
	var ctorArgs []psDotNetArg
	if hasArgs {
		ctorArgs = p.unpackNewObjectArgWord(argWord)
	}
	ev := evalPSDotNetStatic(typeWord.Text, "new", true, false, ctorArgs, p.activeNamespaces)
	return ev.opaque, ev.synthRead
}

func (p *psParser) unpackNewObjectArgWord(w Word) []psDotNetArg {
	raw := strings.TrimSpace(w.Raw)
	if len(raw) >= 2 && raw[0] == '(' && raw[len(raw)-1] == ')' {
		inner := raw[1 : len(raw)-1]
		savedSrc := p.src
		p.src = inner
		args, _ := p.extractCommaExprArgs(0, len(inner), p.line)
		p.src = savedSrc
		return args
	}
	if len(raw) >= 3 && strings.HasPrefix(raw, "@(") && raw[len(raw)-1] == ')' {
		inner := raw[2 : len(raw)-1]
		savedSrc := p.src
		p.src = inner
		args, _ := p.extractCommaExprArgs(0, len(inner), p.line)
		p.src = savedSrc
		return args
	}
	if !w.Quoted && strings.Contains(raw, ",") {
		savedSrc := p.src
		p.src = raw
		args, _ := p.extractCommaExprArgs(0, len(raw), p.line)
		p.src = savedSrc
		return args
	}
	return []psDotNetArg{{word: w, raw: w.Raw}}
}

func (p *psParser) classifyShellAndWrapperOpaque(words []Word, rawExeLower string, modStripped bool, fed bool) psOpaqueKind {
	if modStripped {
		return psOpaqueNone
	}
	// Check wrapper chain depth (>= maxWrapperDepth = 4 -> psOpaqueNestingDepth in both modes).
	if p.peekWrapperChainExceedsDepth(words, 0) {
		return psOpaqueNestingDepth
	}

	switch rawExeLower {
	case "pwsh", "powershell":
		return p.classifyPSHostCLI(words[1:], fed)
	case "sh", "bash", "dash", "ksh":
		return p.classifyPOSIXShellCLI(words[1:], fed)
	}
	return psOpaqueNone
}

func (p *psParser) classifyPSHostCLI(args []Word, fed bool) psOpaqueKind {
	sawEntry := false
	for i := 0; i < len(args); i++ {
		w := args[i]
		lower := asciiLower(w.Text)
		switch lower {
		case "-file", "-f":
			sawEntry = true
			if i+1 < len(args) && args[i+1].Text == "-" {
				return psOpaqueStdinToShell
			}
			return psOpaqueNone
		case "-command", "-c":
			sawEntry = true
			if i+1 < len(args) {
				if args[i+1].Text == "-" {
					return psOpaqueStdinToShell
				}
				if !args[i+1].Literal && p.mode == psModeScript {
					return psOpaqueDynamicC
				}
			}
			return psOpaqueNone
		case "-executionpolicy", "-ep", "-windowstyle", "-w", "-workingdirectory", "-wd",
			"-configurationname", "-configurationfile", "-settingsfile", "-custompipename",
			"-inputformat", "-if", "-outputformat", "-of":
			i++
		}
	}
	if !sawEntry && fed {
		return psOpaqueStdinToShell
	}
	return psOpaqueNone
}

func (p *psParser) classifyPOSIXShellCLI(args []Word, fed bool) psOpaqueKind {
	sawScriptOrCmd := false
	for i := 0; i < len(args); i++ {
		w := args[i]
		if w.Text == "--" {
			if i+1 < len(args) {
				sawScriptOrCmd = true
			}
			break
		}
		if w.Text == "-c" || w.Text == "-lc" {
			sawScriptOrCmd = true
			if i+1 < len(args) {
				if args[i+1].Text == "-" && p.mode == psModeScript {
					return psOpaqueStdinToShell
				}
				if !args[i+1].Literal && p.mode == psModeScript {
					return psOpaqueDynamicC
				}
			}
			return psOpaqueNone
		}
		if strings.HasPrefix(w.Text, "-") && w.Text != "-" {
			continue
		}
		if w.Text == "-" && p.mode == psModeScript {
			return psOpaqueStdinToShell
		}
		sawScriptOrCmd = true
		break
	}
	if !sawScriptOrCmd && fed && p.mode == psModeScript {
		return psOpaqueStdinToShell
	}
	return psOpaqueNone
}

func (p *psParser) peekWrapperChainExceedsDepth(words []Word, curDepth int) bool {
	for len(words) > 0 && words[0].Literal {
		exe := asciiLower(ExecutableName(words[0].Text))
		isDashOrKsh := exe == "dash" || exe == "ksh"
		if !isWrapper(exe) && !isDashOrKsh {
			return false
		}
		if curDepth >= maxWrapperDepth {
			return true
		}
		lookupExe := exe
		if isDashOrKsh {
			lookupExe = "sh"
		}
		reading := readWrapper(p.ctx, lookupExe, words, PowerShell, 0)
		if reading.refusal.Refused() || reading.inspectionOnly {
			return false
		}
		curDepth++
		if reading.commandString != nil {
			if !reading.commandString.Literal {
				return false
			}
			if reading.dialect == PowerShell {
				sub := parsePowerShell(p.ctx, reading.commandString.Text, p.mode, nil, nil)
				for _, subInv := range sub.Invocations {
					if p.peekWrapperChainExceedsDepth(subInv.Words, curDepth) {
						return true
					}
				}
			}
			return false
		}
		words = reading.child
	}
	return false
}

// checkCommandProviderDrives tracks provider writes and removals independently
// of filesystem write-target checking.
func (p *psParser) checkCommandProviderDrives(words []Word, rawCmd string, modStripped bool, line int, invIdx int) {
	if len(words) < 2 || hasExeSuffix(rawCmd) || (strings.ContainsAny(rawCmd, `/\`) && !modStripped) {
		return
	}
	canon := asciiLower(ExecutableName(words[0].Text))
	if canon == "cli" {
		canon = "clear-item"
	}
	if _, tracked := psProviderOptions[canon]; !tracked {
		return
	}
	named, positional, ok := bindPSProviderOptions(canon, words[1:])
	if !ok {
		p.noteScriptInvocationOpaque(psOpaqueDynamicCall, line, invIdx)
		return
	}

	takePositional := func(w Word) Word {
		if w.Raw == "" && len(positional) > 0 {
			w = positional[0]
			positional = positional[1:]
		}
		return w
	}
	noteTarget := func(target, value Word, hasValue bool) {
		kind := p.recordDriveTargetMutation(target, value, hasValue, line)
		p.noteScriptInvocationOpaque(kind, line, invIdx)
	}
	resolved := true
	inspectTargets := func(w Word) []Word {
		targets, ok := p.providerTargets(w)
		resolved = resolved && ok
		return targets
	}
	path := named["literalpath"]
	if path.Raw == "" {
		path = named["path"]
	}
	if canon == "out-file" || canon == "tee-object" {
		if path.Raw == "" {
			path = named["filepath"]
		}
		if canon == "tee-object" && named["variable"].Raw != "" {
			return
		}
	}
	targets := inspectTargets(takePositional(path))
	switch canon {
	case "copy-item", "move-item":
		if canon == "move-item" {
			for _, source := range targets {
				noteTarget(source, Word{}, false)
			}
		}
		for _, dest := range inspectTargets(takePositional(named["destination"])) {
			noteTarget(dest, Word{}, false)
		}
	case "rename-item":
		newName := takePositional(named["newname"])
		for _, source := range targets {
			noteTarget(source, Word{}, false)
			if newName.Raw == "" {
				continue
			}
			dest := newName
			if prefix, ok := psDrivePrefix(source.Text); ok && !strings.Contains(dest.Text, ":") {
				dest.Text = prefix + dest.Text
			}
			noteTarget(dest, Word{}, false)
		}
	default:
		value := named["value"]
		if canon == "set-item" || canon == "set-content" || canon == "add-content" {
			value = takePositional(value)
		}
		name := named["name"]
		for _, target := range targets {
			if name.Raw != "" && strings.HasSuffix(target.Text, ":") {
				target.Text += name.Text
				target.Raw += name.Raw
				target.Literal = target.Literal && name.Literal
			}
			noteTarget(target, value, value.Raw != "")
		}
	}
	if !resolved {
		p.noteScriptInvocationOpaque(psOpaqueDynamicCall, line, invIdx)
	}
}

func psDrivePrefix(s string) (string, bool) {
	lower := asciiLower(s)
	for _, d := range []string{"env:", "function:", "alias:"} {
		if strings.HasPrefix(lower, d) {
			return s[:len(d)], true
		}
	}
	return "", false
}

func (p *psParser) checkProviderDriveTarget(target Word, line int, invIdx int) {
	// Redirection is performed by the shell regardless of which command the
	// module-qualified name ultimately resolves to.
	kind := p.recordDriveTargetMutation(target, Word{}, false, line)
	p.noteScriptOnlyOpaque(kind, line)
	p.noteScriptInvocationOpaque(kind, line, invIdx)
}

func (p *psParser) noteScriptInvocationOpaque(kind psOpaqueKind, line, invIdx int) {
	if p.mode != psModeScript || kind == psOpaqueNone {
		return
	}
	if invIdx < 0 || invIdx >= len(p.res.Meta) {
		p.noteOpaque(kind, line)
		return
	}
	p.noteCommandOpaque(kind, line)
	if p.res.Meta[invIdx].Opaque == psOpaqueNone {
		p.res.Meta[invIdx].Opaque = kind
	}
}

func (p *psParser) recordDriveTargetMutation(target Word, valWord Word, hasVal bool, line int) psOpaqueKind {
	lower := asciiLower(target.Text)
	switch {
	case strings.HasPrefix(lower, "function:"):
		return psOpaqueFunctionShadow
	case strings.HasPrefix(lower, "alias:"):
		return psOpaqueAlias
	case strings.HasPrefix(lower, "env:"):
		envName := strings.TrimPrefix(target.Text[4:], `\`)
		litName := target.Literal && isPSScalarRaw(target.Raw) && envName != "" && !strings.ContainsAny(envName, ",*?[]$`")
		pinned := !litName || isPSPinnedEnv(envName)
		val := ""
		litVal := false
		if hasVal {
			if valWord.Literal && isPSScalarRaw(valWord.Raw) {
				val = valWord.Text
				litVal = true
			} else if !valWord.Quoted && strings.EqualFold(valWord.Raw, "$null") {
				val = ""
				litVal = true
			} else {
				val = valWord.Raw
			}
		}
		p.recordEnvMutation(psEnvMutation{
			Name:         envName,
			LiteralName:  litName,
			Value:        val,
			LiteralValue: litVal,
			Line:         line,
			Pinned:       pinned,
		})
		if pinned {
			return psOpaquePinnedAssignment
		}
	}
	return psOpaqueNone
}

// The options below are the named parameters of the inspected cmdlets. Resolve
// abbreviations against the command's whole set so a value-taking option
// cannot consume a path intended for a switch or a different parameter.
var psProviderCommonOptions = strings.Fields("verbose debug erroraction warningaction informationaction progressaction errorvariable warningvariable informationvariable outvariable outbuffer pipelinevariable whatif confirm")

var psProviderOptions = map[string]string{
	"set-item":      "path literalpath value force passthru filter include exclude credential",
	"remove-item":   "path literalpath filter include exclude recurse force credential stream",
	"clear-item":    "path literalpath force filter include exclude credential",
	"new-item":      "path name itemtype value force credential",
	"set-content":   "path literalpath value passthru filter include exclude force credential nonewline encoding asbytestream stream",
	"add-content":   "path literalpath value passthru filter include exclude force credential nonewline encoding asbytestream stream",
	"clear-content": "path literalpath filter include exclude force credential stream",
	"out-file":      "filepath literalpath encoding append force noclobber width nonewline inputobject",
	"tee-object":    "inputobject filepath literalpath append encoding variable",
	"copy-item":     "path literalpath destination container force filter include exclude recurse passthru credential fromsession tosession",
	"move-item":     "path literalpath destination force filter include exclude passthru credential",
	"rename-item":   "path literalpath newname force passthru credential",
}

var psProviderOptionAliases = map[string]string{
	"pspath": "literalpath", "lp": "literalpath",
	"vb": "verbose", "db": "debug", "ea": "erroraction", "wa": "warningaction",
	"infa": "informationaction", "proga": "progressaction", "ev": "errorvariable",
	"wv": "warningvariable", "iv": "informationvariable", "ov": "outvariable",
	"ob": "outbuffer", "pv": "pipelinevariable", "wi": "whatif", "cf": "confirm",
}

var powershellSwitches = map[string]bool{
	"force": true, "nonewline": true, "whatif": true, "confirm": true,
	"passthru": true, "append": true, "noclobber": true, "recurse": true, "container": true,
	"verbose": true, "debug": true, "asbytestream": true,
}

func bindPSProviderOptions(canon string, args []Word) (named map[string]Word, positional []Word, ok bool) {
	return bindPSOptions(canon, args, psProviderOptions[canon], nil)
}

// bindPSOptions keeps named values, switches and positional words separate. Each caller
// supplies the vocabulary for the command it is inspecting.
func bindPSOptions(canon string, args []Word, parameters string, extraSwitches map[string]bool) (named map[string]Word, positional []Word, ok bool) {
	options := append(strings.Fields(parameters), psProviderCommonOptions...)
	named = make(map[string]Word)
	for i := 0; i < len(args); i++ {
		w := args[i]
		if !isUnquotedFlagHead(w.Raw) {
			positional = append(positional, w)
			continue
		}
		head, value, inline := strings.Cut(normalizeLeadingDashes(w.Text), ":")
		flag := strings.TrimPrefix(asciiLower(head), "-")
		switch {
		case canon == "new-item" && flag == "type":
			flag = "itemtype"
		case canon == "new-item" && flag == "target":
			flag = "value"
		case canon == "out-file" && flag == "nooverwrite":
			flag = "noclobber"
		case (canon == "out-file" || canon == "tee-object") && flag == "path":
			flag = "filepath"
		default:
			if alias := psProviderOptionAliases[flag]; alias != "" {
				flag = alias
			}
		}
		name := ""
		for _, option := range options {
			if option == flag {
				name = option
				break
			}
			if strings.HasPrefix(option, flag) {
				if name != "" {
					name = "-"
				} else {
					name = option
				}
			}
		}
		if name == "" || name == "-" {
			return nil, nil, false
		}
		if powershellSwitches[name] || extraSwitches[name] {
			continue
		}
		if inline {
			_, raw, _ := strings.Cut(w.Raw, ":")
			if raw == "" {
				return nil, nil, false
			}
			w = Word{Text: value, Raw: raw, Literal: w.Literal, Substituted: w.Substituted}
		} else {
			i++
			if i >= len(args) || isUnquotedFlagHead(args[i].Raw) {
				return nil, nil, false
			}
			w = args[i]
		}
		if _, duplicate := named[name]; duplicate {
			return nil, nil, false
		}
		named[name] = w
	}
	return named, positional, true
}

// PowerShellReadPathWords returns the candidate paths within a cmdlet argument
// and reports whether the argument selects literal-path semantics. A colon
// value is read as part of its named option. Quoted commas stay in one path.
func PowerShellReadPathWords(ctx context.Context, word Word) ([]Word, bool, Refusal) {
	literalPath := false
	if isUnquotedFlagHead(word.Raw) {
		head, value, joined := strings.Cut(normalizeLeadingDashes(word.Text), ":")
		flag := asciiLower(head)
		switch {
		case flag == "-lp", flag == "-pspath", len(flag) >= len("-lit") && strings.HasPrefix("-literalpath", flag):
			literalPath = true
		case strings.HasPrefix("-path", flag), strings.HasPrefix("-literalpath", flag):
			// A short ambiguous name does not establish literal-path semantics.
		default:
			return nil, false, Refusal{}
		}
		if !joined {
			return nil, literalPath, Refusal{}
		}
		_, raw, _ := strings.Cut(word.Raw, ":")
		word = Word{Text: value, Raw: raw, Literal: word.Literal, Substituted: word.Substituted}
	}
	// A scalar is already an inspected word. It does not require a second
	// parse or a new proof, even if an unrelated precision check canceled.
	if isPSScalarRaw(word.Raw) {
		return []Word{word}, literalPath, Refusal{}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil {
		return nil, literalPath, cancelRefusal(PowerShell)
	}
	p := psParser{ctx: ctx, mode: psModeTyped, line: 1}
	parts, resolved := p.providerTargets(word)
	if ctx.Err() != nil {
		return nil, false, cancelRefusal(PowerShell)
	}
	if !resolved {
		return nil, literalPath, opaqueRefusal(constructUnsupportedSyntax, nil)
	}
	return parts, literalPath, Refusal{}
}

// Reuse command argument scanning so each element keeps its own spelling and
// provenance. The result indicates whether every list element was resolved.
func (p *psParser) providerTargets(w Word) ([]Word, bool) {
	if w.Raw == "" {
		return nil, true
	}
	if isPSScalarRaw(w.Raw) {
		return []Word{w}, true
	}
	raw := w.Raw
	arrayStatement := strings.HasPrefix(raw, "@(") && strings.HasSuffix(raw, ")")
	if arrayStatement {
		raw = raw[2 : len(raw)-1]
	} else if !strings.Contains(raw, ",") {
		return []Word{w}, true
	}
	lexer := psParser{ctx: p.ctx, src: raw, mode: p.mode, line: p.line}
	var targets []Word
	resolved := true
	for {
		lexer.skipTrivia(true)
		if lexer.res.Canceled {
			return targets, false
		}
		if lexer.pos >= len(raw) {
			return targets, resolved
		}
		if arrayStatement && raw[lexer.pos] == ';' {
			lexer.pos++
			continue
		}
		if arrayStatement && !lexer.isExpressionStageStart() {
			return targets, false
		}
		word, _, ok := lexer.scanSingleArgUnit()
		if !ok || word.Raw == "" {
			return targets, false
		}
		if arrayStatement && !isPSScalarRaw(word.Raw) {
			resolved = false
		} else {
			targets = append(targets, word)
			resolved = resolved && word.Literal
		}
		lexer.skipTrivia(false)
		if lexer.res.Canceled {
			return targets, false
		}
		if lexer.pos == len(raw) {
			return targets, resolved
		}
		sep := raw[lexer.pos]
		if sep != ',' && !(arrayStatement && (sep == ';' || sep == '\r' || sep == '\n')) {
			return targets, false
		}
		lexer.pos++
	}
}
