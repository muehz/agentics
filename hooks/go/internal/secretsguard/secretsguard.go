// Package secretsguard is the secrets-guardian core: it denies a tool call
// that would read a credential file into the transcript, or write a secret
// value to disk.
//
// It matches secret values in content on the write side and credential file
// paths on the read side, and neither pattern set is usable as the other.
//
// Why paths and not content on the read side: once a credential is read it is
// in the transcript, and the transcript goes to the model provider and to
// disk. There is no after-the-fact remedy, which is why this hook fails
// closed — an inspected event it cannot read is a deny, never a diagnostic
// that a fail-open provider treats as permission to continue.
//
// Environment:
//
//	SKIP_SECRETS_GUARD      - exactly "true" disables the hook entirely
//	SECRETS_GUARD_MODE      - warn or block (any other value is block; a
//	                          reported finding names the invalid value)
//	SECRETS_GUARD_ALLOWLIST - comma-separated path substrings that skip scanning
package secretsguard

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/muehz/agentics/hooks/go/internal/argv"
	"github.com/muehz/agentics/hooks/go/internal/asciifold"
	"github.com/muehz/agentics/hooks/go/internal/contract"
	"github.com/muehz/agentics/hooks/go/internal/guardmode"
	"github.com/muehz/agentics/hooks/go/internal/hookspec"
	"github.com/muehz/agentics/hooks/go/internal/secretpatterns"
	"github.com/muehz/agentics/hooks/go/internal/shellwords"
	"github.com/muehz/agentics/hooks/go/internal/toolcall"
	"github.com/muehz/agentics/hooks/go/internal/toolname"
)

// The two fixed error messages a request can end with.
var (
	errValidate = errors.New("secrets-guardian could not validate normalized input")
	errRead     = errors.New("secrets-guardian could not read normalized input")
)

const writeParseErrorMessage = "secrets-guardian could not inspect the apply_patch write because its patch body is malformed"

// writeText is one text about to land on disk. old is the text the same input
// object says it replaces, or "" when the tool writes without one.
type writeText struct{ path, text, old string }

type writeHit struct {
	name, severity, redacted, path string
	advisory                       bool
}

type writeScan struct {
	message  string
	found    bool
	blocking bool
}

// inspectedTools are the tools whose input can put file content in front of
// the model.
//
// This roster is deliberately not command-guardian's. That hook skips
// Read/Grep/Glob precisely because their payloads carry prose and search
// patterns that look dangerous; this hook exists to inspect exactly those
// tools, and it looks at a path rather than at command text, so the false
// positives that forced the other skip do not arise here.
var inspectedTools = map[string]bool{
	"Read": true, "Grep": true, "Glob": true, "NotebookRead": true, "Bash": true, "PowerShell": true,
}

// MCP reads match the final namespace component. GitHub reads preserve their
// existing permissive missing-path behavior; the pinned filesystem shapes
// are strict because their schemas require the named typed fields.
var mcpReadTools = map[string]bool{
	"get_file_contents":   true,
	"read_file":           true,
	"read_text_file":      true,
	"read_media_file":     true,
	"read_multiple_files": true,
}

type mcpReadShape struct {
	field    string
	multiple bool
}

var filesystemMCPReads = map[string]mcpReadShape{
	"read_file":           {field: "path"},
	"read_text_file":      {field: "path"},
	"read_media_file":     {field: "path"},
	"read_multiple_files": {field: "paths", multiple: true},
}

const mcpReadErrorMessage = "secrets-guardian could not inspect a recognized filesystem MCP read because its required path field is missing or has the wrong type"

var mcpWriteTools = map[string]bool{
	"push_files": true, "create_or_update_file": true,
}

// readerCommands are shell commands that put a file's content somewhere it can
// be observed. A credential path is only flagged inside a Bash command when
// one of these consumes it.
//
// Requiring a verb is what keeps prose out of the results: `echo "add .env to
// .gitignore"` names a credential path but reads nothing, and denying it would
// train the user to switch the guard off.
var readerCommands = map[string]bool{
	"cat": true, "bat": true, "less": true, "more": true, "head": true,
	"tail": true, "strings": true, "xxd": true, "od": true, "hexdump": true,
	"base64": true, "cp": true, "mv": true, "scp": true, "rsync": true,
	"sed": true, "awk": true, "grep": true, "rg": true, "ag": true,
	"jq": true, "yq": true, "source": true, ".": true, "dd": true,
	// PowerShell and cmd equivalents.
	"get-content": true, "gc": true, "type": true, "select-string": true,
	"sls": true, "copy-item": true, "cpi": true, "copy": true,
	"move-item": true, "mi": true, "move": true, "format-hex": true,
}

// gitExecutable is the one executable name outside readerCommands that the path reader dispatches on, to read the
// revision paths of git show. It must be Classified in argv like every dispatch name.
const gitExecutable = "git"

// directions reports which directions of the secrets check apply to a tool.
// Local tools match by their exact name; MCP tools match by their last
// namespace component. The two rosters are kept apart so normalizing a name can
// never pull a local tool into a gate it was not in.
func directions(name string) (read, write bool) {
	base := toolname.Base(name)
	return inspectedTools[name] || mcpReadTools[base], toolcall.IsFileWrite(name) || mcpWriteTools[base]
}

