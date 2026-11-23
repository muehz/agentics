// Package hookutil holds the small helpers every hook core needs and none of
// them owns: the comma-list splitter for allowlists and GuardLog, which owns
// where a guard's guard.log lives and how a record is appended. GuardLog
// appends through the unexported appendOwnedLog, which keeps the log from
// being redirected through a symlink or Windows junction.
//
// It sits beside contract in the dependency graph — cores import both — and
// carries no hook-specific knowledge.
package hookutil

import "strings"

// SplitCommaList splits an allowlist value on commas. Empty input is nil, a
// single trailing comma is dropped, and every other empty field is kept, so
// "a,b," is [a b] while ",a" is ["" a].
func SplitCommaList(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	if parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	return parts
}
