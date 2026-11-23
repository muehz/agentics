package shellwords

import (
	"context"
	"testing"
)

// provenPowerShell, like provenPOSIX, is the context a claude-code call with
// only a string command receives, so these tests exercise ForInvocation from a
// context that already allows new proof.
var provenPowerShell = ExecutionContext{Dialect: PowerShell, Proven: true}

func TestExecutionContextPrecisionIdentityRoles(t *testing.T) {
	for _, command := range []string{`Set-Content -Path source.txt -Value 'Function:rg'`, `Set-Content source.txt 'Alias:rg'`} {
		got := provenPowerShell.ForInvocation(Parse(command, PowerShell), 0)
		if !got.AllowsNewProof() {
			t.Errorf("literal write data %q lost eligibility: %+v", command, got)
		}
	}
	for _, pattern := range []string{"Function:.env", "Alias:.env", "$env:PATH", "$env:PATHEXT"} {
		command := "rg --no-config '" + pattern + "' source.txt"
		got := provenPowerShell.ForInvocation(Parse(command, PowerShell), 0)
		if !got.AllowsNewProof() {
			t.Errorf("literal data %q lost eligibility: %+v", command, got)
		}
	}
	for _, command := range []string{`Set-Content Function:rg helper`, `Set-Content -Path:Function:rg helper`, `Set-Content -Unknown source.txt helper`, `Set-Item Alias:rg helper`, `$env:PATH = 'synthetic'`, `$env:PATHEXT = 'synthetic'`, `'helper' > Function:rg`, `'helper' > Alias:rg`} {
		got := provenPowerShell.ForInvocation(Parse(command, PowerShell), 0)
		if got.AllowsNewProof() {
			t.Errorf("identity write %q retained eligibility", command)
		}
	}
}

func TestExecutionContextPrecisionWriterAliasesAndNamespaces(t *testing.T) {
	for _, writer := range []string{"Copy-Item", "cpi", "copy", "cp", "Move-Item", "mi", "move", "mv", "Remove-Item", "rm", "ri", "del", "erase", "rd", "rmdir", "Set-Content", "sc", "Add-Content", "ac", "Clear-Content", "clc", "Rename-Item", "ren", "rni", "Clear-Item", "cli", "Set-Item", "si", "New-Item", "ni"} {
		for _, path := range []string{`Function:rg`, `Alias:rg`, `Microsoft.PowerShell.Core\Function::rg`, `Microsoft.PowerShell.Core\Alias::rg`, `Unknown.Provider\Unknown::rg`, `UnknownDrive:rg`} {
			t.Run(writer+"/"+path, func(t *testing.T) {
				command := writer + " '" + path + "'"
				got := provenPowerShell.ForInvocation(Parse(command, PowerShell), 0)
				if got.AllowsNewProof() {
					t.Fatalf("identity/unknown namespace write retained proof: %q", command)
				}
			})
		}
	}
	for _, command := range []string{
		`cpi Function:old Function:rg`, `copy Alias:old Alias:rg`,
		`Copy-Item source.txt -Destination UnknownDrive:rg`,
		`'helper' > Microsoft.PowerShell.Core\Function::rg`,
		`'helper' > UnknownDrive:rg`, `sal rg helper`, `nal rg helper`,
		`sp Function:rg value helper`, `rp Alias:rg value`,
		`cpp Function:old Function:rg value`, `mp Alias:old Alias:rg value`,
		`clp Function:rg value`, `rnp Alias:rg value other`,
		`set PATH synthetic`, `sv PATH synthetic`, `nv PATH synthetic`,
		`rv PATH`, `rmv PATH`, `clv PATH`,
		`Microsoft.PowerShell.Management\Copy-Item source.txt -Destination Microsoft.PowerShell.Core\Function::rg`,
		`Set-Content -LiteralPath:UnknownDrive:rg helper`,
		`Copy-Item source.txt -Destination C:relative`,
	} {
		got := provenPowerShell.ForInvocation(Parse(command, PowerShell), 0)
		if got.AllowsNewProof() {
			t.Errorf("identity write retained proof: %q", command)
		}
	}
	for _, command := range []string{
		`rg --no-config 'Microsoft.PowerShell.Core\Function::rg' source.txt`,
		`rg --no-config 'UnknownDrive:rg' source.txt`,
		`sc source.txt 'Microsoft.PowerShell.Core\Alias::rg'`,
		`ac -Path source.txt -Value 'UnknownDrive:rg'`,
		`Remove-Item -LiteralPath C:\cache\scratch.txt`,
	} {
		got := provenPowerShell.ForInvocation(Parse(command, PowerShell), 0)
		if !got.AllowsNewProof() {
			t.Errorf("literal data/filesystem path lost eligibility: %q %+v", command, got)
		}
	}
}

