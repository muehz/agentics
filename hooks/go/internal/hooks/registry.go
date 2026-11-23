// Package hooks is the registry: it maps a hook's wire name to the handler
// that runs it. The surfaces that may invoke one are named by internal/surface.
//
// This is the only place in the binary where hook names are written down.
// cmd/hook never names a hook, so adding or renaming one never touches the
// entry point. The keys are wire names — the strings the provider hook
// configs pass on the command line — and they are independent of the Go
// package that serves them.
package hooks

import (
	"maps"
	"slices"

	"github.com/muehz/agentics/hooks/go/internal/commandguard"
	"github.com/muehz/agentics/hooks/go/internal/hookspec"
	"github.com/muehz/agentics/hooks/go/internal/releaseguard"
	"github.com/muehz/agentics/hooks/go/internal/secretsguard"
)

// registry maps a hook name to its handler: the three guardians Agentics ships.
//
// A handler returns the outcome it reached, never an exit code. Mapping an
// outcome to a surface's exit code belongs to internal/provider, because the
// same outcome exits 2 on claude-code and 0 on copilot.
var registry = map[string]hookspec.Handler{
	"command-guardian": commandguard.Run,
	"release-guardian": releaseguard.Run,
	"secrets-guardian": secretsguard.Run,
}

// Lookup resolves a hook name to its handler.
func Lookup(name string) (hookspec.Handler, bool) {
	h, ok := registry[name]
	return h, ok
}

// Names returns every registered hook name, sorted, for diagnostics.
func Names() []string {
	return slices.Sorted(maps.Keys(registry))
}
