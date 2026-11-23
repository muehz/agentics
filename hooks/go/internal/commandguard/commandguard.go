// Package commandguard implements the command-guardian hook: a PreToolUse
// guardrail that inspects a proposed tool call and may deny it.
//
// A decoded event whose tool input cannot be read fails closed with a deny
// outcome, and an event that fails validation denies the same way. Otherwise
// the output is a policy choice:
// in block mode a blocking threat is a deny outcome, in warn mode every
// threat is a context outcome, and an info-tier threat is context in either
// mode.
//
// Environment:
//
//	COMMAND_GUARD_MODE      - warn or block; the default is block (any other
//	                          value is block, with a diagnostic)
//	SKIP_COMMAND_GUARD      - "true" disables the hook entirely
//	COMMAND_GUARD_LOG_DIR   - guard.log directory; the default is
//	                          <log_root>/agentics/logs/command-guardian, or
//	                          .agentics/logs/command-guardian under the process
//	                          working directory when no log root is known
//	COMMAND_GUARD_ALLOWLIST - comma-separated substrings; a match skips the scan
//
// The scanned text is the tool name followed by the compact JSON of the tool
// input (or of its command alone, for the Bash tool), so a newline inside a
// command reaches the rules as the two-character escape \n, a quote as \",
// and non-ASCII as itself. The rule bank is written against that text.
//
// Workflow policy rules (draft pull requests, package publication, review
// thread and alert disposal, command advice) can be switched off by a
// settings key; see rules.go and the settings package. Fixed safety rules
// have no key.
//
// Outcome values never carry a NUL byte: deny and context reasons are built
// from the static rule-table columns, threat counts, and settings file paths,
// and every error message is a fixed string.
package commandguard

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"runtime"
	"slices"
	"strings"

	"github.com/muehz/agentics/hooks/go/internal/argv"
	"github.com/muehz/agentics/hooks/go/internal/contract"
	"github.com/muehz/agentics/hooks/go/internal/guardmode"
	"github.com/muehz/agentics/hooks/go/internal/hookspec"
	"github.com/muehz/agentics/hooks/go/internal/hookutil"
	"github.com/muehz/agentics/hooks/go/internal/shellwords"
	"github.com/muehz/agentics/hooks/go/internal/toolcall"
	"github.com/muehz/agentics/hooks/go/internal/toolname"
)

// skipTools are the non-execution tools whose inputs are natural language,
// structured data, or file content. Their payloads routinely mention
// dangerous-looking strings and must not block.
var skipTools = map[string]bool{
	"Agent": true, "Read": true, "Write": true, "Edit": true, "MultiEdit": true,
	"TaskCreate": true, "TaskUpdate": true, "TaskGet": true, "TaskList": true,
	"TaskOutput": true, "TaskStop": true, "SendMessage": true,
	"NotebookEdit": true, "WebFetch": true, "WebSearch": true,
	"AskUserQuestion": true, "Glob": true, "Grep": true,
	"ExitPlanMode": true, "EnterPlanMode": true,
}

var (
	errValidate = errors.New("command-guardian could not validate normalized input")
	errExtract  = errors.New("command-guardian could not extract normalized input")
)

const (
	unclassifiedTaskInputCategory   = "command_parse_limit"
	unclassifiedTaskInputMatch      = "unclassified task stdin"
	unclassifiedTaskInputSuggestion = "Use a direct command instead: interactive stdin to an existing background task cannot be classified against command, secrets, and release guards."
)

