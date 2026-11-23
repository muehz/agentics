package releaseguard

import (
	"errors"
	"strings"

	"github.com/muehz/agentics/hooks/go/internal/argv"
	"github.com/muehz/agentics/hooks/go/internal/shellwords"
)

var azureGlobalOptionRows = optionRows(
	[]string{"--subscription", "--output", "-o", "--query"},
	[]string{"--debug", "--verbose", "--only-show-errors"},
)

var azureGlobalGrammar = argv.NewSpec(azureGlobalOptionRows...).With(argv.FoldNames)

func matchAzure(inv shellwords.Invocation) (deployShape, bool, error) {
	if inv.Complete && len(inv.Words) == 2 && inv.Words[1].Literal && names("--help", "-h", "--version")[inv.Words[1].Text] {
		return deployShape{}, false, nil
	}
	parts, reason := leadingCommandScan(inv.Words, 1, azureGlobalGrammar)
	if reason != "" {
		return deployShape{}, false, errors.New("az " + reason)
	}
	if len(parts) == 0 || parts[0].Text != "deployment" {
		return deployShape{}, false, nil
	}
	parts, reason = leadingCommandScan(parts, 1, azureGlobalGrammar)
	if reason != "" {
		return deployShape{}, false, errors.New("az deployment " + reason)
	}
	if len(parts) == 0 || !names("group", "sub", "mg", "tenant")[parts[0].Text] {
		return deployShape{}, false, nil
	}
	scope := parts[0].Text
	parts, reason = leadingCommandScan(parts, 1, azureGlobalGrammar)
	if reason != "" {
		return deployShape{}, false, errors.New("az deployment " + scope + " " + reason)
	}
	if len(parts) == 0 || parts[0].Text != "create" {
		return deployShape{}, false, nil
	}
	// Only a standalone help request is exempt: deployment parameters can
	// consume help-looking strings, and confirmation-with-what-if still deploys.
	if inv.Complete && len(parts) == 2 && parts[1].Literal && (parts[1].Text == "--help" || parts[1].Text == "-h") {
		return deployShape{}, false, nil
	}
	return deployShape{"azure_deploy", "az deployment " + scope, "create"}, true, nil
}

var azurePowerShellValuesRows = optionRows([]string{
	"-name", "-resourcegroupname", "-location", "-managementgroupid", "-templatefile", "-templateuri",
	"-templateparameterfile", "-templateparameteruri", "-mode", "-deploymentname", "-templateobject",
	"-templateparameterobject", "-templateversion", "-templatespecid", "-querystring", "-defaultprofile",
}, nil)

// azurePowerShellValuesSpec folds names because PowerShell parameter names are case-insensitive.
var azurePowerShellValuesSpec = argv.NewSpec(azurePowerShellValuesRows...).With(argv.FoldNames)

// azureWhatIfScan reports a complete cmdlet line that carries -WhatIf and nothing else but the known value
// parameters, each with a literal value that does not start with a dash, and the known switches. Every word must
// be literal and unquoted, because a quoted switch is a positional argument to PowerShell. A parameter joined with
// `=` is no PowerShell form, so it answers false, as does every other word.
func azureWhatIfScan(args []shellwords.Word, complete bool) bool {
	if !complete {
		return false
	}
	seen := false
	for _, arg := range azurePowerShellValuesSpec.Scan(args).Args {
		if arg.Role == argv.RoleValue {
			continue
		}
		if !arg.Word.Literal || arg.Word.Quoted {
			return false
		}
		switch arg.Role {
		case argv.RoleOption:
			if arg.Joined || !arg.HasValue || !arg.Value.Literal || strings.HasPrefix(arg.Value.Text, "-") {
				return false
			}
			continue
		case argv.RoleUnknown:
			name := strings.ToLower(arg.Word.Text)
			if name == "-whatif" {
				seen = true
				continue
			}
			if names("-verbose", "-debug", "-asjob", "-force", "-skiptemplateparameterprompt")[name] {
				continue
			}
		}
		return false
	}
	return seen
}

func matchAzurePowerShell(inv shellwords.Invocation, executable string) (deployShape, bool, error) {
	if azureWhatIfScan(inv.Words[1:], inv.Complete) {
		return deployShape{}, false, nil
	}
	return deployShape{"azure_deploy", executable, "deploy"}, true, nil
}
