package commandguard

import (
	"strings"
	"testing"
)

func TestNuGetNonliteralOperationDenied(t *testing.T) {
	for _, command := range []string{`dotnet nu$(printf get) push package.nupkg`, `dotnet nuget pu$(printf sh) package.nupkg`, `nuget pu$(printf sh) package.nupkg`, `dotnet $operation package.nupkg`, `nuget $operation package.nupkg`} {
		tc := tableCase{name: command, command: command, want: "deny"}
		got := recordCase(t, tc)
		assertCaseSemantics(t, tc, got)
		if !strings.Contains(got.Outcome.Payload(), "non-literal NuGet operation") {
			t.Fatalf("wrong denial: %s", got.Outcome)
		}
	}
}

func TestNuGetPushAbbreviationsDenied(t *testing.T) {
	for _, command := range []string{`nuget pu package.nupkg`, `nuget pus package.nupkg`, `nuget.exe Pu package.nupkg`, `nuget pus package.nupkg -Source -Help`} {
		tc := tableCase{name: command, command: command, want: "deny"}
		got := recordCase(t, tc)
		assertCaseSemantics(t, tc, got)
		if !strings.Contains(got.Outcome.Payload(), "NuGet package publication") {
			t.Fatalf("wrong denial: %s", got.Outcome)
		}
	}
}
