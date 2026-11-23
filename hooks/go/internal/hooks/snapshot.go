package hooks

import (
	"github.com/muehz/agentics/hooks/go/internal/scriptsnap"
	"github.com/muehz/agentics/hooks/go/internal/secretsguard"
)

// NewScriptSnapshot builds the one script snapshot a request's guards share.
// Every entry that runs a guard builds it here, so each refuses the same
// credential paths before any read.
func NewScriptSnapshot() *scriptsnap.Snapshot {
	return scriptsnap.New(scriptsnap.WithPathPolicy(secretsguard.ProtectedScriptPath))
}
