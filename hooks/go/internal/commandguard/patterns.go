package commandguard

import (
	"regexp"
	"sync"
)

// threatPattern is one row of the generic threat table.
//
// The rows are RE2 expressions over the scan text, which is the tool name
// followed by compact JSON, so a newline, carriage return or tab inside the
// data reaches a rule as the two-character escape \n, \r or \t. Conventions
// the rows share:
//
//   - Every row is case-insensitive: the (?i) prefix.
//   - [[:space:]] is the ASCII whitespace set; the rows that must also cross
//     a JSON-escaped newline or tab spell that as (?:[[:space:]]|\\[nrt]).
//   - The scan text always ends in one newline, so a rule that must match at
//     the end of the text uses the alternation (?:\n\z|\z).
//   - The two Windows drive-root rows need a boundary after the root that
//     RE2's lack of lookahead cannot express zero-width, so they consume it
//     inside a named group and find() trims the reported match back to the
//     group's start. The match text then stops at the boundary, as a reader
//     of the diagnostic table or the audit log expects.
//
// Category, severity and suggestion are the source of truth for the deny and
// context reasons. Handler tests exercise these rows through the observable
// command-guardian outcome instead of pinning this internal table's shape.
type threatPattern struct {
	category   string
	severity   string
	re         *regexp.Regexp
	suggestion string

	// trimNames are named groups holding boundary text the pattern consumed
	// that the match should not report; see find.
	trimNames []string

	// gate is a lowercase literal every match of re must contain, or empty.
	// scanThreatPatternBank skips the row when the folded scan text lacks
	// it; asciifold.String is exactly the fold (?i) applies to ASCII
	// letters, so the skip can never lose a match. Rows whose every match
	// has no such literal stay ungated. TestGateLiteralsAreSound checks it.
	gate string

	// rule is the row's identity; its category and severity equal the
	// row's. Each row has its own rule.
	rule rule

	// fallbackOnly marks a legacy row retained solely for an
	// execution-shaped input that has no decoded command field. A parsed
	// command is read by invocation-aware checks and skips these rows,
	// which prevents duplicate findings and quoted-prose false positives.
	fallbackOnly bool

	// superseded marks the .env and .git deletion rows, which the protected
	// deletion check replaces when it could read every deletion in the
	// command.
	superseded bool
}

// find returns the first match of the pattern in text, trimmed back to the
// start of the first present trimNames group.
func (p threatPattern) find(text string) string {
	if len(p.trimNames) == 0 {
		return p.re.FindString(text)
	}
	idx := p.re.FindStringSubmatchIndex(text)
	if idx == nil {
		return ""
	}
	end := idx[1]
	for _, name := range p.trimNames {
		gi := p.re.SubexpIndex(name)
		if gi >= 0 && 2*gi < len(idx) && idx[2*gi] >= 0 {
			end = idx[2*gi]
			break
		}
	}
	return text[idx[0]:end]
}

// severityInfo is the advisory tier: reported in the log and the diagnostic
// table, never blocking.
const severityInfo = "info"

// rmBroadTargetTerminator is the shared terminator class of the four anchored
// rm -rf rules: a quote, whitespace, a glob, a shell separator, a backslash
// (the start of a JSON-escaped newline), a substitution or grouping closer,
// or the end of the text.
const rmBroadTargetTerminator = `([*[:space:]"';&|\\)}` + "`" + `]|\n\z|\z)`

// windowsRootBoundary is the drive-root boundary set of the rd and del rules:
// what may follow `C:\` for the target to be the root itself.
const windowsRootBoundary = `(?:["[:space:]&|;<>)%,=]|\\[nrt])`

// windowsGap is the token gap of the Windows rules: whitespace or a
// JSON-escaped newline, carriage return or tab.
const windowsGap = `(?:[[:space:]]|\\[nrt])`

var (
	threatPatternOnce sync.Once
	threatPatternRows []threatPattern
)

