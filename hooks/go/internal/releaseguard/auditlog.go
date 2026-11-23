package releaseguard

import (
	"github.com/muehz/agentics/hooks/go/internal/hookutil"
)

// guardLog owns where guard.log lives and how a record is appended; the
// record schema and the response to a failed append stay in this package.
var guardLog = hookutil.GuardLog{Guard: "release-guardian", EnvDir: "RELEASE_GUARD_LOG_DIR"}

// deployRecord is one guard.log line (hooks/README.md, Release-guardian
// log format). Field order is declaration order.
type deployRecord struct {
	Timestamp       string `json:"timestamp"`
	Event           string `json:"event"`
	Tool            string `json:"tool"`
	Executable      string `json:"executable"`
	Operation       string `json:"operation"`
	Category        string `json:"category"`
	EnvironmentRule string `json:"environment_rule,omitempty"`
	Reason          string `json:"reason,omitempty"`
	Record          string `json:"record"`
}

// writeAuditLog appends one record to guard.log under logRoot, creating the
// directory on the way. It is the only write the guard makes: once for a
// matched, authorized deploy (deploy_authorized) and once for an authorized
// pass through a wrapper the parser could not fully read
// (deploy_parse_limit_authorized). Run denies on a failed append, so the
// guard stays fail-closed.
func writeAuditLog(logRoot string, rec deployRecord) error {
	return guardLog.Append(logRoot, rec)
}