// Inspects is the one answer to whether the secrets check inspects a tool,
// in either direction. Local names match exactly and MCP names by their last
// namespace component, as Run applies them.
func Inspects(name string) bool {
	read, write := directions(name)
	return read || write
}

// Run is the secrets-guardian handler.
func Run(req hookspec.Request) hookspec.Outcome {
	diag := req.Diag
	if diag == nil {
		diag = io.Discard
	}

	control := guardmode.Control{
		Guard:   "secrets-guardian",
		SkipVar: "SKIP_SECRETS_GUARD",
		ModeVar: "SECRETS_GUARD_MODE",
	}.Resolve(os.Getenv)
	if control.Skipped {
		return contract.Pass()
	}

	// An empty event carries nothing to inspect.
	if !req.Event.Present() {
		return contract.Pass()
	}

	// Providers discard the core-only diagnostic stream for pass and deny
	// outcomes, so the warning also leads every reported reason: reportMessage
	// carries it in the outcome itself.
	mode := control.Mode
	modeDiagnostic := control.Warning
	if modeDiagnostic != "" {
		fmt.Fprintln(diag, modeDiagnostic)
	}

	event, err := req.Event.Read()
	if err != nil {
		return contract.Error(errValidate.Error())
	}

	toolName := ""
	if event.Tool.Name != nil {
		toolName = *event.Tool.Name
	}
	base := toolname.Base(toolName)
	readDirection, writeDirection := directions(toolName)
	if !readDirection && !writeDirection {
		return contract.Pass()
	}

	// The decode happens only for a tool this hook inspects, and a failure is
	// a deny rather than a provider fault. tool_input is agent-controlled, so
	// a value the decoder cannot represent — an integer beyond float64's
	// range is enough — would otherwise be a way to walk the gate: the guard
	// would see no path and no content, and allow the very read it exists to
	// stop. Copilot and Codex deliberately fail open on provider faults, so an
	// error outcome would still permit the call on those surfaces.
	call, err := toolcall.Read(event)
	if err != nil {
		return contract.Deny(reportMessage(modeDiagnostic, errRead.Error()))
	}
	allowlist := parsePathAllowlist(os.Getenv("SECRETS_GUARD_ALLOWLIST"))

	// The write direction: content about to land on disk, checked for secret
	// values. Anything that reaches the working tree by another route — a
	// script's output, a build artifact — is outside what a PreToolUse hook can
	// observe.
	if writeDirection {
		writes := call.Writes()
		if writes.ParseError {
			return hookspec.Outcome(contract.Deny(reportMessage(modeDiagnostic, writeParseErrorMessage)))
		}
		firstPath := ""
		if paths := call.PathArguments(); len(paths) > 0 {
			firstPath = paths[0]
		}
		result := scanWrite(writes, firstPath, allowlist)
		if !result.found {
			return contract.Pass()
		}
		if result.blocking {
			return report(diag, mode, modeDiagnostic, result.message)
		}
		fmt.Fprintln(diag, result.message)
		return hookspec.Outcome(contract.Context(reportMessage(modeDiagnostic, result.message)))
	}

	var candidates []string
	if toolname.IsShell(toolName) {
		command, _ := call.Command()
		var parsed toolcall.Inspection
		if command != "" {
			parsed = call.Inspect(req.ExecutionContext(), req.Scripts)
		}
		if parsed.DataCanceled {
			mode = guardmode.Block
		}
		inspection := inspectShellReads(req.ExecutionContext(), call, parsed)
		if inspection.Reason.Refused() {
			return hookspec.Outcome(contract.Deny(reportMessage(modeDiagnostic, shellParseMessage(inspection.Reason))))
		}
		for _, invocation := range inspection.Invocations {
			if reason, denied := commandSecretReason(invocation); denied {
				return report(diag, mode, modeDiagnostic, reason)
			}
		}
		if reason, denied := callerArgumentReason(inspection.Calls, allowlist); denied {
			return report(diag, mode, modeDiagnostic, reason)
		}
		findings, globErr := collectShell(inspection.Paths, allowlist, inspection.Overrides)
		if globErr != nil {
			return contract.Deny(reportMessage(modeDiagnostic, "secrets-guardian could not inspect glob: "+globErr.Error()))
		}
		if len(findings) > 0 {
			return report(diag, mode, modeDiagnostic, renderMessage(findings))
		}
		if parsed.DataCanceled {
			return contract.Deny(reportMessage(modeDiagnostic, "secrets-guardian could not inspect a recognized execution path (PowerShell inspection canceled)"))
		}
		// Content a shell command writes (a heredoc, printf, sed replacement)
		// is in the command text itself, and a secret on a command line is
		// exposed in transcripts and logs even when nothing is written. The
		// path allowlist does not apply here: without shellwords there is no
		// reliable redirect target to compare against it.
		if result := scanTexts([]writeText{{text: command}}, "the shell command"); result.found {
			if result.blocking {
				return report(diag, mode, modeDiagnostic, result.message)
			}
			return hookspec.Outcome(contract.Context(reportMessage(modeDiagnostic, result.message)))
		}
		return contract.Pass()
	} else if shape, strict := filesystemMCPReads[base]; strict {
		var valid bool
		candidates, valid = typedMCPReadPaths(call.Layers(), shape)
		if !valid {
			return hookspec.Outcome(contract.Deny(reportMessage(modeDiagnostic, mcpReadErrorMessage)))
		}
	} else {
		candidates = call.PathArguments()
	}

	findings := collect(candidates, allowlist)
	if len(findings) == 0 {
		return contract.Pass()
	}
	return report(diag, mode, modeDiagnostic, renderMessage(findings))
}