// threatPatternBank builds the generic rule bank once, on first use. A
// process that never scans a command (release-guardian)
// then never compiles these rows. Severity decides the deny reason first
// (mostSevere): among rows of equal severity, order is contract — the first
// matching row of the winning severity names the reason. The categories are
// documented in hooks/README.md, Command-guardian threat categories.
func threatPatternBank() []threatPattern {
	threatPatternOnce.Do(func() {
		threatPatternRows = buildThreatPatterns()
	})
	return threatPatternRows
}

func buildThreatPatterns() []threatPattern {
	return []threatPattern{
		{"destructive_file_ops", "critical",
			regexp.MustCompile(`(?i)rm -rf /` + rmBroadTargetTerminator),
			"Remove only specific, verified paths instead of the filesystem root. If the user still wants to delete the root, tell the user that agents cannot perform this action and ask them to run it manually in their own terminal.", nil, "rm -rf /", ruleBankRmRfRoot, true, false},
		{"destructive_file_ops", "critical",
			regexp.MustCompile(`(?i)rm -rf ~` + rmBroadTargetTerminator),
			"Remove only specific, verified paths instead of the home directory. If the user still wants to delete the entire home directory, tell the user that agents cannot perform this action and ask them to run it manually in their own terminal.", nil, "rm -rf ~", ruleBankRmRfHome, true, false},
		{"destructive_file_ops", "critical",
			regexp.MustCompile(`(?i)rm -rf \.` + rmBroadTargetTerminator),
			"Remove only specific, verified paths instead of the current directory. If the user still wants to delete the entire current directory, tell the user that agents cannot perform this action and ask them to run it manually in their own terminal.", nil, "rm -rf .", ruleBankRmRfCurrentDir, true, false},
		{"destructive_file_ops", "critical",
			regexp.MustCompile(`(?i)rm -rf \.\.` + rmBroadTargetTerminator),
			"Remove only specific, verified paths instead of a parent directory. If the user still wants to delete the parent directory recursively, tell the user that agents cannot perform this action and ask them to run it manually in their own terminal.", nil, "rm -rf .", ruleBankRmRfParentDir, true, false},
		{"destructive_file_ops", "critical",
			regexp.MustCompile(`(?i)\b(rm|del|unlink)\b.*\.env\b`),
			"Move the .env file to a backup location instead of removing it.", nil, ".env", ruleBankEnvDeletion, false, true},
		{"destructive_file_ops", "critical",
			regexp.MustCompile(`(?i)\b(rm|del|unlink)\b.*\.git\b`),
			"Do not delete the .git directory. Use Git commands to manage repository state.", nil, ".git", ruleBankGitDirDeletion, false, true},

		// Destructive git operations (git reset --hard, git clean) are bespoke checks in git.go.

		{"database_destruction", "critical",
			regexp.MustCompile(`(?i)DROP(?:[[:space:]]|\\[nrt])+TABLE`),
			"Use 'ALTER TABLE' or create a reversible migration instead of dropping the table.", nil, "drop", ruleBankDropTable, false, false},
		{"database_destruction", "critical",
			regexp.MustCompile(`(?i)DROP(?:[[:space:]]|\\[nrt])+DATABASE`),
			"Create a verified backup and use a reversible migration instead of dropping the database.", nil, "drop", ruleBankDropDatabase, false, false},
		{"database_destruction", "critical",
			regexp.MustCompile(`(?i)TRUNCATE(?:[[:space:]]|\\[nrt])+(TABLE|ONLY)`),
			"Use 'DELETE FROM ... WHERE' with a specific condition instead of truncating the table.", nil, "truncate", ruleBankTruncateTable, true, false},
		{"database_destruction", "high",
			regexp.MustCompile(`(?i)DELETE(?:[[:space:]]|\\[nrt])+FROM(?:[[:space:]]|\\[nrt])+[a-zA-Z_]+(?:[[:space:]]|\\[nrt])*;`),
			"Add a specific WHERE clause to 'DELETE FROM' instead of deleting every row.", nil, "delete", ruleBankUnboundedDeleteFrom, true, false},

		{"permission_abuse", "high",
			regexp.MustCompile(`(?i)chmod 777`),
			"Use 'chmod 755' for directories or 'chmod 644' for files instead.", nil, "chmod 777", ruleBankChmod777, true, false},
		{"permission_abuse", "high",
			regexp.MustCompile(`(?i)chmod -R 777`),
			"Use specific permissions such as 'chmod -R 755' and limit the operation to verified paths.", nil, "chmod -r 777", ruleBankRecursiveChmod777, true, false},

		{"network_exfiltration", "critical",
			regexp.MustCompile(`(?i)curl.*\|.*bash`),
			"Download the script to a file, inspect it, and run it only after verifying its contents.", nil, "curl", ruleBankCurlPipeBash, true, false},
		{"network_exfiltration", "critical",
			regexp.MustCompile(`(?i)wget.*\|.*sh`),
			"Download the script to a file, inspect it, and run it only after verifying its contents.", nil, "wget", ruleBankWgetPipeShell, true, false},
		{"network_exfiltration", "high",
			regexp.MustCompile(`(?i)curl.*--data.*@`),
			"Inspect the file and destination before sending data with 'curl --data @file'.", nil, "curl", ruleBankCurlDataUpload, true, false},

		{"system_danger", "high",
			regexp.MustCompile(`(?i)sudo(?:[[:space:]]|\\[nrt])+`),
			"Avoid 'sudo' and run the command with the least privilege required.", nil, "sudo", ruleBankSudo, false, false},
		{"system_danger", "high",
			regexp.MustCompile(`(?i)npm publish`),
			npmPublishSuggestion, nil, "npm publish", ruleBankNpmPublish, true, false},

		// Windows: destructive file operations. See the type comment for the
		// boundary groups the rd and del rows trim.
		{category: "destructive_file_ops", severity: "critical",
			re: regexp.MustCompile(`(?i)rd` + windowsGap + `+` +
				`(?:` +
				`(?:/[A-Za-z]+` + windowsGap + `*)*?/s(?:` + windowsGap + `*/[A-Za-z]+)*` + windowsGap + `+[A-Z]:\\\\(?P<rootBoundary>` + windowsRootBoundary + `)` +
				`|` +
				`[A-Z]:\\\\` + windowsGap + `+(?:/[A-Za-z]+` + windowsGap + `*)*?/s(?P<flagBoundary>[^A-Za-z]|\z)` +
				`)`),
			suggestion: "Do not recursively delete a drive root. Tell the user that agents cannot perform this action and ask them to run the deletion manually in their own terminal if they still want to proceed.",
			trimNames:  []string{"rootBoundary", "flagBoundary"},
			gate:       "rd",
			rule:       ruleBankRdDriveRoot},
		{category: "destructive_file_ops", severity: "critical",
			re: regexp.MustCompile(`(?i)del` + windowsGap + `+` +
				`(?:` +
				`(?:/[A-Za-z]+` + windowsGap + `*)*?/s(?:` + windowsGap + `*/[A-Za-z]+)*` + windowsGap + `+[A-Z]:\\\\(?P<rootBoundary>` + windowsRootBoundary + `)` +
				`|` +
				`[A-Z]:\\\\` + windowsGap + `+(?:/[A-Za-z]+` + windowsGap + `*)*?/s(?P<flagBoundary>[^A-Za-z]|\z)` +
				`)`),
			suggestion: "Do not force-delete all files from a drive root. Tell the user that agents cannot perform this action and ask them to run the deletion manually in their own terminal if they still want to proceed.",
			trimNames:  []string{"rootBoundary", "flagBoundary"},
			gate:       "del",
			rule:       ruleBankDelDriveRoot},
		{"destructive_file_ops", "critical",
			regexp.MustCompile(`(?i)Format-Volume`),
			"Do not format a volume. Tell the user that agents cannot perform this action and ask them to format it manually in Windows Disk Management if they still want to proceed.", nil, "format-volume", ruleBankFormatVolume, false, false},
		{"destructive_file_ops", "critical",
			regexp.MustCompile(`(?i)Clear-Disk`),
			"Do not wipe disk partitions. Tell the user that agents cannot perform this action and ask them to clear the disk manually in Windows Disk Management if they still want to proceed.", nil, "clear-disk", ruleBankClearDisk, false, false},
		{"destructive_file_ops", "critical",
			regexp.MustCompile(`(?i)format [A-Z][:]`),
			"Do not format a drive. Tell the user that agents cannot perform this action and ask them to format it manually in Windows Disk Management if they still want to proceed.", nil, "format ", ruleBankFormatDrive, false, false},

		{"registry_destruction", "critical",
			regexp.MustCompile(`(?i)reg delete.*HKLM.*/f`),
			"Do not force-delete machine-wide registry keys. Tell the user that agents cannot perform this action and ask them to make the change manually in Registry Editor if they still want to proceed.", nil, "reg delete", ruleBankRegDeleteMachineKey, false, false},
		{"registry_destruction", "critical",
			regexp.MustCompile(`(?i)Remove-Item.*Registry[:][:] `),
			"Do not delete an entire registry key. Use a targeted registry edit instead. If the user still wants to delete the key, tell the user that agents cannot perform this action and ask them to make the change manually in Registry Editor.", nil, "remove-item", ruleBankRemoveRegistryKey, false, false},
		{"registry_destruction", "critical",
			regexp.MustCompile(`(?i)Remove-ItemProperty.*HKLM`),
			"Do not modify machine-wide registry values. Tell the user that agents cannot perform this action and ask them to make the change manually in Registry Editor if they still want to proceed.", nil, "remove-itemproperty", ruleBankRemoveMachineRegistryValue, false, false},

		{"service_disruption", "critical",
			regexp.MustCompile(`(?i)sc delete`),
			"Do not permanently delete a Windows service registration. Tell the user that agents cannot perform this action and ask them to remove the service manually with Windows administrative tooling if they still want to proceed.", nil, "sc delete", ruleBankServiceDelete, false, false},

		{"permission_abuse", "critical",
			regexp.MustCompile(`(?i)net localgroup administrators.*/add`),
			"Do not grant a user administrator privileges. Tell the user that agents cannot perform this action and ask them to change the account manually in Windows account settings if they still want to proceed.", nil, "net localgroup administrators", ruleBankAddLocalAdministrator, false, false},
		{"permission_abuse", "high",
			regexp.MustCompile(`(?i)net user.*/add`),
			"Do not create an operating-system user account. Tell the user that agents cannot perform this action and ask them to create the account manually in Windows account settings if they still want to proceed.", nil, "net user", ruleBankAddUser, false, false},
		{"permission_abuse", "high",
			regexp.MustCompile(`(?i)netsh advfirewall.*disable`),
			"Do not disable Windows Firewall. Tell the user that agents cannot perform this action and ask them to change the firewall state manually in Windows Security if they still want to proceed.", nil, "netsh advfirewall", ruleBankDisableFirewall, false, false},
		{"permission_abuse", "high",
			regexp.MustCompile(`(?i)Set-ExecutionPolicy.*(Unrestricted|Bypass)`),
			"Use the per-process '-ExecutionPolicy Bypass' flag instead of changing the system policy.", nil, "set-executionpolicy", ruleBankSystemExecutionPolicy, false, false},
		{"permission_abuse", "high",
			regexp.MustCompile(`(?i)icacls.*/grant.*:F`),
			"Avoid granting Full Control and use a least-privilege ACL instead.", nil, "icacls", ruleBankFullControlGrant, false, false},

		{"network_exfiltration", "critical",
			regexp.MustCompile(`(?i)Invoke-Expression.*(Invoke-WebRequest|iwr)`),
			"Download the content to a file, inspect it, and run it only after verifying its contents.", nil, "invoke-expression", ruleBankInvokeExpressionDownload, true, false},

		{"system_danger", "critical",
			regexp.MustCompile(`(?i)bcdedit`),
			"Do not modify the boot configuration. Tell the user that agents cannot perform this action and ask them to make the change manually with Windows recovery or administrative tooling if they still want to proceed.", nil, "bcdedit", ruleBankBootConfiguration, false, false},
		{"system_danger", "critical",
			regexp.MustCompile(`(?i)Disable-Computer`),
			"Do not disable the computer account. Tell the user that agents cannot perform this action and ask them to make the change manually with Windows administrative tooling if they still want to proceed.", nil, "disable-computer", ruleBankDisableComputer, false, false},
		{"system_danger", "critical",
			regexp.MustCompile(`(?i)Restart-Computer.*-Force`),
			"Do not force-restart the machine. Tell the user that agents cannot perform this action and ask them to restart it manually if they still want to proceed.", nil, "restart-computer", ruleBankForceRestart, false, false},
		{"system_danger", "critical",
			regexp.MustCompile(`(?i)Stop-Computer.*-Force`),
			"Do not force-shut down the machine. Tell the user that agents cannot perform this action and ask them to shut it down manually if they still want to proceed.", nil, "stop-computer", ruleBankForceShutdown, false, false},

		{"unsafe_finding_disposal", "high",
			regexp.MustCompile(`(?i)resolveReviewThread`),
			reviewThreadSuggestion, nil, "resolvereviewthread", ruleBankResolveReviewThread, false, false},
		{"unsafe_finding_disposal", "high",
			regexp.MustCompile(`(?i)(code[-_]scanning|dependabot)[/_]alerts?.*[^?&]state=[\\"']*dismissed|[^?&]state=[\\"']*dismissed.*(code[-_]scanning|dependabot)[/_]alerts?`),
			alertDismissalSuggestion, nil, "dismissed", ruleBankAlertDismissedState, false, false},
		{"unsafe_finding_disposal", "high",
			regexp.MustCompile(`(?i)(code[-_]scanning|dependabot)[/_]alerts?.*dismissed_reason[\\"[:space:]]*[=:]|dismissed_reason[\\"[:space:]]*[=:].*(code[-_]scanning|dependabot)[/_]alerts?`),
			alertDismissalSuggestion, nil, "dismissed_reason", ruleBankAlertDismissedReason, false, false},

		{"command_reformulation", severityInfo,
			regexp.MustCompile(`(?i)find.*-exec.*grep`),
			`Use 'grep -r --include="<ext>" "<pattern>" <path>' instead of 'find -exec grep' for simpler permissions`, nil, "find", ruleBankFindExecGrep, false, false},
		{"command_reformulation", severityInfo,
			regexp.MustCompile(`(?i)find.*-exec.*cat`),
			"Use the Read tool to read matched files instead of 'find -exec cat'", nil, "find", ruleBankFindExecCat, false, false},
		{"command_reformulation", severityInfo,
			regexp.MustCompile(`(?i)find.*-exec.*wc`),
			"Use 'find ... | xargs wc' instead of 'find -exec wc'", nil, "find", ruleBankFindExecWc, false, false},
		{"command_reformulation", severityInfo,
			regexp.MustCompile(`(?i)find.*-exec.*head`),
			"Use the Read tool with offset/limit instead of 'find -exec head'", nil, "find", ruleBankFindExecHead, false, false},
		{"command_reformulation", severityInfo,
			regexp.MustCompile(`(?i)find.*-exec.*sed`),
			"Use the Edit tool for file modifications instead of 'find -exec sed'", nil, "find", ruleBankFindExecSed, false, false},
		{"command_reformulation", severityInfo,
			regexp.MustCompile(`(?i)find.*-delete`),
			"Use targeted removal on specific known paths instead of 'find -delete'", nil, "find", ruleBankFindDelete, true, false},
		{"command_reformulation", severityInfo,
			regexp.MustCompile(`(?i)rm -(rf|fr)[[:space:]]`),
			"In a git repo, prefer 'git clean -f <path>' over 'rm -rf' for non-ignored untracked files — pathspec-aware and visible to this guard. Ignored paths (node_modules, build caches) are skipped by 'git clean -f' and its -x/-X forms are blocked, so targeted 'rm -rf' remains the right tool for those", nil, "rm -", ruleBankRecursiveForceRm, true, false},
	}
}
