package shellwords

import "strings"

// RedactedMatch is what a guard shows in place of text that came from an inspected script.
func RedactedMatch(label string) string { return "script content redacted (" + label + ")" }

// Redactor decides what a finding may show when the call ran inspected scripts. It fails closed: a match is shown as
// written only when the typed call itself explains it, so the script's own text can reach a deny reason, a
// diagnostics stream or a log only through a path that cannot place it. Placing a match only picks which label
// replaces it; it never decides whether to replace it. A caller that already holds a label reports it directly and
// does not pass it through Redact, so no input spelling can opt out of redaction.
type Redactor struct {
	parsed Result
	typed  []string // typed texts with all whitespace removed, so spacing around operators cannot hide a match
}

// NewRedactor returns nil when no script was inspected, in which case nothing is redacted. typedTexts are the forms of
// the typed call the rules read.
func NewRedactor(parsed Result, typedTexts ...string) *Redactor {
	if len(parsed.Scripts) == 0 {
		return nil
	}
	r := &Redactor{parsed: parsed}
	for _, text := range typedTexts {
		r.typed = append(r.typed, stripSpace(text), stripSpace(jsonUnescape.Replace(text)))
	}
	return r
}

func stripSpace(s string) string {
	return strings.Join(strings.Fields(s), "")
}

// Redact returns the match to show, the label to add to the suggestion, and whether the match was replaced.
func (r *Redactor) Redact(match string) (shown, label string, redacted bool) {
	if r == nil || r.typedExplains(match) {
		return match, "", false
	}
	if label, ok := r.parsed.ScriptLabelFor(match); ok {
		return RedactedMatch(label), label, true
	}
	label = "inspected script"
	if len(r.parsed.Scripts) == 1 {
		label = r.parsed.Scripts[0].SafePath()
	}
	return RedactedMatch(label), label, true
}

// typedExplains reports whether the match is text of the typed call: part of it, ignoring whitespace, or composed
// from the words of one of its own invocations the way the rules compose "rm" and a target.
func (r *Redactor) typedExplains(match string) bool {
	for _, form := range []string{match, jsonUnescape.Replace(match)} {
		stripped := stripSpace(form)
		if stripped == "" {
			return true
		}
		for _, typed := range r.typed {
			if strings.Contains(typed, stripped) {
				return true
			}
		}
		for i := range r.parsed.Invocations {
			if inv := &r.parsed.Invocations[i]; inv.Script == nil && ComposedFromWords(form, inv.Words) {
				return true
			}
		}
	}
	return false
}

const (
	hintOpaque   = "Rewrite the script line so the command it runs is literal; eval, trap, a dynamic source and an expansion in command position cannot be inspected."
	hintFile     = "The script must be a plain UTF-8 file that stays unchanged while it is inspected; check the path and the file."
	hintLimit    = "The script tree exceeds an inspection limit; split it or run the inner commands as their own tool calls."
	hintForm     = "Call the script as bash or sh with a literal .sh path from a proven working directory and no options other than --, --noprofile, --norc, -e or -u."
	hintWriter   = "Run the command that writes files or shell state as its own tool call before the script call, and drop the & or loop that runs beside it."
	hintCallerIn = "Do not pass a protected target, a check-bypass flag or an unquoted pattern as an argument to a script that uses its parameters; quote a pattern, and run the command itself when it needs a protected target."
)

// scriptHints maps a script refusal reason prefix to its remedy. A hint never repeats the reason, which the caller
// appends, and never carries script text.
var scriptHints = map[string]string{
	"script-opaque:":                 hintOpaque,
	"script-construct":               hintOpaque,
	"script-unreadable":              hintFile,
	"script-encoding":                hintFile,
	"script-changing":                hintFile,
	"script-budget":                  hintLimit,
	"script-depth":                   hintLimit,
	"script-cycle":                   hintLimit,
	"script-too-large":               hintLimit,
	"script-too-many-files":          hintLimit,
	"script-form-unsupported-option": hintForm,
	"script-context-unqualified":     hintForm,
	"script-extension":               hintForm,
	"script-path-not-literal":        hintForm,
	"script-cwd-unproven":            hintForm,
	"script-path-protected":          "Scripts under a credential or device directory are not inspected; move the script.",
	"script-operand-not-literal":     "Give the protected command inside the script a literal operand, or run that command as its own tool call.",
	"script-caller-argument":         hintCallerIn,
	"script-stdin-fed":               "Do not feed standard input to a script that reads it; pass the data as a file argument.",
	"script-env-pinned":              "Do not set PATH, IFS or another name the shell trusts on the script call; set it inside the script.",
	"script-predecessor-may-write":   hintWriter,
	"script-predecessor-state":       hintWriter,
	"script-writes-inspected":        hintWriter,
}

// ScriptHint returns the remedy for a script refusal reason, "" for any other reason. The longest matching prefix wins.
func ScriptHint(reason string) string {
	best, hint := 0, ""
	for prefix, text := range scriptHints {
		if len(prefix) > best && strings.HasPrefix(reason, prefix) {
			best, hint = len(prefix), text
		}
	}
	return hint
}