// Run is the command-guardian handler.
//
// It writes to req.Diag on a filesystem fault, an unusable COMMAND_GUARD_MODE
// value, and for the threat table; the surface adapter discards that stream
// on every outcome kind but error.
func Run(req hookspec.Request) hookspec.Outcome {
	diag := req.Diag
	if diag == nil {
		diag = io.Discard
	}

	control := guardmode.Control{
		Guard:   "command-guardian",
		SkipVar: "SKIP_COMMAND_GUARD",
		ModeVar: "COMMAND_GUARD_MODE",
	}.Resolve(os.Getenv)
	if control.Skipped {
		return contract.Pass()
	}

	// An empty event carries nothing to inspect.
	if !req.Event.Present() {
		return contract.Pass()
	}

	// Providers discard the diagnostic stream on pass, deny, and context
	// outcomes, so the warning also leads every message reported from here on.
	mode, warning := control.Mode, control.Warning
	if warning != "" {
		fmt.Fprintln(diag, warning)
	}

	// Two stages with two messages: the shared contract validates the event,
	// then the guard reads its own fields.
	event, err := req.Event.Read()
	if err != nil {
		return contract.Deny(reportMessage(warning, errValidate.Error()))
	}
	fields, err := extractFields(event)
	if err != nil {
		return contract.Deny(reportMessage(warning, err.Error()))
	}

	// A write to a hook settings file is judged by its path alone, ahead of
	// the allowlist and of skipTools: the file-writing tools are skipped for
	// their content, and an allowlisted substring in that content must not
	// vouch for the target.
	if path, ok := settingsEditTarget(fields); ok {
		reporting := newThreatReport(diag, mode, warning, fields, nil, nil, req.Settings)
		reporting.report(ruleSettingsFileEdit, path, settingsEditSuggestion)
		return reporting.outcome(nil)
	}

	// A write that points core.hooksPath elsewhere or empties a Git hook removes the checks the repository runs. It
	// is judged by its target and text alone, ahead of the allowlist, for the reason the settings-file write is.
	if path, ok := hookEditTarget(fields); ok {
		reporting := newThreatReport(diag, mode, warning, fields, nil, nil, req.Settings)
		reporting.report(ruleGitHookFileEdit, path, hookEditSuggestion)
		return reporting.outcome(nil)
	}

	// Interactive stdin sent to an existing background task arrives without the
	// command or interpreter reading behind the task handle, so the guard
	// cannot classify it against command, secrets, or release rules. Judge it
	// ahead of the allowlist so an allowlist substring in opaque stdin cannot
	// vouch for an unclassified background interpreter.
	if fields.unclassifiedTaskInput {
		reporting := newThreatReport(diag, mode, warning, fields, nil, nil, req.Settings)
		reporting.report(ruleUnclassifiedTaskInput, unclassifiedTaskInputMatch, unclassifiedTaskInputSuggestion)
		return reporting.outcome(nil)
	}

	// The promoted-command rule: the Bash tool scans its command alone, so
	// sibling metadata cannot trigger or vouch for executable behavior. Every
	// other tool, including structured MCP and unknown tools that carry a
	// command field, scans its whole input.
	source := newScanSource(fields)
	combined := fields.toolName + " " + source.text

	// A trimmed allowlist entry found anywhere in the scan text skips the
	// scan.
	for _, pattern := range hookutil.SplitCommaList(os.Getenv("COMMAND_GUARD_ALLOWLIST")) {
		pattern = strings.TrimSpace(pattern)
		if pattern != "" && strings.Contains(combined, pattern) {
			return contract.Pass()
		}
	}

	if skipTools[fields.toolName] {
		return contract.Pass()
	}

	// A bash or sh script call is expanded into the script's own commands, so
	// every rule below judges what the script runs, not the call that names it.
	inspection := fields.call.Inspect(req.ExecutionContext(), req.Scripts)
	parsed, policyParsed := inspection.Source, inspection.Policy
	rd := newReadings(req.ExecutionContext(), source, fields, inspection)
	invocations := rd.invocations

	reporting := newThreatReport(diag, mode, warning, fields, rd.redactor, parsed.Scripts, req.Settings)
	report, reportRedacted := reporting.report, reporting.reportLabeled

	// Command-guardian's posture for a refused invocation: the first one is
	// reported as command_parse_limit at high severity, so a command line the
	// guard cannot read is never treated as harmless.
	if fields.commandPresent {
		for index, inv := range invocations {
			if _, refused := inspection.Refusal(req.ExecutionContext(), index); refused {
				// A typed environment effect supplies a specific, sanitized
				// failure even when an env wrapper has no child executable.
				if hooksPathAssignment(req.ExecutionContext(), inv).kind != noGitHookIssue {
					continue
				}
				suggestion := "Use a command spelling without the unsupported wrapper or option so the guard can inspect the literal invocation."
				// A script limit names itself, so the caller can tell a missing or
				// oversized script from an unsupported wrapper.
				if remedy, ok := inv.Reason.Remedy(); ok {
					suggestion = remedy
				}
				switch {
				case powerShellEnvironmentSource(inv) || inv.Reason.Canceled() && inv.Dialect == shellwords.PowerShell:
					report(ruleUninspectableInvocation, powerShellEnvironmentMatch, suggestion)
				case inv.Script != nil:
					// The refused child names its own location, which its reason in the suggestion repeats.
					reportRedacted(ruleUninspectableInvocation, inv.Script.Location(), suggestion)
				default:
					report(ruleUninspectableInvocation, inv.Raw, suggestion)
				}
				break
			}
		}
	}

	checkDestructiveCommands(invocations, report)
	// The data-view reparse of a PowerShell call expands no scripts, so the calls come from the guard's own parse.
	checkScriptCallerArguments(parsed.ScriptCalls, reportRedacted)
	checkSettingsFileWrites(invocations, parsed.ScriptCalls, report)
	protectedDeletionComplete := checkProtectedDeletion(invocations, report)
	checkHookFileEdits(invocations, report)
	for i, inv := range invocations {
		if inv.Dialect == shellwords.PowerShell && !inspection.Authority(i).AllowsNewProof() {
			protectedDeletionComplete = false
		}
	}
	gitEnvironmentIssue := gitEnvironmentEffectsIssue(req.ExecutionContext(), policyParsed.EnvironmentEffects)
	if alternateGitConfigOverride(fields.call, runtime.GOOS == "windows") {
		gitEnvironmentIssue = preferGitIssue(gitEnvironmentIssue, alternateGitConfigIssue())
	}
	checkGitEnvironmentCommands(req.ExecutionContext(), invocations, policyParsed.BareAssignments, gitEnvironmentIssue, report)
	gh := ghScanner{toolName: fields.toolName, src: rd.gh}
	checkGhPublication(&gh, invocations, report)

	// The same publication and destructive policy on the other transport. The
	// gh rules read a command line, so without this a GitHub MCP tool would
	// reach the generic bank alone and every publication gate would be one
	// tool call away from a bypass. It runs before the generic bank so a typed
	// MCP match names the deny reason ahead of any text match on the payload.
	checkMCP(fields.toolName, fields.call.Layers(), report)
	checkDestructiveSQL(fields, invocations, report)
	checkTransferCommands(invocations, report)

	// The generic bank: each rule takes its first match, original text before
	// joined text. Parsed commands skip superseded legacy rows.
	scanThreatPatternBank(rd.texts, fields.commandPresent, protectedDeletionComplete, report)

	return reporting.outcome(func() bool {
		return gitEnvironmentIssue.kind != noGitHookIssue ||
			gitConfigurationDiagnosticSource(req.ExecutionContext(), policyParsed)
	})
}

// normalizedFields are the values the guard reads out of the neutral event.
type normalizedFields struct {
	toolName              string
	inputJSON             string
	command               string
	commandPresent        bool
	unclassifiedTaskInput bool
	scanInput             string
	logRoot               string
	// cwd is the event's working directory, or "" when the event has none.
	cwd string

	// input is the decoded tool input. The text rules read inputJSON; the SQL
	// and file-write rules read typed fields off this object.
	input map[string]any

	// call is the tool call the input was read from. The execution context,
	// the script options, the argument layers, and the overrides come from it.
	call toolcall.Call
}

// scanSource is the promoted-command rule as one value. The rule makes one
// decision — does this event scan the Bash tool's command alone, or its whole
// input document? — and that decision has three consequences applied far
// apart: the text the threat scan reads, the text the gh rules read, and the
// tokenizer mode segmentShellLine runs in.
type scanSource struct {
	// promotedCommand reports that the Bash tool carried a command field.
	promotedCommand bool

	// text is the input text the threat scan reads: the compact one-key
	// {"command":...} document when promotedCommand, otherwise the contract's
	// own compact bytes for the whole tool input.
	text string

	// command is the promoted command the gh rules read when
	// promotedCommand; empty otherwise.
	command string

	// lines is the fallback text the gh rules segment when no promoted
	// command is available; newReadings fills it.
	lines []string
}

// newScanSource applies the promoted-command rule to one event's fields.
func newScanSource(fields normalizedFields) scanSource {
	if toolname.IsShell(fields.toolName) && fields.commandPresent {
		return scanSource{promotedCommand: true, text: fields.scanInput, command: fields.command}
	}
	return scanSource{text: fields.inputJSON}
}

