package main

// The latency baseline: how long each hook takes end to end, measured
// against a committed number so a regression is visible during a rewrite.
//
// It is advisory and off by default — wall-clock numbers vary by machine, so
// this never runs under `go test ./...` or in CI. Enable it with -latency:
//
//	go test ./cmd/hook -run '^TestLatencyBaseline$' -count=1 -v -latency
//
// which benchmarks the probes in latencyProbes through run() and
// fails when any hook exceeds twice its recorded ns/op. Re-record the
// baseline deliberately, from the machine the numbers describe, with
//
//	go test ./cmd/hook -run '^TestLatencyBaseline$' -count=1 -latency -latency-update
//
// bench-baseline.txt is in `go test -bench` output format — one
// `Benchmark<Hook> <iterations> <ns> ns/op` line per hook — preceded by a
// `#` comment naming the machine that recorded it. Lines that do not begin
// with "Benchmark" are ignored when reading.

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/muehz/agentics/hooks/go/internal/hookcost"
	"github.com/muehz/agentics/hooks/go/internal/hosttest"
	"github.com/muehz/agentics/hooks/go/internal/surface"
)

var (
	latency       = flag.Bool("latency", false, "run the latency baseline comparison (advisory; off by default)")
	latencyUpdate = flag.Bool("latency-update", false, "re-record bench-baseline.txt from this machine (implies -latency)")
)

// latencyBaselinePath is relative to this package; the file lives at the
// module root so it sits beside go.mod and the Makefile that drive it.
const latencyBaselinePath = "../../bench-baseline.txt"

// latencyBudget is the regression multiplier: a hook that takes more than
// this many times its recorded ns/op fails.
const latencyBudget = 2.0

// latencyProbe names one benchmarked path. entry names a corpus entry;
// synthetic, when set, builds an entry in code instead, for inputs too large
// or too specific to record in the corpus.
type latencyProbe struct {
	name      string
	entry     string
	synthetic func() corpusEntry
	// scripted builds an entry whose command calls a script it writes into dir, so the guards read it from disk.
	scripted func(tb testing.TB, dir string) corpusEntry
}

// latencyProbes are the paths benchmarked, all on the claude-code surface.
// Three are corpus entries, one per hook: a benign command scan, a
// credential-read deny and a benign release command path. Three are synthetic
// allows on the hottest scan paths: a 64 KB Write through the secret
// detectors, a seven-segment Bash command, and a non-ASCII Bash command that
// exercises the fold path. Two are script calls that read a 26 KiB script from disk, through one guard and through
// the grouped pre-tool event.
var latencyProbes = []latencyProbe{
	{name: "BenchmarkCommandGuardian", entry: "command-guardian/pass-benign"},
	{name: "BenchmarkReleaseGuardian", entry: "release-guardian/pass-benign"},
	{name: "BenchmarkSecretsGuardian", entry: "secrets-guardian/deny-local-credential-read"},
	{name: "BenchmarkSecretsGuardianWrite64KB", synthetic: func() corpusEntry {
		return syntheticPreToolUse("secrets-guardian", "Write", map[string]any{
			"file_path": "/tmp/corpus/bench.go",
			"content":   benignGoSource(64 << 10),
		})
	}},
	{name: "BenchmarkCommandGuardianSevenSegments", synthetic: func() corpusEntry {
		return syntheticPreToolUse("command-guardian", "Bash", map[string]any{
			"command": "git status --short && go vet ./... && go test ./... | tee out.txt; ls -la; echo done && git diff --stat",
		})
	}},
	{name: "BenchmarkCommandGuardianNonASCII", synthetic: func() corpusEntry {
		return syntheticPreToolUse("command-guardian", "Bash", map[string]any{
			"command": "echo 'naïve café — ſtatus Kelvin' && ls ./données",
		})
	}},
	// A script call: the guard reads a 26 KiB benign script from disk and judges its content. The grouped probe sends
	// the same event through the pre-tool dispatcher, which runs all three guards on one script snapshot.
	{name: "BenchmarkScriptCall", scripted: func(tb testing.TB, dir string) corpusEntry {
		return scriptCallEntry(tb, "command-guardian", dir, 26<<10)
	}},
	{name: "BenchmarkScriptCallGrouped", scripted: func(tb testing.TB, dir string) corpusEntry {
		return scriptCallEntry(tb, "pre-tool", dir, 26<<10)
	}},
}

// syntheticPreToolUse builds a claude-code PreToolUse entry for hook.
func syntheticPreToolUse(hook, tool string, input map[string]any) corpusEntry {
	return syntheticPreToolUseIn(hook, "/tmp/corpus", tool, input)
}