// report writes the finding to the diagnostic stream and returns the outcome
// the mode calls for: a deny in block mode, a context outcome in warn mode. A
// mode diagnostic is also carried in the outcome so provider rendering cannot
// discard the warning before it reaches the user.
func report(diag io.Writer, mode guardmode.Mode, modeDiagnostic, message string) hookspec.Outcome {
	fmt.Fprintf(diag, "%s\n", message)
	message = reportMessage(modeDiagnostic, message)
	if mode.Blocks() {
		return contract.Deny(message)
	}
	return contract.Context(message)
}

func reportMessage(modeDiagnostic, message string) string {
	if modeDiagnostic == "" {
		return message
	}
	return modeDiagnostic + "\n\n" + message
}

// scanWrite is the write-tool entry point: it scans the texts a call adds,
// except those whose path the allowlist covers, against the shared secret
// table. The result is reported against the call's first path, or "the target
// file" when firstPath is empty. Allowlisted content is never converted into a
// writeText.
//
// A file-writing call carries content about to land on disk, so the check is
// against secret values rather than credential paths. The path is deliberately
// not classified: writing a `.env` is ordinary setup work, and it is reading
// one back into the transcript that cannot be undone, so a write to a
// credential path is allowed and only its content is examined.
func scanWrite(writes toolcall.Writes, firstPath string, allowlist []string) writeScan {
	var texts []writeText
	for _, text := range writes.Texts {
		if allowed(text.Path, allowlist) {
			continue
		}
		texts = append(texts, writeText{text.Path, text.Text, text.Old})
	}
	target := firstPath
	if target == "" {
		target = "the target file"
	}
	return scanTexts(texts, target)
}

// scanTexts runs the shared secret table against every text about to land
// somewhere — a file write's content, or a whole shell command line — and
// returns the message and whether any match blocks. target names the
// destination in the message: a file path, "the target file" when the call
// carries none, or "the shell command" for the shell-text scan.
//
// Placeholders are skipped through the shared placeholder filter, so a
// README showing an example key does not block a documentation edit.
// Redaction keeps the matched value out of the message: a deny reason is
// replayed to the model, and printing the secret to explain why it must not be
// printed would defeat the hook.
//
// Each text is folded once and every pattern reuses the fold through
// FindAllFolded; the shell scan runs on every Bash call, so per-pattern
// folding would dominate its cost.
func scanTexts(texts []writeText, target string) writeScan {
	if len(texts) == 0 {
		return writeScan{}
	}

	var hits []writeHit
	seen := map[string]bool{}
	blocking, valueHit, markerHit := false, false, false
	for _, t := range texts {
		folded := asciifold.String(t.text)
		var foldedOld string
		if t.old != "" {
			foldedOld = asciifold.String(t.old)
		}
		for _, pattern := range secretpatterns.All() {
			carried := carriedSecrets(pattern, t.old, foldedOld)
			for _, match := range secretpatterns.FindAllFolded(pattern, t.text, folded) {
				if carried[match.Secret] > 0 {
					carried[match.Secret]--
					continue
				}
				key := pattern.Name + "\x00" + match.Full + "\x00" + t.path
				if seen[key] {
					continue
				}
				seen[key] = true
				hits = append(hits, writeHit{
					name: pattern.Name, severity: pattern.Severity,
					redacted: maskAfterPrefix(match.Secret), path: t.path,
					advisory: pattern.Advisory,
				})
				if !pattern.Advisory {
					blocking = true
					if pattern.MarkerOnly {
						markerHit = true
					} else {
						valueHit = true
					}
				}
			}
		}
	}
	if len(hits) == 0 {
		return writeScan{}
	}

	var b strings.Builder
	if blocking {
		fmt.Fprintf(&b, "%d potential secret(s) found before writing to %s:", len(hits), target)
	} else {
		fmt.Fprintf(&b, "%d informational network endpoint(s) found before writing to %s:", len(hits), target)
	}
	for _, hit := range hits {
		label := hit.severity
		if hit.advisory {
			label += ", advisory"
		}
		fmt.Fprintf(&b, "\n- %s (%s) %s", hit.name, label, hit.redacted)
		if hit.path != "" && hit.path != target {
			fmt.Fprintf(&b, " in %s", hit.path)
		}
	}
	if blocking {
		b.WriteString("\nUse a placeholder and load the real value from the environment at run time.")
		// A shell command has no old text to repeat and no file to write
		// instead — the edit-carry and marker-carry advice below is
		// meaningless for a command line, so the shell target gets one
		// steering line instead of either sentence.
		if target == "the shell command" {
			b.WriteString("\nKeep the value out of the command line: write it through a file tool with a placeholder, or read it from the environment inside the command.")
		} else {
			// The carry advice holds only for rules that capture the secret.
			// A marker-only match is flagged whatever the old text carries,
			// so telling the agent to narrow the edit would send it in
			// circles.
			if valueHit {
				b.WriteString("\nA value the edit only repeats from the text it replaces is not flagged; if this one is already in the file, narrow the edit to the lines you change. Do not write it through a shell command instead.")
			}
			if markerHit {
				b.WriteString("\nA private-key header or service-account marker is flagged even when the text it replaces carries it, because the marker says nothing about the key after it; keep key material out of the file. Do not write it through a shell command instead.")
			}
		}
	} else {
		b.WriteString("\nConfirm the internal endpoint is appropriate for committed or generated content.")
	}
	return writeScan{message: b.String(), found: true, blocking: blocking}
}

