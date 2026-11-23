package shellwords

import (
	"fmt"
	"strings"
)

// Rule is a named script refusal rule. The set is closed: every spelling a rule renders is declared here, and the
// zero value RuleNone names no rule.
type Rule int

// The named script rules. Each renders as its script- name and carries a remedy hint.
const (
	RuleNone Rule = iota
	RuleOpaque
	RuleUnreadable
	RuleEncoding
	RuleChanging
	RuleBudget
	RuleDepth
	RuleCycle
	RuleTooLarge
	RuleTooManyFiles
	RuleFormUnsupportedOption
	RuleContextUnqualified
	RuleExtension
	RulePathNotLiteral
	RuleCwdUnproven
	RulePathProtected
	RuleOperandNotLiteral
	RuleCallerArgument
	RuleStdinFed
	RuleEnvPinned
	RulePredecessorMayWrite
	RulePredecessorState
	RuleWritesInspected
	ruleEnd

	ruleFirst = RuleOpaque
	ruleLast  = ruleEnd - 1
)

// The remedies the rules carry. A hint never repeats the reason, which the caller appends, and never carries script
// text.
const (
	hintOpaque   = "Rewrite the script line so the command it runs is literal; eval, trap, a dynamic source and an expansion in command position cannot be inspected."
	hintFile     = "The script must be a plain UTF-8 file that stays unchanged while it is inspected; check the path and the file."
	hintLimit    = "The script tree exceeds an inspection limit; split it or run the inner commands as their own tool calls."
	hintForm     = "Call the script as bash or sh with a literal .sh path from a proven working directory and no options other than --, --noprofile, --norc, -e or -u."
	hintWriter   = "Run the command that writes files or shell state as its own tool call before the script call, and drop the & or loop that runs beside it."
	hintCallerIn = "Do not pass a protected target, a check-bypass flag or an unquoted pattern as an argument to a script that uses its parameters; quote a pattern, and run the command itself when it needs a protected target."
)

var ruleSpellings = [ruleEnd]struct{ name, hint string }{
	RuleOpaque:                {"script-opaque", hintOpaque},
	RuleUnreadable:            {"script-unreadable", hintFile},
	RuleEncoding:              {"script-encoding", hintFile},
	RuleChanging:              {"script-changing", hintFile},
	RuleBudget:                {"script-budget", hintLimit},
	RuleDepth:                 {"script-depth", hintLimit},
	RuleCycle:                 {"script-cycle", hintLimit},
	RuleTooLarge:              {"script-too-large", hintLimit},
	RuleTooManyFiles:          {"script-too-many-files", hintLimit},
	RuleFormUnsupportedOption: {"script-form-unsupported-option", hintForm},
	RuleContextUnqualified:    {"script-context-unqualified", hintForm},
	RuleExtension:             {"script-extension", hintForm},
	RulePathNotLiteral:        {"script-path-not-literal", hintForm},
	RuleCwdUnproven:           {"script-cwd-unproven", hintForm},
	RulePathProtected:         {"script-path-protected", "Scripts under a credential or device directory are not inspected; move the script."},
	RuleOperandNotLiteral:     {"script-operand-not-literal", "Give the protected command inside the script a literal operand, or run that command as its own tool call."},
	RuleCallerArgument:        {"script-caller-argument", hintCallerIn},
	RuleStdinFed:              {"script-stdin-fed", "Do not feed standard input to a script that reads it; pass the data as a file argument."},
	RuleEnvPinned:             {"script-env-pinned", "Do not set PATH, IFS or another name the shell trusts on the script call; set it inside the script."},
	RulePredecessorMayWrite:   {"script-predecessor-may-write", hintWriter},
	RulePredecessorState:      {"script-predecessor-state", hintWriter},
	RuleWritesInspected:       {"script-writes-inspected", hintWriter},
}

func (r Rule) valid() bool { return r >= ruleFirst && r <= ruleLast }

// String returns the rule's script- name, "" for RuleNone or a value outside the set. RuleOpaque renders without the
// construct, which a Refusal appends.
func (r Rule) String() string {
	if !r.valid() {
		return ""
	}
	return ruleSpellings[r].name
}

// Hint returns the rule's remedy, "" for RuleNone or a value outside the set. A hint never repeats the reason and never
// carries script text.
func (r Rule) Hint() string {
	if !r.valid() {
		return ""
	}
	return ruleSpellings[r].hint
}

// MarshalText encodes the rule as its name. A value outside the set is an error rather than an empty name.
func (r Rule) MarshalText() ([]byte, error) {
	if r != RuleNone && !r.valid() {
		return nil, fmt.Errorf("shellwords: unknown rule %d", int(r))
	}
	return []byte(r.String()), nil
}

