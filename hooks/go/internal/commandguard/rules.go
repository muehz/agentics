package commandguard

import "github.com/muehz/agentics/hooks/go/internal/settings"

// ruleID names one rule inside the package. It is never rendered: the deny
// reason, the context message, and the log carry the category, severity,
// match, and suggestion, so renaming an id changes no output.
type ruleID string

// rule is the identity of one detection: what it reports and which setting,
// if any, may switch it off. A detection's suggestion and regex are
// presentation; the rule is what a setting governs and what a dedupe keys on.
//
// governed is true exactly when policy names a setting. A rule with no policy
// is fixed safety, and no setting can switch it off.
type rule struct {
	id       ruleID
	category string
	severity string
	policy   settings.Key
	governed bool
}

// The parsed rules: detections an invocation-aware check reports.
var (
	ruleSettingsFileEdit      = rule{id: "settings-file-edit", category: settingsEditCategory, severity: "critical"}
	ruleGitHookFileEdit       = rule{id: "git-hook-file-edit", category: hookEditCategory, severity: "high"}
	ruleUnclassifiedTaskInput = rule{id: "unclassified-task-input", category: unclassifiedTaskInputCategory, severity: "high"}

	// ruleUninspectableInvocation is a command line the parser refused, so the
	// guard cannot read what it runs.
	ruleUninspectableInvocation = rule{id: "uninspectable-invocation", category: "command_parse_limit", severity: "high"}

	ruleGitClean            = rule{id: "git-clean", category: "destructive_git_ops", severity: "high"}
	ruleGitResetHard        = rule{id: "git-reset-hard", category: "destructive_git_ops", severity: "high"}
	ruleConfirmedHookBypass = rule{id: "git-hook-bypass", category: "git_hook_bypass", severity: "high"}

	// The four unknown hook effects: a Git invocation whose hook behaviour the
	// guard cannot settle, each named by what hides it.
	ruleUnknownHookPowerShellEnvironment = rule{id: "unknown-hook-effect-powershell-environment", category: "command_parse_limit", severity: "high"}
	ruleUnknownHookAlternateConfig       = rule{id: "unknown-hook-effect-alternate-config", category: "command_parse_limit", severity: "high"}
	ruleUnknownHookMutation              = rule{id: "unknown-hook-effect-mutation", category: "command_parse_limit", severity: "high"}
	ruleUnknownHookEffect                = rule{id: "unknown-hook-effect", category: "command_parse_limit", severity: "high"}

	ruleBroadRemoveItem    = rule{id: "broad-remove-item", category: "destructive_file_ops", severity: "critical"}
	ruleBroadRm            = rule{id: "broad-rm", category: "destructive_file_ops", severity: "critical"}
	ruleTargetedRmAdvice   = rule{id: "targeted-rm-advice", category: "command_reformulation", severity: severityInfo, policy: settings.CommandReformulation, governed: true}
	ruleBroadFindDelete    = rule{id: "broad-find-delete", category: "destructive_file_ops", severity: "high"}
	ruleFindDeleteAdvice   = rule{id: "find-delete-advice", category: "command_reformulation", severity: severityInfo, policy: settings.CommandReformulation, governed: true}
	ruleWorldWritableChmod = rule{id: "world-writable-chmod", category: "permission_abuse", severity: "high"}

	// ruleProtectedDeletion removes a .env file or a .git directory named by a
	// literal operand; ruleUninspectableDeletion is a deletion whose operands
	// or options the guard cannot read.
	ruleProtectedDeletion     = rule{id: "protected-deletion", category: "destructive_file_ops", severity: "critical"}
	ruleUninspectableDeletion = rule{id: "uninspectable-deletion", category: "command_parse_limit", severity: "high"}

	ruleDownloadExecute    = rule{id: "download-execute", category: "network_exfiltration", severity: "critical"}
	rulePowerShellDownload = rule{id: "powershell-download", category: "network_exfiltration", severity: "critical"}
	ruleFileUpload         = rule{id: "file-upload", category: "network_exfiltration", severity: "high"}

	ruleDestructiveSQL       = rule{id: "destructive-sql", category: "database_destruction", severity: "critical"}
	ruleScriptCallerArgument = rule{id: "script-caller-argument", category: "command_parse_limit", severity: "high"}

	// The four gh verbs that publish a pull request. Each is one rule, which
	// both the regex reading and the parsed reading of the verb report.
	ruleGhPrCreate    = rule{id: "gh-pr-create", category: "unsafe_publication", severity: "high", policy: settings.DraftPRs, governed: true}
	ruleGhPrReady     = rule{id: "gh-pr-ready", category: "unsafe_publication", severity: "high", policy: settings.DraftPRs, governed: true}
	ruleGhStackSubmit = rule{id: "gh-stack-submit", category: "unsafe_publication", severity: "high", policy: settings.DraftPRs, governed: true}
	ruleGhStackLink   = rule{id: "gh-stack-link", category: "unsafe_publication", severity: "high", policy: settings.DraftPRs, governed: true}

	ruleNpmPublish = rule{id: "npm-publish", category: "system_danger", severity: "high", policy: settings.BlockPackagePublish, governed: true}

	// ruleNonLiteralNuGet is a NuGet operation the guard cannot name. It stays
	// ungoverned: switching package publication off must not also let an
	// unreadable invocation through.
	ruleNonLiteralNuGet = rule{id: "non-literal-nuget", category: "unsafe_publication", severity: "high"}
	ruleNuGetPublish    = rule{id: "nuget-publish", category: "unsafe_publication", severity: "high", policy: settings.BlockPackagePublish, governed: true}

	// The gated GitHub MCP tools, one rule per entry of mcpRules.
	ruleMCPCreatePullRequest     = rule{id: "mcp-create-pull-request", category: "unsafe_publication", severity: "high", policy: settings.DraftPRs, governed: true}
	ruleMCPUpdatePullRequest     = rule{id: "mcp-update-pull-request", category: "unsafe_publication", severity: "high", policy: settings.DraftPRs, governed: true}
	ruleMCPDraftState            = rule{id: "mcp-update-pull-request-draft-state", category: "unsafe_publication", severity: "high", policy: settings.DraftPRs, governed: true}
	ruleMCPDeleteRepository      = rule{id: "mcp-delete-repository", category: "destructive_git_ops", severity: "critical"}
	ruleMCPResolveReviewThread   = rule{id: "mcp-resolve-review-thread", category: "unsafe_finding_disposal", severity: "high", policy: settings.BlockReviewThreadChanges, governed: true}
	ruleMCPUnresolveReviewThread = rule{id: "mcp-unresolve-review-thread", category: "unsafe_finding_disposal", severity: "high", policy: settings.BlockReviewThreadChanges, governed: true}
)

