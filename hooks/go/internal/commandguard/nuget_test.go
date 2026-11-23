package commandguard

import (
	"strings"
	"testing"
)

func TestNuGetPublicationThroughRun(t *testing.T) {
	for _, command := range []string{
		`dotnet nuget push package.nupkg`,
		`dotnet --diagnostics nuget push package.nupkg --source private`,
		`nuget push package.nupkg`,
		`nuget.exe PUSH package.nupkg -Source private`,
		`dotnet nuget push package.nupkg --source ./local-feed`,
		`dotnet nuget push package.nupkg --source --help`,
		`dotnet nuget push package.nupkg --skip-duplicate`,
		`dotnet nuget push package.nupkg --dry-run`,
		`nuget push package.nupkg -Source -Help`,
		`dotnet nuget push package.nupkg # --help`,
	} {
		tc := tableCase{name: command, command: command, want: "deny"}
		got := recordCase(t, tc)
		assertCaseSemantics(t, tc, got)
		if !strings.Contains(got.Outcome.Payload(), "NuGet package publication") {
			t.Errorf("wrong denial for %s: %s", command, got.Outcome)
		}
	}
}

func TestNuGetPublicationControls(t *testing.T) {
	for _, command := range []string{
		`dotnet build`, `dotnet test`, `dotnet restore`, `dotnet pack`, `dotnet publish`,
		`nuget pack project.nuspec`, `nuget restore`, `nuget help push`,
		`dotnet nuget push --help`, `nuget push -Help`,
		`nuget pu -Help`, `nuget pus -?`, `nuget p package.nupkg`, `dotnet nuget pu package.nupkg`,
		`printf '%s' 'dotnet nuget push package.nupkg'`,
	} {
		tc := tableCase{name: command, command: command, want: "pass"}
		assertCaseSemantics(t, tc, recordCase(t, tc))
	}
}