// carriedSecrets counts the secret values pattern finds in the text an edit
// replaces, so scanTexts can skip that many equal values in the new text. The
// capture, not the full match, is counted: a renamed key around an unchanged
// value adds nothing to disk. A marker-only rule carries nothing: an equal
// PEM header or service-account field says nothing about the key after it, so
// counting it would let an edit swap in new key material.
//
// foldedOld is asciifold.String(old), folded once by the caller per writeText
// rather than once per pattern here.
func carriedSecrets(pattern secretpatterns.Pattern, old, foldedOld string) map[string]int {
	if old == "" || pattern.MarkerOnly {
		return nil
	}
	counts := map[string]int{}
	for _, match := range secretpatterns.FindAllFolded(pattern, old, foldedOld) {
		counts[match.Secret]++
	}
	return counts
}

// typedMCPReadPaths reads a filesystem MCP read's paths from the call's
// argument layers: the first layer that has the shape's field decides, and a
// missing field, a wrong type, or an empty list is not readable.
func typedMCPReadPaths(layers []map[string]any, shape mcpReadShape) ([]string, bool) {
	var value any
	found := false
	for _, object := range layers {
		if candidate, ok := object[shape.field]; ok {
			value, found = candidate, true
			break
		}
	}
	if !found {
		return nil, false
	}
	if !shape.multiple {
		pathValue, ok := value.(string)
		if !ok {
			return nil, false
		}
		return []string{pathValue}, true
	}
	items, ok := value.([]any)
	if !ok || len(items) == 0 {
		return nil, false
	}
	paths := make([]string, len(items))
	for i, item := range items {
		pathValue, ok := item.(string)
		if !ok {
			return nil, false
		}
		paths[i] = pathValue
	}
	return paths, true
}

// maskAfterPrefix keeps a short prefix so a reader can tell which credential
// was found, without reproducing enough of it to be usable.
func maskAfterPrefix(match string) string {
	const keep = 4
	runes := []rune(match)
	if len(runes) <= keep {
		return strings.Repeat("*", len(runes))
	}
	return string(runes[:keep]) + strings.Repeat("*", min(len(runes)-keep, 12))
}

// collect classifies every candidate, drops allowlisted and duplicate paths,
// and returns the findings sorted: critical before high, then by path, so the
// message reads the same way for the same call.
func collect(candidates []string, allowlist []string) []finding {
	seen := map[string]bool{}
	var out []finding
	for _, candidate := range candidates {
		if candidate == "" || seen[candidate] {
			continue
		}
		seen[candidate] = true
		if allowed(candidate, allowlist) {
			continue
		}
		if f, ok := classify(candidate); ok {
			out = append(out, f)
		}
	}
	slices.SortFunc(out, func(a, b finding) int {
		if a.severity != b.severity {
			if a.severity == sevCritical {
				return -1
			}
			return 1
		}
		return strings.Compare(a.path, b.path)
	})
	return out
}

// renderMessage is the deny reason. It names every match and one action the
// caller can take instead, because a deny with no alternative just gets
// retried a different way.
func renderMessage(findings []finding) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d credential file read(s) blocked — this content would enter the transcript:", len(findings))
	for _, f := range findings {
		shown := f.path
		if f.origin != nil {
			shown = shellwords.RedactedMatch(f.origin.Location())
		}
		fmt.Fprintf(&b, "\n- %s (%s, %s)", shown, f.category, f.severity)
	}
	fmt.Fprintf(&b, "\n%s", suggestionFor(findings[0].category))
	return b.String()
}

type shellPath struct {
	Text          string
	Pattern       string
	ExpandPattern bool
	Cwd           string
	Dialect       shellwords.Dialect
	GitObject     bool
	GlobReason    string
	// Origin is the script line the operand was read from, nil for a typed operand.
	Origin *shellwords.ScriptOrigin
}

// globError names the operand whose glob could not be resolved. A script operand shows only its line, and the
// resolver's error is dropped with it, because that error can name a path the glob reached.
func (p shellPath) globError(err error) error {
	if p.Origin != nil {
		return fmt.Errorf("%s (glob could not be resolved)", shellwords.RedactedMatch(p.Origin.Location()))
	}
	return fmt.Errorf("%q (%w)", p.Text, err)
}

type shellInspection struct {
	Invocations []shellwords.Invocation
	Paths       []shellPath
	Overrides   []string
	Reason      shellwords.Refusal
	// Scripts are the script files read as data while parsing, kept so the
	// guard can check a script's own path and log what it inspected.
	Scripts []shellwords.ScriptRecord
	// Calls are the expanded script calls with their literal arguments.
	Calls []shellwords.ScriptCall
}