// construct names a script construct that hides the commands it runs. It renders after script-opaque:.
type construct int

const (
	constructNone construct = iota
	constructEval
	constructTrap
	constructSource
	constructOtherShell
	constructCommandPositionExpansion
	constructDynamicC
	constructStdinToShell
	constructNestingDepth
	constructUnsupportedSyntax
	constructPinnedAssignment
	constructPersistentIFS
	constructIFSPrefix
	constructNameref
	constructAlias
	constructCoproc
	constructPrintfV
	constructFunctionShadow
	constructCommandNotFoundHandle
	constructPromptCommand
	constructArithmeticSubstitution
	constructHeredocDelimiter
	constructDotNetCall
	constructBatchFile
	constructDynamicCall
	constructEnd

	constructFirst = constructEval
	constructLast  = constructEnd - 1
)

var constructNames = [constructEnd]string{
	constructDotNetCall:               "dotnet-call",
	constructBatchFile:                "batch-file",
	constructDynamicCall:              "dynamic-call",
	constructEval:                     "eval",
	constructTrap:                     "trap",
	constructSource:                   "source",
	constructOtherShell:               "other-shell",
	constructCommandPositionExpansion: "command-position-expansion",
	constructDynamicC:                 "dynamic-c",
	constructStdinToShell:             "stdin-to-shell",
	constructNestingDepth:             "nesting-depth",
	constructUnsupportedSyntax:        "unsupported-syntax",
	constructPinnedAssignment:         "pinned-assignment",
	constructPersistentIFS:            "persistent-ifs",
	constructIFSPrefix:                "ifs-prefix",
	constructNameref:                  "nameref",
	constructAlias:                    "alias",
	constructCoproc:                   "coproc",
	constructPrintfV:                  "printf-v",
	constructFunctionShadow:           "function-shadow",
	constructCommandNotFoundHandle:    "command-not-found-handle",
	constructPromptCommand:            "prompt-command",
	constructArithmeticSubstitution:   "arithmetic-substitution",
	constructHeredocDelimiter:         "heredoc-delimiter",
}

func (c construct) valid() bool { return c >= constructFirst && c <= constructLast }

// String returns the construct name the README documents, "" for constructNone or a value outside the set.
func (c construct) String() string {
	if !c.valid() {
		return ""
	}
	return constructNames[c]
}

// MarshalText encodes the construct as its name. A value outside the set is an error.
func (c construct) MarshalText() ([]byte, error) {
	if c != constructNone && !c.valid() {
		return nil, fmt.Errorf("shellwords: unknown construct %d", int(c))
	}
	return []byte(c.String()), nil
}

// formKind is a refusal of a typed command form. Each template quotes only words the parser has classified: a
// wrapper or shell executable, an option from a wrapper's fixed set, a redirection operator, or, for the three
// option kinds, the whole dash word the wrapper does not accept. That dash word can hold an attached value or the
// source of a substitution, which is why these kinds are not content-free.
type formKind int

const (
	formNone formKind = iota
	formShellContract
	formPowerShellContract
	formEnvName
	formWrapperOption
	formWrapperValue
	formNoChild
	formXargsChild
	formTimeoutDuration
	formTimeoutOption
	formFlockOption
	formChildNotLiteral
	formCommandString
	formWrapperDepth
	formUnterminated
	formRedirection
	formANSIControl
	formANSIByte
	formANSIUnterminated
	formEnd

	formFirst = formShellContract
	formLast  = formEnd - 1
)

// formTemplates are the texts the form kinds render. formWrapperDepth fills its %d with maxWrapperDepth itself.
var formTemplates = [formEnd]string{
	formShellContract:      "%s requires exactly -c or -lc and a literal command string",
	formPowerShellContract: "%s requires optional PowerShell flags, then -File <literal .ps1> or -Command and one literal token",
	formEnvName:            "env %s requires a literal name",
	formWrapperOption:      "%s option %s is outside the literal wrapper contract",
	formWrapperValue:       "%s %s requires a literal value",
	formNoChild:            "%s has no child executable",
	formXargsChild:         "xargs has no literal child executable",
	formTimeoutDuration:    "timeout requires a literal duration before the command",
	formTimeoutOption:      "timeout option %s after the duration is outside the literal wrapper contract",
	formFlockOption:        "flock option %s after the lock file is outside the literal wrapper contract",
	formChildNotLiteral:    "%s child executable is not literal",
	formCommandString:      "%s command string is not literal",
	formWrapperDepth:       "wrapper depth exceeds %d at %s",
	formUnterminated:       "unterminated quoted word",
	formRedirection:        "redirection %s has no target",
	formANSIControl:        "ANSI-C control escape is outside the literal parser contract",
	formANSIByte:           "ANSI-C non-ASCII or NUL escape is outside the literal parser contract",
	formANSIUnterminated:   "unterminated ANSI-C quoted word",
}

