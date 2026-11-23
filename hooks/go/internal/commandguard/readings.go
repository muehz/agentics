package commandguard

import (
	"context"
	"strings"

	"github.com/muehz/agentics/hooks/go/internal/shellwords"
	"github.com/muehz/agentics/hooks/go/internal/toolcall"
)

// readings is the prepared text each rule family reads for one tool call.
type readings struct {
	// texts are the threat bank's scan texts in the order the bank reads them:
	// the scan text, then its continuation-joined form when that differs, then
	// each inspected script's text.
	texts []string

	// scanBody is the text the redactor is built from, before the trailing
	// newline the bank texts carry.
	scanBody string

	// redactor blanks the typed command and scan body out of a reported match.
	redactor *shellwords.Redactor

	// gh is the scan source the gh rules read: the command, or the fallback
	// lines, and the segmentation mode.
	gh scanSource

	// dataProven reports that the data view, not the raw text, fed the scan.
	dataProven bool

	// invocations and dialect are the policy invocations and the shell dialect
	// the parsed rules read.
	invocations []shellwords.Invocation
	dialect     shellwords.Dialect
}

// newReadings prepares the text each rule family reads from the inspection.
// The whole-command inert proof runs at most once: its one result decides the
// Bash data view and the cat and tee strip. The PowerShell data view and its
// proof are the inspection's. The context is unused: the inspection has
// already read everything that needed it.
func newReadings(_ context.Context, source scanSource, fields normalizedFields, in toolcall.Inspection) readings {
	combined := fields.toolName + " " + source.text
	dialect := fields.call.Execution().Dialect
	parsed, policyParsed := in.Source, in.Policy
	invocations := policyParsed.Invocations

	// The proof is read whether or not the call carries a command, so the
	// cat and tee strip still runs over a Bash call that has none.
	bashData := bashDataDialect(fields.call)
	var view shellwords.HeredocView
	proven := false
	if bashData {
		view = shellwords.QuotedHeredocView(fields.command)
		proven = view.Complete && view.Reliable && fullInertInvocationProof(view.Text, parsed, emptyGitPager(fields.call))
	}

	// Both text scanners consume the same proven data view. PowerShell's
	// ordinary quote parser can invent operations inside a here-string, so
	// only the independently proven whole-call sink permits a policy reparse,
	// which the shared inspection makes. The original source and parse remain
	// available as evidence.
	dataCommand, dataProven := in.DataView, in.DataProven
	if dialect != shellwords.PowerShell {
		dataCommand, dataProven = bashDataView(fields, parsed, view, proven)
	}
	scanBody := combined
	if dataProven {
		if encoded, err := encodeCompact(map[string]string{"command": dataCommand}); err == nil {
			scanBody = fields.toolName + " " + encoded
		}
	} else if bashData {
		if stripped := stripHeredocsFor(combined, fields.command, invocations, proven); stripped != "" {
			scanBody = stripped
		}
	}
	scanText := scanBody + "\n"
	redactor := shellwords.NewRedactor(parsed, fields.command, scanBody)

	// scanLines almost always holds exactly one element: the scan body is
	// the tool name plus a compact JSON document, so every newline inside the
	// data is the two-character escape \n, not a newline byte. The one case
	// that produces a second element is a tool name containing a newline,
	// which the contract admits.
	scanLines := strings.Split(scanBody, "\n")

	// A second scan text with backslash-newline continuations joined, kept
	// separate from the first so the join can never destroy a boundary
	// character the Windows rules depend on.
	texts := []string{scanText}
	if strings.Contains(scanBody, `\`) {
		joinedText := scanText
		if joinedBody := joinScanContinuations(scanBody); joinedBody != "" {
			joinedText = joinedBody + "\n"
		}
		if joinedText != scanText {
			texts = append(texts, joinedText)
		}
	}
	// The text rules see only the typed command, so each inspected script's text
	// is scanned as well; otherwise a rule with no parsed counterpart would never
	// fire on a command that runs from a script.
	for _, script := range parsed.Scripts {
		texts = append(texts, script.Text+"\n")
	}

	gh := scanSource{
		promotedCommand: source.promotedCommand,
		text:            source.text,
		command:         source.command,
		lines:           scanLines,
	}
	if gh.promotedCommand {
		if dataProven {
			gh.command = dataCommand
		} else {
			// The gh rules read the command with only git commit message
			// heredocs stripped; cat/tee bodies stay visible to them because
			// text written to a file can be executed later in the same command.
			ghCommand := gh.command
			if bashData {
				if commitOnly := stripHeredocsFor(combined, fields.command, invocations, false); commitOnly != combined {
					ghCommand = ghCommandFromStripped(fields.toolName, commitOnly, ghCommand)
				}
			}
			gh.command = blankedGhCommand(ghCommand, invocations, dialect)
		}
	}

	return readings{
		texts:       texts,
		scanBody:    scanBody,
		redactor:    redactor,
		gh:          gh,
		dataProven:  dataProven,
		invocations: invocations,
		dialect:     dialect,
	}
}