// callerArgumentReason denies a call whose literal arguments reach a script that uses its parameters and name a
// credential path outside the allowlist, or are an unquoted pattern that expands to names the call does not show.
// Matching is on whole words. The reason never repeats an argument.
func callerArgumentReason(calls []shellwords.ScriptCall, allowlist []string) (string, bool) {
	for _, call := range calls {
		if !call.ConsumesParams {
			continue
		}
		denied := call.PatternArgs
		for _, word := range call.CallerWords() {
			if !allowed(word, allowlist) {
				if _, hit := classify(word); hit {
					denied = true
				}
			}
		}
		if denied {
			// A named rule always has a remedy, so the second result is not read.
			remedy, _ := shellwords.CallerArgumentRefusal(call).Remedy()
			return "secrets-guardian: " + remedy, true
		}
	}
	return "", false
}

// readUncertainty records what the call's overrides or the host environment
// leave unknown for a shell read: the ripgrep configuration, CDPATH, and the
// glob options.
type readUncertainty struct{ searchConfig, cdpath, globOptions bool }

// newReadUncertainty reads the call's override containers once and derives
// all three flags from that one reading.
func newReadUncertainty(call toolcall.Call) readUncertainty {
	overrides := call.Environment()
	return readUncertainty{
		searchConfig: searchConfigUncertain(overrides),
		cdpath:       cdpathUncertain(overrides),
		globOptions:  globOptionsUncertain(overrides),
	}
}