func (k formKind) valid() bool { return k >= formFirst && k <= formLast }

// String returns the kind's template, "" for formNone or a value outside the set.
func (k formKind) String() string {
	if !k.valid() {
		return ""
	}
	return formTemplates[k]
}

// MarshalText encodes the kind as its template. A value outside the set is an error.
func (k formKind) MarshalText() ([]byte, error) {
	if k != formNone && !k.valid() {
		return nil, fmt.Errorf("shellwords: unknown form kind %d", int(k))
	}
	return []byte(k.String()), nil
}

// formArity is the number of words the kind quotes.
func formArity(k formKind) int { return strings.Count(formTemplates[k], "%s") }

// cancelKind is an inspection cut short by its context, one per dialect.
type cancelKind int

const (
	cancelNone cancelKind = iota
	cancelShell
	cancelPowerShell
	cancelEnd

	cancelFirst = cancelShell
	cancelLast  = cancelEnd - 1
)

var cancelTexts = [cancelEnd]string{
	cancelShell:      "shell inspection canceled",
	cancelPowerShell: "PowerShell environment inspection canceled",
}

// GuardReason is a refusal a guard raises itself rather than reads from the parser. The zero value GuardNone is no
// reason.
type GuardReason int

// The guard reasons, each rendering its literal text.
const (
	GuardNone GuardReason = iota
	GuardBashAuthorityUnavailable
	GuardBashSyntaxOperandUnavailable
	GuardSearchCanceled
	GuardBashCanceled
	GuardSearchBraceExpansion
	guardEnd

	guardFirst = GuardBashAuthorityUnavailable
	guardLast  = guardEnd - 1
)

var guardTexts = [guardEnd]string{
	GuardBashAuthorityUnavailable:     "Bash inspection authority unavailable",
	GuardBashSyntaxOperandUnavailable: "Bash syntax file read operand unavailable",
	GuardSearchCanceled:               "search argument inspection canceled",
	GuardBashCanceled:                 "Bash inspection canceled",
	GuardSearchBraceExpansion:         "unsupported search brace expansion",
}

func (g GuardReason) valid() bool { return g >= guardFirst && g <= guardLast }

// String returns the reason's literal text, "" for GuardNone or a value outside the set.
func (g GuardReason) String() string {
	if !g.valid() {
		return ""
	}
	return guardTexts[g]
}

// MarshalText encodes the reason as its text. A value outside the set is an error.
func (g GuardReason) MarshalText() ([]byte, error) {
	if g != GuardNone && !g.valid() {
		return nil, fmt.Errorf("shellwords: unknown guard reason %d", int(g))
	}
	return []byte(g.String()), nil
}

// Refusal is the reason the parser, or a guard reading its result, will not vouch for an invocation. It is built only
// from the closed kinds in this file and holds no free text, so what it renders is fixed by its kind: a named rule
// with an optional construct and script location, a typed-form template over classified words, a cancellation, or a
// guard reason. The zero value refuses nothing and renders "".
type Refusal struct {
	rule      Rule
	construct construct
	origin    *ScriptOrigin
	form      formKind
	args      [2]string
	cancel    cancelKind
	guard     GuardReason
}

// CallerArgumentRefusal refuses a script call for the arguments its caller passes, located at the script's path.
func CallerArgumentRefusal(call ScriptCall) Refusal {
	return ruleRefusal(RuleCallerArgument, &ScriptOrigin{Path: call.Path})
}

// GuardRefusal returns the refusal for a guard reason. GuardNone gives the zero Refusal, because an empty
// reason means no refusal. A value outside the set panics, since only a programming error can produce one.
func GuardRefusal(g GuardReason) Refusal {
	if g == GuardNone {
		return Refusal{}
	}
	if !g.valid() {
		panic(fmt.Sprintf("shellwords: GuardRefusal of unknown GuardReason(%d)", int(g)))
	}
	return Refusal{guard: g}
}

// ruleRefusal refuses under a named rule. A nil origin renders the bare name; RuleNone gives the zero Refusal.
func ruleRefusal(r Rule, origin *ScriptOrigin) Refusal {
	if r == RuleNone {
		return Refusal{}
	}
	return Refusal{rule: r, origin: copyOrigin(origin)}
}