// syntheticPreToolUseIn is syntheticPreToolUse with the event's working directory named.
func syntheticPreToolUseIn(hook, cwd, tool string, input map[string]any) corpusEntry {
	stdin := hosttest.Stdin(surface.ClaudeCode, hosttest.Call{Tool: tool, Input: input, Cwd: cwd})
	argv := []string{"--mode", "block"}
	if hook == "pre-tool" {
		argv = legalArgs("claude-code", hook)
	}
	return corpusEntry{Name: "synthetic/" + hook, Hook: hook, Argv: argv, Stdin: stdin, Only: []string{"claude-code"}}
}

// benignShellScript returns at least n bytes of varied shell that no guard refuses: assignments, tests, loops,
// substitutions, a function with an unshadowed name, a quoted here-document and comments.
func benignShellScript(n int) string {
	block := []string{
		"#!/usr/bin/env bash",
		"set -eu",
		"# note: nothing here changes the environment",
		"count=0",
		"for item in alpha beta gamma; do",
		"  count=$((count + 1))",
		"  echo \"item $item of $count\"",
		"done",
		"if [ -n \"${name:-}\" ] && [ \"$count\" -gt 2 ]; then",
		"  printf '%s\\n' \"$name\" | sed -e 's/a/b/' | grep -c b",
		"fi",
		"report() {",
		"  local label=$1",
		"  echo \"report: $label\"",
		"}",
		"report summary",
		"value=$(echo hello | tr a-z A-Z)",
		"if [[ \"$value\" == HELLO ]]; then echo ok; fi",
		"cat <<'EOF'",
		"usage: tool [--flag] file",
		"EOF",
		"git status --short",
		"ls -la ./docs | head -n 3",
		"echo done # trailing comment",
		"",
	}
	var b strings.Builder
	for b.Len() < n {
		b.WriteString(strings.Join(block, "\n"))
		b.WriteByte('\n')
	}
	return b.String()
}

// scriptCallEntry writes a benign script of about size bytes into dir and returns the event that calls it from there.
func scriptCallEntry(tb testing.TB, hook, dir string, size int) corpusEntry {
	tb.Helper()
	if err := os.WriteFile(filepath.Join(dir, "s.sh"), []byte(benignShellScript(size)), 0o755); err != nil {
		tb.Fatal(err)
	}
	return syntheticPreToolUseIn(hook, dir, "Bash", map[string]any{"command": "cd " + dir + " && bash ./s.sh"})
}

// benignGoSource returns at least n bytes of Go source no detector matches.
func benignGoSource(n int) string {
	const line = "func add(a, b int) int { return a + b } // arithmetic helper\n"
	return strings.Repeat(line, n/len(line)+1)
}

// latencyProbeInput resolves a probe to run()'s argv and stdin. A synthetic
// probe must be an allow: it measures the hot path, not a deny.
func latencyProbeInput(tb testing.TB, table corpusTable, entries map[string]corpusEntry, probe latencyProbe) ([]string, string) {
	tb.Helper()
	entry, ok := entries[probe.entry]
	if probe.synthetic != nil {
		entry, ok = probe.synthetic(), true
	}
	if probe.scripted != nil {
		entry, ok = probe.scripted(tb, filepath.ToSlash(tb.TempDir())), true
	}
	if !ok {
		tb.Fatalf("%s: corpus entry %q is not in %s", probe.name, probe.entry, corpusEntriesPath)
	}
	argv, stdin, _ := prepareCorpusEntry(tb, table, "claude-code", entry)
	if probe.synthetic != nil || probe.scripted != nil {
		var stdout, stderr bytes.Buffer
		if code := run(argv, strings.NewReader(stdin), &stdout, &stderr); code != 0 || stdout.String() != "" || stderr.String() != "" {
			tb.Fatalf("%s: synthetic probe did not allow (want exit=0, empty stdout/stderr like command-guardian/pass-benign); got exit=%d stdout=%q stderr=%q",
				probe.name, code, stdout.String(), stderr.String())
		}
	}
	return argv, stdin
}