// inspectShellReads gathers the reads of one shell call from its inspection,
// which the caller obtains from the same call. With a script reader behind the
// inspection, a bash or sh script call contributes the reads of the commands
// the script runs, and a script limit is an uninspectable invocation that
// denies. An empty command reads nothing.
func inspectShellReads(ctx context.Context, call toolcall.Call, inspection toolcall.Inspection) shellInspection {
	command, _ := call.Command()
	if command == "" {
		return shellInspection{}
	}
	dialect := call.Execution().Dialect
	uncertainty := newReadUncertainty(call)
	configUnknown := uncertainty.searchConfig
	globUnknown := uncertainty.globOptions
	parsed, policyParsed := inspection.Source, inspection.Policy
	result := shellInspection{Invocations: policyParsed.Invocations, Scripts: parsed.Scripts, Calls: parsed.ScriptCalls}
	searchProofAttempted := false
	inspectionProofAttempted := false
	context := shellGlobContext{cwd: call.ProvenCwd(), cdpath: uncertainty.cdpath}
	for _, assignment := range policyParsed.BareAssignments {
		context.observeAssignment(assignment)
		configUnknown = configUnknown || searchConfigAssignment(assignment)
	}
	// Secrets-guardian's posture for a refused invocation: the first one ends
	// the inspection with its reason, and Run denies it with shellParseMessage,
	// so a command line the guard cannot read is never treated as harmless.
	for index, invocation := range policyParsed.Invocations {
		context.advance(invocation)
		if refusal, refused := inspection.Refusal(ctx, index); refused {
			result.Reason = refusal
			return result
		}
		if invocation.Inspection != shellwords.NoInspection {
			inspectionProofAttempted = true
			if invocation.Inspection == shellwords.BashSyntaxRead {
				if invocation.SyntaxOperand == nil {
					result.Reason = shellwords.GuardRefusal(shellwords.GuardBashSyntaxOperandUnavailable)
					return result
				}
				// Classification proves the operand has no active expansion.
				// Quoted metacharacters remain a literal filename, not a glob.
				candidateCwd, candidateDialect := context.candidateContext(invocation, dialect)
				result.Paths = append(result.Paths, shellPath{Text: invocation.SyntaxOperand.Text, Cwd: candidateCwd, Dialect: candidateDialect, Origin: invocation.Script})
			}
			continue
		}
		for _, assignment := range invocation.Assignments {
			context.observeAssignment(assignment)
			configUnknown = configUnknown || searchConfigAssignment(assignment)
			name, value, ok := strings.Cut(assignment.Text, "=")
			if ok && assignment.Literal && slices.Contains(hostCredentialOverrideNames, name) && value != "" {
				result.Overrides = append(result.Overrides, value)
			}
		}
		if len(invocation.Words) == 0 || !invocation.Words[0].Literal {
			configUnknown = true
			continue
		}
		verb := readExecutableName(invocation.Words[0])
		// A preceding shell builtin or opaque command may write/export the
		// config variable indirectly. Do not try to interpret shell state.
		if !readerCommands[verb] || verb == "source" || verb == "." || verb == "eval" {
			configUnknown = true
		}
		_, invocationDialect := context.candidateContext(invocation, dialect)
		cwdVerb := verb
		if invocationDialect == shellwords.PowerShell && verb == "set-location" {
			switch strings.ToLower(invocation.Words[0].Raw) {
			case "cd", "chdir", "set-location":
				cwdVerb = "cd"
			}
		}
		context.observeCommand(invocation, cwdVerb, index+1 < len(policyParsed.Invocations) && policyParsed.Invocations[index+1].Before == shellwords.And, invocationDialect)
		if readerCommands[verb] {
			candidateCwd, candidateDialect := context.candidateContext(invocation, dialect)
			makePath := func(word shellwords.Word, expand bool) shellPath {
				reason := context.globReason
				// Preserve the resolver's location diagnostic when a relative
				// operand already has no proven directory.
				if globUnknown && candidateDialect == shellwords.POSIX && (candidateCwd != "" || filepath.IsAbs(nativeShellPath(word.Text))) {
					reason = "inherited or tool glob options are unsupported"
				}
				pattern := ""
				if candidateDialect == shellwords.POSIX {
					var err error
					pattern, err = posixGlobPattern(word.Raw)
					if err != nil {
						reason = err.Error()
					}
				}
				if !word.Literal && expand {
					reason = "dynamic wildcard operand"
				}
				if expand && (strings.HasPrefix(word.Raw, "~") || candidateDialect == shellwords.PowerShell && strings.HasPrefix(word.Text, "~")) {
					reason = "home expansion in wildcard operands is unsupported"
				}
				return shellPath{Text: word.Text, Pattern: pattern, ExpandPattern: expand, Cwd: candidateCwd, Dialect: candidateDialect, GlobReason: reason, Origin: invocation.Script}
			}
			literalPath := hasLiteralPathFlag(invocation.Words[1:])
			var pathWords [][]shellwords.Word
			if candidateDialect == shellwords.PowerShell && powerShellPathListReader(verb) {
				pathWords = make([][]shellwords.Word, len(invocation.Words)-1)
				for i, arg := range invocation.Words[1:] {
					parts, literal, refusal := shellwords.PowerShellReadPathWords(ctx, arg)
					if refusal.Refused() {
						result.Reason = refusal
						return result
					}
					pathWords[i] = parts
					literalPath = literalPath || literal
				}
			}
			for _, redirect := range invocation.InputRedirects {
				expand := candidateDialect == shellwords.POSIX && posixExpandsPattern(redirect.Target.Raw)
				if (redirect.Target.Literal || expand) && redirect.Target.Text != "" && redirect.Target.Text != "-" {
					result.Paths = append(result.Paths, makePath(redirect.Target, expand))
				}
			}
			search := searchReadArguments(ctx, verb, invocation.Words[1:], candidateDialect, configUnknown)
			if candidateDialect == shellwords.PowerShell && (verb == "rg" || verb == "grep") {
				searchProofAttempted = true
			}
			if candidateDialect == shellwords.PowerShell {
				// Only the closed bare external-tool spelling may borrow its
				// argument roles; basename normalization also accepts modules.
				first := invocation.Words[0]
				if !inspection.Authority(index).AllowsNewProof() || first.Raw != verb || first.Text != verb {
					search.patterns = nil
				}
			}
			if len(invocation.Wrappers) != 0 || invocation.Parent >= 0 {
				search.patterns = nil
			}
			if searchProofAttempted && ctx.Err() != nil {
				search.reason = shellwords.GuardSearchCanceled
			}
			if search.reason != shellwords.GuardNone {
				result.Reason = shellwords.GuardRefusal(search.reason)
				return result
			}
			for _, path := range search.joinedPaths {
				if path.data && search.patterns != nil {
					continue
				}
				// A joined option's suffix is not independently shell-expanded.
				candidate := shellPath{Text: path.text, Cwd: candidateCwd, Dialect: candidateDialect, Origin: invocation.Script}
				if path.file && strings.ContainsAny(path.text, "*?[") {
					candidate.ExpandPattern = true
					candidate.GlobReason = "joined search file wildcard is unsupported"
				}
				result.Paths = append(result.Paths, candidate)
			}
			for index, arg := range invocation.Words[1:] {
				if search.patterns[index] {
					continue
				}
				parts := []shellwords.Word{arg}
				if pathWords != nil {
					parts = pathWords[index]
				}
				for _, part := range parts {
					expand := candidateDialect == shellwords.PowerShell && !literalPath || candidateDialect == shellwords.POSIX && posixExpandsPattern(part.Raw)
					if (!part.Literal && !expand) || part.Text == "" || part.Text == "-" || strings.HasPrefix(part.Text, "-") {
						continue
					}
					result.Paths = append(result.Paths, makePath(part, expand))
				}
			}
			continue
		}
		if verb == gitExecutable {
			for _, candidate := range gitShowObjectPaths(invocation.Words[1:]) {
				result.Paths = append(result.Paths, shellPath{Text: candidate, ExpandPattern: true, GitObject: true, Origin: invocation.Script})
			}
		}
	}
	if searchProofAttempted && ctx.Err() != nil {
		result.Reason = shellwords.GuardRefusal(shellwords.GuardSearchCanceled)
	}
	if inspectionProofAttempted && ctx.Err() != nil {
		result.Reason = shellwords.GuardRefusal(shellwords.GuardBashCanceled)
	}
	return result
}