// opaqueRefusal refuses a script construct that hides the commands it runs, as script-opaque:<construct>.
func opaqueRefusal(c construct, origin *ScriptOrigin) Refusal {
	return Refusal{rule: RuleOpaque, construct: c, origin: copyOrigin(origin)}
}

// formRefusal refuses a typed command form, quoting args in the kind's template. The depth form takes only the
// executable and fills in maxWrapperDepth itself. A wrong kind or argument count panics: every call site is fixed in
// the source, so only a programming error reaches it.
func formRefusal(k formKind, args ...string) Refusal {
	if !k.valid() || len(args) != formArity(k) {
		panic(fmt.Sprintf("shellwords: formRefusal(%d) with %d arguments", int(k), len(args)))
	}
	r := Refusal{form: k}
	copy(r.args[:], args)
	return r
}

// cancelRefusal records an inspection cut short by its context, in the dialect's own wording.
func cancelRefusal(d Dialect) Refusal {
	if d == PowerShell {
		return Refusal{cancel: cancelPowerShell}
	}
	return Refusal{cancel: cancelShell}
}

// locatedAt returns a copy of a named-rule refusal located at o. Any other kind does not render a location, so it is
// returned unchanged and Origin stays nil.
func (r Refusal) locatedAt(o *ScriptOrigin) Refusal {
	if r.isNamedRule() {
		r.origin = copyOrigin(o)
	}
	return r
}

// isNamedRule reports a refusal under a script- rule, the kinds that carry a remedy.
func (r Refusal) isNamedRule() bool { return r.rule != RuleNone }

func copyOrigin(o *ScriptOrigin) *ScriptOrigin {
	if o == nil {
		return nil
	}
	c := *o
	c.Chain = append([]string(nil), o.Chain...)
	return &c
}

// Refused reports whether there is a refusal at all.
func (r Refusal) Refused() bool {
	return r.rule != RuleNone || r.form != formNone || r.cancel != cancelNone || r.guard != GuardNone
}

// Rule returns the named rule, RuleNone for any other kind.
func (r Refusal) Rule() Rule { return r.rule }

// Construct returns the construct name of a script-opaque refusal, "" otherwise.
func (r Refusal) Construct() string {
	if r.rule != RuleOpaque {
		return ""
	}
	return r.construct.String()
}

// IsOpaqueEval reports whether the refusal is specifically script-opaque:eval.
func (r Refusal) IsOpaqueEval() bool {
	return r.rule == RuleOpaque && r.construct == constructEval
}

// Origin returns a copy of the script location the refusal renders, nil when none was appended.
func (r Refusal) Origin() *ScriptOrigin { return copyOrigin(r.origin) }

// Canceled reports a refusal caused by a canceled inspection, from the parser or from secrets-guardian.
func (r Refusal) Canceled() bool {
	return r.cancel != cancelNone || r.guard == GuardSearchCanceled || r.guard == GuardBashCanceled
}

// ContentFree reports whether the text renders nothing from the command beyond script and chain paths. Only the
// typed-form kinds quote words of the command, so only they report false.
func (r Refusal) ContentFree() bool { return r.form == formNone }

// Hint returns the remedy of a named rule, "" for any other kind.
func (r Refusal) Hint() string { return r.rule.Hint() }

// String renders the refusal as the parser and guards have always spelled it.
func (r Refusal) String() string {
	switch {
	case r.rule != RuleNone:
		s := r.rule.String()
		if r.rule == RuleOpaque {
			s += ":" + r.construct.String()
		}
		if r.origin != nil {
			s += " in " + r.origin.label()
		}
		return s
	case r.form == formWrapperDepth:
		return fmt.Sprintf(formTemplates[r.form], maxWrapperDepth, r.args[0])
	case r.form != formNone:
		args := make([]any, formArity(r.form))
		for i := range args {
			args[i] = r.args[i]
		}
		return fmt.Sprintf(formTemplates[r.form], args...)
	case r.cancel != cancelNone:
		return cancelTexts[r.cancel]
	}
	return r.guard.String()
}

// Remedy returns the hint followed by the refusal in parentheses, the form a guard shows, and true for a named rule.
// Any other kind has no remedy and returns "" and false.
func (r Refusal) Remedy() (string, bool) {
	if !r.isNamedRule() {
		return "", false
	}
	return r.Hint() + " (" + r.String() + ")", true
}

// MarshalText encodes the refusal as its rendered text, so a Refusal field encodes as the string it replaces.
func (r Refusal) MarshalText() ([]byte, error) { return []byte(r.String()), nil }
