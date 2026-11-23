// Package settings resolves the Agentics hook workflow settings: which workflow
// policy rules a guard enforces. Two optional files supply them, a repository
// file and a user file, both named .agentics/hooks.json. A missing file changes
// nothing, and a file that cannot be trusted makes every policy default apply,
// so a typo can never switch a rule off.
//
// The package reads files and returns values. It writes nothing and executes
// nothing, and it stays free of any guard-specific knowledge: a guard asks
// whether a Key is enabled and renders a Note into its own message.
package settings

import (
	"fmt"
	"slices"
	"strings"
)

// Key names one workflow policy setting, spelled as it is in the settings file
// and in a block message.
type Key string

// The policy keys of settings file version 1.
const (
	DraftPRs                    Key = "policy.draft_prs"
	BlockPackagePublish         Key = "policy.block_package_publish"
	RequireDeployRecord         Key = "policy.require_deploy_record"
	BlockReviewThreadChanges    Key = "policy.block_review_thread_changes"
	BlockSecurityAlertDismissal Key = "policy.block_security_alert_dismissal"
	CommandReformulation        Key = "policy.command_reformulation"
)

// Keys lists every key in the fixed order a message names them.
var Keys = []Key{
	DraftPRs,
	BlockPackagePublish,
	RequireDeployRecord,
	BlockReviewThreadChanges,
	BlockSecurityAlertDismissal,
	CommandReformulation,
}

// The error classes a Note reports. They name the kind of fault and never
// quote file contents or parser text.
const (
	ClassUnreadable   = "unreadable"
	ClassNotAFile     = "not-a-file"
	ClassTooLarge     = "too-large"
	ClassNotJSON      = "not-json"
	ClassTrailingData = "trailing-data"
	ClassDuplicateKey = "duplicate-key"
	ClassUnknownKey   = "unknown-key"
	ClassBadType      = "bad-type"
	ClassBadVersion   = "bad-version"
)

// Policy holds the effective value of each key. The zero value enables every
// key, which is the safe default.
type Policy map[Key]bool

// Defaults returns the policy with every key on.
func Defaults() Policy {
	p := make(Policy, len(Keys))
	for _, k := range Keys {
		p[k] = true
	}
	return p
}

// Enabled reports whether a key is on. A key the policy does not mention is on.
func (p Policy) Enabled(k Key) bool {
	v, ok := p[k]
	return !ok || v
}

// Invalid records one settings file that was ignored and why.
type Invalid struct {
	Path  string
	Class string
}

// Result is the outcome of resolving the settings for one event.
type Result struct {
	// Policy is the effective policy. It is the full default policy whenever
	// Invalid is not empty.
	Policy Policy
	// Invalid lists every file that was ignored. When it is not empty, no
	// override from any file applies.
	Invalid []Invalid
	// RepoPath is the repository settings file's path, or "" outside a
	// repository.
	RepoPath string
	// UserPath is the user settings file's path, or "" when the home
	// directory cannot be resolved.
	UserPath string
}

const userWhere = "~/.agentics/hooks.json"

// Note renders the lines a guard appends to a message about a policy rule: the
// keys that govern it, where the user changes them, and any settings file that
// was ignored. It returns "" when keys is empty.
func Note(res Result, keys []Key) string {
	ordered := make([]Key, 0, len(keys))
	for _, k := range Keys {
		if slices.Contains(keys, k) {
			ordered = append(ordered, k)
		}
	}
	if len(ordered) == 0 {
		return ""
	}
	names := make([]string, len(ordered))
	for i, k := range ordered {
		names[i] = string(k)
	}
	label, pronoun := "Setting", "it"
	if len(ordered) > 1 {
		label, pronoun = "Settings", "each"
	}
	repoWhere := res.RepoPath
	if repoWhere == "" {
		repoWhere = ".agentics/hooks.json in the repository root"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s: %s. To allow this, ask the user to set %s to false in %s or %s; an agent must not edit those files.",
		label, strings.Join(names, ", "), pronoun, repoWhere, userWhere)
	for _, bad := range res.Invalid {
		fmt.Fprintf(&b, "\nAgentics hook settings: ignored invalid %s (%s); every policy default applies.", bad.Path, bad.Class)
	}
	return b.String()
}