// readExecutableName retains the legacy module-qualified PowerShell reader
// spelling that can arrive through Codex's Bash-shaped command field. In a
// POSIX parse the backslash is an escape, so Raw is the only place where the
// qualification remains available.
func readExecutableName(word shellwords.Word) string {
	verb := strings.ToLower(shellwords.ExecutableName(word.Text))
	if readerCommands[verb] {
		return verb
	}
	raw := strings.Trim(word.Raw, `"'`)
	if _, qualified, ok := strings.Cut(raw, `\`); ok && strings.Contains(qualified, "-") {
		qualified = strings.ToLower(shellwords.ExecutableName(qualified))
		if readerCommands[qualified] {
			return qualified
		}
	}
	return verb
}

func powerShellPathListReader(verb string) bool {
	switch verb {
	case "get-content", "gc", "type", "select-string", "sls", "copy-item", "cpi", "copy", "move-item", "mi", "move", "format-hex":
		return true
	}
	return false
}

func hasLiteralPathFlag(words []shellwords.Word) bool {
	for _, word := range words {
		if word.Text == "--" {
			break
		}
		if word.Literal && strings.EqualFold(word.Text, "-LiteralPath") {
			return true
		}
	}
	return false
}

// posixExpandsPattern reports whether a glob metacharacter occurs outside
// POSIX quotes and escapes. Word.Quoted is intentionally insufficient here:
// a word may mix quoted text with an unquoted bracket expression.
func posixExpandsPattern(raw string) bool {
	inSingle, inDouble := false, false
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if inSingle {
			if c == '\'' {
				inSingle = false
			}
			continue
		}
		if inDouble {
			if c == '"' {
				inDouble = false
			} else if c == '\\' && i+1 < len(raw) && strings.ContainsRune("$`\"\\\n", rune(raw[i+1])) {
				i++
			}
			continue
		}
		if c == '$' && i+1 < len(raw) && raw[i+1] == '\'' {
			end, ok := posixANSIQuoteEnd(raw, i)
			if !ok {
				return false
			}
			i = end - 1
			continue
		}
		switch c {
		case '\\':
			if i+1 < len(raw) {
				i++
			}
		case '\'':
			inSingle = true
		case '"':
			inDouble = true
		case '*', '?', '[':
			return true
		}
	}
	return false
}

// posixGlobPattern decodes a literal POSIX word while escaping glob
// metacharacters that appeared inside quotes or after an escape. This keeps a
// quoted metacharacter inert even when another part of the same word is an
// active glob.
func posixGlobPattern(raw string) (string, error) {
	var out strings.Builder
	inSingle, inDouble := false, false
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if inSingle {
			if c == '\'' {
				inSingle = false
				continue
			}
			if strings.ContainsRune("*?[", rune(c)) {
				out.WriteByte('\\')
			}
			if c == '\\' && runtime.GOOS == "windows" {
				c = '/'
			}
			out.WriteByte(c)
			continue
		}
		if inDouble {
			if c == '"' {
				inDouble = false
				continue
			}
			if c == '\\' && i+1 < len(raw) && strings.ContainsRune("$`\"\\\n", rune(raw[i+1])) {
				i++
				c = raw[i]
			}
			if strings.ContainsRune("*?[", rune(c)) {
				out.WriteByte('\\')
			}
			if c == '\\' && runtime.GOOS == "windows" {
				c = '/'
			}
			out.WriteByte(c)
			continue
		}
		if c == '$' && i+1 < len(raw) && raw[i+1] == '\'' {
			end, ok := posixANSIQuoteEnd(raw, i)
			if !ok {
				return "", errors.New("malformed ANSI-C wildcard quote")
			}
			// Reuse the shell parser's decoder so escape support cannot diverge.
			parsed := shellwords.Parse("echo "+raw[i:end], shellwords.POSIX)
			if len(parsed.Invocations) != 1 || !parsed.Invocations[0].Inspectable || len(parsed.Invocations[0].Words) != 2 || !parsed.Invocations[0].Words[1].Literal {
				return "", errors.New("unsupported ANSI-C wildcard quote")
			}
			decoded := parsed.Invocations[0].Words[1].Text
			for j := 0; j < len(decoded); j++ {
				literal := decoded[j]
				if literal == '\\' && runtime.GOOS == "windows" {
					literal = '/'
				}
				if strings.ContainsRune("*?[\\", rune(literal)) {
					out.WriteByte('\\')
				}
				out.WriteByte(literal)
			}
			i = end - 1
			continue
		}
		switch c {
		case '\\':
			if i+1 < len(raw) {
				i++
				c = raw[i]
				if strings.ContainsRune("*?[", rune(c)) {
					out.WriteByte('\\')
				}
				if c == '\\' && runtime.GOOS == "windows" {
					c = '/'
				}
				out.WriteByte(c)
			} else {
				out.WriteByte(c)
			}
		case '\'':
			inSingle = true
		case '"':
			inDouble = true
		default:
			out.WriteByte(c)
		}
	}
	return out.String(), nil
}

func posixANSIQuoteEnd(raw string, start int) (int, bool) {
	for i := start + 2; i < len(raw); i++ {
		if raw[i] == '\\' {
			i++
			continue
		}
		if raw[i] == '\'' {
			return i + 1, true
		}
	}
	return len(raw), false
}

func normalizeGlobPattern(pattern string) string {
	var out strings.Builder
	for i := 0; i < len(pattern); i++ {
		if pattern[i] == '\\' && i+1 < len(pattern) && strings.ContainsRune("*?[", rune(pattern[i+1])) {
			out.WriteByte('\\')
			out.WriteByte(pattern[i+1])
			i++
			continue
		}
		if pattern[i] == '\\' {
			out.WriteByte('/')
			continue
		}
		out.WriteByte(pattern[i])
	}
	return strings.ToLower(out.String())
}