// strictSegmentation reports the tokenizer mode for this source. Without the
// promoted command the gh rules scan stringified JSON, in which a quote or a
// backslash is an escape and not shell syntax, so each ends a segment and no
// flag may be vouched for across one. With the promoted command the text is
// real shell, and quotes open and close quoted spans.
func (s scanSource) strictSegmentation() bool { return !s.promotedCommand }

// extractFields reads the guard's fields out of a validated event. Every
// failure is errExtract.
func extractFields(event contract.NeutralEvent) (normalizedFields, error) {
	// The decode is separate from the event validation on purpose: an
	// integer literal too large for a float64 survives the contract and fails
	// here, so it reports the extract message rather than the validate one.
	// See toolcall.Read.
	call, err := toolcall.Read(event)
	if err != nil {
		return normalizedFields{}, errExtract
	}
	inputObject := call.Input()

	fields := normalizedFields{
		// The contract's own compact bytes: the scan reads them as text, and
		// they keep '<', '>' and '&' raw, which the rules and the allowlist
		// match literally.
		inputJSON: string(event.Tool.InputRaw),
		input:     inputObject,
		call:      call,
	}
	if event.Tool.Name != nil {
		fields.toolName = *event.Tool.Name
	}
	if command, isString := inputObject["command"].(string); isString {
		fields.commandPresent = true
		fields.command = command
	}
	if unclassified, ok := inputObject["unclassified_task_input"].(bool); ok && unclassified {
		fields.unclassifiedTaskInput = true
	}
	if event.Runtime.LogRoot != nil {
		fields.logRoot = *event.Runtime.LogRoot
	}
	if event.Cwd != nil {
		fields.cwd = *event.Cwd
	}
	if fields.commandPresent {
		scanInput, err := encodeCompact(map[string]string{"command": fields.command})
		if err != nil {
			return normalizedFields{}, errExtract
		}
		fields.scanInput = scanInput
	}
	return fields, nil
}

// encodeCompact renders v as compact JSON with HTML escaping off and no
// trailing newline: the same spelling the contract uses for the tool input,
// so the promoted command scans as the contract would have written it.
func encodeCompact(v any) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return "", err
	}
	return strings.TrimSuffix(buf.String(), "\n"), nil
}

// ---------------------------------------------------------------------------
// Inert heredoc stripping
// ---------------------------------------------------------------------------

var (
	// heredocIntroRe matches a top-level cat or tee whose heredoc delimiter is
	// a plain word.
	heredocIntroRe = regexp.MustCompile(`(^|[;|&"[:space:]])(cat|tee)[^;|&<]*<<[[:space:]]*'?[A-Za-z_][A-Za-z0-9_]*`)
	// heredocRestRe accepts what may follow the delimiter: nothing, or one
	// >/>> redirect to a plain path.
	heredocRestRe = regexp.MustCompile(`^'?[[:space:]]*(>>?[[:space:]]*[^[:space:]|&;<>()$` + "`" + `"]+[[:space:]]*)?$`)
	// heredocQuotedRe detects the quoted-delimiter form, which disables
	// expansion entirely.
	heredocQuotedRe = regexp.MustCompile(`<<[[:space:]]*'`)

	// commitStdinIntroRe and commitSubstIntroRe recognize a git commit whose
	// message is a single-quoted heredoc: read from stdin (-F -, --file=-,
	// --file -) or as the exact -m/--message "$(cat <<'D' … )" value. git
	// must begin a command, only flag-like words may sit between commit and
	// the message flag, and nothing may follow the delimiter. The last group
	// is the delimiter.
	commitStdinIntroRe = regexp.MustCompile(`(^|:"|;|&&|\|\|)[[:space:]]*git[[:space:]]+commit([[:space:]]+[-A-Za-z0-9_=.:/+]+)*?[[:space:]]+(-F[[:space:]]+|--file=|--file[[:space:]]+)-[[:space:]]*<<[[:space:]]*'([A-Za-z_][A-Za-z0-9_]*)'$`)
	commitSubstIntroRe = regexp.MustCompile(`(^|:"|;|&&|\|\|)[[:space:]]*git[[:space:]]+commit([[:space:]]+[-A-Za-z0-9_=.:/+]+)*?[[:space:]]+(-m[[:space:]]+|--message=|--message[[:space:]]+)\\"\$\(cat[[:space:]]+<<[[:space:]]*'([A-Za-z_][A-Za-z0-9_]*)'$`)
)

// heredocBodyInert reports whether a heredoc body is data: its delimiter is
// quoted, or it carries no command substitution.
func heredocBodyInert(quoted bool, body string) bool {
	return quoted || (!strings.Contains(body, "$(") && !strings.Contains(body, "`"))
}

// commitHeredocForm names a git commit shape whose heredoc body is the
// commit message; zero is none.
type commitHeredocForm int

const (
	commitFromStdin commitHeredocForm = iota + 1
	commitFromSubstitution
)

// commitHeredocIntro reports whether segment ends in a git commit heredoc
// intro, and its form and delimiter. A # anywhere on the line disqualifies
// it: the intro may sit inside a comment.
func commitHeredocIntro(segment string) (commitHeredocForm, string) {
	if strings.Contains(segment, "#") {
		return 0, ""
	}
	// A second heredoc on the same line would have its body swallowed by
	// this one's strip; only a line whose sole heredoc is the commit's
	// qualifies.
	if strings.Count(segment, "<<") > 1 {
		return 0, ""
	}
	if m := commitStdinIntroRe.FindStringSubmatch(segment); m != nil {
		return commitFromStdin, m[len(m)-1]
	}
	if m := commitSubstIntroRe.FindStringSubmatch(segment); m != nil {
		return commitFromSubstitution, m[len(m)-1]
	}
	return 0, ""
}

// commitHeredocKey pairs a commit heredoc form with the single-quoted
// delimiter its intro names, so eligibility is tied to the exact heredoc an
// invocation's own text introduces, not merely to the form. Counting by form
// alone would let an eligible invocation with no heredoc at all (or one
// closed by a different delimiter) "pay" for an unrelated fake intro that
// happens to share its form (ADR-0004: the parse, not the text, decides
// intent).
type commitHeredocKey struct {
	form  commitHeredocForm
	delim string
}

// commitHeredocDelimRe matches a single-quoted heredoc delimiter, the only
// form the commit intro regexes accept (an unquoted delimiter lets the shell
// expand the body, so it is never eligible).
var commitHeredocDelimRe = regexp.MustCompile(`<<[[:space:]]*'([A-Za-z_][A-Za-z0-9_]*)'`)

