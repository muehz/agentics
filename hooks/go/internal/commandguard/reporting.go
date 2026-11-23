package commandguard

import (
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/muehz/agentics/hooks/go/internal/contract"
	"github.com/muehz/agentics/hooks/go/internal/guardmode"
	"github.com/muehz/agentics/hooks/go/internal/hookspec"
	"github.com/muehz/agentics/hooks/go/internal/hookutil"
	"github.com/muehz/agentics/hooks/go/internal/settings"
	"github.com/muehz/agentics/hooks/go/internal/shellwords"
)

// threat is one detection: the rule that fired, the text it matched, and the
// suggestion shown for it. The category and severity are the rule's.
type threat struct {
	rule       rule
	match      string
	suggestion string
}

// reportFunc records one threat. The rule families — the gh verbs, the MCP
// gates, the git checks, and the generic bank — all report through it.
type reportFunc func(r rule, match, suggestion string)

var errLog = errors.New("command-guardian could not write its log")

// guardLog owns where guard.log lives and how a record is appended; the
// record schema and the response to a failed append stay in this package.
var guardLog = hookutil.GuardLog{Guard: "command-guardian", EnvDir: "COMMAND_GUARD_LOG_DIR"}

type threatReport struct {
	diag                             io.Writer
	mode                             guardmode.Mode
	warning                          string // the unusable-mode warning, or ""
	fields                           normalizedFields
	redactor                         *shellwords.Redactor
	scripts                          []shellwords.ScriptRecord
	threats, blocking, fixedBlocking []threat
	blockingKeys, adviceKeys         []settings.Key
	source                           *settings.Source
	resolved                         *settings.Result
}

// newThreatReport builds a report that reads workflow settings through source;
// a nil source reads the settings files.
func newThreatReport(diag io.Writer, mode guardmode.Mode, warning string, fields normalizedFields,
	redactor *shellwords.Redactor, scripts []shellwords.ScriptRecord, source *settings.Source) *threatReport {
	return &threatReport{diag: diag, mode: mode, warning: warning, fields: fields, redactor: redactor, scripts: scripts, source: source}
}

// reportMessage leads message with the unusable-mode warning when there is
// one, so a host that shows only the reported reason still names the value.
func reportMessage(warning, message string) string {
	if warning == "" {
		return message
	}
	return warning + "\n\n" + message
}

// resolve returns the workflow settings for the event working directory, once
// per report. Run reaches it only after a workflow-policy detection, so an
// event that matches no policy rule never reads a settings file.
func (r *threatReport) resolve() settings.Result {
	if r.resolved == nil {
		res := r.source.Resolve(r.fields.cwd)
		r.resolved = &res
	}
	return *r.resolved
}

func (r *threatReport) record(rl rule, match, suggestion, location string) {
	// The rule names its setting, so the script location below only decorates
	// the suggestion text.
	key, governed := rl.policy, rl.governed
	if location != "" {
		suggestion += " (in " + location + ")"
	}
	if governed {
		if !r.resolve().Policy.Enabled(key) {
			return
		}
		if rl.severity != severityInfo {
			r.blockingKeys = append(r.blockingKeys, key)
		} else {
			r.adviceKeys = append(r.adviceKeys, key)
		}
	}
	t := threat{rl, match, suggestion}
	r.threats = append(r.threats, t)
	if rl.severity != severityInfo {
		r.blocking = append(r.blocking, t)
		if !governed {
			r.fixedBlocking = append(r.fixedBlocking, t)
		}
	}
}

func (r *threatReport) report(rl rule, match, suggestion string) {
	shown, label, _ := r.redactor.Redact(match)
	r.record(rl, shown, suggestion, label)
}

func (r *threatReport) reportLabeled(rl rule, label, suggestion string) {
	// A trusted script label must not be judged as source text again.
	r.record(rl, shellwords.RedactedMatch(label), suggestion, "")
}

