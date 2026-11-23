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