// addCommitHeredocDelims records one eligibility credit per distinct
// delimiter that raw text introduces for form, deduplicating within a single
// invocation so one heredoc cannot be counted twice.
func addCommitHeredocDelims(counts map[commitHeredocKey]int, form commitHeredocForm, raw string) {
	seen := map[string]bool{}
	for _, m := range commitHeredocDelimRe.FindAllStringSubmatch(raw, -1) {
		delim := m[1]
		if seen[delim] {
			continue
		}
		seen[delim] = true
		counts[commitHeredocKey{form, delim}]++
	}
}

// eligibleCommitHeredocs counts, per (form, delimiter), the parsed git
// commit invocations that could own such an intro: inspectable, top-level,
// unwrapped, literally git commit, carrying the stdin flag, or -m/--message
// whose value a cat substitution child fills. The delimiter comes from the
// invocation's own Raw (or its cat child's, for the substitution form), so
// an invocation with no heredoc in its text contributes nothing, and one
// whose heredoc closes on a different delimiter cannot back an intro naming
// another.
func eligibleCommitHeredocs(invocations []shellwords.Invocation) map[commitHeredocKey]int {
	counts := map[commitHeredocKey]int{}
	for idx, inv := range invocations {
		if inv.Parent >= 0 || !inv.Inspectable || len(inv.Wrappers) > 0 || len(inv.Words) < 3 {
			continue
		}
		if !inv.Words[0].Literal || inv.Words[0].Text != "git" || !commitHeredocGit(argv.ReadGit(inv.Words[1:])) {
			continue
		}
		stdin, message := false, false
		for i := 2; i < len(inv.Words); i++ {
			w := inv.Words[i]
			next := ""
			if i+1 < len(inv.Words) {
				next = inv.Words[i+1].Text
			}
			switch {
			case w.Text == "--file=-", (w.Text == "-F" || w.Text == "--file") && next == "-":
				stdin = true
			case w.Text == "-m" || w.Text == "--message" || strings.HasPrefix(w.Text, "--message="):
				message = true
			}
		}
		if stdin {
			addCommitHeredocDelims(counts, commitFromStdin, inv.Raw)
		}
		if message {
			for _, child := range invocations {
				if child.Parent == idx && child.Before == shellwords.Substitution && len(child.Words) > 0 && child.Words[0].Text == "cat" {
					addCommitHeredocDelims(counts, commitFromSubstitution, inv.Raw)
					break
				}
			}
		}
	}
	return counts
}

// commitHeredocGit reports a git line read to its verb, with no global option
// before it, whose verb is exactly `commit`. Any global option, an unreadable
// line, or a line that only prints makes the invocation ineligible, so only
// `git commit` with nothing between the two words is eligible.
// The exact `git` head test stays with the caller, because folding it would
// make more invocations eligible.
func commitHeredocGit(g argv.Git) bool {
	return g.State == argv.GitVerb && len(g.Globals) == 0 && g.Verb.Text == "commit"
}

// stripHeredocsFor removes the bodies of heredocs consumed by a top-level cat
// or tee writing to a file or stdout, which are data rather than a command
// stream (hooks/README.md, Command-guardian false-positive controls), and the
// two git commit message heredoc forms. It carries a consumer switch:
// catTee=false skips the cat/tee intro branch entirely, so only the two git
// commit message heredoc forms (-F -, --file=-, --file - forms, and the exact
// -m "$(cat <<'D' … )" value) are stripped. The gh rules use catTee=false,
// because text a cat/tee heredoc writes to a file can be executed later in
// the same command (bash p.sh) and must stay visible to those rules; commit
// message text can never be executed, so it is always safe to strip. Both
// forms are gated by a text anchor and the parsed invocations. The input is a
// JSON-stringified command, so "lines" are found by splitJSONLines rather
// than a plain split on the two-character \n escape, and the segment that
// terminates a heredoc at the end of the string is the delimiter followed by
// the closing quote of the JSON value. Scanning resumes at the terminator.
//
// A segment can only open a heredoc — commit or cat/tee — when command's own
// quote tracker (shellwords.LineStarts) proves the matching physical line of
// the raw command begins outside every quote, outside a heredoc body, and
// not as a continuation (#651): a fake intro sitting inside a quoted string
// elsewhere in the command can never pay for or open a strip. When the
// segment count does not match the line count LineStarts reports, some other
// field in the JSON document carries a newline the command text does not, so
// no segment can be matched to a line's quote state one-for-one; nothing is
// stripped.
func stripHeredocsFor(text, command string, invocations []shellwords.Invocation, catTee bool) string {
	if !strings.Contains(text, "<<") {
		return text
	}
	segments := splitJSONLines(text)
	starts := shellwords.LineStarts(command)
	if len(starts) != len(segments) {
		return text
	}
	var out strings.Builder
	var buf strings.Builder
	skipping := false
	quoted := false
	delim := ""

	// A git commit form is enabled only when the parse holds at least as
	// many eligible invocations of that exact (form, delimiter) as the text
	// holds intros naming it, so an intro-shaped string inside another
	// command's argument never borrows a real commit's eligibility — not
	// even from a real commit invocation whose own heredoc (if it has one at
	// all) closes on a different delimiter. Only intros on a line
	// shellwords.LineStarts proves unquoted are counted: a fake intro
	// hiding inside a quoted string can never open a strip on its own line
	// (starts[i] gates that below), and it must not be allowed to distort
	// the eligibility count for a real intro elsewhere either — inflating it
	// away from a legitimate strip, or (were fake intros ever double
	// counted) deflating it into paying for another fake one.
	intros := map[commitHeredocKey]int{}
	for i, segment := range segments {
		if !starts[i] {
			continue
		}
		if form, delim := commitHeredocIntro(segment); form != 0 {
			intros[commitHeredocKey{form, delim}]++
		}
	}
	eligible := eligibleCommitHeredocs(invocations)
	commitForm := commitHeredocForm(0)

	for i, segment := range segments {
		if skipping {
			buf.WriteString(`\n`)
			buf.WriteString(segment)
			terminated := false
			if strings.HasPrefix(segment, delim) {
				rest := segment[len(delim):]
				if rest == "" || rest[0] == '"' {
					terminated = true
				}
			}
			if terminated {
				skipping = false
				inert := heredocBodyInert(quoted, buf.String())
				if commitForm == commitFromSubstitution && (i+1 >= len(segments) || !strings.HasPrefix(segments[i+1], `)\"`)) {
					inert = false
				}
				if inert {
					out.WriteString(`\n`)
					out.WriteString(segment)
				} else {
					out.WriteString(buf.String())
				}
				buf.Reset()
			}
			continue
		}

		if i > 0 {
			out.WriteString(`\n`)
		}
		out.WriteString(segment)

		if form, delimiter := commitHeredocIntro(segment); starts[i] && form != 0 && intros[commitHeredocKey{form, delimiter}] <= eligible[commitHeredocKey{form, delimiter}] {
			commitForm = form
			quoted = true
			delim = delimiter
			skipping = true
			buf.Reset()
			continue
		}
		if !catTee {
			continue
		}
		if !starts[i] {
			continue
		}
		loc := heredocIntroRe.FindStringIndex(segment)
		if loc == nil {
			continue
		}
		commitForm = 0
		pre, intro, rest := segment[:loc[0]], segment[loc[0]:loc[1]], segment[loc[1]:]
		// A comment can contain a fake introduction whose following lines
		// are executable. Ambiguous hash characters keep the body scanned.
		if strings.Contains(pre+intro, "#") {
			continue
		}
		if strings.ContainsAny(pre, "$`()") || strings.ContainsAny(intro, "$`()") {
			continue
		}
		if !heredocRestRe.MatchString(rest) {
			continue
		}
		quoted = heredocQuotedRe.MatchString(intro)
		marker := strings.Index(intro, "<<")
		tail := strings.TrimLeft(intro[marker+2:], " \t\n\v\f\r")
		delim = strings.TrimPrefix(tail, "'")
		skipping = true
		buf.Reset()
	}

	// An unterminated body is stripped only under the same test; otherwise a
	// missing terminator would hide everything after the intro.
	if skipping && (commitForm != 0 || !heredocBodyInert(quoted, buf.String())) {
		out.WriteString(buf.String())
	}
	return out.String()
}