func TestLatencyBaseline(t *testing.T) {
	if !*latency && !*latencyUpdate {
		t.Skip("latency baseline is advisory; enable with -latency (see the file comment)")
	}
	table := loadCorpusTable(t)
	entries := map[string]corpusEntry{}
	for _, e := range table.Entries {
		entries[e.Name] = e
	}

	measured := map[string]int64{}
	for _, probe := range latencyProbes {
		argv, stdin := latencyProbeInput(t, table, entries, probe)
		p := hookcost.Probe{Name: probe.name, Argv: argv, Stdin: stdin}
		route := hookcost.InProcess(run)
		if err := route.Prepare(p); err != nil {
			t.Fatalf("%s: %v", probe.name, err)
		}
		var invokeErr error
		result := testing.Benchmark(func(b *testing.B) {
			if invokeErr = invokeLoop(b, route, p); invokeErr != nil {
				b.FailNow()
			}
		})
		if invokeErr != nil {
			t.Fatalf("%s: hook output changed: %v", probe.name, invokeErr)
		}
		measured[probe.name] = result.NsPerOp()
		t.Logf("%s\t%d\t%d ns/op", probe.name, result.N, result.NsPerOp())
	}

	if *latencyUpdate {
		// Recorded in go test -bench format under a comment naming the machine
		// and date.
		header := fmt.Sprintf("# hook latency baseline, recorded %s on %s/%s with %s; compare with `make -C hooks/go bench`\n",
			time.Now().UTC().Format("2006-01-02"), runtime.GOOS, runtime.GOARCH, runtime.Version())
		names := make([]string, 0, len(latencyProbes))
		for _, probe := range latencyProbes {
			names = append(names, probe.name)
		}
		if err := hookcost.WriteBaseline(latencyBaselinePath, header, names, measured); err != nil {
			t.Fatal(err)
		}
		t.Logf("recorded %d baselines to %s", len(measured), latencyBaselinePath)
		return
	}

	baseline, err := hookcost.ReadBaseline(latencyBaselinePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, probe := range latencyProbes {
		want, ok := baseline[probe.name]
		if !ok {
			t.Errorf("%s: no baseline line in %s — run with -latency-update", probe.name, latencyBaselinePath)
			continue
		}
		got := measured[probe.name]
		ratio := float64(got) / float64(want)
		if ratio > latencyBudget {
			t.Errorf("%s: %d ns/op is %.2fx the baseline %d ns/op (budget %.1fx)", probe.name, got, ratio, want, latencyBudget)
		}
	}
}

// TestLatencyBaselineFileNamesEveryProbe pins the committed file without
// measuring. It runs without -latency, so a probe added or renamed without a
// re-record fails here rather than only under the advisory run.
func TestLatencyBaselineFileNamesEveryProbe(t *testing.T) {
	baseline, err := hookcost.ReadBaseline(latencyBaselinePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, probe := range latencyProbes {
		if _, ok := baseline[probe.name]; !ok {
			t.Errorf("%s: no baseline line in %s — run with -latency-update", probe.name, latencyBaselinePath)
		}
	}
}

// invokeLoop times b.N calls of route's Invoke on p, which fails when the
// hook's result differs from the route's reference. The caller prepares the
// route.
func invokeLoop(b *testing.B, route hookcost.Route, p hookcost.Probe) error {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := route.Invoke(p); err != nil {
			return err
		}
	}
	return nil
}

// benchmarkInvoke benchmarks the hook on p in process: its first result is the
// reference, and every iteration must equal it.
func benchmarkInvoke(b *testing.B, p hookcost.Probe) {
	b.Helper()
	route := hookcost.InProcess(run)
	if err := route.Prepare(p); err != nil {
		b.Fatal(err)
	}
	if err := invokeLoop(b, route, p); err != nil {
		b.Fatalf("hook output changed: %v", err)
	}
}

func benchmarkLatencyProbe(b *testing.B, name string) {
	table := loadCorpusTable(b)
	entries := map[string]corpusEntry{}
	for _, e := range table.Entries {
		entries[e.Name] = e
	}
	for _, probe := range latencyProbes {
		if probe.name != name {
			continue
		}
		argv, stdin := latencyProbeInput(b, table, entries, probe)
		benchmarkInvoke(b, hookcost.Probe{Name: probe.name, Argv: argv, Stdin: stdin})
		return
	}
	b.Fatalf("no latency probe named %s", name)
}

// BenchmarkScriptCall measures one guard reading and judging a 26 KiB script call.
func BenchmarkScriptCall(b *testing.B) { benchmarkLatencyProbe(b, "BenchmarkScriptCall") }

// BenchmarkScriptCallGrouped measures the grouped pre-tool event on the same call: three guards, one script snapshot.
func BenchmarkScriptCallGrouped(b *testing.B) { benchmarkLatencyProbe(b, "BenchmarkScriptCallGrouped") }

// runResult is one in-process call of the hook.
func runResult(argv []string, stdin string) hookcost.Result {
	var stdout, stderr strings.Builder
	code := run(argv, strings.NewReader(stdin), &stdout, &stderr)
	return hookcost.Result{Code: code, Stdout: stdout.String(), Stderr: stderr.String()}
}

// medianRunTime runs hook on the Bash command, then times it runs times, each with a fresh request and so a fresh
// script snapshot, and returns the median wall-clock time. Before any timing it makes untimed calls to record the
// reference result, which must be Silent (a pass with no output), to prepare the in-process route, and to warm the
// caches. Every timed run must repeat the reference result.
func medianRunTime(t *testing.T, hook, dir, command string, runs int) time.Duration {
	t.Helper()
	argv := append([]string{"claude-code", hook}, legalArgs("claude-code", hook)...)
	stdin := string(hosttest.Stdin(surface.ClaudeCode, hosttest.Call{Tool: "Bash", Input: map[string]any{"command": command}, Cwd: dir}))
	reference := runResult(argv, stdin)
	if !hosttest.Read(surface.ClaudeCode, reference.Code, []byte(reference.Stdout), []byte(reference.Stderr)).Silent() {
		t.Fatalf("%s did not pass %q: exit %d, stdout %d bytes, stderr %d bytes", hook, command, reference.Code, len(reference.Stdout), len(reference.Stderr))
	}
	times, err := hookcost.Samples(hookcost.InProcess(run), hookcost.Probe{Name: hook, Argv: argv, Stdin: stdin}, 1, runs)
	if err != nil {
		t.Fatalf("%s on %q: %v", hook, command, err)
	}
	sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })
	return times[len(times)/2]
}

