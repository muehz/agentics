package shellwords

import (
	"context"
	"strings"
	"testing"
)

func TestPowerShellEnvironmentMetadataPolling(t *testing.T) {
	const units = 6
	for _, tc := range []struct {
		name, source string
		minimum      int
		call         bool
	}{
		{"assignment", "$" + strings.Repeat("a", units*64*1024) + "=[Environment]::SetEnvironmentVariable($name,$value)", units, true},
		{"words", "Write-Output " + strings.TrimSpace(strings.Repeat(strings.Repeat("a", 32)+" ", 4096)), 4096 / 256, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parsed := Parse(tc.source, PowerShell)
			if len(parsed.Invocations) == 0 {
				t.Fatalf("invocations=%d", len(parsed.Invocations))
			}
			words := lexSegment(tc.source, PowerShell).words
			probe := &callCancelContext{Context: context.Background()}
			got := environmentCallMetadata(probe, tc.source, words)
			if probe.calls < tc.minimum {
				t.Fatalf("metadata %s polling: got=%d minimum=%d", tc.name, probe.calls, tc.minimum)
			}
			if (got != nil) != tc.call {
				t.Fatalf("metadata %s normal call=%v", tc.name, got != nil)
			}
			probe = &callCancelContext{Context: context.Background(), at: 4}
			got = environmentCallMetadata(probe, tc.source, words)
			if probe.calls < 4 || got == nil || len(got.Arguments) != 0 || got.NullValue {
				t.Fatalf("metadata %s canceled proof: checks=%d call=%v", tc.name, probe.calls, got != nil)
			}
			probe = &callCancelContext{Context: context.Background(), at: 4}
			canceled := ParseContext(probe, tc.source, PowerShell)
			originalRetained := false
			for _, inv := range canceled.Invocations {
				originalRetained = originalRetained || inv.Raw == parsed.Invocations[0].Raw
				if inv.Inspectable || inv.EnvironmentCall != nil && (len(inv.EnvironmentCall.Arguments) != 0 || inv.EnvironmentCall.NullValue) {
					t.Fatal("canceled parser published metadata proof")
				}
			}
			if probe.calls < 4 || !originalRetained {
				t.Fatalf("canceled metadata original=%v checks=%d", originalRetained, probe.calls)
			}
		})
	}
}

func TestPowerShellEnvironmentNullEvidence(t *testing.T) {
	for _, tc := range []struct {
		value          string
		complete, null bool
	}{
		{`$null`, true, true}, {`''`, true, false}, {`'$null'`, true, false},
		{`$nullish`, false, false}, {`$null + 'x'`, false, false}, {`($null)`, false, false},
	} {
		t.Run(tc.value, func(t *testing.T) {
			parsed := Parse(`[Environment]::SetEnvironmentVariable('EXAMPLE', `+tc.value+`, 'Process')`, PowerShell)
			found := false
			for _, inv := range parsed.Invocations {
				if call := inv.EnvironmentCall; call != nil {
					found = true
					if (len(call.Arguments) == 3) != tc.complete || call.NullValue != tc.null {
						t.Fatalf("call=%#v", call)
					}
				}
			}
			if !found {
				t.Fatal("setter evidence lost")
			}
		})
	}
	command := `[Environment]::SetEnvironmentVariable('EXAMPLE', $null + $(git status), 'Process')`
	found := false
	for _, inv := range Parse(command, PowerShell).Invocations {
		if len(inv.Words) > 0 && inv.Words[0].Text == "git" {
			found = true
		}
		if inv.EnvironmentCall != nil && inv.EnvironmentCall.NullValue {
			t.Fatal("compound null was consumed")
		}
	}
	if !found {
		t.Fatal("lost compound null executable child")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, inv := range ParseContext(ctx, `[Environment]::SetEnvironmentVariable('EXAMPLE', $null)`, PowerShell).Invocations {
		if inv.EnvironmentCall != nil && (inv.EnvironmentCall.NullValue || len(inv.EnvironmentCall.Arguments) != 0) {
			t.Fatal("canceled null proof survived")
		}
	}
}

type callCancelContext struct {
	context.Context
	calls, at int
}

func (c *callCancelContext) Err() error {
	c.calls++
	if c.at > 0 && c.calls >= c.at {
		return context.Canceled
	}
	return nil
}

func TestPowerShellEnvironmentCallContext(t *testing.T) {
	command := `[Environment]::SetEnvironmentVariable('EXAMPLE','` + strings.Repeat("x", 256*1024) + `')`
	for _, at := range []int{0, 1, 4} {
		t.Run(string(rune('0'+at)), func(t *testing.T) {
			ctx := &callCancelContext{Context: context.Background(), at: at}
			result := ParseContext(ctx, command, PowerShell)
			if len(result.Invocations) == 0 {
				t.Fatal("lost invocation")
			}
			if at == 0 {
				if len(result.Invocations) != 1 || result.Invocations[0].EnvironmentCall == nil || len(result.Invocations[0].EnvironmentCall.Arguments) != 2 || ctx.calls < 8 {
					t.Fatalf("large literal proof missing or insufficient polling: calls=%d invocations=%d", ctx.calls, len(result.Invocations))
				}
			} else {
				for _, inv := range result.Invocations {
					if inv.Inspectable || inv.EnvironmentCall != nil && len(inv.EnvironmentCall.Arguments) != 0 {
						t.Fatal("published partial literal proof")
					}
				}
			}
		})
	}
}

func TestPowerShellEnvironmentCallChildren(t *testing.T) {
	for _, tc := range []struct {
		command      string
		dialect      Dialect
		child        string
		childDialect Dialect
	}{
		{`[Environment]::SetEnvironmentVariable('GIT_CONFIG_KEY_0',$(git commit --no-verify))`, PowerShell, "git", PowerShell},
		{`pwsh -Command "[Environment]::SetEnvironmentVariable('EXAMPLE','ordinary'); git commit --no-verify"`, POSIX, "git", PowerShell},
		{`bash -c 'git commit --no-verify'`, PowerShell, "git", POSIX},
	} {
		t.Run(tc.command, func(t *testing.T) {
			found := false
			for _, inv := range Parse(tc.command, tc.dialect).Invocations {
				if len(inv.Words) > 0 && inv.Words[0].Text == tc.child {
					found = true
					if inv.Dialect != tc.childDialect {
						t.Fatalf("child dialect=%v", inv.Dialect)
					}
				}
			}
			if !found {
				t.Fatal("lost executable child")
			}
		})
	}
}