// The bank rules: one per row of buildThreatPatterns, in row order.
var (
	ruleBankRmRfRoot       = rule{id: "bank-rm-rf-root", category: "destructive_file_ops", severity: "critical"}
	ruleBankRmRfHome       = rule{id: "bank-rm-rf-home", category: "destructive_file_ops", severity: "critical"}
	ruleBankRmRfCurrentDir = rule{id: "bank-rm-rf-current-dir", category: "destructive_file_ops", severity: "critical"}
	ruleBankRmRfParentDir  = rule{id: "bank-rm-rf-parent-dir", category: "destructive_file_ops", severity: "critical"}
	ruleBankEnvDeletion    = rule{id: "bank-env-deletion", category: "destructive_file_ops", severity: "critical"}
	ruleBankGitDirDeletion = rule{id: "bank-git-dir-deletion", category: "destructive_file_ops", severity: "critical"}

	ruleBankDropTable           = rule{id: "bank-drop-table", category: "database_destruction", severity: "critical"}
	ruleBankDropDatabase        = rule{id: "bank-drop-database", category: "database_destruction", severity: "critical"}
	ruleBankTruncateTable       = rule{id: "bank-truncate-table", category: "database_destruction", severity: "critical"}
	ruleBankUnboundedDeleteFrom = rule{id: "bank-unbounded-delete-from", category: "database_destruction", severity: "high"}

	ruleBankChmod777          = rule{id: "bank-chmod-777", category: "permission_abuse", severity: "high"}
	ruleBankRecursiveChmod777 = rule{id: "bank-recursive-chmod-777", category: "permission_abuse", severity: "high"}

	ruleBankCurlPipeBash   = rule{id: "bank-curl-pipe-bash", category: "network_exfiltration", severity: "critical"}
	ruleBankWgetPipeShell  = rule{id: "bank-wget-pipe-shell", category: "network_exfiltration", severity: "critical"}
	ruleBankCurlDataUpload = rule{id: "bank-curl-data-upload", category: "network_exfiltration", severity: "high"}

	ruleBankSudo       = rule{id: "bank-sudo", category: "system_danger", severity: "high"}
	ruleBankNpmPublish = rule{id: "bank-npm-publish", category: "system_danger", severity: "high", policy: settings.BlockPackagePublish, governed: true}

	ruleBankRdDriveRoot  = rule{id: "bank-rd-drive-root", category: "destructive_file_ops", severity: "critical"}
	ruleBankDelDriveRoot = rule{id: "bank-del-drive-root", category: "destructive_file_ops", severity: "critical"}
	ruleBankFormatVolume = rule{id: "bank-format-volume", category: "destructive_file_ops", severity: "critical"}
	ruleBankClearDisk    = rule{id: "bank-clear-disk", category: "destructive_file_ops", severity: "critical"}
	ruleBankFormatDrive  = rule{id: "bank-format-drive", category: "destructive_file_ops", severity: "critical"}

	ruleBankRegDeleteMachineKey        = rule{id: "bank-reg-delete-machine-key", category: "registry_destruction", severity: "critical"}
	ruleBankRemoveRegistryKey          = rule{id: "bank-remove-registry-key", category: "registry_destruction", severity: "critical"}
	ruleBankRemoveMachineRegistryValue = rule{id: "bank-remove-machine-registry-value", category: "registry_destruction", severity: "critical"}

	ruleBankServiceDelete = rule{id: "bank-service-delete", category: "service_disruption", severity: "critical"}

	ruleBankAddLocalAdministrator = rule{id: "bank-add-local-administrator", category: "permission_abuse", severity: "critical"}
	ruleBankAddUser               = rule{id: "bank-add-user", category: "permission_abuse", severity: "high"}
	ruleBankDisableFirewall       = rule{id: "bank-disable-firewall", category: "permission_abuse", severity: "high"}
	ruleBankSystemExecutionPolicy = rule{id: "bank-system-execution-policy", category: "permission_abuse", severity: "high"}
	ruleBankFullControlGrant      = rule{id: "bank-full-control-grant", category: "permission_abuse", severity: "high"}

	ruleBankInvokeExpressionDownload = rule{id: "bank-invoke-expression-download", category: "network_exfiltration", severity: "critical"}

	ruleBankBootConfiguration = rule{id: "bank-boot-configuration", category: "system_danger", severity: "critical"}
	ruleBankDisableComputer   = rule{id: "bank-disable-computer", category: "system_danger", severity: "critical"}
	ruleBankForceRestart      = rule{id: "bank-force-restart", category: "system_danger", severity: "critical"}
	ruleBankForceShutdown     = rule{id: "bank-force-shutdown", category: "system_danger", severity: "critical"}

	ruleBankResolveReviewThread  = rule{id: "bank-resolve-review-thread", category: "unsafe_finding_disposal", severity: "high", policy: settings.BlockReviewThreadChanges, governed: true}
	ruleBankAlertDismissedState  = rule{id: "bank-alert-dismissed-state", category: "unsafe_finding_disposal", severity: "high", policy: settings.BlockSecurityAlertDismissal, governed: true}
	ruleBankAlertDismissedReason = rule{id: "bank-alert-dismissed-reason", category: "unsafe_finding_disposal", severity: "high", policy: settings.BlockSecurityAlertDismissal, governed: true}

	ruleBankFindExecGrep     = rule{id: "bank-find-exec-grep", category: "command_reformulation", severity: severityInfo, policy: settings.CommandReformulation, governed: true}
	ruleBankFindExecCat      = rule{id: "bank-find-exec-cat", category: "command_reformulation", severity: severityInfo, policy: settings.CommandReformulation, governed: true}
	ruleBankFindExecWc       = rule{id: "bank-find-exec-wc", category: "command_reformulation", severity: severityInfo, policy: settings.CommandReformulation, governed: true}
	ruleBankFindExecHead     = rule{id: "bank-find-exec-head", category: "command_reformulation", severity: severityInfo, policy: settings.CommandReformulation, governed: true}
	ruleBankFindExecSed      = rule{id: "bank-find-exec-sed", category: "command_reformulation", severity: severityInfo, policy: settings.CommandReformulation, governed: true}
	ruleBankFindDelete       = rule{id: "bank-find-delete", category: "command_reformulation", severity: severityInfo, policy: settings.CommandReformulation, governed: true}
	ruleBankRecursiveForceRm = rule{id: "bank-recursive-force-rm", category: "command_reformulation", severity: severityInfo, policy: settings.CommandReformulation, governed: true}
)

