package commandguard

import (
	"context"
	"runtime"
	"strings"

	"github.com/muehz/agentics/hooks/go/internal/argv"
	"github.com/muehz/agentics/hooks/go/internal/shellwords"
	"github.com/muehz/agentics/hooks/go/internal/toolcall"
)

// hookBypassSuggestion is the deny text for every Git hook bypass form.
// Hooks are a repository's last check before a commit or push lands, so
// the agent is sent back to the hook's failure, not to a narrower bypass.
const hookBypassSuggestion = "Git hooks are the repository's last check before a commit or push lands. Fix what the hook reports instead of skipping it. If the user still wants to skip hooks, tell the user that agents cannot bypass Git hooks and ask them to run the command in their own terminal."

type gitHookIssueKind uint8

const (
	noGitHookIssue gitHookIssueKind = iota
	confirmedGitHookBypass
	unknownGitHookEffect
)

// gitHookIssue is one finding of the Git hook readers: its kind, the match the
// report shows, and the rule it is reported under. Only the constructors below
// build a non-zero issue, so every issue carries the rule of its effect; the
// zero value means no issue.
type gitHookIssue struct {
	kind  gitHookIssueKind
	match string
	rule  rule
}

// bypassIssue is a confirmed Git hook bypass. Every match is reported under the
// one bypass rule, so the match is the only thing that differs.
func bypassIssue(match string) gitHookIssue {
	return gitHookIssue{confirmedGitHookBypass, match, ruleConfirmedHookBypass}
}

// powerShellEnvironmentIssue is a PowerShell environment write the guard could
// not read.
func powerShellEnvironmentIssue() gitHookIssue {
	return gitHookIssue{unknownGitHookEffect, powerShellEnvironmentMatch, ruleUnknownHookPowerShellEnvironment}
}

// alternateGitConfigIssue is a change to where Git reads its configuration that
// the guard could not read.
func alternateGitConfigIssue() gitHookIssue {
	return gitHookIssue{unknownGitHookEffect, alternateGitConfigMatch, ruleUnknownHookAlternateConfig}
}

// gitMutationIssue is a Git configuration mutation the guard could not read.
func gitMutationIssue() gitHookIssue {
	return gitHookIssue{unknownGitHookEffect, gitMutationMatch, ruleUnknownHookMutation}
}

// unknownEffectIssue is an unknown effect with no rule of its own, reported
// under the general unknown-effect rule with its match.
func unknownEffectIssue(match string) gitHookIssue {
	return gitHookIssue{unknownGitHookEffect, match, ruleUnknownHookEffect}
}

// gitIssueSuggestion is the suggestion of an unknown-effect rule. A rule it does
// not name gets the general inspection suggestion, as an unrecognized match did,
// so no message is ever blank. A confirmed bypass is reported with
// hookBypassSuggestion and never reaches it.
func gitIssueSuggestion(r rule) string {
	switch r.id {
	case ruleUnknownHookPowerShellEnvironment.id:
		return powerShellEnvironmentSuggestion
	case ruleUnknownHookAlternateConfig.id:
		return alternateGitConfigSuggestion
	case ruleUnknownHookMutation.id:
		return gitMutationSuggestion
	}
	return gitHookInspectionSuggestion
}

const gitHookInspectionSuggestion = "The guard could not inspect the Git configuration section operation. Use literal section names and supported options so it can determine whether repository hooks are affected."

const alternateGitConfigMatch = "alternate Git configuration"
const alternateGitConfigSuggestion = "The guard could not inspect alternate Git configuration. Use the established Git configuration sources so it can determine whether repository hooks are affected."
const gitMutationMatch = "Git configuration mutation"
const gitMutationSuggestion = "The guard could not inspect the Git configuration mutation. Use literal configuration keys and supported options so it can determine whether repository hooks are affected."

func preferGitIssue(a, b gitHookIssue) gitHookIssue {
	if b.kind == confirmedGitHookBypass || a.kind == noGitHookIssue {
		return b
	}
	return a
}

func alternateGitConfigName(name string, windows bool) bool {
	if windows {
		name = strings.ToUpper(name)
	}
	return name == "GIT_CONFIG_GLOBAL" || name == "GIT_CONFIG_SYSTEM"
}

