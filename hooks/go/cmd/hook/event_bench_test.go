package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/muehz/agentics/hooks/go/internal/hookcost"
	"github.com/muehz/agentics/hooks/go/internal/hookprovenance"
	"github.com/muehz/agentics/hooks/go/internal/hosttest"
	"github.com/muehz/agentics/hooks/go/internal/surface"
)

var (
	hookCost          = flag.Bool("hook-cost", false, "capture hook launch and config event replay costs (opt-in)")
	hookCostSamples   = flag.Int("hook-cost-samples", 100, "warm samples per case, after five warmups")
	hookCostOutput    = flag.String("hook-cost-output", "", "new JSON output path; default is a unique bench-e2e-results run directory")
	hookCostCold      = flag.Bool("hook-cost-cold", false, "one selected first-call capture, without preflight or warmups")
	hookCostCase      = flag.String("hook-cost-case", "", "exact case id to capture, or empty for all warm cases")
	hookCostCondition = flag.String("hook-cost-condition", "", "operator evidence of fresh install/reboot and no previous target invocation")
)

// hookCostBuildTarget returns where a scratch hooks directory keeps the
// binary a test builds. The binary is absent until the build, so a missing
// file is expected here; only a platform with no contract binary fails.
func hookCostBuildTarget(t testing.TB, hooksDir string) string {
	t.Helper()
	path, state := hosttest.CommittedBinaryPath(hooksDir)
	if state == hosttest.OffContract {
		t.Fatalf("no hook binary is committed for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	return path
}

func stageFixtureProvisioning(t testing.TB, binary string) {
	t.Helper()
	root := filepath.Dir(filepath.Dir(filepath.Dir(binary)))
	if err := os.MkdirAll(filepath.Join(root, ".claude-plugin"), 0755); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{".claude-plugin/plugin.json", "hooks/fetch-hook.ps1"} {
		data, err := os.ReadFile(filepath.Join(hosttest.Checkout(t), rel))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, rel), data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	fixtureRuntimeDigest(t, binary)
}

func fixtureRuntimeDigest(t testing.TB, binary string) {
	t.Helper()
	data, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	line := fmt.Sprintf("%x  bin/%s\n", sha256.Sum256(data), filepath.Base(binary))
	if err := os.WriteFile(filepath.Join(filepath.Dir(filepath.Dir(binary)), "SHA256SUMS"), []byte(line), 0644); err != nil {
		t.Fatal(err)
	}
}

// stageHookCostTarget sets up a disposable plugin with the current dispatcher and a native runtime built from source.
func stageHookCostTarget(t testing.TB, source, binary string) hookcost.Target {
	t.Helper()
	binDir := filepath.Dir(binary)
	hooksDir := filepath.Dir(binDir)
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	wrapper, err := os.ReadFile(filepath.Join(source, "run-hook.cmd"))
	if err != nil {
		t.Fatal(err)
	}
	dispatcher := filepath.Join(hooksDir, "run-hook.cmd")
	if err := os.WriteFile(dispatcher, wrapper, 0755); err != nil {
		t.Fatal(err)
	}
	module := filepath.Join(source, "go")
	digest, err := hookprovenance.SourceDigest(module)
	if err != nil {
		t.Fatal(err)
	}
	eventProcessBuild(t, module, binary, "-ldflags=-s -w -X main.sourceDigest=agentics-src-"+digest, "./cmd/hook")
	stageFixtureProvisioning(t, binary)
	return hookcost.Target{HooksDir: hooksDir, Binary: binary, Dispatcher: dispatcher}
}

func TestHookCostLaunchInputs(t *testing.T) {
	source, err := filepath.Abs(filepath.Join(hosttest.Checkout(t), "hooks"))
	if err != nil {
		t.Fatal(err)
	}
	registrations, err := hosttest.Registrations(source)
	if err != nil {
		t.Fatal(err)
	}
	hooksDir := filepath.Join(t.TempDir(), "hooks")
	target := stageHookCostTarget(t, source, hookCostBuildTarget(t, hooksDir))
	fixture, env := hookcost.Prepare(t, target)
	for _, provider := range surface.All() {
		for _, tool := range []string{"Bash", "Read"} {
			selected, configs := hookcost.Selected(t, registrations, provider, tool, runtime.GOOS)
			for i, r := range selected {
				c := configs[i]
				arm := hookCostConfigArm(t, string(provider)+"/"+c.Hook, r, c, tool, target, fixture, env)
				if err := arm.Preflight(); err != nil {
					t.Fatal(err)
				}
				if _, err := arm.Run(); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}

func TestHookCostRootExpansion(t *testing.T) {
	source, err := filepath.Abs(filepath.Join(hosttest.Checkout(t), "hooks"))
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "literal-$name-$(echo harmless)-`tick")
	hooksDir := filepath.Join(root, "hooks")
	target := stageHookCostTarget(t, source, hookCostBuildTarget(t, hooksDir))
	registrations, err := hosttest.Registrations(source)
	if err != nil {
		t.Fatal(err)
	}
	fixture, env := hookcost.Prepare(t, target)
	tested := 0
	for _, provider := range surface.All() {
		selected, configs := hookcost.Selected(t, registrations, provider, "Bash", runtime.GOOS)
		for i, r := range selected {
			c := configs[i]
			tested++
			arm := hookCostConfigArm(t, string(provider), r, c, "Bash", target, fixture, env)
			if err := arm.Preflight(); err != nil {
				t.Fatal(err)
			}
			if _, err := arm.Run(); err != nil {
				t.Fatal(err)
			}
		}
	}
	if tested != 4 {
		t.Fatalf("root expansion exercised %d cases, want 4", tested)
	}
}

// hookCostStdin is the stdin the surface's host sends for a neutral tool
// call in fixture.
func hookCostStdin(provider surface.ID, tool, fixture string) string {
	input := map[string]any{"command": "git status --short"}
	if tool == "Read" {
		input = map[string]any{"file_path": filepath.Join(fixture, "fixture.txt")}
	}
	return string(hosttest.Stdin(provider, hosttest.Call{Tool: tool, Input: input, Cwd: fixture}))
}

// hookCostProbe is the call c registers, fed payload.
func hookCostProbe(c hookcost.Config, payload string) hookcost.Probe {
	return hookcost.Probe{Name: c.Hook, Argv: c.Argv, Stdin: payload, Timeout: time.Duration(c.TimeoutSeconds) * time.Second}
}

// hookCostConfigArm is the arm that replays r's shipped command for a
// neutral tool call, as a host launches it.
func hookCostConfigArm(tb testing.TB, id string, r hosttest.Registration, c hookcost.Config, tool string, target hookcost.Target, fixture string, env []string) hookcost.Arm {
	tb.Helper()
	route, err := hookcost.Configured(r, target, fixture, env, run)
	if err != nil {
		tb.Fatal(err)
	}
	return hookcost.LaunchArm(id, "config-command-replay", route, hookCostProbe(c, hookCostStdin(r.Surface, tool, fixture)), c)
}

// hookCostEventArm is the arm that replays every registration a host runs for
// one tool call in order, then the tool itself, and records each step as a
// span.
func hookCostEventArm(provider surface.ID, tool string, registrations []hosttest.Registration, target hookcost.Target, fixture string, env []string) (hookcost.Arm, error) {
	arm := hookcost.Arm{ID: string(provider) + "/" + tool + "/event-replay", Layer: "sequential-config-tool-call-replay"}
	var children []hookcost.Arm
	for i, c := range hookcost.ConfigsFrom(registrations) {
		route, err := hookcost.Configured(registrations[i], target, fixture, env, run)
		if err != nil {
			return hookcost.Arm{}, err
		}
		probe := hookCostProbe(c, hookCostStdin(provider, tool, fixture))
		children = append(children, hookcost.LaunchArm(c.Event+"/"+c.Hook, "config-command-replay", route, probe, c))
		arm.Commands = append(arm.Commands, route.Command(probe))
	}
	git, err := exec.LookPath("git")
	if err != nil {
		return hookcost.Arm{}, err
	}
	toolCommand := hookcost.Command{Executable: git, Args: []string{"status", "--short"}, TimeoutSeconds: 30}
	if tool == "Bash" {
		arm.Commands = append(arm.Commands, toolCommand)
	}
	var spans []hookcost.Span
	arm.Preflight = func() error {
		for _, child := range children {
			if err := child.Preflight(); err != nil {
				return err
			}
		}
		return nil
	}
	arm.Validate = func() error {
		for _, child := range children {
			if err := child.Validate(); err != nil {
				return err
			}
		}
		return nil
	}
	arm.Run = func() (int64, error) {
		start := time.Now()
		spans = nil
		for _, child := range children {
			begin := time.Since(start).Nanoseconds()
			_, err := child.Run()
			spans = append(spans, hookcost.Span{Name: child.ID, StartNS: begin, EndNS: time.Since(start).Nanoseconds()})
			if err != nil {
				return time.Since(start).Nanoseconds(), err
			}
		}
		begin := time.Since(start).Nanoseconds()
		var err error
		if tool == "Bash" {
			_, _, _, err = hookcost.Exec(toolCommand, fixture, env)
		} else {
			_, err = os.ReadFile(filepath.Join(fixture, "fixture.txt"))
		}
		spans = append(spans, hookcost.Span{Name: "tool/" + tool, StartNS: begin, EndNS: time.Since(start).Nanoseconds()})
		if err != nil {
			return time.Since(start).Nanoseconds(), err
		}
		return time.Since(start).Nanoseconds(), nil
	}
	arm.Spans = func() []hookcost.Span { return append([]hookcost.Span(nil), spans...) }
	return arm, nil
}

// hookCostInProcessArm is the arm that times the hook's own entry point for
// probe, paired with the native launch command it is subtracted from. Its
// preflight requires the reference call to exit zero, as every launch arm
// does, so a capture never records the timing of a rejected input; the timed
// calls then have to repeat that result. hookcost.InProcess alone accepts a
// denial because the latency probes include one on purpose.
func hookCostInProcessArm(id string, reference hookcost.RunFunc, command hookcost.Command, probe hookcost.Probe) hookcost.Arm {
	route := hookcost.InProcess(reference)
	return hookcost.Arm{ID: id, Layer: "in-process-run", Commands: []hookcost.Command{command},
		Preflight: func() error {
			var stderr strings.Builder
			if code := reference(probe.Argv, strings.NewReader(probe.Stdin), io.Discard, &stderr); code != 0 {
				return fmt.Errorf("reference input rejected: exit=%d stderr=%q", code, stderr.String())
			}
			return route.Prepare(probe)
		},
		Run: func() (int64, error) {
			elapsed, err := route.Invoke(probe)
			return elapsed.Nanoseconds(), err
		}}
}

func TestHookCostInProcessArmRequiresAnAcceptedReference(t *testing.T) {
	probe := hookcost.Probe{Name: "stub", Argv: []string{"stub"}, Stdin: "{}", Timeout: time.Second}
	exits := func(code int) hookcost.RunFunc {
		return func(argv []string, stdin io.Reader, stdout, stderr io.Writer) int {
			fmt.Fprint(stderr, "rejected")
			return code
		}
	}
	rejected := hookCostInProcessArm("id", exits(2), hookcost.Command{}, probe)
	if err := rejected.Preflight(); err == nil || !strings.Contains(err.Error(), "exit=2") {
		t.Fatalf("a reference that exits non-zero passed the preflight: %v", err)
	}
	accepted := hookCostInProcessArm("id", exits(0), hookcost.Command{}, probe)
	if err := accepted.Preflight(); err != nil {
		t.Fatalf("a reference that exits zero failed the preflight: %v", err)
	}
	if _, err := accepted.Run(); err != nil {
		t.Fatalf("a repeat of the accepted reference failed: %v", err)
	}
}

func TestHookCostCapture(t *testing.T) {
	if !*hookCost {
		t.Skip("hook cost capture is advisory; enable with -hook-cost")
	}
	n := *hookCostSamples
	if *hookCostCold {
		if *hookCostCase == "" || *hookCostCondition == "" {
			t.Fatal("cold requires one exact -hook-cost-case and documented -hook-cost-condition")
		}
		flag.Visit(func(f *flag.Flag) {
			if f.Name == "hook-cost-samples" {
				t.Fatal("cold cannot be combined with warm -hook-cost-samples")
			}
		})
		n = 1
	}
	if n <= 0 {
		t.Fatal("-hook-cost-samples must be positive")
	}
	var output *os.File
	if *hookCostOutput != "" {
		var err error
		output, err = os.OpenFile(*hookCostOutput, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if err != nil {
			t.Fatal(err)
		}
		defer output.Close()
	}
	root, err := filepath.Abs(hosttest.Checkout(t))
	if err != nil {
		t.Fatal(err)
	}
	target, err := hookcost.ResolveTarget(root, os.Getenv)
	if err != nil {
		t.Fatal(err)
	}
	hooksDir, binary := target.HooksDir, target.Binary
	registrations, err := hosttest.Registrations(hooksDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := hookcost.CheckBinary(binary); err != nil {
		t.Fatal(err)
	}
	fixture, env := hookcost.Prepare(t, target)
	capture := hookcost.Capture{Platform: runtime.GOOS + "/" + runtime.GOARCH, Method: "warm: preflight, five warmups, individually timed launches; case order recorded; no outlier removal", Warmups: 5, Units: "nanoseconds", FixtureDir: fixture, FixtureFiles: map[string]string{"fixture.txt": "safe baseline\n"}, Configs: hookcost.ConfigsFrom(registrations)}
	if *hookCostCold {
		capture.Method = "cold: one selected case before any target preflight/warmup; post-call reference/resolution validation and hashes"
		capture.Warmups = 0
		capture.Condition = *hookCostCondition
		capture.ColdEvidence = "operator-supplied condition; collector ordering alone does not independently prove install/reboot"
	}
	capture.Gaps = []string{"real-host full event timing and actual serial/concurrent scheduling unobserved", "genuine cold not measured by warm capture; cold condition needs operator evidence", "macOS arm64/Linux amd64/Windows amd64 other than the executing platform unavailable in this capture", "SessionStart timing is outside this PreToolUse capture", "config shell selection is replay convention, not live-host observation", "native startup/initialization and check costs are bundled; component controls cannot be added as independent costs"}
	var arms []hookcost.Arm
	for _, provider := range surface.All() {
		for _, tool := range []string{"Bash", "Read"} {
			selected, configs := hookcost.Selected(t, registrations, provider, tool, runtime.GOOS)
			for i, c := range configs {
				probe := hookCostProbe(c, hookCostStdin(provider, tool, fixture))
				prefix := string(provider) + "/" + tool + "/"
				native := hookcost.Native(target, fixture, env, run)
				arms = append(arms, hookcost.LaunchArm(prefix+"native/"+c.Hook, "native-launch", native, probe, c))
				dispatcher := hookcost.Dispatcher(target, fixture, env, run)
				arms = append(arms, hookcost.LaunchArm(prefix+"dispatcher/"+c.Hook, "dispatcher-launch", dispatcher, probe, c))
				config, err := hookcost.Configured(selected[i], target, fixture, env, run)
				if err != nil {
					capture.Gaps = append(capture.Gaps, prefix+c.Field+": "+err.Error())
					continue
				}
				arms = append(arms, hookcost.LaunchArm(prefix+"config/"+c.Hook, "config-command-replay", config, probe, c))
				arms = append(arms, hookCostInProcessArm(prefix+"in-process/"+c.Hook, run, native.Command(probe), probe))
			}
			event, err := hookCostEventArm(provider, tool, selected, target, fixture, env)
			if err != nil {
				capture.Gaps = append(capture.Gaps, string(provider)+"/"+tool+" event replay: "+err.Error())
			} else {
				arms = append(arms, event)
			}
		}
	}
	shell := hookcost.Command{TimeoutSeconds: 30}
	if runtime.GOOS == "windows" {
		shell.Executable = os.Getenv("ComSpec")
		shell.Args = []string{"/d", "/s", "/c", "exit /b 0"}
	} else {
		shell.Executable = "sh"
		shell.Args = []string{"-c", "true"}
	}
	arms = append(arms, hookcost.Control("control/host-shell", shell, fixture, env))
	stdin := shell
	stdin.Payload = `{"safe":"input"}`
	if runtime.GOOS == "windows" {
		stdin.Args = []string{"/d", "/s", "/c", "more >nul"}
	} else {
		stdin.Args = []string{"-c", "cat >/dev/null"}
	}
	arms = append(arms, hookcost.Control("control/shell-stdin-consumer", stdin, fixture, env))
	detection := shell
	if runtime.GOOS == "windows" {
		detection.Args = []string{"/d", "/s", "/c", `set "HOOK_ARCH=amd64" & if /i "%PROCESSOR_ARCHITECTURE%"=="ARM64" set "HOOK_ARCH=arm64" & if /i "%PROCESSOR_ARCHITEW6432%"=="ARM64" set "HOOK_ARCH=arm64" & if defined PROCESSOR_IDENTIFIER if /i "%PROCESSOR_IDENTIFIER:~0,5%"=="ARMv8" set "HOOK_ARCH=arm64"`}
	} else {
		detection.Args = []string{"-c", "uname -sm"}
	}
	arms = append(arms, hookcost.Control("control/platform-detection", detection, fixture, env))
	var selectedArms []hookcost.Arm
	for _, arm := range arms {
		if *hookCostCase != "" && arm.ID != *hookCostCase {
			continue
		}
		if *hookCostCold && (arm.Layer == "component-control" || arm.Layer == "in-process-run") {
			t.Fatal("cold case must launch a hook or configured event")
		}
		selectedArms = append(selectedArms, arm)
	}
	if len(selectedArms) == 0 {
		t.Fatalf("unknown -hook-cost-case %q", *hookCostCase)
	}
	seen := map[string]bool{}
	for _, arm := range selectedArms {
		if seen[arm.ID] {
			t.Fatalf("ambiguous duplicate case id %q", arm.ID)
		}
		seen[arm.ID] = true
	}
	if *hookCostCold && len(selectedArms) != 1 {
		t.Fatal("cold requires exactly one resolved case")
	}
	capture.StartedUTC = time.Now().UTC().Format(time.RFC3339Nano)
	capture.Records = hookcost.Measure(selectedArms, *hookCostCold, n, func(round int) { t.Logf("capture progress: %d/%d interleaved rounds", round, n) })
	if !*hookCostCold {
		capture.Method += "; round-robin recorded case order, paired deltas by sample index"
		capture.PairedDeltas = hookcost.PairedDeltas(capture.Records)
	}
	for _, record := range capture.Records {
		if record.Error != "" {
			capture.Gaps = append(capture.Gaps, record.ID+": "+record.Error)
		}
		t.Logf("%s n=%d median=%.3fms p95=%.3fms error=%q", record.ID, record.Summary.Count, record.Summary.MedianNS/1e6, float64(record.Summary.P95NS)/1e6, record.Error)
	}
	// Binary hashing and version reads come after a cold target's first invocation.
	capture.RecordedUTC = time.Now().UTC().Format(time.RFC3339Nano)
	sources, err := filepath.Glob(filepath.Join(root, "hooks", "go", "internal", "hookcost", "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	sources = append(sources, filepath.Join(root, "hooks", "go", "cmd", "hook", "event_bench_test.go"))
	capture.Metadata, capture.Hashes = hookcost.Metadata(root, hooksDir, binary, sources)
	capture.Metadata["capture_test_argv"] = strings.Join(os.Args, " ")
	capture.Metadata["replay_shell_convention"] = "Claude Unix sh, Windows CMD with host-style root binding; Codex Unix sh, Windows CMD around commandWindows; Copilot bash or Windows PowerShell -NoProfile. These conventions are not live-host launch observations."
	capture.Environment = map[string]string{}
	keys := append(append([]string(nil), hosttest.GuardEnv...), "PATH", "GIT_CONFIG_GLOBAL", "GIT_CONFIG_NOSYSTEM", "LC_ALL", "LANG", "AGX_NO_FETCH", "NoDefaultCurrentDirectoryInExePath", "PROCESSOR_ARCHITECTURE", "PROCESSOR_ARCHITEW6432", "PROCESSOR_IDENTIFIER")
	for _, key := range keys {
		capture.Environment[key] = os.Getenv(key)
	}
	data, err := json.MarshalIndent(capture, "", "    ")
	if err != nil {
		t.Fatal(err)
	}
	outputPath := *hookCostOutput
	if output == nil {
		revision := capture.Metadata["source_revision"]
		if !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(revision) {
			revision = "unknown"
		} else {
			revision = revision[:12]
		}
		runID := time.Now().UTC().Format("20060102T150405.000000000Z") + "-" + revision + "-" + runtime.GOOS + "-" + runtime.GOARCH
		dir := filepath.Join(root, "hooks", "go", "bench-e2e-results", runID)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("save capture: %v\nraw capture: %s", err, data)
		}
		name := "warm.json"
		if *hookCostCold {
			name = "cold.json"
		}
		outputPath = filepath.Join(dir, name)
		output, err = os.OpenFile(outputPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if err != nil {
			t.Fatalf("save capture: %v\nraw capture: %s", err, data)
		}
		defer output.Close()
	}
	if _, err := output.Write(append(data, '\n')); err != nil {
		t.Fatalf("save capture: %v\nraw capture: %s", err, data)
	}
	if err := output.Close(); err != nil {
		t.Fatal(err)
	}
	t.Logf("raw capture: %s", outputPath)
	for _, record := range capture.Records {
		if record.Error != "" {
			t.Errorf("invalid capture %s: %s", record.ID, record.Error)
		}
	}
}

func TestHookCostOutputRetention(t *testing.T) {
	path := filepath.Join(t.TempDir(), "existing.json")
	if err := os.WriteFile(path, []byte("retain original\n"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := hookcost.Process(ctx, os.Args[0], []string{"-test.run=^TestHookCostCapture$", "-hook-cost", "-hook-cost-samples=1", "-hook-cost-case=control/host-shell", "-hook-cost-output=" + path})
	output, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(strings.ToLower(string(output)), "exist") {
		t.Fatalf("capture did not refuse overwrite: %v %s", err, output)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "retain original\n" {
		t.Fatalf("capture replaced evidence: %q", data)
	}
}

func TestHookCostCaptureRejectsAFileAsHooksDir(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(file, []byte("a file\n"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := hookcost.Process(ctx, os.Args[0], []string{"-test.run=^TestHookCostCapture$", "-hook-cost", "-hook-cost-samples=1", "-hook-cost-case=control/host-shell"})
	cmd.Env = append(os.Environ(), "AGX_BENCH_HOOKS_DIR="+file)
	output, err := cmd.CombinedOutput()
	want := fmt.Sprintf("AGX_BENCH_HOOKS_DIR %q is not a directory", file)
	if err == nil || !strings.Contains(string(output), want) {
		t.Fatalf("capture did not refuse a file as the hooks directory: %v\nwant output containing %s\n%s", err, want, output)
	}
}
