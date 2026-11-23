package commandguard

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/muehz/agentics/hooks/go/internal/hookspec/hookspectest"
	"github.com/muehz/agentics/hooks/go/internal/hosttest"
	"github.com/muehz/agentics/hooks/go/internal/shellwords"
	"github.com/muehz/agentics/hooks/go/internal/toolcall"
)

// readingsLogRoot is the log root of the events the readings tests build. No reading writes a log, so the directory is
// never created.
const readingsLogRoot = "/tmp/readings-log-root"

// readingsCase is one tool call the readings are read over: a table case, the script files its request carries, and
// whether its inspection runs under a context that is already cancelled.
type readingsCase struct {
	tableCase
	scripts   hookspectest.Scripts
	cancelled bool
}

// preparedCall holds what Run hands a reading for one tool call.
type preparedCall struct {
	ctx        context.Context
	source     scanSource
	fields     normalizedFields
	inspection toolcall.Inspection
}

// prepareCall builds c's event and prepares the scan source, the fields, and the inspection with the calls Run makes.
// ok is false when the contract or the field read refuses the input, which Run denies before any reading.
func prepareCall(t *testing.T, c readingsCase) (preparedCall, bool) {
	t.Helper()
	hosttest.ScrubGuardEnv(t)
	for k, v := range c.env {
		t.Setenv(k, v)
	}
	event := buildEventIn(t, c.workingDir(), c.toolName(), c.toolInput(t), readingsLogRoot)
	read, err := event.Read()
	if err != nil {
		return preparedCall{}, false
	}
	fields, err := extractFields(read)
	if err != nil {
		return preparedCall{}, false
	}
	ctx := context.Background()
	if c.cancelled {
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		ctx = cancelled
	}
	var reader shellwords.ScriptReader
	if c.scripts != nil {
		reader = c.scripts
	}
	return preparedCall{
		ctx:        ctx,
		source:     newScanSource(fields),
		fields:     fields,
		inspection: fields.call.Inspect(ctx, reader),
	}, true
}

// readingsMarker is body text no rule matches, so a reading that keeps or drops it is the only thing a test observes.
const readingsMarker = "readings-marker-body"

// newReadingsFor prepares c as Run does and returns the new reading of it with the prepared call.
func newReadingsFor(t *testing.T, c readingsCase) (readings, preparedCall) {
	t.Helper()
	p, ok := prepareCall(t, c)
	if !ok {
		t.Fatalf("%s: the contract refused the input", c.name)
	}
	return newReadings(p.ctx, p.source, p.fields, p.inspection), p
}

// bankHolds reports whether any bank text holds s.
func bankHolds(rd readings, s string) bool {
	for _, text := range rd.texts {
		if strings.Contains(text, s) {
			return true
		}
	}
	return false
}

// TestNewReadingsCatTeeStripNeedsWholeCommandProof: the bank loses a cat or tee body only when every invocation of the
// command is proven inert. One unknown invocation keeps the body, in both the quoted form the data view blanks and the
// unquoted form only the strip removes.
func TestNewReadingsCatTeeStripNeedsWholeCommandProof(t *testing.T) {
	for _, tc := range []struct {
		name, command string
		kept          bool
	}{
		{"quoted cat, all inert", "cat > draft.md <<'EOF'\n" + readingsMarker + "\nEOF\n", false},
		{"quoted tee, all inert", "tee draft.md <<'EOF'\n" + readingsMarker + "\nEOF\n", false},
		{"unquoted cat, all inert", "cat > draft.md <<EOF\n" + readingsMarker + "\nEOF\n", false},
		{"quoted cat, one unknown", "cat > draft.md <<'EOF'\n" + readingsMarker + "\nEOF\nunknown-tool x\n", true},
		{"quoted tee, one unknown", "tee draft.md <<'EOF'\n" + readingsMarker + "\nEOF\nunknown-tool x\n", true},
		{"unquoted cat, one unknown", "cat > draft.md <<EOF\n" + readingsMarker + "\nEOF\nunknown-tool x\n", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rd, _ := newReadingsFor(t, readingsCase{tableCase: tableCase{name: tc.name, command: tc.command}})
			if got := bankHolds(rd, readingsMarker); got != tc.kept {
				t.Fatalf("bank holds the body = %v, want %v: %q", got, tc.kept, rd.texts)
			}
		})
	}
}

// TestNewReadingsGhKeepsCatTeeBodies: the gh reading keeps cat and tee bodies, which a later invocation can run, and
// loses a commit-message heredoc, which nothing runs.
func TestNewReadingsGhKeepsCatTeeBodies(t *testing.T) {
	const commitMarker, catMarker, teeMarker = "readings-commit-body", "readings-cat-body", "readings-tee-body"
	command := "git commit -F - <<'A'\n" + commitMarker + "\nA\n" +
		"cat > p.sh <<'B'\n" + catMarker + "\nB\n" +
		"tee q.sh <<'C'\n" + teeMarker + "\nC\n" +
		"bash p.sh"
	rd, _ := newReadingsFor(t, readingsCase{tableCase: tableCase{name: "commit beside cat and tee", command: command}})
	if rd.dataProven {
		t.Fatal("the command was proven inert; the case no longer reaches the gh strip")
	}
	if !strings.Contains(rd.gh.command, catMarker) || !strings.Contains(rd.gh.command, teeMarker) {
		t.Fatalf("the gh reading lost a cat or tee body: %q", rd.gh.command)
	}
	if strings.Contains(rd.gh.command, commitMarker) {
		t.Fatalf("the gh reading kept the commit message: %q", rd.gh.command)
	}
}