// gitConfigurationDiagnosticSource recognizes contexts whose values must not
// be copied by any finding's diagnostic match. This is output provenance only:
// it grants no data exemption and never changes the source seen by policy.
func gitConfigurationDiagnosticSource(ctx context.Context, parsed shellwords.Result) bool {
	relevantName := func(name string) bool {
		return alternateGitConfigName(name, runtime.GOOS == "windows") || strings.EqualFold(name, "GIT_CONFIG_PARAMETERS") || powerShellKeyEnvironment(name)
	}
	assignments := func(words []shellwords.Word) bool {
		for _, word := range words {
			if ctx.Err() != nil {
				return true
			}
			name, _, assigned := strings.Cut(word.Text, "=")
			if assigned && relevantName(name) {
				return true
			}
		}
		return false
	}
	if assignments(parsed.BareAssignments) {
		return true
	}
	for _, effect := range parsed.EnvironmentEffects {
		if ctx.Err() != nil {
			return true
		}
		if effect.Kind == shellwords.EnvironmentAssignment && assignments([]shellwords.Word{effect.Word}) {
			return true
		}
	}
	for _, inv := range parsed.Invocations {
		if ctx.Err() != nil {
			return true
		}
		if assignments(inv.Assignments) {
			return true
		}
		if call := inv.EnvironmentCall; call != nil && (len(call.Arguments) == 0 || relevantName(call.Arguments[0])) {
			return true
		}
		if len(inv.Words) == 0 {
			continue
		}
		head := inv.Words[0]
		// An unresolved environment-expression head can carry a value even
		// when the complete mutation grammar could not be established.
		if strings.HasPrefix(strings.ToLower(head.Raw), "$env:") || strings.HasPrefix(strings.ToLower(head.Raw), "${env:") {
			return true
		}
		if !head.Literal {
			continue
		}
		switch strings.ToLower(shellwords.ExecutableName(head.Text)) {
		case "env", "export", "declare", "typeset":
			// Failed env unwrapping retains argument words rather than lifted
			// assignments. Their explicit names still establish output privacy,
			// even when unknown options prevent a complete execution proof.
			if assignments(inv.Words[1:]) {
				return true
			}
		case "set-item", "remove-item":
			if inv.Dialect != shellwords.PowerShell {
				continue
			}
			for _, word := range inv.Words[1:] {
				if ctx.Err() != nil {
					return true
				}
				if strings.HasPrefix(strings.ToLower(word.Text), "env:") {
					name := strings.TrimPrefix(word.Text[4:], `\`)
					if relevantName(name) || !word.Literal {
						return true
					}
				}
			}
		case "git":
			if gitLineIsPrivate(ctx, argv.ReadGit(inv.Words[1:])) {
				return true
			}
		}
	}
	return ctx.Err() != nil
}

// gitLineIsPrivate reports a git line, read from the words after `git`, whose
// words may carry configuration values. Every -c and --config-env global makes the line private, with
// or without its value. A line read to its verb is private when the verb is
// `config` in any letter case. A line the grammar cannot read, or one that
// only prints, is private whenever words follow the point where the reading
// stopped, because those words may still set or show configuration. A
// canceled ctx makes the line private, so a deadline never exposes a value;
// ctx is polled once per global, which bounds the work on a long option run.
func gitLineIsPrivate(ctx context.Context, g argv.Git) bool {
	for _, global := range g.Globals {
		if ctx.Err() != nil {
			return true
		}
		if global.Name == "-c" || global.Name == "--config-env" {
			return true
		}
	}
	if g.State == argv.GitVerb {
		return g.Subcommand() == "config"
	}
	return len(g.Rest) > 0
}

func gitEnvironmentEffectsIssue(ctx context.Context, effects []shellwords.EnvironmentEffect) gitHookIssue {
	var issue gitHookIssue
	for _, effect := range effects {
		if ctx.Err() != nil {
			return preferGitIssue(issue, gitMutationIssue())
		}
		switch effect.Kind {
		case shellwords.EnvironmentAssignment:
			issue = preferGitIssue(issue, hooksPathCandidate([]shellwords.Word{effect.Word}))
		case shellwords.EnvironmentResetAll:
			issue = preferGitIssue(issue, alternateGitConfigIssue())
		case shellwords.EnvironmentUnset:
			if !effect.Word.Literal || effect.Word.Text == "" || alternateGitConfigName(effect.Word.Text, runtime.GOOS == "windows") {
				issue = preferGitIssue(issue, alternateGitConfigIssue())
			}
		}
	}
	return issue
}

// alternateGitConfigOverride reads only canonical caller argument layers;
// inherited process configuration is deliberately not consulted. It reads the
// lenient layers, Layers and not ProofLayers, so an unreadable envelope still
// yields the selector evidence at the two recognized locations, and it ignores
// override uncertainty, so a variant or malformed container still yields its
// keys. windows selects the host's letter-case rule for variable names.
func alternateGitConfigOverride(call toolcall.Call, windows bool) bool {
	for _, name := range call.Environment().Names() {
		if alternateGitConfigName(name, windows) {
			return true
		}
	}
	return false
}

// inspectGitHookBypass keeps uninspectable configuration mutations distinct
// from operations whose literal arguments establish a hook bypass. isGit is
// readGitInvocation's head test and g its reading of the words after the
// head. The -c and --config-env specs are read on every git-headed line,
// inspectable or not and whatever its state, because a runtime-selected or
// unreadable subcommand still follows global options that set
// core.hooksPath. The config and verb checks read arguments, so they run only
// on an inspectable line read to its verb.
func inspectGitHookBypass(ctx context.Context, inv shellwords.Invocation, g argv.Git, isGit bool) gitHookIssue {
	issue := hooksPathAssignment(ctx, inv)
	if !isGit {
		return issue
	}
	issue = preferGitIssue(issue, gitHooksPathOverride(g))
	if !inv.Inspectable || g.State != argv.GitVerb {
		return issue
	}
	verb, args := g.Subcommand(), g.Args
	if verb == "config" {
		issue = preferGitIssue(issue, scanHooksPathConfigSection(args))
		issue = preferGitIssue(issue, scanHooksPathConfigWrite(args))
	}
	if match, found := scanVerbHookBypass(verb, args); found {
		return bypassIssue(match)
	}
	return issue
}

// gitHooksPathOverride reports a -c or --config-env global option that sets
// core.hooksPath; a key the shell would still expand fails closed. It reads
// the -c and --config-env specs through ConfigSpecs, whose lenient walk
// stops at the first word that is neither an option nor an option's value, so
// it finds the same specs on a line read to its verb and on one the grammar
// cannot read. A spec missing its word is an unknown mutation, never a pass.
func gitHooksPathOverride(g argv.Git) gitHookIssue {
	var issue gitHookIssue
	specs, missing := g.ConfigSpecs()
	for _, spec := range specs {
		key, _, _ := strings.Cut(spec.Text, "=")
		if isHooksPathKey(key) {
			return bypassIssue("git -c core.hooksPath")
		}
		if !spec.Literal && dynamicText(key) {
			issue = gitMutationIssue()
		}
	}
	if missing {
		return preferGitIssue(issue, gitMutationIssue())
	}
	return issue
}

// hooksPathAssignment reports an environment assignment that sets
// core.hooksPath through GIT_CONFIG_KEY_<n> or GIT_CONFIG_PARAMETERS. It
// reads command-prefix and env-wrapper assignments, export, declare, and
// typeset arguments, and PowerShell $env: writes. A value the shell would
// still expand fails closed.
func hooksPathAssignment(ctx context.Context, inv shellwords.Invocation) gitHookIssue {
	issue := powerShellEnvironmentWrite(ctx, inv)
	for _, reset := range inv.EnvironmentResets {
		if reset.All || !reset.Name.Literal || reset.Name.Text == "" || alternateGitConfigName(reset.Name.Text, runtime.GOOS == "windows") {
			issue = preferGitIssue(issue, alternateGitConfigIssue())
		}
	}
	issue = preferGitIssue(issue, posixEnvironmentReset(inv))
	candidates := append([]shellwords.Word(nil), inv.Assignments...)
	if len(inv.Words) > 0 {
		head := inv.Words[0].Text
		switch {
		case strings.EqualFold(head, "export") || strings.EqualFold(head, "declare") || strings.EqualFold(head, "typeset"):
			candidates = append(candidates, inv.Words[1:]...)
			for _, word := range inv.Words[1:] {
				if alternateGitConfigName(word.Text, runtime.GOOS == "windows") {
					issue = preferGitIssue(issue, alternateGitConfigIssue())
				}
			}
		case inv.Dialect != shellwords.PowerShell && len(head) > len("$env:") && strings.EqualFold(head[:len("$env:")], "$env:"):
			// Legacy mixed-syntax input retains a conservative deny, but only
			// an unquoted mutation head/operator is evidence of a write. It
			// cannot establish PowerShell execution or a confirmed bypass.
			rawHead := inv.Words[0].Raw
			if len(rawHead) < len("$env:") || !strings.EqualFold(rawHead[:len("$env:")], "$env:") {
				break
			}
			assignment := head[len("$env:"):]
			name, _, joined := strings.Cut(assignment, "=")
			rawName, _, rawJoined := strings.Cut(rawHead[len("$env:"):], "=")
			if rawName != name || joined != rawJoined {
				break
			}
			rest := inv.Words[1:]
			if !joined && (len(rest) == 0 || !strings.HasPrefix(rest[0].Raw, "=")) {
				break
			}
			if !joined && strings.HasPrefix(rest[0].Text, "=") {
				assignment += rest[0].Text
				rest = rest[1:]
			}
			if strings.HasSuffix(assignment, "=") && len(rest) > 0 {
				assignment += rest[0].Text
			}
			legacy := hooksPathCandidate([]shellwords.Word{{Text: assignment, Literal: !dynamicText(assignment)}})
			if legacy.kind == confirmedGitHookBypass {
				legacy = gitMutationIssue()
			}
			issue = preferGitIssue(issue, legacy)
		}
	}
	return preferGitIssue(issue, hooksPathCandidate(candidates))
}

func posixEnvironmentReset(inv shellwords.Invocation) gitHookIssue {
	if inv.Dialect != shellwords.POSIX || len(inv.Words) == 0 || !inv.Words[0].Literal {
		return gitHookIssue{}
	}
	head := inv.Words[0].Text
	reset := head == "unset"
	if !reset && head != "export" && head != "declare" && head != "typeset" {
		return gitHookIssue{}
	}
	var names []shellwords.Word
	uncertain, end, functions, variables := false, false, false, false
	for _, word := range inv.Words[1:] {
		if !end && word.Literal && word.Text == "--" {
			end = true
			continue
		}
		if !end && strings.HasPrefix(word.Text, "-") || !end && strings.HasPrefix(word.Text, "+") {
			switch word.Text {
			case "-f":
				if head == "unset" {
					functions = true
				} else {
					uncertain = true
				}
			case "-v":
				if head == "unset" {
					variables = true
				} else {
					uncertain = true
				}
			case "-n":
				if head == "export" {
					reset = true
				} else {
					uncertain = true
				}
			case "+x":
				if head == "declare" || head == "typeset" {
					reset = true
				} else {
					uncertain = true
				}
			default:
				uncertain = true
			}
			continue
		}
		names = append(names, word)
	}
	if !reset {
		return gitHookIssue{}
	}
	if functions && !variables && !uncertain {
		return gitHookIssue{}
	}
	uncertain = uncertain || functions && variables
	if uncertain {
		return alternateGitConfigIssue()
	}
	for _, name := range names {
		text, _, _ := strings.Cut(name.Text, "=")
		if !name.Literal || alternateGitConfigName(text, runtime.GOOS == "windows") {
			return alternateGitConfigIssue()
		}
	}
	return gitHookIssue{}
}

// hooksPathCandidate distinguishes proven hook overrides from unresolved
// configuration effects, without putting environment values in diagnostics.
func hooksPathCandidate(candidates []shellwords.Word) gitHookIssue {
	var issue gitHookIssue
	for _, word := range candidates {
		name, value, ok := strings.Cut(word.Text, "=")
		if !ok {
			continue
		}
		upper := strings.ToUpper(name)
		switch {
		case alternateGitConfigName(name, runtime.GOOS == "windows"):
			issue = preferGitIssue(issue, alternateGitConfigIssue())
		case upper == "GIT_CONFIG_PARAMETERS":
			if strings.Contains(strings.ToLower(value), "core.hookspath") {
				return bypassIssue("GIT_CONFIG_PARAMETERS hook override")
			}
			if !word.Literal && dynamicText(value) {
				issue = preferGitIssue(issue, gitMutationIssue())
			}
		case strings.HasPrefix(upper, "GIT_CONFIG_KEY_"):
			if isHooksPathKey(value) {
				return bypassIssue("GIT_CONFIG_KEY hook override")
			}
			if !word.Literal && dynamicText(value) {
				issue = preferGitIssue(issue, gitMutationIssue())
			}
		}
	}
	return issue
}

// isHooksPathKey compares case-insensitively because Git config section
// and variable names are case-insensitive.
func isHooksPathKey(key string) bool { return strings.EqualFold(key, "core.hooksPath") }

// dynamicText reports text the shell would still expand.
func dynamicText(text string) bool { return strings.ContainsAny(text, "$`") }

// Option lists as `git config [<subcommand>] -h` prints them (git 2.55).
// "[no-]" marks a negatable option and a trailing "=" one that takes a
// value. The negated spellings are listed because they compete when Git
// resolves an abbreviation.
const (
	configLocationOptions = "[no-]global [no-]system [no-]local [no-]worktree [no-]file= [no-]blob="
	configTypeOptions     = "[no-]type= bool int bool-or-int bool-or-str path expiry-date"
	configDisplayOptions  = "[no-]null [no-]name-only [no-]show-origin [no-]show-scope [no-]show-names"
)

// isNoVerifyAbbrev matches --no-verify and every prefix that could resolve
// to it, down to --no-v: Git accepts an unambiguous abbreviation of a long
// option, so --no-verif skips hooks too. Prefixes Git rejects as ambiguous
// with --no-verbose are denied as well, since Git refuses them anyway.
func isNoVerifyAbbrev(text string) bool {
	return len(text) >= len("--no-v") && strings.HasPrefix("--no-verify", text)
}

// gitHelpRows reads a `git <command> -h` option list into grammar rows, in
// the notation of the git config lists above: a trailing "=" marks an option that takes a
// value, and "[no-]" adds the negation, which takes none. The negations are
// rows because they compete when git resolves an abbreviation.
func gitHelpRows(help string) []argv.Option {
	var rows []argv.Option
	for _, field := range strings.Fields(help) {
		arity := argv.ArityFlag
		if name, ok := strings.CutSuffix(field, "="); ok {
			field, arity = name, argv.ArityValue
		}
		if name, negatable := strings.CutPrefix(field, "[no-]"); negatable {
			rows = append(rows, argv.Option{Name: "--" + name, Arity: arity}, argv.Option{Name: "--no-" + name, Arity: argv.ArityFlag})
			continue
		}
		rows = append(rows, argv.Option{Name: "--" + field, Arity: arity})
	}
	return rows
}

// gitLetterRows gives each letter its single-letter row.
func gitLetterRows(letters string, arity argv.Arity) []argv.Option {
	rows := make([]argv.Option, 0, len(letters))
	for _, letter := range []byte(letters) {
		rows = append(rows, argv.Option{Name: "-" + string(letter), Arity: arity})
	}
	return rows
}

// gitVerbRows joins a verb's long options with its short letters: a value
// letter takes the rest of the cluster or the next word, an attached letter
// takes only the rest of the cluster, and a flag letter lets the cluster go
// on.
func gitVerbRows(help, value, attached, flags string) []argv.Option {
	rows := gitHelpRows(help)
	rows = append(rows, gitLetterRows(value, argv.ArityValue)...)
	rows = append(rows, gitLetterRows(attached, argv.ArityJoined)...)
	return append(rows, gitLetterRows(flags, argv.ArityFlag)...)
}

// hookBypassVerbOptions are the grammars of the verbs whose -h output lists
// --no-verify, transcribed from git 2.55's command-specific -h options,
// including non-value options that compete with value abbreviations.
// Optional long values are attached-only, so they are flag rows: gpg-sign,
// untracked-files, log, recurse-submodules (pull only), rebase, signoff (pull
// only), jobs, rebase-merges, show-current-patch, force-with-lease and signed
// therefore have no trailing "=". Each verb's letters are its value letters,
// which take the rest of a cluster or the next word, its attached letters,
// which never take the next word, and the non-value letters from its help;
// a letter outside them cannot establish a value role later in the cluster.
// Only commit and am give -n the no-verify meaning. Other subcommands reject
// --no-verify, and matching it there would block searches such as
// `git grep -e --no-verify`, so they have no grammar here.
var hookBypassVerbOptions = map[string][]argv.Option{
	"commit": gitVerbRows("[no-]quiet [no-]verbose [no-]file= [no-]author= [no-]date= [no-]message= [no-]reedit-message= [no-]reuse-message= [no-]fixup= [no-]squash= [no-]reset-author [no-]trailer= [no-]signoff [no-]template= [no-]edit [no-]cleanup= [no-]status [no-]gpg-sign [no-]all [no-]include [no-]interactive [no-]patch unified= inter-hunk-context= [no-]only no-verify verify [no-]dry-run [no-]short [no-]branch [no-]ahead-behind [no-]porcelain [no-]long [no-]null [no-]amend no-post-rewrite post-rewrite [no-]untracked-files [no-]pathspec-from-file= [no-]pathspec-file-nul allow-empty allow-empty-message", "mFCctU", "uS", "qvseaiponz"),
	"am":     gitVerbRows("[no-]interactive no-verify verify [no-]3way [no-]quiet [no-]signoff [no-]utf8 [no-]keep [no-]keep-non-patch [no-]message-id [no-]keep-cr [no-]scissors quoted-cr= [no-]whitespace= [no-]ignore-space-change [no-]ignore-whitespace [no-]directory= [no-]exclude= [no-]include= [no-]patch-format= [no-]reject [no-]resolvemsg= continue resolved skip abort quit show-current-patch retry allow-empty [no-]committer-date-is-author-date [no-]ignore-date [no-]rerere-autoupdate [no-]gpg-sign empty=", "Cp", "S", "in3qsukmcr"),
	"merge":  gitVerbRows("[no-]stat [no-]summary [no-]compact-summary [no-]log [no-]squash [no-]commit [no-]edit [no-]cleanup= [no-]ff ff-only [no-]rerere-autoupdate [no-]verify-signatures [no-]strategy= [no-]strategy-option= [no-]message= file= [no-]into-name= [no-]verbose [no-]quiet [no-]abort [no-]quit [no-]continue [no-]allow-unrelated-histories [no-]progress [no-]gpg-sign [no-]autostash [no-]overwrite-ignore [no-]signoff no-verify verify", "mFsX", "S", "nevq"),
	"pull":   gitVerbRows("[no-]verbose [no-]quiet [no-]progress [no-]recurse-submodules [no-]rebase [no-]stat [no-]compact-summary [no-]log [no-]signoff [no-]squash [no-]commit [no-]edit [no-]cleanup= [no-]ff ff-only [no-]verify [no-]verify-signatures [no-]autostash [no-]strategy= [no-]strategy-option= [no-]gpg-sign [no-]allow-unrelated-histories [no-]all [no-]append [no-]upload-pack= [no-]force [no-]tags [no-]prune [no-]jobs [no-]dry-run [no-]keep [no-]depth= [no-]shallow-since= [no-]shallow-exclude= [no-]deepen= unshallow [no-]update-shallow refmap= [no-]server-option= [no-]ipv4 [no-]ipv6 [no-]negotiation-restrict= [no-]negotiation-tip= [no-]negotiation-include= [no-]show-forced-updates [no-]set-upstream", "sXo", "rjS", "vqnaftpk46"),
	"push":   gitVerbRows("[no-]verbose [no-]quiet [no-]repo= [no-]all [no-]branches [no-]mirror [no-]delete [no-]tags [no-]dry-run [no-]porcelain [no-]force [no-]force-with-lease [no-]force-if-includes [no-]recurse-submodules= [no-]thin [no-]receive-pack= [no-]exec= [no-]set-upstream [no-]progress [no-]prune no-verify verify [no-]follow-tags [no-]signed [no-]atomic [no-]push-option= ipv4 ipv6", "o", "", "vqdnfu46"),
	"rebase": gitVerbRows("[no-]onto= [no-]keep-base no-verify verify [no-]quiet [no-]verbose no-stat stat [no-]trailer= [no-]signoff [no-]committer-date-is-author-date [no-]reset-author-date [no-]ignore-whitespace [no-]whitespace= [no-]force-rebase no-ff ff continue skip abort quit edit-todo show-current-patch apply merge interactive [no-]rerere-autoupdate empty= [no-]autosquash [no-]update-refs [no-]gpg-sign [no-]autostash [no-]exec= [no-]rebase-merges [no-]fork-point [no-]strategy= [no-]strategy-option= [no-]root [no-]reschedule-failed-exec [no-]reapply-cherry-picks", "CxsX", "rS", "qvnfmi"),
}

// gitOptionSpecs builds the git-option grammars. They read every dash word by
// its text, literal or not, because the walkers they replace never tested
// Literal: skipping a word such as `-n$X` would stop denying it. Long names
// resolve the way git resolves them, and an unresolved long name consumes
// nothing, so an unknown spelling can never swallow a later bypass.
func gitOptionSpecs(options map[string][]argv.Option, traits ...argv.Trait) map[string]argv.Spec {
	specs := make(map[string]argv.Spec, len(options))
	for key, rows := range options {
		specs[key] = argv.NewSpec(rows...).With(append([]argv.Trait{argv.Abbreviations, argv.ReadNonLiteral}, traits...)...)
	}
	return specs
}

// hookBypassVerbSpecs read clusters letter by letter; a letter that is no row
// ends the cluster, as git rejects it.
var hookBypassVerbSpecs = gitOptionSpecs(hookBypassVerbOptions, argv.Clusters)

// gitSkipsHooksByLetter reports a short-option word that git commit and git
// am read as -n. A cluster's Letters stop at the first letter that ends it,
// so an n after a value, attached or unknown letter is not read; a word that
// is exactly -n carries its name instead.
func gitSkipsHooksByLetter(arg argv.Arg) bool {
	return arg.Role != argv.RoleValue && (arg.Name == "-n" || strings.IndexByte(arg.Letters, 'n') >= 0)
}

// scanVerbHookBypass reports the argument that makes a git verb skip its
// hooks: a spelling of --no-verify, the -n of commit or am, or send-email's
// negation of --validate.
func scanVerbHookBypass(verb string, args []shellwords.Word) (string, bool) {
	if verb == "send-email" {
		return scanSendEmailHookBypass(args)
	}
	spec, ok := hookBypassVerbSpecs[verb]
	if !ok {
		return "", false
	}
	noVerifyLetter := verb == "commit" || verb == "am"
	for _, arg := range spec.Scan(args).Args {
		switch {
		case arg.Role == argv.RoleEnd:
			return "", false
		case arg.Role == argv.RoleValue:
			continue
		case isNoVerifyAbbrev(arg.Word.Text), noVerifyLetter && gitSkipsHooksByLetter(arg):
			return "git " + verb + " " + arg.Word.Text, true
		}
	}
	return "", false
}

// sendEmailPassOptions are the grammars of send-email's three Getopt::Long
// passes; send-email uses them rather than Git's parse-options. Required
// string values consume the next word even when it resembles an option.
// Optional strings and numeric values cannot consume another option.
var sendEmailPassOptions = [][]argv.Option{
	gitHelpRows("identity= no-identity"),
	gitHelpRows("h dump-aliases translate-aliases"),
	gitHelpRows("sender= from= in-reply-to= reply-to= subject= to= to-cmd= no-to cc= no-cc bcc= no-bcc chain-reply-to no-chain-reply-to nochain-reply-to sendmail-cmd= smtp-server= smtp-server-option= smtp-server-port= smtp-user= smtp-pass smtp-ssl smtp-encryption= smtp-ssl-cert-path= smtp-ssl-client-cert= smtp-ssl-client-key= smtp-debug smtp-domain smtp-auth= no-smtp-auth imap-sent-folder= use-imap-only no-use-imap-only nouse-imap-only annotate no-annotate noannotate compose quiet cc-cmd= header-cmd= no-header-cmd suppress-from no-suppress-from nosuppress-from suppress-cc= signed-off-cc no-signed-off-cc nosigned-off-cc signed-off-by-cc no-signed-off-by-cc nosigned-off-by-cc cc-cover no-cc-cover nocc-cover to-cover no-to-cover noto-cover confirm= dry-run envelope-sender= thread no-thread nothread validate no-validate novalidate transfer-encoding= mailmap no-mailmap nomailmap use-mailmap no-use-mailmap nouse-mailmap format-patch no-format-patch noformat-patch 8bit-encoding= compose-encoding= force xmailer no-xmailer noxmailer batch-size relogin-delay git-completion-helper v= outlook-id-fix no-outlook-id-fix nooutlook-id-fix"),
}

var sendEmailPassSpecs = func() []argv.Spec {
	specs := make([]argv.Spec, len(sendEmailPassOptions))
	for i, rows := range sendEmailPassOptions {
		specs[i] = argv.NewSpec(rows...).With(argv.Abbreviations, argv.ReadNonLiteral)
	}
	return specs
}()

// sendEmailOptionWord spells a send-email option the way its grammar's rows
// are spelled. Getopt::Long reads a name after `--`, `-` or `+` in any letter
// case, so the word becomes `--` and the lower-cased name. A lone `-` or `+`
// keeps its text: rewritten, it would read as the `--` that ends the options.
func sendEmailOptionWord(word shellwords.Word) shellwords.Word {
	text := word.Text
	var name string
	switch {
	case text == "-" || text == "+" || text == "--":
		return word
	case strings.HasPrefix(text, "--"):
		name = text[2:]
	case strings.HasPrefix(text, "-") || strings.HasPrefix(text, "+"):
		name = text[1:]
	default:
		return word
	}
	word.Text = "--" + strings.ToLower(name)
	return word
}

// scanSendEmailHookBypass reports a send-email argument that skips the hooks.
// Each pass reads the words the earlier passes left: operands, words no row of
// this pass names, and everything from the `--` on. Only a bare negation of
// --validate skips the hooks; an attached value makes git reject the option.
func scanSendEmailHookBypass(args []shellwords.Word) (string, bool) {
	for _, spec := range sendEmailPassSpecs {
		spelled := make([]shellwords.Word, len(args))
		for i, word := range args {
			spelled[i] = sendEmailOptionWord(word)
		}
		var remaining []shellwords.Word
	pass:
		for _, arg := range spec.Scan(spelled).Args {
			switch arg.Role {
			case argv.RoleEnd:
				remaining = append(remaining, args[arg.At:]...)
				break pass
			case argv.RoleOperand, argv.RoleUnknown:
				remaining = append(remaining, args[arg.At])
			case argv.RoleOption:
				if !arg.Joined && (arg.Name == "--no-validate" || arg.Name == "--novalidate") {
					return "git send-email " + args[arg.At].Text, true
				}
			}
		}
		args = remaining
	}
	return "", false
}

// configGitOptions are git config's grammars per subcommand, with the key ""
// for the legacy syntax, from the -h lists above. The write
// reader reads -f and -t as value letters on every subcommand it knows; after
// remove-section or rename-section git reads only -f.
var configGitOptions = map[string][]argv.Option{
	"":               append(gitHelpRows(configLocationOptions+" get get-all get-regexp get-urlmatch replace-all add unset unset-all rename-section remove-section list edit get-color get-colorbool "+configDisplayOptions+" "+configTypeOptions+" [no-]default= [no-]comment= [no-]fixed-value [no-]includes"), gitLetterRows("ft", argv.ArityValue)...),
	"set":            append(gitHelpRows(configLocationOptions+" "+configTypeOptions+" [no-]all [no-]value= [no-]fixed-value [no-]comment= [no-]append"), gitLetterRows("ft", argv.ArityValue)...),
	"unset":          append(gitHelpRows(configLocationOptions+" [no-]all [no-]value= [no-]fixed-value"), gitLetterRows("ft", argv.ArityValue)...),
	"get":            append(gitHelpRows(configLocationOptions+" [no-]all [no-]regexp [no-]value= [no-]fixed-value [no-]url= "+configDisplayOptions+" "+configTypeOptions+" [no-]includes [no-]default="), gitLetterRows("ft", argv.ArityValue)...),
	"remove-section": append(gitHelpRows(configLocationOptions), gitLetterRows("f", argv.ArityValue)...),
	"rename-section": append(gitHelpRows(configLocationOptions), gitLetterRows("f", argv.ArityValue)...),
}

// configGitSpecs read a cluster letter that is no row as a flag, because the
// config walkers skipped letters they did not know; the section reader then
// counts each one as uncertain.
var configGitSpecs = gitOptionSpecs(configGitOptions, argv.Clusters, argv.AnyLetter)

// configValueRows names the value rows of each config grammar, so a reader
// can tell a value option that ends the line from a flag.
var configValueRows = func() map[string]map[string]bool {
	values := make(map[string]map[string]bool, len(configGitOptions))
	for key, rows := range configGitOptions {
		values[key] = map[string]bool{}
		for _, row := range rows {
			if row.Arity == argv.ArityValue {
				values[key][row.Name] = true
			}
		}
	}
	return values
}()

// scanHooksPathConfigSection classifies the old section operand of Git's
// legacy and subcommand section mutations. Only a literal core section proves
// bypass. A word the grammar cannot name, a value option without its value, and a
// cluster letter before its value letter leave the operation uncertain, so an
// unreadable line is an unknown effect and never a pass. A lone `-` is an
// operand, as git reads it.
func scanHooksPathConfigSection(args []shellwords.Word) gitHookIssue {
	operation, subcommand := "", ""
	if len(args) > 0 && (args[0].Text == "remove-section" || args[0].Text == "rename-section") {
		operation, subcommand = args[0].Text, args[0].Text
		args = args[1:]
	}
	values := configValueRows[subcommand]
	action := operation
	var operands []shellwords.Word
	uncertain := false
	for _, arg := range configGitSpecs[subcommand].Scan(args).Args {
		text := arg.Word.Text
		missing := arg.Role == argv.RoleOption && values[arg.Name] && !arg.HasValue
		switch {
		case arg.Role == argv.RoleOperand, arg.Role == argv.RoleUnknown && text == "-":
			operands = append(operands, arg.Word)
		case arg.Role == argv.RoleUnknown:
			uncertain = true
			// An unresolved legacy action prefix can itself select a
			// section mutation; do not silently treat it as a key read.
			name, _, _ := strings.Cut(strings.TrimPrefix(text, "--"), "=")
			if subcommand == "" && name != "" && (strings.HasPrefix("remove-section", name) || strings.HasPrefix("rename-section", name)) {
				operation = "unknown"
			}
		case arg.Role == argv.RoleOption && strings.HasPrefix(text, "--"):
			name := strings.TrimPrefix(arg.Name, "--")
			switch name {
			case "get", "get-all", "get-regexp", "get-urlmatch", "replace-all", "add", "unset", "unset-all", "rename-section", "remove-section", "list", "edit", "get-color", "get-colorbool":
				if action != "" && action != name {
					uncertain = true
				}
				action = name
			}
			if name == "remove-section" || name == "rename-section" {
				operation = name
				uncertain = uncertain || arg.Joined || !arg.Word.Literal
			}
			uncertain = uncertain || missing
		case arg.Role == argv.RoleOption:
			// Every letter before the value letter is uncertain, except a
			// legacy -z, which only changes how values are printed.
			letters := arg.Letters
			if arg.Name != "" && letters != "" {
				letters = letters[:len(letters)-1]
			}
			for i := 0; i < len(letters); i++ {
				if subcommand != "" || letters[i] != 'z' {
					uncertain = true
				}
			}
			uncertain = uncertain || missing
		}
	}
	if operation == "" {
		return gitHookIssue{}
	}
	count := 1
	if operation == "rename-section" {
		count = 2
	}
	if uncertain || len(operands) != count || !operands[0].Literal {
		return unknownEffectIssue("git config section mutation")
	}
	if strings.EqualFold(operands[0].Text, "core") {
		return bypassIssue("git config " + operation + " core")
	}
	return gitHookIssue{}
}

// scanHooksPathConfigWrite reports whether git config args write or unset
// core.hooksPath. Reads stay allowed: --get and its variants, the get
// subcommand, and the one-argument legacy read `git config core.hooksPath`.
// Long options are resolved as Git resolves them, so an abbreviation such
// as --unset-a or --fil counts as the option it stands for. A key the
// shell would still expand fails closed. The reading has no end-of-options
// rule, so a `--` is skipped and the words after it are read again as options
// and operands. A word the grammar
// cannot name, a lone `-` included, consumes nothing.
func scanHooksPathConfigWrite(args []shellwords.Word) gitHookIssue {
	subcommand := ""
	if len(args) > 0 && (args[0].Text == "set" || args[0].Text == "unset" || args[0].Text == "get") {
		subcommand = args[0].Text
		args = args[1:]
	}
	spec := configGitSpecs[subcommand]
	write, read := subcommand == "set" || subcommand == "unset", subcommand == "get"
	keyed, positional, valueAfterKey := false, false, false
	unknown := false
	for {
		scan := spec.Scan(args)
		end := scan.End()
		for _, arg := range scan.Args {
			if end >= 0 && arg.At >= end {
				break
			}
			switch arg.Role {
			case argv.RoleOption:
				switch arg.Name {
				case "--unset", "--unset-all", "--add", "--replace-all":
					write = true
				case "--get", "--get-all", "--get-regexp", "--get-urlmatch":
					read = true
				}
			case argv.RoleOperand:
				t := arg.Word.Text
				switch {
				case !positional:
					positional = true
					keyed = isHooksPathKey(t)
					unknown = !arg.Word.Literal && dynamicText(t)
				case keyed || unknown:
					valueAfterKey = true
				}
			}
		}
		if end < 0 {
			break
		}
		args = args[end+1:]
	}
	if write || valueAfterKey && !read {
		if keyed {
			return bypassIssue("git config core.hooksPath")
		}
		if unknown {
			return gitMutationIssue()
		}
	}
	return gitHookIssue{}
}
