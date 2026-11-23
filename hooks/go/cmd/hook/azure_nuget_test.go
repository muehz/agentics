package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/surface"
)

func TestAzureAndNuGetProviderCoverage(t *testing.T) {
	for _, surface := range surface.All() {
		t.Run(string(surface), func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("AGX_RELEASE_RECORD", "")
			t.Setenv("RELEASE_GUARD_LOG_DIR", filepath.Join(root, "logs"))
			for _, tc := range []struct{ hook, tool, command, reason string }{
				{"release-guardian", "Bash", `az deployment group create -g demo -f main.bicep`, "azure_deploy"},
				{"release-guardian", "Bash", `az deployment sub create -l eastus -f main.json`, "azure_deploy"},
				{"release-guardian", "Bash", `az deployment mg create -m demo -l eastus -f main.bicep`, "azure_deploy"},
				{"release-guardian", "Bash", `az deployment tenant create -l eastus -f main.json`, "azure_deploy"},
				{"release-guardian", "PowerShell", `Az.Resources\New-AzResourceGroupDeployment -TemplateFile main.bicep`, "azure_deploy"},
				{"release-guardian", "PowerShell", `New-AzDeployment -TemplateFile main.json -WhatIf:$false`, "azure_deploy"},
				{"release-guardian", "Bash", `az deployment group what-if -g demo -f main.bicep`, ""},
				{"release-guardian", "PowerShell", `New-AzResourceGroupDeployment -TemplateFile main.bicep -WhatIf`, ""},
				{"release-guardian", "Bash", `az bicep build --file main.bicep`, ""},
				{"command-guardian", "Bash", `dotnet nuget push package.nupkg`, "NuGet package publication"},
				{"command-guardian", "PowerShell", `nuget.exe push package.nupkg -Source private`, "NuGet package publication"},
				{"command-guardian", "Bash", `env -- dotnet nuget push package.nupkg --source --help`, "NuGet package publication"},
				{"command-guardian", "Bash", `dotnet pack`, ""},
				{"command-guardian", "Bash", `dotnet publish`, ""},
				{"command-guardian", "Bash", `dotnet nuget push --help`, ""},
			} {
				t.Run(tc.command, func(t *testing.T) {
					tool, command := tc.tool, tc.command
					if (surface == "codex" || surface == "antigravity") && tool == "PowerShell" {
						tool, command = "Bash", "pwsh -Command '"+command+"'"
					}
					payload := secretsShellStdin(surface, root, tool, command)
					code, out, diagnostic := invokeWithLegalArgs(t, payload, surface, tc.hook)
					if tc.reason == "" {
						requireSilent(t, surface, code, out, diagnostic)
						return
					}
					requireDeny(t, surface, code, out, diagnostic)
					if !strings.Contains(out+diagnostic, tc.reason) {
						t.Fatalf("wrong reason: %s %s", out, diagnostic)
					}
				})
			}
			record := filepath.Join(root, "change.md")
			if err := os.WriteFile(record, []byte("# Change Record: synthetic verification\n"), 0600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("AGX_RELEASE_RECORD", record)
			payload := secretsShellStdin(surface, root, "Bash", `az deployment group create -g demo -f main.bicep`)
			code, out, diagnostic := invokeWithLegalArgs(t, payload, surface, "release-guardian")
			requireSilent(t, surface, code, out, diagnostic)
			log, err := os.ReadFile(filepath.Join(root, "logs", "guard.log"))
			if err != nil || !strings.Contains(string(log), `"category":"azure_deploy"`) {
				t.Fatalf("Azure audit: %s (%v)", log, err)
			}
			payload = secretsShellStdin(surface, root, "Bash", `dotnet nuget push package.nupkg`)
			code, out, diagnostic = invokeWithLegalArgs(t, payload, surface, "command-guardian")
			requireDeny(t, surface, code, out, diagnostic)
		})
	}
}