const (
	// The target for one guard on a script call is 5 ms. It holds for scripts of about 16 KiB (4 to 5 ms measured) and
	// is missed for 64 KiB, which is reported to the user rather than hidden: reaching it would take one shared parse
	// per event. The bounds here are measured, not the target. On the development host (an i9-12900KS, Windows, Go
	// 1.27) a 64 KiB call takes 14 to 22 ms per guard in process, down from 21 to 26 ms before the construct text
	// scan was gated by substring tests, so the absolute bound is about twice the measured worst median. It fails a
	// change that doubles the cost; the linearity ratio below fails one that makes the cost superlinear.
	scriptLatency64KiBBound = 45 * time.Millisecond
	// A script four times as long may take at most this many times as long: the cost per byte stays flat.
	scriptLatencyLinearity = 6.0
	// The repository's own 26 KiB helper through the grouped event, which runs the three guards on one snapshot
	// (about 9 ms measured).
	scriptLatencyHelperBound = 30 * time.Millisecond
	scriptLatencyRuns        = 20
)

// TestScriptCallLatencyBudget holds the cost of judging a script call to measured numbers: each guard reading a 64 KiB
// benign script stays under an absolute bound, the cost grows linearly with the script, and the repository's helper
// through the grouped event stays under its own bound. Advisory like the baseline: enable it with -latency.
func TestScriptCallLatencyBudget(t *testing.T) {
	if !*latency && !*latencyUpdate {
		t.Skip("latency budget is advisory; enable with -latency (see the file comment)")
	}
	pinScriptGuardEnv(t)
	dir := filepath.ToSlash(t.TempDir())
	for name, size := range map[string]int{"s64.sh": 64 << 10, "s16.sh": 16 << 10} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(benignShellScript(size)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, guard := range []string{"command-guardian", "release-guardian", "secrets-guardian"} {
		large := medianRunTime(t, guard, dir, "cd "+dir+" && bash ./s64.sh", scriptLatencyRuns)
		small := medianRunTime(t, guard, dir, "cd "+dir+" && bash ./s16.sh", scriptLatencyRuns)
		ratio := float64(large) / float64(small)
		t.Logf("%s: 64 KiB median %v, 16 KiB median %v, ratio %.2f (target %v per guard)", guard, large, small, ratio, 5*time.Millisecond)
		if large > scriptLatency64KiBBound {
			t.Errorf("%s: a 64 KiB script call took %v, over the %v bound", guard, large, scriptLatency64KiBBound)
		}
		if ratio > scriptLatencyLinearity {
			t.Errorf("%s: 64 KiB took %.1fx the 16 KiB time (%v and %v), over %.0fx", guard, ratio, large, small, scriptLatencyLinearity)
		}
	}
	helper := medianRunTime(t, "pre-tool", filepath.ToSlash(hosttest.Checkout(t)), "bash hooks/go/internal/shellwords/testdata/flow.sh status", scriptLatencyRuns)
	t.Logf("grouped event on hooks/go/internal/shellwords/testdata/flow.sh: median %v", helper)
	if helper > scriptLatencyHelperBound {
		t.Errorf("the grouped event on the repository helper took %v, over the %v bound", helper, scriptLatencyHelperBound)
	}
}