// ghCommandFromStripped recovers the promoted command from a stripped scan
// body. The gh rules pass the commit-only strip (stripHeredocsFor with
// catTee=false), so cat/tee bodies stay visible to them. The body is the
// tool name, a space, and the compact {"command":…} document; any other
// shape, or a document that is not exactly one command string, returns
// fallback so the gh rules keep the unstripped command.
func ghCommandFromStripped(toolName, scanBody, fallback string) string {
	doc, ok := strings.CutPrefix(scanBody, toolName+" ")
	if !ok {
		return fallback
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(doc), &decoded); err != nil || len(decoded) != 1 {
		return fallback
	}
	command, ok := decoded["command"].(string)
	if !ok {
		return fallback
	}
	return command
}

// splitJSONLines splits a JSON-stringified command's scan text on the
// two-character \n escape that stands for one literal newline byte in the
// underlying command, leaving every other occurrence of the letter n after a
// backslash — including a literal backslash-n pair in the command's own text,
// which the JSON encoder renders as the four bytes \, \, \, n — inside its
// segment. Walking the run of backslashes immediately before each 'n' and
// splitting only when that run is odd-length is the same rule JSON string
// escaping itself uses: an even run is that many literal backslashes (each
// \\ pair collapsing to one) followed by a bare 'n' character, while an odd
// run's last backslash pairs with the 'n' to form the \n escape. Plain
// strings.Split(text, `\n`) cannot make this distinction, so it would also
// cut a line inside a quoted string wherever the command happened to spell
// out a literal backslash-n (#651).
func splitJSONLines(text string) []string {
	var segments []string
	var seg strings.Builder
	run := 0 // length of the backslash run at the end of seg so far
	for i := 0; i < len(text); i++ {
		c := text[i]
		switch {
		case c == '\\':
			run++
			seg.WriteByte(c)
		case c == 'n' && run%2 == 1:
			s := seg.String()
			segments = append(segments, s[:len(s)-1])
			seg.Reset()
			run = 0
		default:
			run = 0
			seg.WriteByte(c)
		}
	}
	segments = append(segments, seg.String())
	return segments
}

// joinScanContinuations produces the continuation-joined scan text: the
// JSON-stringified backslash-newline pair (the four characters \ \ \ n) is
// deleted, and a trailing backslash joins its line to the next with nothing
// between them, so a token split across a continuation reads whole.
func joinScanContinuations(body string) string {
	var out strings.Builder
	for _, line := range strings.Split(body, "\n") {
		line = strings.ReplaceAll(line, `\\\n`, "")
		if strings.HasSuffix(line, `\`) {
			out.WriteString(strings.TrimSuffix(line, `\`))
		} else {
			out.WriteString(line)
			out.WriteString("\n")
		}
	}
	return strings.TrimRight(out.String(), "\n")
}

// ---------------------------------------------------------------------------
// Publish-shaped gh commands
// ---------------------------------------------------------------------------

// ghVerb pairs the fast-reject and extraction regexes for one gated verb.
type ghVerb struct {
	pre     *regexp.Regexp
	extract *regexp.Regexp
}

func newGhVerb(verb string) ghVerb {
	return ghVerb{
		pre:     regexp.MustCompile(`(?i)\bgh[[:space:]]+` + verb),
		extract: regexp.MustCompile(`(?i)\bgh[[:space:]]+` + verb + `.*`),
	}
}

var (
	ghPrCreate    = newGhVerb(`pr[[:space:]]+create`)
	ghPrReady     = newGhVerb(`pr[[:space:]]+ready`)
	ghStackSubmit = newGhVerb(`stack[[:space:]]+submit`)
	ghStackLink   = newGhVerb(`stack[[:space:]]+link`)
)

var (
	// ghStripDQ / ghStripSQ blank quoted spans before flag matching; the
	// double-quote form skips escaped quotes.
	ghStripDQ = regexp.MustCompile(`"([^"\\]|\\.)*"`)
	ghStripSQ = regexp.MustCompile(`'[^']*'`)
	// ghDoubleDashRe removes the positional region a bare `--` opens. It is
	// anchored at the end, so only one match exists.
	ghDoubleDashRe = regexp.MustCompile(`(^|[[:space:]])--([[:space:]].*)?$`)
)

// ghTruthyValues are the spellings pflag parses as true for a bool flag. The
// matcher and the value reader both take their list from here.
var ghTruthyValues = []string{"true", "True", "TRUE", "t", "T", "1"}

