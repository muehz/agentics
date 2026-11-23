package hooks

import (
	"errors"
	"strings"

	"github.com/muehz/agentics/hooks/go/internal/hookspec"
	"github.com/muehz/agentics/hooks/go/internal/secretsguard"
	"github.com/muehz/agentics/hooks/go/internal/surface"
	"github.com/muehz/agentics/hooks/go/internal/toolcall"
	"github.com/muehz/agentics/hooks/go/internal/toolname"
)

// Step is one rule set a check runs. Hook names a registry hook, so the hook
// registry answers for every rule set the check runs, and Label names the rule
// set in a fault message.
type Step struct {
	Label, Hook string
}

// EventCheck carries a check's execution metadata, independently of settings or policy.
type EventCheck struct {
	Name  string
	Guard bool
	// Steps lists the registry hooks the check runs, in order. The commands check
	// is event-only: it is not a hook name and has no registry key, so it
	// declares its command rules and then its release rules here. Every other
	// check is the registry hook of its own Name and has no steps.
	Steps []Step
}

// SelectEventChecks selects ordered checks from validated, provider-normalized input.
func SelectEventChecks(provider string, input hookspec.Event) ([]EventCheck, error) {
	event, err := input.Read()
	if err != nil || event.Tool.Name == nil {
		return nil, errors.New("hooks: grouped tool event is invalid")
	}
	if id, ok := surface.Parse(provider); !ok || event.Surface != string(id) {
		return nil, errors.New("hooks: grouped tool event surface does not match")
	}
	name := *event.Tool.Name
	var selected []EventCheck
	// Claude's command registration excludes the local prose-bearing tools. It
	// keeps the file-writing tools, which the guard judges by target path alone
	// to protect the hook settings files, and the namespaced tools of an MCP
	// server.
	//
	// The release rules must see every shell spelling, including namespaced
	// ones, so the selection is the command rules' condition or
	// toolname.IsShell(name). On claude-code the command rules therefore also
	// apply to those namespaced shell spellings, which is stricter than their
	// registration alone.
	if provider != "claude-code" || strings.HasPrefix(name, "mcp__") || toolcall.IsFileWrite(name) || toolname.IsShell(name) {
		selected = append(selected, EventCheck{Name: "commands", Guard: true, Steps: commandsSteps()})
	}
	if secretsguard.Inspects(name) {
		selected = append(selected, EventCheck{Name: "secrets-guardian", Guard: true})
	}
	return selected, nil
}

// commandsSteps returns the commands check's rule sets in the order they run.
// The slice is fresh on every call so no selection can alter another's.
func commandsSteps() []Step {
	return []Step{{"command rules", "command-guardian"}, {"release rules", "release-guardian"}}
}
