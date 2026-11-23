package releaseguard

import (
	"strings"
	"testing"
)

func TestOrchestratorMutations(t *testing.T) {
	for _, command := range []string{
		"terragrunt apply", "terragrunt destroy", "terragrunt run --all apply",
		"terragrunt run --all -- destroy", "terragrunt run-all apply", "terragrunt stack run destroy",
		"terragrunt --working-dir prod run --filter prod -- apply saved.plan",
		"terragrunt --terragrunt-config prod.hcl run-all destroy",
		"terragrunt run --config --help apply", "terragrunt apply --help=false",
		"terragrunt apply -var --help", "terragrunt apply --dry-run", "terragrunt -- apply",
		"helmfile apply", "helmfile sync", "helmfile delete", "helmfile destroy",
		"helmfile --file prod.yaml --environment prod --selector name=api sync",
		"helmfile --interactive destroy", "helmfile -f --help apply", "helmfile apply --args --help",
		"helmfile -- sync", "helmfile sync --dry-run", "helmfile apply --help=false",
		"env -- terragrunt run --all apply", "pwsh -Command 'helmfile sync'",
	} {
		t.Run(command, func(t *testing.T) {
			kind, message := bashCommand(t, command)
			category := "terragrunt_deploy"
			if strings.Contains(command, "helmfile") {
				category = "helmfile_deploy"
			}
			if kind != "deny" || !strings.Contains(message, category) {
				t.Fatalf("got %s %q; want %s denial", kind, message, category)
			}
		})
	}
}

func TestOrchestratorControls(t *testing.T) {
	assertEach(t, []string{
		"terragrunt plan -destroy", "terragrunt run --all -- plan", "terragrunt run-all validate",
		"terragrunt stack run plan", "terragrunt fmt", "terragrunt --help", "terragrunt apply --help",
		"helmfile diff", "helmfile --file prod.yaml template", "helmfile lint", "helmfile list",
		"helmfile --help", "helmfile sync --help", "printf '%s' 'helmfile apply'",
	}, "pass")
	assertEach(t, []string{
		"terragrunt --unknown x apply", "terragrunt run $OPERATION", "terragrunt --config",
		"helmfile --unknown x sync", "helmfile $OPERATION", "helmfile --file",
	}, "deny")
}