// ghFlagRegexp builds the matcher for a flag: the bare flag, or the =value
// form with a value pflag parses as true. Case-sensitive, as flags are.
func ghFlagRegexp(flags string) *regexp.Regexp {
	return regexp.MustCompile(
		`(^|[[:space:]])(` + flags + `)(([[:space:]]|"|$)|=(` + strings.Join(ghTruthyValues, "|") + `)([[:space:]]|"|$))`)
}

var (
	ghHelpFlags  = ghFlagRegexp(`--help|-h`)
	ghDraftFlags = ghFlagRegexp(`--draft|-d`)
	ghAutoFlag   = ghFlagRegexp(`--auto`)
	ghOpenFlag   = ghFlagRegexp(`--open`)
	ghUndoFlag   = ghFlagRegexp(`--undo`)
)

func ghSegmentHasFlag(segment string, flag *regexp.Regexp) bool {
	stripped := ghStripSQ.ReplaceAllString(ghStripDQ.ReplaceAllString(segment, ""), "")
	return flag.MatchString(stripped)
}

func ghSegmentHasEffectiveFlag(segment string, flag *regexp.Regexp) bool {
	parsed := ghDoubleDashRe.ReplaceAllString(segment, "${1}")
	return ghSegmentHasFlag(parsed, flag)
}

// ghUndoValueRe captures the value of each --undo=<value> occurrence in a
// segment already reduced to its effective (post `--`, post-quote-strip)
// region, the same region ghSegmentHasEffectiveFlag inspects.
var ghUndoValueRe = regexp.MustCompile(`(^|[[:space:]])--undo=([^[:space:]"]*)`)

// ghBoolFromValue reports how pflag parses value for a bool flag: ok is
// false when pflag would reject the spelling outright (an invalid value),
// and truthy is meaningful only when ok is true. This is the one vocabulary
// every gh boolean-flag parser in this package uses, so a spelling pflag
// accepts is never denied by one gh code path and allowed by another.
func ghBoolFromValue(value string) (truthy, ok bool) {
	if slices.Contains(ghTruthyValues, value) {
		return true, true
	}
	switch value {
	case "false", "False", "FALSE", "f", "F", "0":
		return false, true
	default:
		return false, false
	}
}

// ghUndoTruthyValue reports whether value is one of the spellings pflag
// parses as true for a bool flag.
func ghUndoTruthyValue(value string) bool {
	truthy, ok := ghBoolFromValue(value)
	return ok && truthy
}

// ghSegmentHasConflictingUndo reports whether segment carries a --undo=<value>
// occurrence whose value is not truthy (false, or an invalid spelling pflag
// would reject). pflag applies the last occurrence of a repeated flag, so
// `--undo --undo=false` actually leaves the PR ready, not draft; treating any
// non-truthy occurrence as a conflict — not only the last one — keeps the
// exemption from firing on that command, per the rule that conflicting safety
// flags do not grant exceptions (hooks/README.md, Command-guardian overview).
func ghSegmentHasConflictingUndo(segment string) bool {
	parsed := ghDoubleDashRe.ReplaceAllString(segment, "${1}")
	stripped := ghStripSQ.ReplaceAllString(ghStripDQ.ReplaceAllString(parsed, ""), "")
	for _, m := range ghUndoValueRe.FindAllStringSubmatch(stripped, -1) {
		if !ghUndoTruthyValue(m[2]) {
			return true
		}
	}
	return false
}

// ghScanner carries the per-event inputs of the gh rules: the scan source and,
// separately, the tool name. src.promotedCommand is false for two different
// events — Bash with no command, which scans the fallback text in strict
// mode, and a non-Bash tool with a command, which scans nothing — and only
// the tool name tells them apart.
type ghScanner struct {
	toolName string
	src      scanSource

	// prepared, lines, hasGH, and segments are computed once per scanner:
	// the four verbs read the same lines and the same segments.
	prepared bool
	lines    []string
	hasGH    bool
	segments [][]string
}

// lineHasFoldedGH reports whether s holds g or G immediately followed by h or
// H. Every gh extract regexp needs that pair, and Go's (?i) folds no rune
// outside ASCII into g or h, so a false answer means no verb can match.
func lineHasFoldedGH(s string) bool {
	for i := 0; i+1 < len(s); i++ {
		if s[i]|0x20 == 'g' && s[i+1]|0x20 == 'h' {
			return true
		}
	}
	return false
}

// prepare reads the lines the gh rules scan and, when any holds a gh pair,
// segments each line once. A segment is built from contiguous runs of its
// line joined by a space or nothing, so a segment holds a gh pair only when
// its line does (TestGhGateIsSound).
func (g *ghScanner) prepare() {
	if g.prepared {
		return
	}
	g.prepared = true
	if g.src.promotedCommand {
		g.lines = strings.Split(joinGhContinuations(g.src.command), "\n")
	} else {
		g.lines = g.src.lines
	}
	for _, line := range g.lines {
		if lineHasFoldedGH(line) {
			g.hasGH = true
			break
		}
	}
	if !g.hasGH {
		return
	}
	strict := g.src.strictSegmentation()
	g.segments = make([][]string, len(g.lines))
	for i, line := range g.lines {
		g.segments[i] = segmentShellLine(line, strict)
	}
}

// invocations returns every segment of the command that invokes verb.
func (g *ghScanner) invocations(verb ghVerb) []string {
	if !toolname.IsShell(g.toolName) {
		return nil
	}
	g.prepare()
	if !g.hasGH {
		return nil
	}
	if !g.src.promotedCommand {
		found := false
		for _, line := range g.lines {
			if verb.pre.MatchString(line) {
				found = true
				break
			}
		}
		if !found {
			return nil
		}
	}
	var out []string
	for _, lineSegments := range g.segments {
		for _, segment := range lineSegments {
			if m := verb.extract.FindString(segment); m != "" {
				out = append(out, m)
			}
		}
	}
	return out
}

