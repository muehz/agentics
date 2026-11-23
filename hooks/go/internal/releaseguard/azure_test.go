package releaseguard

import (
	"strings"
	"testing"
)

func TestAzureDeployments(t *testing.T) {
	for _, scope := range []string{"group", "sub", "mg", "tenant"} {
		for _, file := range []string{"main.bicep", "azuredeploy.json"} {
			command := "az deployment " + scope + " create --template-file " + file
			kind, message := bashCommand(t, command)
			if kind != "deny" || !strings.Contains(message, "azure_deploy") {
				t.Errorf("%s: got %s %q; want Azure deploy denial", command, kind, message)
			}
		}
	}
}

func TestAzurePowerShellDeployments(t *testing.T) {
	for _, cmdlet := range []string{"New-AzResourceGroupDeployment", "New-AzDeployment", "New-AzSubscriptionDeployment", "New-AzManagementGroupDeployment", "New-AzTenantDeployment", `Az.Resources\New-AzResourceGroupDeployment`} {
		kind, message := toolCommand(t, "PowerShell", cmdlet+" -TemplateFile main.bicep")
		if kind != "deny" || !strings.Contains(message, "azure_deploy") {
			t.Errorf("%s: got %s %q", cmdlet, kind, message)
		}
	}
}

func TestAzureControlsAndOperandBoundaries(t *testing.T) {
	assertEach(t, []string{
		`az --help`, `az -h`, `az --version`,
		`az deployment group what-if --template-file main.bicep`,
		`az deployment sub validate --template-file azuredeploy.json`,
		`az deployment group show -g demo -n rollout`,
		`az bicep build --file main.bicep`,
		`az deployment group create --help`,
		`printf '%s' 'az deployment group create --template-file main.bicep'`,
	}, "pass")
	assertEach(t, []string{
		`az --subscription demo deployment group create -g demo -f main.bicep`,
		`az deployment --subscription demo sub create -l eastus -f main.bicep`,
		`az deployment group create --confirm-with-what-if --template-file main.bicep`,
		`az deployment group create --parameters --help`,
		`az deployment group create --name=--help`,
		`az deployment group create --help=false`,
		`az deployment group create -- --help`,
		`az deployment group create # --help`,
		`az deployment group $OPERATION -f main.bicep`,
		`env -- az deployment tenant create -f azuredeploy.json`,
	}, "deny")
	assertEachTool(t, "PowerShell", []string{
		`New-AzResourceGroupDeployment -ResourceGroupName demo -TemplateFile main.bicep -WhatIf`,
		`New-AzDeployment -WhatIf -Location eastus -TemplateFile main.bicep`,
		`Test-AzResourceGroupDeployment -TemplateFile main.bicep`,
		`Get-AzResourceGroupDeploymentWhatIfResult -TemplateFile main.bicep`,
	}, "pass")
	assertEachTool(t, "PowerShell", []string{
		`New-AzResourceGroupDeployment -TemplateFile main.bicep -WhatIf:$false`,
		`New-AzResourceGroupDeployment -TemplateFile main.bicep '-WhatIf'`,
		`New-AzResourceGroupDeployment -TemplateFile main.bicep -WhatIf -WhatIf:$false`,
		`New-AzResourceGroupDeployment -TemplateFile '-WhatIf'`,
		`New-AzResourceGroupDeployment -CustomParameter '-WhatIf'`,
		`New-AzResourceGroupDeployment -TemplateFile main.bicep # -WhatIf`,
		`New-AzResourceGroupDeployment -Confirm:$false`,
	}, "deny")
}