// declaredRules is every rule the package can report, parsed and bank alike.
// TestRuleTableIsClosed requires each bank row's rule to be listed here.
var declaredRules = []rule{
	ruleSettingsFileEdit,
	ruleGitHookFileEdit,
	ruleUnclassifiedTaskInput,
	ruleUninspectableInvocation,
	ruleGitClean,
	ruleGitResetHard,
	ruleConfirmedHookBypass,
	ruleUnknownHookPowerShellEnvironment,
	ruleUnknownHookAlternateConfig,
	ruleUnknownHookMutation,
	ruleUnknownHookEffect,
	ruleBroadRemoveItem,
	ruleBroadRm,
	ruleTargetedRmAdvice,
	ruleBroadFindDelete,
	ruleFindDeleteAdvice,
	ruleWorldWritableChmod,
	ruleProtectedDeletion,
	ruleUninspectableDeletion,
	ruleDownloadExecute,
	rulePowerShellDownload,
	ruleFileUpload,
	ruleDestructiveSQL,
	ruleScriptCallerArgument,
	ruleGhPrCreate,
	ruleGhPrReady,
	ruleGhStackSubmit,
	ruleGhStackLink,
	ruleNpmPublish,
	ruleNonLiteralNuGet,
	ruleNuGetPublish,
	ruleMCPCreatePullRequest,
	ruleMCPUpdatePullRequest,
	ruleMCPDraftState,
	ruleMCPDeleteRepository,
	ruleMCPResolveReviewThread,
	ruleMCPUnresolveReviewThread,

	ruleBankRmRfRoot,
	ruleBankRmRfHome,
	ruleBankRmRfCurrentDir,
	ruleBankRmRfParentDir,
	ruleBankEnvDeletion,
	ruleBankGitDirDeletion,
	ruleBankDropTable,
	ruleBankDropDatabase,
	ruleBankTruncateTable,
	ruleBankUnboundedDeleteFrom,
	ruleBankChmod777,
	ruleBankRecursiveChmod777,
	ruleBankCurlPipeBash,
	ruleBankWgetPipeShell,
	ruleBankCurlDataUpload,
	ruleBankSudo,
	ruleBankNpmPublish,
	ruleBankRdDriveRoot,
	ruleBankDelDriveRoot,
	ruleBankFormatVolume,
	ruleBankClearDisk,
	ruleBankFormatDrive,
	ruleBankRegDeleteMachineKey,
	ruleBankRemoveRegistryKey,
	ruleBankRemoveMachineRegistryValue,
	ruleBankServiceDelete,
	ruleBankAddLocalAdministrator,
	ruleBankAddUser,
	ruleBankDisableFirewall,
	ruleBankSystemExecutionPolicy,
	ruleBankFullControlGrant,
	ruleBankInvokeExpressionDownload,
	ruleBankBootConfiguration,
	ruleBankDisableComputer,
	ruleBankForceRestart,
	ruleBankForceShutdown,
	ruleBankResolveReviewThread,
	ruleBankAlertDismissedState,
	ruleBankAlertDismissedReason,
	ruleBankFindExecGrep,
	ruleBankFindExecCat,
	ruleBankFindExecWc,
	ruleBankFindExecHead,
	ruleBankFindExecSed,
	ruleBankFindDelete,
	ruleBankRecursiveForceRm,
}