// check runs the four gated-verb rules, most specific first. Each reports at
// most one threat: the first segment that is not a help probe and does not
// carry the exempting flag.
func (g *ghScanner) check(report reportFunc) {
	// `gh pr create` must include --draft (or -d).
	for _, segment := range g.invocations(ghPrCreate) {
		if segment == "" || ghSegmentHasEffectiveFlag(segment, ghHelpFlags) || ghSegmentHasEffectiveFlag(segment, ghDraftFlags) {
			continue
		}
		report(ruleGhPrCreate, segment, ghPrCreateSuggestion)
		break
	}

	// `gh pr ready` promotes a draft PR. --undo converts it back to a draft and
	// this rule does not gate it; the gh skill tells agents never to run it.
	for _, segment := range g.invocations(ghPrReady) {
		if segment == "" || ghSegmentHasEffectiveFlag(segment, ghHelpFlags) {
			continue
		}
		if ghSegmentHasEffectiveFlag(segment, ghUndoFlag) && !ghSegmentHasConflictingUndo(segment) {
			continue
		}
		report(ruleGhPrReady, segment,
			ghPrReadySuggestion)
		break
	}

	// `gh stack submit`: only --auto without --open is draft-only.
	for _, segment := range g.invocations(ghStackSubmit) {
		if segment == "" || ghSegmentHasEffectiveFlag(segment, ghHelpFlags) {
			continue
		}
		if ghSegmentHasEffectiveFlag(segment, ghAutoFlag) && !ghSegmentHasFlag(segment, ghOpenFlag) {
			continue
		}
		report(ruleGhStackSubmit, segment, ghStackSubmitSuggestion)
		break
	}

	// `gh stack link --open` marks PRs ready.
	for _, segment := range g.invocations(ghStackLink) {
		if segment == "" || ghSegmentHasEffectiveFlag(segment, ghHelpFlags) {
			continue
		}
		if ghSegmentHasFlag(segment, ghOpenFlag) {
			report(ruleGhStackLink, segment, ghStackLinkSuggestion)
			break
		}
	}
}

// checkGhPublication runs both readings of the gated gh verbs, keying the
// dedupe on the rule.
//
// The regex rules and the invocation path judge the same gh verbs from
// two readings of one command. Both always run; a threat the invocation
// path finds is dropped only when the regex rules already reported that
// verb, so one command never carries two threats for one verb and a
// spelling one reading cannot see is still caught by the other.
func checkGhPublication(gh *ghScanner, invocations []shellwords.Invocation, report reportFunc) {
	ghReported := map[ruleID]bool{}
	gh.check(func(r rule, match, suggestion string) {
		ghReported[r.id] = true
		report(r, match, suggestion)
	})
	checkInvocationPublication(invocations, func(r rule, match, suggestion string) {
		if r.category == "unsafe_publication" && ghReported[r.id] {
			return
		}
		report(r, match, suggestion)
	})
}

type booleanFlagState struct {
	trueSeen    bool
	falseSeen   bool
	invalidSeen bool
}

func (s booleanFlagState) effectiveTrue() bool {
	return s.trueSeen && !s.falseSeen && !s.invalidSeen
}

func observeBooleanFlag(state *booleanFlagState, token, name string, negativeNames ...string) bool {
	if token == name {
		state.trueSeen = true
		return true
	}
	for _, negative := range negativeNames {
		if token == negative {
			state.falseSeen = true
			return true
		}
		prefix := negative + "="
		if strings.HasPrefix(token, prefix) {
			switch strings.TrimPrefix(token, prefix) {
			case "true":
				state.falseSeen = true
			case "false":
				state.trueSeen = true
			default:
				state.invalidSeen = true
			}
			return true
		}
	}
	prefix := name + "="
	if !strings.HasPrefix(token, prefix) {
		return false
	}
	switch strings.TrimPrefix(token, prefix) {
	case "true":
		state.trueSeen = true
	case "false":
		state.falseSeen = true
	default:
		state.invalidSeen = true
	}
	return true
}

// observeGhBooleanFlag is observeBooleanFlag's counterpart for gh's own
// pflag-backed boolean flags: the =value form accepts pflag's full boolean
// vocabulary (True/TRUE/t/T/1, False/FALSE/f/F/0) via ghBoolFromValue,
// matching the vocabulary the regex-based gh rules already accept, instead
// of only lowercase true/false.
func observeGhBooleanFlag(state *booleanFlagState, token, name string) bool {
	if token == name {
		state.trueSeen = true
		return true
	}
	prefix := name + "="
	if !strings.HasPrefix(token, prefix) {
		return false
	}
	truthy, ok := ghBoolFromValue(strings.TrimPrefix(token, prefix))
	switch {
	case !ok:
		state.invalidSeen = true
	case truthy:
		state.trueSeen = true
	default:
		state.falseSeen = true
	}
	return true
}

func checkInvocationPublication(invocations []shellwords.Invocation, report reportFunc) {
	for _, inv := range invocations {
		if !inv.Inspectable || len(inv.Words) < 2 || !inv.Words[0].Literal {
			continue
		}
		executable := strings.ToLower(shellwords.ExecutableName(inv.Words[0].Text))
		if check, ok := publicationClients[executable]; ok {
			check(inv, executable, report)
		}
	}
}

// publicationCheck judges one invocation of a command that publishes. The executable is the lower-case name the
// invocation was dispatched on, for the check that serves several names.
type publicationCheck func(inv shellwords.Invocation, executable string, report reportFunc)

// publicationClients holds the check for every command the guard reads for publication. Each name must be Classified
// in argv.
var publicationClients = map[string]publicationCheck{
	"gh": func(inv shellwords.Invocation, _ string, report reportFunc) {
		checkGhInvocationPublicationComplete(inv.Words, inv.Complete, report)
	},
	"npm": func(inv shellwords.Invocation, _ string, report reportFunc) {
		checkNpmPublicationComplete(inv.Words, inv.Complete, report)
	},
	"dotnet": checkNuGetPublication,
	"nuget":  checkNuGetPublication,
}

func checkGhInvocationPublicationComplete(words []shellwords.Word, complete bool, report reportFunc) {
	command, args, globalHelp, ok := ghSubcommand(words[1:])
	if !ok {
		return
	}
	switch command {
	case "pr ready":
		help, undo := booleanFlagState{}, booleanFlagState{}
		for i := 0; i < len(args); i++ {
			token := args[i].Text
			if isPosixCommentWord(args[i]) {
				break
			}
			if token == "--" {
				break
			}
			if observeGhBooleanFlag(&help, token, "--help") || observeGhBooleanFlag(&help, token, "-h") {
				continue
			}
			observeGhBooleanFlag(&undo, token, "--undo")
		}
		if complete && (globalHelp || help.effectiveTrue()) {
			return
		}
		if complete && undo.effectiveTrue() {
			return
		}
		report(ruleGhPrReady, joinInvocation(words),
			ghPrReadySuggestion)
	default:
		checkGhVerbInvocation(command, words, args, globalHelp, complete, report)
	}
}

