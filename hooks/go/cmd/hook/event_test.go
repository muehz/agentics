package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/contract"
	"github.com/muehz/agentics/hooks/go/internal/hookspec"
	"github.com/muehz/agentics/hooks/go/internal/hosttest"
	"github.com/muehz/agentics/hooks/go/internal/provider"
	"github.com/muehz/agentics/hooks/go/internal/shellwords"
	"github.com/muehz/agentics/hooks/go/internal/surface"
)

type precisionCheckpointContext struct {
	context.Context
	checks, cancelAt int
	cancel           context.CancelFunc
}

func (c *precisionCheckpointContext) Err() error {
	c.checks++
	if c.cancel != nil && c.checks == c.cancelAt {
		c.cancel()
	}
	return c.Context.Err()
}

func TestGuardPrecisionPartialProof(t *testing.T) {
	hosttest.ScrubGuardEnv(t)
	clearHardeningCredentialOverrides(t)
	for _, name := range []string{"POSIXLY_CORRECT", "CDPATH", "BASHOPTS", "GLOBIGNORE"} {
		t.Setenv(name, "")
	}
	const padding = 6 * 64 * 1024
	data := "@'\ndon't; gh stack submit; cat .env; kubectl apply -f prod.yaml; 'literal\n" + strings.Repeat("x", padding) + "\n'@ | Set-Content -LiteralPath draft.txt" + strings.Repeat(" ", padding)
	guards := []string{"command-guardian", "release-guardian", "secrets-guardian"}
	// commandSteps are the two rule sets the grouped commands check runs as
	// steps; a case that targets "commands" targets both.
	commandSteps := map[string]bool{"command-guardian": true, "release-guardian": true}
	for _, tc := range []struct {
		name, tool, command string
		stage               string
		only                string
	}{
		{"bash-inspection", "Bash", "bash" + strings.Repeat(" ", padding) + "--help", "parse", ""},
		{"bash-consumer-publication", "Bash", "bash --help", "consumer", "commands"},
		{"powershell-environment", "PowerShell", "[Environment]::SetEnvironmentVariable('EXAMPLE','" + strings.Repeat("x", padding) + "')", "parse", ""},
		{"powershell-data-body", "PowerShell", data, "body", ""},
		{"powershell-data-suffix", "PowerShell", data, "suffix", ""},
		{"powershell-data-publication", "PowerShell", data, "publication", ""},
		{"powershell-search", "PowerShell", "rg --no-config '.env" + strings.Repeat("x", padding) + "' source.txt", "search", "secrets-guardian"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checkpoint := &precisionCheckpointContext{Context: context.Background()}
			if tc.stage != "parse" {
				dialect := shellwords.PowerShell
				if tc.tool == "Bash" {
					dialect = shellwords.POSIX
				}
				parsed := shellwords.ParseContext(checkpoint, tc.command, dialect)
				if tc.stage == "consumer" {
					execution := shellwords.ExecutionContext{Dialect: dialect, Proven: true}
					if len(parsed.Invocations) != 1 || parsed.Invocations[0].Inspection != shellwords.BashMetadata || !execution.ForInvocation(parsed, 0).AllowsNewProof() {
						t.Fatal("consumer fixture has no qualified metadata invocation")
					}
				}
			}
			if tc.stage == "suffix" {
				shellwords.PowerShellLiteralData(checkpoint, tc.command)
			}
			// The prefix is measured with the same parser used by the handlers;
			// cancellation then occurs after entry into the next bounded scan.
			cancelAt := checkpoint.checks + 4
			if tc.stage == "consumer" {
				cancelAt = checkpoint.checks + 1
			}
			if tc.stage == "publication" {
				execution := shellwords.ExecutionContext{Dialect: shellwords.PowerShell, Proven: true}
				view, proven, err := shellwords.PowerShellFileData(checkpoint, tc.command, execution)
				if err != nil || !proven {
					t.Fatalf("publication fixture proven=%v err=%v", proven, err)
				}
				candidate := shellwords.ParseContext(checkpoint, view, execution.Dialect)
				if len(candidate.Invocations) == 0 {
					t.Fatal("publication fixture has no candidate")
				}
				for _, inv := range candidate.Invocations {
					if !inv.Inspectable {
						t.Fatal("publication fixture candidate is incomplete")
					}
				}
				cancelAt = checkpoint.checks + 1
			}
			for _, surface := range surface.All() {
				t.Run(string(surface), func(t *testing.T) {
					root := t.TempDir()
					writeHardeningFile(t, root+"/source.txt", "public source")
					// The neutral PowerShell name reaches Codex and Antigravity unchanged:
					// this fixture exercises adapter passthrough, not native shell tools.
					payload := string(hosttest.Stdin(surface, hosttest.Call{
						Tool: tc.tool, Input: map[string]any{"command": tc.command}, Cwd: root,
						Native: (surface == "codex" || surface == "antigravity") && tc.tool == "PowerShell",
					}))
					for _, canceled := range []bool{false, true} {
						name := "complete"
						if canceled {
							name = "canceled"
						}
						t.Run(name, func(t *testing.T) {
							deps := productionHookDependencies()
							lookup := deps.lookup
							var mu sync.Mutex
							calls := map[string]int{}
							// The two command steps of one run share one probe, made by the
							// first and reused by the second, so a cancellation that lands in
							// the command rules still governs the release rules after them.
							// The steps run in order in one goroutine, so the probe needs no
							// lock, and it is canceled once the invocation has answered.
							var stepBase context.Context
							var stepProbe *precisionCheckpointContext
							stepCancel := context.CancelFunc(func() {})
							defer func() { stepCancel() }()
							deps.lookup = func(guard string) (hookspec.Handler, bool) {
								handler, ok := lookup(guard)
								if !ok {
									return nil, false
								}
								return func(req hookspec.Request) hookspec.Outcome {
									mu.Lock()
									calls[guard]++
									mu.Unlock()
									if req.Context == nil {
										t.Error("grouped guard lost its request context")
									}
									target := tc.only == "" || tc.only == guard || tc.only == "commands" && commandSteps[guard]
									var base context.Context
									var probe *precisionCheckpointContext
									if commandSteps[guard] {
										if stepProbe == nil {
											stepBase, stepCancel = context.WithCancel(req.ExecutionContext())
											stepProbe = &precisionCheckpointContext{Context: stepBase}
											if canceled && target {
												stepProbe.cancelAt, stepProbe.cancel = cancelAt, stepCancel
											}
										}
										base, probe = stepBase, stepProbe
									} else {
										own, cancel := context.WithCancel(req.ExecutionContext())
										defer cancel()
										base, probe = own, &precisionCheckpointContext{Context: own}
										if canceled && target {
											probe.cancelAt, probe.cancel = cancelAt, cancel
										}
									}
									req.Context = probe
									var table bytes.Buffer
									if req.Diag != nil {
										req.Diag = io.MultiWriter(req.Diag, &table)
									} else {
										req.Diag = &table
									}
									outcome := handler(req)
									want := contract.KindPass
									if canceled && target {
										want = contract.KindDeny
										if base.Err() == nil || probe.checks < cancelAt {
											t.Errorf("%s did not reach %s checkpoint: checks=%d want=%d canceled=%v", guard, tc.stage, probe.checks, cancelAt, base.Err())
										}
									} else if target && (probe.checks < cancelAt || tc.stage != "publication" && tc.stage != "consumer" && probe.checks == cancelAt) {
										t.Errorf("%s control did not finish beyond checkpoint: checks=%d checkpoint=%d", guard, probe.checks, cancelAt)
									}
									if outcome.Kind() != want {
										t.Errorf("%s outcome=%s, want %s: %s", guard, outcome.Kind(), want, outcome.Payload())
									}
									if canceled && target {
										// The two command steps share one probe, so the cancellation
										// lands in the command rules and the release rules then run on
										// a canceled context. Each rule set's own checkpoint is proved
										// in the hooks package tests.
										//
										// The cancellation adds a fixed parse-limit threat, and a fixed
										// threat names the reason alone whenever one is present, while the
										// publication threat is governed by a settings key. Only the
										// diagnostic table, which lists every threat, carries it.
										reasons := map[string][]string{
											"command-guardian": {"command_parse_limit"},
											"release-guardian": {"release-guardian could not inspect"},
											"secrets-guardian": {"canceled"},
										}[guard]
										if tc.stage == "body" || tc.stage == "suffix" || tc.stage == "publication" {
											reasons = map[string][]string{
												"command-guardian": {"unsafe_publication"},
												"release-guardian": {"release-guardian could not inspect"},
												"secrets-guardian": {"credential file read(s) blocked"},
											}[guard]
										}
										for _, reason := range reasons {
											if !strings.Contains(outcome.Payload()+table.String(), reason) {
												t.Errorf("%s lost cancellation/source reason %q: %s\n%s", guard, reason, outcome.Payload(), table.String())
											}
										}
									}
									t.Logf("%s checkpoints=%d cancel_at=%d", guard, probe.checks, probe.cancelAt)
									return outcome
								}, true
							}
							code, out, diagnostic := invokeWithLegalArgsAndDependencies(t, deps, payload, surface, "pre-tool")
							mu.Lock()
							defer mu.Unlock()
							if len(calls) != len(guards) {
								t.Errorf("unexpected grouped guards: %v", calls)
							}
							for _, guard := range guards {
								if calls[guard] != 1 {
									t.Errorf("grouped %s invoked %d times, want once", guard, calls[guard])
								}
							}
							if canceled {
								requireDeny(t, surface, code, out, diagnostic)
							} else {
								requireSilent(t, surface, code, out, diagnostic)
							}
						})
					}
				})
			}
		})
	}
}