// TestNewReadingsProvenDataViewFeedsBoth: a proven data view, the Bash one the guard proves and the PowerShell one the
// inspection proves, is what both the bank and the gh rules read.
func TestNewReadingsProvenDataViewFeedsBoth(t *testing.T) {
	for _, tc := range []struct{ name, tool, command string }{
		{"bash", "Bash", "cat > draft.md <<'EOF'\n" + readingsMarker + "\nEOF\n"},
		{"powershell", "PowerShell", "@'\n" + readingsMarker + "\n'@ | Set-Content -LiteralPath draft.txt"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rd, p := newReadingsFor(t, readingsCase{tableCase: tableCase{name: tc.name, tool: tc.tool, command: tc.command}})
			if !rd.dataProven {
				t.Fatal("the data view was not proven")
			}
			if strings.Contains(rd.gh.command, readingsMarker) || bankHolds(rd, readingsMarker) {
				t.Fatalf("a reading kept the body: gh %q, bank %q", rd.gh.command, rd.texts)
			}
			want, err := encodeCompact(map[string]string{"command": rd.gh.command})
			if err != nil {
				t.Fatal(err)
			}
			if rd.scanBody != p.fields.toolName+" "+want {
				t.Fatalf("the bank did not read the gh view: scan body %q, gh %q", rd.scanBody, rd.gh.command)
			}
			if tc.tool == "PowerShell" && rd.gh.command != p.inspection.DataView {
				t.Fatalf("the gh view %q is not the inspection's %q", rd.gh.command, p.inspection.DataView)
			}
		})
	}
}

// TestNewReadingsNonShellToolGetsNoGhLines: a tool that is not a shell gets no promoted command even when its input
// carries one, so the gh rules find no invocation in it.
func TestNewReadingsNonShellToolGetsNoGhLines(t *testing.T) {
	create := "gh pr " + "create --title x"
	rd, _ := newReadingsFor(t, readingsCase{tableCase: tableCase{
		name: "non-shell", tool: "mcp__shell__run", input: mustEncodeCompact(t, map[string]string{"command": create}),
	}})
	if rd.gh.promotedCommand || rd.gh.command != "" {
		t.Fatalf("gh source = %#v, want no promoted command", rd.gh)
	}
	gh := ghScanner{toolName: "mcp__shell__run", src: rd.gh}
	for _, verb := range []ghVerb{ghPrCreate, ghPrReady, ghStackSubmit, ghStackLink} {
		if got := gh.invocations(verb); got != nil {
			t.Fatalf("gh invocations = %q, want none", got)
		}
	}
}

// TestNewReadingsScriptTextsFollowTypedTexts: each inspected script's text joins the bank after the typed command's
// texts.
func TestNewReadingsScriptTextsFollowTypedTexts(t *testing.T) {
	script := "echo " + readingsMarker + "\n"
	rd, p := newReadingsFor(t, readingsCase{
		tableCase: tableCase{name: "script", command: "bash /ws/t.sh", cwd: t.TempDir()},
		scripts:   hookspectest.Scripts{"/ws/t.sh": script},
	})
	if len(p.inspection.Source.Scripts) != 1 {
		t.Fatalf("inspected %d scripts, want 1", len(p.inspection.Source.Scripts))
	}
	if len(rd.texts) != 2 || rd.texts[0] != rd.scanBody+"\n" || rd.texts[1] != p.inspection.Source.Scripts[0].Text+"\n" {
		t.Fatalf("bank texts = %q, want the scan text then the script text", rd.texts)
	}
	if !strings.Contains(rd.texts[1], readingsMarker) {
		t.Fatalf("the script text %q is not the script", rd.texts[1])
	}
}

// TestNewReadingsLongLine: the new reading of a long proven line stays linear in its words. Only the reading is
// timed, so the build of the event and the inspection do not count against it.
func TestNewReadingsLongLine(t *testing.T) {
	var body strings.Builder
	for body.Len() < 100*1024 {
		body.WriteString(readingsMarker + " line of the draft body\n")
	}
	command := "cat > draft.md <<'EOF'\n" + body.String() + "EOF\ngh issue create --title example --body example\n"
	p, ok := prepareCall(t, readingsCase{tableCase: tableCase{name: "long", command: command}})
	if !ok {
		t.Fatal("the contract refused the input")
	}
	start := time.Now()
	rd := newReadings(p.ctx, p.source, p.fields, p.inspection)
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("newReadings took %v on a %d-byte line, want under 2s", elapsed, len(command))
	}
	if !rd.dataProven {
		t.Fatal("the long line was not proven inert")
	}
	if bankHolds(rd, readingsMarker) {
		t.Fatal("the bank kept the proven body")
	}
}