// ghSubcommand walks past gh's global flags to find the two-word subcommand
// name and the words after it. globalHelp reports whether a global --help/-h
// (one that precedes the subcommand, so gh shows top-level or subcommand help
// instead of running it) was seen among the skipped flags; a caller must
// treat that the same as a --help found among args, or a help request behind
// a global flag like -R is denied as if it were the real invocation.
func ghSubcommand(words []shellwords.Word) (command string, args []shellwords.Word, globalHelp, ok bool) {
	for i := 0; i < len(words); i++ {
		text := words[i].Text
		if text == "--" {
			return "", nil, globalHelp, false
		}
		if recognized, consumesNext := consumesGhGlobalValue(text); recognized {
			if consumesNext && i+1 < len(words) {
				i++
			}
			continue
		}
		if text == "--help" || text == "-h" {
			globalHelp = true
			continue
		}
		if strings.HasPrefix(text, "-") {
			continue
		}
		if i+1 >= len(words) || !words[i].Literal || !words[i+1].Literal {
			return "", nil, globalHelp, false
		}
		return strings.ToLower(text + " " + words[i+1].Text), words[i+2:], globalHelp, true
	}
	return "", nil, globalHelp, false
}

func consumesGhGlobalValue(token string) (recognized, consumesNext bool) {
	for _, flag := range []string{"--repo", "-R", "--hostname", "--config-dir"} {
		if token == flag {
			return true, true
		}
		if strings.HasPrefix(token, flag+"=") {
			return true, false
		}
		if flag == "-R" && strings.HasPrefix(token, flag) && len(token) > len(flag) {
			return true, false
		}
	}
	return false, false
}

// checkNpmPublicationComplete reports an npm publish that is not a preview. A non-literal or missing subcommand is
// not reported, because nothing proves which subcommand runs. Only a complete invocation that is effectively help or a
// dry run is a preview; words the reader could not see might undo the flag.
func checkNpmPublicationComplete(words []shellwords.Word, complete bool, report reportFunc) {
	cmd := readNpm(words)
	if !cmd.found || cmd.subcommand.Text != "publish" {
		return
	}
	if complete && (cmd.help.effectiveTrue() || cmd.dryRun.effectiveTrue()) {
		return
	}
	report(ruleNpmPublish, joinInvocation(words), npmPublishSuggestion)
}

func joinInvocation(words []shellwords.Word) string {
	parts := make([]string, 0, len(words))
	for _, word := range words {
		parts = append(parts, word.Raw)
	}
	return strings.Join(parts, " ")
}

func isPosixCommentWord(word shellwords.Word) bool {
	return strings.HasPrefix(word.Raw, "#") && strings.HasPrefix(word.Text, "#")
}

// joinGhContinuations joins a trailing backslash's line to the next with a
// space, the way the shell reads a continued command line. (The pattern-scan
// join uses the empty string instead, so a token split across a continuation
// reads whole there.)
func joinGhContinuations(command string) string {
	var out strings.Builder
	for _, line := range strings.Split(command, "\n") {
		if strings.HasSuffix(line, `\`) {
			out.WriteString(strings.TrimSuffix(line, `\`))
			out.WriteString(" ")
		} else {
			out.WriteString(line)
			out.WriteString("\n")
		}
	}
	return strings.TrimRight(out.String(), "\n")
}

// segmentShellLine splits one line into command segments on ; | &, tracking
// quotes, treating an unquoted # at a word boundary as a comment, consuming
// redirect operators with their operands (a parenthesized operand — process
// or command substitution — is emitted as its own segment with the parens
// blanked), and, in strict mode, treating quotes and backslashes as further
// segment boundaries.
func segmentShellLine(line string, strict bool) []string {
	var segments []string
	var seg []byte
	inSQ, inDQ := false, false
	n := len(line)
	flush := func() {
		segments = append(segments, string(seg))
		seg = seg[:0]
	}

	for i := 0; i < n; i++ {
		c := line[i]
		if inSQ {
			if c == '\'' {
				inSQ = false
			}
			seg = append(seg, c)
			continue
		}
		if inDQ {
			if c == '\\' {
				seg = append(seg, c)
				if i+1 < n {
					seg = append(seg, line[i+1])
				}
				i++
				continue
			}
			if c == '"' {
				inDQ = false
			}
			seg = append(seg, c)
			continue
		}
		// An unquoted, unescaped # at a word boundary starts a shell comment:
		// the rest of the line never executes.
		if c == '#' && (i == 0 || line[i-1] == ' ' || line[i-1] == '\t') {
			break
		}
		if strict && (c == '"' || c == '\'' || c == '\\') {
			flush()
			continue
		}
		if c == '\'' {
			inSQ = true
			seg = append(seg, c)
			continue
		}
		if c == '"' {
			inDQ = true
			seg = append(seg, c)
			continue
		}
		if c == '<' || c == '>' || (c == '&' && i+1 < n && line[i+1] == '>') {
			j := i
			if c == '&' {
				j++
			}
			for j < n && (line[j] == '<' || line[j] == '>') {
				j++
			}
			if j < n && line[j] == '&' {
				// fd duplication (2>&1, >&-): the target is part of the
				// operator; there is no operand word to skip.
				j++
				for j < n && (line[j] == '-' || (line[j] >= '0' && line[j] <= '9')) {
					j++
				}
			} else {
				for j < n && (line[j] == ' ' || line[j] == '\t') {
					j++
				}
				var operand []byte
				depth := 0
				for j < n {
					oc := line[j]
					if depth == 0 && (oc == ' ' || oc == '\t' || oc == ';' || oc == '|' || oc == '&' || oc == '<' || oc == '>') {
						break
					}
					if oc == '(' {
						depth++
					} else if oc == ')' && depth > 0 {
						depth--
					}
					operand = append(operand, oc)
					j++
				}
				if bytes.IndexByte(operand, '(') >= 0 {
					blanked := bytes.Map(func(r rune) rune {
						if r == '(' || r == ')' {
							return ' '
						}
						return r
					}, operand)
					segments = append(segments, string(blanked))
				}
			}
			seg = append(seg, ' ')
			i = j - 1
			continue
		}
		if c == ';' || c == '|' || c == '&' {
			flush()
			continue
		}
		seg = append(seg, c)
	}
	segments = append(segments, string(seg))
	return segments
}