// safeShellStdin is the payload a surface's host writes for an uninteresting
// shell call in a fresh directory.
func safeShellStdin(t testing.TB, surface surface.ID) []byte {
	t.Helper()
	return hosttest.Stdin(surface, hosttest.Call{Tool: "Bash", Input: map[string]any{"command": "echo safe"}, Cwd: t.TempDir()})
}

// requireDeny fails the test unless the surface's host reads the hook's exit
// code and output as a deny, and returns the reason the host shows.
func requireDeny(t testing.TB, surface surface.ID, exit int, stdout, stderr string) string {
	t.Helper()
	verdict := hosttest.Read(surface, exit, []byte(stdout), []byte(stderr))
	if verdict.Decision != hosttest.Deny {
		t.Fatalf("%s host reads %v, want Deny: exit=%d stdout=%q stderr=%q", surface, verdict.Decision, exit, stdout, stderr)
	}
	return verdict.Reason
}

func TestEventDispatchNormalizesOnce(t *testing.T) {
	for _, surface := range surface.All() {
		verb := "pre-tool"
		t.Run(string(surface)+"/"+verb, func(t *testing.T) {
			deps := productionHookDependencies()
			normalize := deps.normalize
			var normalized, started atomic.Int32
			deps.normalize = func(surface, event string, raw []byte) (hookspec.Event, *provider.Rendered) {
				normalized.Add(1)
				return normalize(surface, event, raw)
			}
			deps.lookup = func(string) (hookspec.Handler, bool) {
				return func(hookspec.Request) hookspec.Outcome {
					started.Add(1)
					return contract.Pass()
				}, true
			}
			var out, errout bytes.Buffer
			exit := runWithDependencies(deps, []string{string(surface), verb}, bytes.NewReader(safeShellStdin(t, surface)), &out, &errout)
			want := int32(2)
			if exit != 0 || normalized.Load() != 1 || started.Load() != want || errout.Len() != 0 {
				t.Fatalf("exit=%d normalize=%d checks=%d want=%d stdout=%q stderr=%q", exit, normalized.Load(), started.Load(), want, out.String(), errout.String())
			}
		})
	}
}