func (r *threatReport) outcome(configurationBearing func() bool) hookspec.Outcome {
	if len(r.threats) == 0 {
		return contract.Pass()
	}
	if configurationBearing != nil && configurationBearing() {
		for i := range r.threats {
			r.threats[i].match = "configuration-bearing command (match redacted)"
		}
	}
	leading, note := r.blocking, ""
	if len(r.fixedBlocking) > 0 {
		leading = r.fixedBlocking
	} else if len(r.blocking) > 0 {
		note = settings.Note(r.resolve(), r.blockingKeys)
	} else if len(r.adviceKeys) > 0 {
		note = settings.Note(r.resolve(), r.adviceKeys)
	}
	writeThreatTable(r.diag, r.fields.toolName, r.threats)
	suffix := ""
	if note != "" {
		suffix = "\n\n" + note
	}
	if err := writeGuardLog(r.fields.logRoot, r.mode.String(), r.fields.toolName, r.threats, r.scripts); err != nil {
		fmt.Fprintf(r.diag, "%v\n", err)
		// A log-write failure must not turn a block-mode deny into a pass.
		if r.mode.Blocks() && len(r.blocking) > 0 {
			lead := mostSevere(leading)
			return contract.Deny(reportMessage(r.warning, fmt.Sprintf("%d blocking threat(s) detected: %s - %s; guard.log was not written: %s%s",
				len(r.blocking), lead.rule.category, lead.suggestion, err.Error(), suffix)))
		}
		// An error outcome keeps its fixed text; the warning is on the
		// diagnostic stream, which providers keep for error outcomes.
		return contract.Error(errLog.Error())
	}
	if r.mode.Blocks() && len(r.blocking) > 0 {
		lead := mostSevere(leading)
		return contract.Deny(reportMessage(r.warning, fmt.Sprintf("%d blocking threat(s) detected: %s - %s%s",
			len(r.blocking), lead.rule.category, lead.suggestion, suffix)))
	}
	lead := mostSevere(r.threats)
	return contract.Context(reportMessage(r.warning, fmt.Sprintf("%d threat(s) detected: %s - %s%s",
		len(r.threats), lead.rule.category, lead.suggestion, suffix)))
}

// writeThreatTable writes the diagnostic table: one row per threat, the match
// shortened to 38 characters, columns padded by character so a non-ASCII
// match keeps the table aligned.
func writeThreatTable(w io.Writer, toolName string, threats []threat) {
	fmt.Fprintf(w, "\n🛡️  Tool Guardian: %d threat(s) detected in '%s' invocation\n\n", len(threats), toolName)
	fmt.Fprintf(w, "  %-24s %-10s %-40s %s\n", "CATEGORY", "SEVERITY", "MATCH", "SUGGESTION")
	fmt.Fprintf(w, "  %-24s %-10s %-40s %s\n", "--------", "--------", "-----", "----------")
	for _, t := range threats {
		display := t.match
		if r := []rune(display); len(r) > 38 {
			display = string(r[:35]) + "..."
		}
		fmt.Fprintf(w, "  %-24s %-10s %-40s %s\n", t.rule.category, t.rule.severity, display, t.suggestion)
	}
	fmt.Fprintf(w, "\n")
}

// guardLogRecord is one guard.log line (hooks/README.md, Command-guardian
// log format). Field order is declaration order.
type guardLogRecord struct {
	Timestamp   string           `json:"timestamp"`
	Event       string           `json:"event"`
	Mode        string           `json:"mode"`
	Tool        string           `json:"tool"`
	ThreatCount int              `json:"threat_count"`
	Threats     []guardLogThreat `json:"threats"`
	Scripts     []guardLogScript `json:"scripts,omitempty"`
}

// guardLogScript identifies one inspected script by its raw path, SHA-256 and
// size. The record is machine evidence, so the JSON encoder's escaping is the
// rendering; the script's text and the call's arguments are never logged.
type guardLogScript struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Bytes  int    `json:"bytes"`
}

// guardLogThreat is one entry of a record's threats array.
type guardLogThreat struct {
	Category   string `json:"category"`
	Severity   string `json:"severity"`
	Match      string `json:"match"`
	Suggestion string `json:"suggestion"`
}

// writeGuardLog appends one threats_detected record to guard.log under
// logRoot, creating the directory on the way. It is the only write the guard
// makes, and it happens only when a threat was found: a clean invocation
// touches nothing. The caller folds an error into the deny reason in block
// mode.
func writeGuardLog(logRoot, mode, toolName string, threats []threat, scripts []shellwords.ScriptRecord) error {
	record := guardLogRecord{
		Timestamp:   time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		Event:       "threats_detected",
		Mode:        mode,
		Tool:        toolName,
		ThreatCount: len(threats),
		Threats:     make([]guardLogThreat, 0, len(threats)),
	}
	for _, t := range threats {
		record.Threats = append(record.Threats, guardLogThreat{t.rule.category, t.rule.severity, t.match, t.suggestion})
	}
	for _, rec := range scripts {
		record.Scripts = append(record.Scripts, guardLogScript{rec.Path, rec.SHA256, rec.Bytes})
	}
	return guardLog.Append(logRoot, record)
}