// gitShowObjectPaths returns the object paths a `git show rev:path` line
// names, reading git's global options with argv.ReadGit. A line that is not
// readable as a show, whether git prints and exits (GitPrints) or the options
// cannot be read (GitUnread), gives no paths, so such a line is not read as a
// credential. The verb is matched with EqualFold, because a lower-cased copy
// would miss spellings such as U+017F that fold to it.
func gitShowObjectPaths(words []shellwords.Word) []string {
	g := argv.ReadGit(words)
	if g.State != argv.GitVerb || !strings.EqualFold(g.Verb.Text, "show") {
		return nil
	}
	var out []string
	for _, word := range g.Args {
		if word.Text == "--" {
			break
		}
		if !word.Literal || strings.HasPrefix(word.Text, "-") {
			continue
		}
		colon := strings.IndexByte(word.Text, ':')
		if colon > 0 && colon+1 < len(word.Text) {
			out = append(out, word.Text[colon+1:])
		}
	}
	return out
}

// classifyLiteralCandidate classifies literal operands and Git object patterns.
// Local active globs must be resolved before reaching this classifier.
func classifyLiteralCandidate(candidate shellPath, commandOverrides []string) (finding, bool) {
	if direct, ok := classify(candidate.Text); ok {
		return direct, true
	}
	if override, ok := classifyWithOverrides(candidate.Text, commandOverrides); ok {
		return override, true
	}
	if !candidate.GitObject || !candidate.ExpandPattern {
		return finding{}, false
	}
	pattern := candidate.Pattern
	if pattern == "" {
		pattern = candidate.Text
	}
	normalized := normalizeGlobPattern(pattern)
	basePattern := path.Base(normalized)
	if !strings.ContainsAny(basePattern, "*?[") {
		return finding{}, false
	}
	result, ok := classifyPattern(path.Dir(normalized), basePattern)
	if !ok {
		return finding{}, false
	}
	result.path = candidate.Text
	return result, true
}

func collectShell(candidates []shellPath, allowlist, commandOverrides []string) ([]finding, error) {
	budget := globBudget{}
	seenCandidates := map[shellPath]bool{}
	seenFindings := map[string]bool{}
	var out []finding
	for _, candidate := range candidates {
		if candidate.Text == "" || allowed(candidate.Text, allowlist) {
			continue
		}
		if seenCandidates[candidate] {
			continue
		}
		seenCandidates[candidate] = true
		resolved := []shellPath{candidate}
		overrides := commandOverrides
		if !candidate.GitObject && candidate.ExpandPattern && strings.ContainsAny(candidate.Text, "*?[") {
			overrides = globOverridePaths(append(append([]string(nil), commandOverrides...), hostCredentialOverridePaths()...), candidate.Cwd)
			matches, err := budget.resolveShellGlob(candidate)
			if err != nil {
				return nil, candidate.globError(err)
			}
			resolved = nil
			for _, match := range matches {
				paths, err := matchedCredentialPaths(match)
				if err != nil {
					return nil, candidate.globError(err)
				}
				for _, p := range paths {
					resolved = append(resolved, shellPath{Text: p})
				}
			}
		}
		for _, actual := range resolved {
			if allowed(actual.Text, allowlist) {
				continue
			}
			if result, ok := classifyLiteralCandidate(actual, overrides); ok {
				result.path, result.origin = candidate.Text, candidate.Origin
				findingKey := result.path + "\x00" + result.category + "\x00" + result.severity
				if seenFindings[findingKey] {
					continue
				}
				seenFindings[findingKey] = true
				out = append(out, result)
			}
		}
	}
	slices.SortFunc(out, func(a, b finding) int {
		if a.severity != b.severity {
			if a.severity == sevCritical {
				return -1
			}
			return 1
		}
		return strings.Compare(a.path, b.path)
	})
	return out, nil
}

// shellParseMessage names the refusal; a refusal under a named script rule leads with its remedy. Script refusals
// carry a path and line, never script text.
func shellParseMessage(refusal shellwords.Refusal) string {
	if remedy, ok := refusal.Remedy(); ok {
		return "secrets-guardian could not inspect a recognized execution path: " + remedy
	}
	return fmt.Sprintf("secrets-guardian could not inspect a recognized execution path (%s)", refusal.String())
}

// allowed reports whether a path matches an allowlist entry. Matching is
// substring, as COMMAND_GUARD_ALLOWLIST is, so one entry can cover a tree.
func allowed(candidate string, allowlist []string) bool {
	if len(allowlist) == 0 {
		return false
	}
	lower := strings.ToLower(strings.ReplaceAll(candidate, "\\", "/"))
	for _, entry := range allowlist {
		if entry != "" && strings.Contains(lower, entry) {
			return true
		}
	}
	return false
}

// parsePathAllowlist splits SECRETS_GUARD_ALLOWLIST into comparable entries:
// lowercased, trimmed, with backslashes normalized to forward slashes, and
// empty entries dropped. Candidates are normalized the same way in allowed,
// so one entry matches a path however the surface spelled it.
//
// It is deliberately not hookutil.SplitCommaList: that helper splits a list
// and nothing else, and the two guardians' allowlists compare differently.
func parsePathAllowlist(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if trimmed := strings.ToLower(strings.TrimSpace(part)); trimmed != "" {
			out = append(out, strings.ReplaceAll(trimmed, "\\", "/"))
		}
	}
	return out
}