// A check that returns an invalid outcome may still have written the only
// clue to its failure, so the grouped path keeps that text on stderr under
// the check's label while the event denies.
func TestEventInvalidOutcomeKeepsChatter(t *testing.T) {
	for _, surface := range surface.All() {
		t.Run(string(surface), func(t *testing.T) {
			deps := productionHookDependencies()
			var mu sync.Mutex
			var names []string
			deps.lookup = func(name string) (hookspec.Handler, bool) {
				mu.Lock()
				names = append(names, name)
				mu.Unlock()
				return func(req hookspec.Request) hookspec.Outcome {
					_, _ = req.Diag.Write([]byte("chatter\n"))
					return hookspec.Outcome{}
				}, true
			}
			var out, errout bytes.Buffer
			exit := runWithDependencies(deps, []string{string(surface), "pre-tool"}, bytes.NewReader(safeShellStdin(t, surface)), &out, &errout)
			requireDeny(t, surface, exit, out.String(), errout.String())
			if len(names) == 0 {
				t.Fatal("no check ran")
			}
			for _, name := range names {
				want := "[" + name + "] chatter"
				if !strings.Contains(errout.String(), want) {
					t.Errorf("stderr = %q, want it to hold %q", errout.String(), want)
				}
			}
		})
	}
}