func TestExecutionContextPrecisionVisibleState(t *testing.T) {
	for _, command := range []string{
		`[Environment]::SetEnvironmentVariable('PATH','synthetic','Process')`,
		"opaque-helper; rg --no-config 'needle' source.txt",
		"echo harmless; rg --no-config 'needle' source.txt",
		"Set-Alias rg helper; rg --no-config 'needle' source.txt",
		"function rg { helper }; rg --no-config 'needle' source.txt",
		"Set-Item Function:rg helper; rg --no-config 'needle' source.txt",
		"Set-Item Alias:rg helper; rg --no-config 'needle' source.txt",
		"$env:PATH = 'synthetic'; rg --no-config 'needle' source.txt",
		"$env:PATHEXT = 'synthetic'; rg --no-config 'needle' source.txt",
		". helper.ps1; rg --no-config 'needle' source.txt",
		"Invoke-Expression 'helper'; rg --no-config 'needle' source.txt",
	} {
		parsed := Parse(command, PowerShell)
		ctx := provenPowerShell
		if got := ctx.ForInvocation(parsed, len(parsed.Invocations)-1); got.AllowsNewProof() {
			t.Errorf("command %q retained proof: %+v", command, got)
		}
	}
	for _, command := range []string{"PATH=synthetic; rg needle source.txt", "env -i rg needle source.txt"} {
		ctx := provenPOSIX
		parsed := Parse(command, POSIX)
		if ctx.ForInvocation(parsed, len(parsed.Invocations)-1).AllowsNewProof() {
			t.Errorf("environment effects retained proof: %q", command)
		}
	}
	command := "rg --no-config 'needle' source.txt"
	ctx := provenPowerShell
	if !ctx.ForInvocation(Parse(command, PowerShell), 0).AllowsNewProof() {
		t.Fatal("standalone search lost context eligibility")
	}
}

func TestExecutionContextPrecisionWrapperDialect(t *testing.T) {
	command := `bash -c 'pwsh -Command "Get-Content source.txt"'`
	parsed := Parse(command, POSIX)
	ctx := provenPOSIX
	if len(parsed.Invocations) != 1 {
		t.Fatalf("invocations=%+v", parsed.Invocations)
	}
	got := ctx.ForInvocation(parsed, 0)
	if got.Dialect != PowerShell || !got.AllowsNewProof() {
		t.Fatalf("child context=%+v", got)
	}
	if parsed.Invocations[0].Words[1].Raw != "source.txt" {
		t.Fatal("word source changed")
	}
}

func TestForInvocationIsUnchangedByScriptSupport(t *testing.T) {
	ctx := ExecutionContext{Dialect: POSIX, Proven: true}
	first := ParseScripts(context.Background(), "bash --version", POSIX, ScriptOptions{Reader: fakeReader{}, Context: ctx, Cwd: "/ws"})
	if !ctx.ForInvocation(first, 0).AllowsNewProof() {
		t.Fatal("bash --version as the first command lost proof")
	}
	for _, reader := range []ScriptReader{nil, fakeReader{}} {
		chained := ParseScripts(context.Background(), "cd /tmp && bash --version", POSIX, ScriptOptions{Reader: reader, Context: ctx, Cwd: "/ws"})
		last := len(chained.Invocations) - 1
		got := ctx.ForInvocation(chained, last)
		if got.AllowsNewProof() || got.Reason != "unknown preceding execution state" {
			t.Fatalf("reader=%v: chained inspection form got proof or the wrong reason: %+v", reader != nil, got)
		}
	}
}
