package releaseguard

import (
	"fmt"
	"strings"

	"github.com/muehz/agentics/hooks/go/internal/argv"
	"github.com/muehz/agentics/hooks/go/internal/shellwords"
)

// The orchestrator grammars match names exactly and never fold case: `--CONFIG` is outside the contract there.
var terragruntOptionRows = optionRows(
	[]string{"--working-dir", "--config", "--source", "--download-dir", "--tf-path", "--log-level", "--log-format", "--parallelism", "--filter", "--filters-file", "--queue-include-dir", "--queue-exclude-dir", "--terragrunt-working-dir", "--terragrunt-config", "--terragrunt-source", "--terragrunt-download-dir", "--terragrunt-tfpath", "--terragrunt-log-level", "--terragrunt-parallelism", "--terragrunt-include-dir", "--terragrunt-exclude-dir"},
	[]string{"--all", "--graph", "--non-interactive", "--no-color", "--source-update", "--no-auto-init", "--no-auto-approve", "--no-auto-retry", "--no-hooks", "--no-stack-generate", "--filter-affected", "--queue-include-external", "--queue-exclude-external", "--queue-ignore-errors", "--queue-ignore-dag-order", "--terragrunt-non-interactive", "--terragrunt-source-update", "--terragrunt-no-auto-init", "--terragrunt-ignore-external-dependencies"},
)

var terragruntGrammar = argv.NewSpec(terragruntOptionRows...)

var helmfileOptionRows = optionRows(
	[]string{"--file", "-f", "--environment", "-e", "--selector", "-l", "--namespace", "-n", "--kube-context", "--helm-binary", "--kustomize-binary", "--log-level", "--state-values-file", "--state-values-set", "--state-values-set-string", "--chart"},
	[]string{"--interactive", "-i", "--debug", "--quiet", "-q", "--no-color", "--allow-no-matching-release", "--enable-live-output", "--skip-refresh", "--skip-deps", "--sequential-helmfiles"},
)

var helmfileGrammar = argv.NewSpec(helmfileOptionRows...)

func matchOrchestrator(inv shellwords.Invocation, executable string) (deployShape, bool, error) {
	grammar := helmfileGrammar
	if executable == "terragrunt" {
		grammar = terragruntGrammar
	}
	parts, err := orchestratorCommandScan(inv.Words[1:], grammar)
	if err != nil {
		return deployShape{}, false, fmt.Errorf("%s %w", executable, err)
	}
	if inv.Complete && standaloneOrchestratorHelp(parts) {
		return deployShape{}, false, nil
	}
	if executable == "terragrunt" && len(parts) > 0 {
		if parts[0].Text == "stack" {
			parts, err = orchestratorCommandScan(parts[1:], grammar)
			if err != nil {
				return deployShape{}, false, fmt.Errorf("%s stack %w", executable, err)
			}
			if len(parts) == 0 || parts[0].Text != "run" {
				return deployShape{}, false, nil
			}
		}
		if len(parts) > 0 && (parts[0].Text == "run" || parts[0].Text == "run-all") {
			parts, err = orchestratorCommandScan(parts[1:], grammar)
			if err != nil {
				return deployShape{}, false, fmt.Errorf("%s run %w", executable, err)
			}
		}
	}
	if len(parts) == 0 {
		return deployShape{}, false, nil
	}
	verb := parts[0].Text
	mutating := verb == "apply" || verb == "destroy"
	if executable == "helmfile" {
		mutating = mutating || verb == "sync" || verb == "delete"
	}
	if !mutating || (inv.Complete && standaloneOrchestratorHelp(parts[1:])) {
		return deployShape{}, false, nil
	}
	return deployShape{executable + "_deploy", executable, verb}, true, nil
}

func standaloneOrchestratorHelp(words []shellwords.Word) bool {
	return len(words) == 1 && words[0].Literal && (words[0].Text == "--help" || words[0].Text == "-h")
}

// orchestratorCommandScan returns the words of the orchestrated command, or an error for the first word before it
// that cannot be read. A literal `--` forwards every word after it unless the next one is not literal, and a lone
// final `--help` or `-h` is the command. Each word is tested for being literal before its role, in order, and a
// value word must be literal and non-empty. The error quotes the option name without any `=` value.
func orchestratorCommandScan(words []shellwords.Word, spec argv.Spec) ([]shellwords.Word, error) {
	scan := spec.Scan(words)
	for _, arg := range scan.Args {
		if arg.Role == argv.RoleValue {
			continue
		}
		if !arg.Word.Literal {
			return nil, fmt.Errorf("non-literal option or operation")
		}
		if arg.Role == argv.RoleEnd {
			if next := arg.At + 1; next < len(words) && !words[next].Literal {
				return nil, fmt.Errorf("non-literal operation after --")
			}
			return words[arg.At+1:], nil
		}
		if standaloneOrchestratorHelp(words[arg.At:]) {
			return words[arg.At:], nil
		}
		name, _, _ := strings.Cut(arg.Word.Text, "=")
		switch arg.Role {
		case argv.RoleOperand:
			return words[arg.At:], nil
		case argv.RoleOption:
			if missingLiteralValue(scan, arg) {
				return nil, fmt.Errorf("%s is missing its literal value", name)
			}
		default:
			return nil, fmt.Errorf("option %s is outside the frozen parser contract", name)
		}
	}
	return nil, nil
}