type eventObservedReader struct{ read atomic.Bool }

func (r *eventObservedReader) Read([]byte) (int, error) {
	r.read.Store(true)
	return 0, io.EOF
}

type eventErrorReader struct{}

func (eventErrorReader) Read([]byte) (int, error) { return 0, errors.New("private input error") }

func TestEventDispatchFailurePosture(t *testing.T) {
	for _, surface := range surface.All() {
		verb := "pre-tool"
		for _, fault := range []string{"arguments", "read", "malformed", "normalize panic", "mismatched event"} {
			t.Run(string(surface)+"/"+verb+"/"+fault, func(t *testing.T) {
				deps := productionHookDependencies()
				var called atomic.Bool
				deps.lookup = func(string) (hookspec.Handler, bool) {
					called.Store(true)
					return nil, false
				}
				argv := []string{string(surface), verb}
				observed := &eventObservedReader{}
				var reader io.Reader = strings.NewReader("{")
				switch fault {
				case "arguments":
					argv = append(argv, "private argument")
					reader = observed
				case "read":
					reader = eventErrorReader{}
				case "normalize panic":
					deps.normalize = func(string, string, []byte) (hookspec.Event, *provider.Rendered) { panic("private panic value") }
				case "mismatched event":
					if surface == "copilot" {
						// Copilot carries no event discriminator, so use an invalid normalized event at the adapter seam.
						deps.normalize = func(string, string, []byte) (hookspec.Event, *provider.Rendered) { return hookspec.Event{}, nil }
					} else {
						// The dispatcher no longer routes this event; it only builds a
						// payload whose event does not match the verb.
						var document map[string]any
						if err := json.Unmarshal(safeShellStdin(t, surface), &document); err != nil {
							t.Fatal(err)
						}
						if surface == "antigravity" {
							document["hookEventName"] = "PostToolUse"
						} else {
							document["hook_event_name"] = "PostToolUse"
							document["tool_response"] = map[string]any{"status": "completed"}
						}
						wrongEvent, err := json.Marshal(document)
						if err != nil {
							t.Fatal(err)
						}
						reader = bytes.NewReader(wrongEvent)
					}
				}
				var stdout, stderr bytes.Buffer
				exit := runWithDependencies(deps, argv, reader, &stdout, &stderr)
				requireDeny(t, surface, exit, stdout.String(), stderr.String())
				if called.Load() || observed.read.Load() {
					t.Fatalf("fault reached handler lookup or rejected argv consumed stdin: lookup=%v read=%v", called.Load(), observed.read.Load())
				}
				if strings.Contains(stdout.String()+stderr.String(), "private") {
					t.Fatalf("event infrastructure fault leaked input: %q %q", stdout.String(), stderr.String())
				}
			})
		}
	}
}

func TestEventRoutingNativeInputs(t *testing.T) {
	cases := []struct {
		surface    surface.ID
		verb, tool string
		want       []string
	}{
		{"claude-code", "pre-tool", "MultiEdit", []string{"commands", "secrets-guardian"}},
		{"claude-code", "pre-tool", "Write", []string{"commands", "secrets-guardian"}},
		{"claude-code", "pre-tool", "NotebookRead", []string{"secrets-guardian"}},
		{"claude-code", "pre-tool", "Read", []string{"secrets-guardian"}},
		{"codex", "pre-tool", "exec_command", []string{"commands", "secrets-guardian"}},
		{"codex", "pre-tool", "apply_patch", []string{"commands", "secrets-guardian"}},
		{"codex", "pre-tool", "UnknownTool", []string{"commands"}},
		{"copilot", "pre-tool", "view", []string{"commands", "secrets-guardian"}},
		{"copilot", "pre-tool", "apply_patch", []string{"commands", "secrets-guardian"}},
		{"copilot", "pre-tool", "powershell", []string{"commands", "secrets-guardian"}},
		{"antigravity", "pre-tool", "run_command", []string{"commands", "secrets-guardian"}},
		{"antigravity", "pre-tool", "view_file", []string{"commands", "secrets-guardian"}},
		{"antigravity", "pre-tool", "write_to_file", []string{"commands", "secrets-guardian"}},
		{"antigravity", "pre-tool", "replace_file_content", []string{"commands", "secrets-guardian"}},
		{"antigravity", "pre-tool", "multi_replace_file_content", []string{"commands", "secrets-guardian"}},
		{"antigravity", "pre-tool", "UnknownTool", []string{"commands"}},
	}
	for _, tc := range cases {
		t.Run(string(tc.surface)+"/"+tc.verb+"/"+tc.tool, func(t *testing.T) {
			deps := productionHookDependencies()
			started := make(chan string, 5)
			deps.lookup = func(name string) (hookspec.Handler, bool) {
				return func(req hookspec.Request) hookspec.Outcome {
					started <- name
					return contract.Pass()
				}, true
			}
			// The tool is the surface's own name for it, so the call is written
			// as given and carries the input keys every surface's tools use.
			input := map[string]any{"command": "echo safe", "cmd": "echo safe", "path": "fixture.txt"}
			if tc.surface == "antigravity" {
				input = map[string]any{
					"CommandLine":        "echo safe",
					"AbsolutePath":       "fixture.txt",
					"TargetFile":         "fixture.txt",
					"CodeContent":        "safe",
					"TargetContent":      "old",
					"ReplacementContent": "new",
					"ReplacementChunks":  []any{map[string]any{"TargetContent": "old", "ReplacementContent": "new"}},
				}
			}
			patch := "*** Begin Patch\n*** Add File: fixture.txt\n+safe\n*** End Patch"
			if tc.surface == "copilot" {
				input["input"] = patch
			} else if tc.tool == "apply_patch" {
				input["command"] = patch
			}
			stdin := hosttest.Stdin(tc.surface, hosttest.Call{Tool: tc.tool, Input: input, Cwd: t.TempDir(), Native: true})
			var out, errout bytes.Buffer
			exit := runWithDependencies(deps, []string{string(tc.surface), tc.verb}, bytes.NewReader(stdin), &out, &errout)
			if exit != 0 || errout.Len() != 0 {
				t.Fatalf("routing failed: exit=%d stdout=%q stderr=%q", exit, out.String(), errout.String())
			}
			close(started)
			var got []string
			for name := range started {
				got = append(got, name)
			}
			slices.Sort(got)
			if !slices.Equal(got, tc.want) {
				t.Fatalf("actual native input invoked checks=%v want=%v", got, tc.want)
			}
		})
	}
}

func TestEventDispatchAdapterFaultRecovery(t *testing.T) {
	for _, surface := range surface.All() {
		verb := "pre-tool"
		for _, stage := range []string{"dispatch", "render", "wrong event"} {
			t.Run(string(surface)+"/"+verb+"/"+stage, func(t *testing.T) {
				deps := productionHookDependencies()
				deps.lookup = func(string) (hookspec.Handler, bool) {
					return func(hookspec.Request) hookspec.Outcome { return contract.Pass() }, true
				}
				switch stage {
				case "dispatch":
					deps.dispatch = func(string, string, []string) (string, *provider.Rendered) { panic("private dispatch value") }
				case "render":
					deps.render = func(string, string, hookspec.Outcome, []byte) provider.Rendered { panic("private render value") }
				case "wrong event":
					deps.dispatch = func(string, string, []string) (string, *provider.Rendered) { return "wrong", nil }
				}
				var out, errout bytes.Buffer
				exit := runWithDependencies(deps, []string{string(surface), verb}, bytes.NewReader(safeShellStdin(t, surface)), &out, &errout)
				requireDeny(t, surface, exit, out.String(), errout.String())
				if strings.Contains(out.String()+errout.String(), "private") {
					t.Fatal("adapter panic leaked its value")
				}
			})
		}
	}
}
