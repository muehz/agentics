// Package releaseguard denies production-shaped deploy commands that do not
// carry an existing change record.
package releaseguard

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/muehz/agentics/hooks/go/internal/argv"
	"github.com/muehz/agentics/hooks/go/internal/contract"
	"github.com/muehz/agentics/hooks/go/internal/hookspec"
	"github.com/muehz/agentics/hooks/go/internal/settings"
	"github.com/muehz/agentics/hooks/go/internal/shellwords"
	"github.com/muehz/agentics/hooks/go/internal/toolcall"
	"github.com/muehz/agentics/hooks/go/internal/toolname"
)

// destructiveOperations are the operations whose name deletes what they
// target. They keep the release-record gate whatever policy.require_deploy_record
// says. Every other recognized operation is workflow policy. The split is by
// name and never by an inferred effect, so a change set, plan, or manifest is
// not judged by its content.
var destructiveOperations = map[string]bool{
	"delete":       true,
	"uninstall":    true,
	"destroy":      true,
	"delete-stack": true,
}

type deployShape struct {
	category   string
	executable string
	operation  string
}

type inspection struct {
	shape deployShape
	match bool
	// err holds only the guard's own matcher and authority errors; a parser refusal goes in refusal.
	err error
	// refusal is the parser's reason for a non-inspectable invocation.
	refusal shellwords.Refusal
	// uninspectable is set for every non-inspectable invocation, so one whose refusal is the zero value still fails
	// closed.
	uninspectable bool
	// rule is the index into the configured AGX_RELEASE_ENVIRONMENTS
	// markers that matched this invocation, or -1 when a non-literal word
	// matched fail-closed. It is meaningless unless match is true and the
	// filter is configured.
	rule int
	// droppable is true when every recognized invocation in the command is a
	// workflow-policy operation and none was opaque. Only then may
	// policy.require_deploy_record switch the gate off.
	droppable bool
	// origin is the script line of the deciding invocation, nil for a typed one. A message then names the line in
	// place of any text read from it.
	origin *shellwords.ScriptOrigin
}

// failed reports an inspection that could not vouch for the command: a guard error, a non-inspectable invocation, or a
// parser refusal.
func (r inspection) failed() bool {
	return r.err != nil || r.uninspectable || r.refusal.Refused()
}

// reason is the text the deny and the audit log carry for a failed inspection. A named-rule refusal names no content
// and is kept. Any other failure on a script line, a matcher error that quotes an option or operand of the script or
// a canceled child, is replaced by the line.
func (r inspection) reason() string {
	if r.refusal.Rule() != shellwords.RuleNone {
		return r.refusal.String()
	}
	if r.origin != nil && r.failed() {
		return "deploy option could not be read in " + r.origin.Location()
	}
	if r.refusal.Refused() {
		return r.refusal.String()
	}
	if r.err != nil {
		return r.err.Error()
	}
	return ""
}

// inspectionMessage is the deny for a failed inspection; a named-rule refusal leads with its remedy.
func inspectionMessage(r inspection) string {
	reason := r.reason()
	if r.refusal.Rule() != shellwords.RuleNone {
		if hint := r.refusal.Hint(); hint != "" {
			return fmt.Sprintf("release-guardian could not inspect a recognized execution path: %s (%s); %s", hint, reason, recordRemedy)
		}
	}
	return fmt.Sprintf("release-guardian could not inspect a recognized execution path (%s); %s", reason, recordRemedy)
}

const unreadableInputMessage = "release-guardian could not read normalized input; run /agentics:gate change ($agentics:gate change on Codex) and set the release record in the host environment"

// Run denies recognized production deploy commands unless a valid release
// change record is present in the host environment.
func Run(req hookspec.Request) hookspec.Outcome {
	if !req.Event.Present() {
		return contract.Pass()
	}
	event, err := req.Event.Read()
	if err != nil {
		return contract.Deny("release-guardian could not validate normalized input")
	}
	if event.Tool.Name == nil {
		return contract.Pass()
	}
	if !toolname.IsShell(*event.Tool.Name) {
		return contract.Pass()
	}
	call, err := toolcall.Read(event)
	if err != nil {
		return contract.Deny(unreadableInputMessage)
	}
	if _, ok := call.Command(); !ok {
		return contract.Deny(unreadableInputMessage)
	}
	markers, filtered, err := environments()
	if err != nil {
		return contract.Deny(err.Error())
	}
	ctx := req.ExecutionContext()
	result := inspectDeployExecution(ctx, call.Inspect(ctx, req.Scripts), markers, filtered)
	if !result.match && !result.failed() {
		return contract.Pass()
	}
	// note names the setting that governs the gate. It is appended to every
	// denial of a workflow-policy deployment, and to no other denial. Only a
	// command whose every recognized deployment is workflow policy asks the
	// request's source, so a destructive or opaque command, and a command that
	// is no deployment, never read a settings file.
	note := ""
	if result.droppable {
		cwd := ""
		if event.Cwd != nil {
			cwd = *event.Cwd
		}
		res := req.Settings.Resolve(cwd)
		if !res.Policy.Enabled(settings.RequireDeployRecord) {
			return contract.Pass()
		}
		note = "\n\n" + settings.Note(res, []settings.Key{settings.RequireDeployRecord})
	}
	recordErr := validReleaseRecord(os.Getenv("AGX_RELEASE_RECORD"), time.Now())
	if recordErr == nil {
		logRoot := ""
		if event.Runtime.LogRoot != nil {
			logRoot = *event.Runtime.LogRoot
		}
		if result.match {
			rec := deployRecord{
				Timestamp:       time.Now().UTC().Format("2006-01-02T15:04:05Z"),
				Event:           "deploy_authorized",
				Tool:            *event.Tool.Name,
				Executable:      result.shape.executable,
				Operation:       result.shape.operation,
				Category:        result.shape.category,
				EnvironmentRule: environmentRuleLabel(filtered, result.rule),
				Record:          os.Getenv("AGX_RELEASE_RECORD"),
			}
			if err := writeAuditLog(logRoot, rec); err != nil {
				return contract.Deny(fmt.Sprintf("release-guardian could not record the authorized deploy: %v", err) + note)
			}
		} else {
			// result.failed() is true here: the early return above already
			// sent the !match && !failed() case to Pass. A wrapper the parser
			// could not fully read still gets an audit line when a record
			// authorizes it, so the pass is traceable the same way a match is.
			rec := deployRecord{
				Timestamp: time.Now().UTC().Format("2006-01-02T15:04:05Z"),
				Event:     "deploy_parse_limit_authorized",
				Tool:      *event.Tool.Name,
				Reason:    result.reason(),
				Record:    os.Getenv("AGX_RELEASE_RECORD"),
			}
			if err := writeAuditLog(logRoot, rec); err != nil {
				return contract.Deny(fmt.Sprintf("release-guardian could not record the authorized deploy: %v", err))
			}
		}
		return contract.Pass()
	}
	if result.failed() {
		return contract.Deny(inspectionMessage(result))
	}
	if !errors.Is(recordErr, errRecordInvalid) {
		return contract.Deny(fmt.Sprintf("release-guardian: %v; %s", recordErr, recordRemedy) + note)
	}
	// The shape words come from closed vocabularies, so only the location is added for a script line.
	where := ""
	if result.origin != nil {
		where = " in " + result.origin.Location()
	}
	return contract.Deny(fmt.Sprintf("%s %s is a production-shaped deploy command (%s)%s; %s", result.shape.executable, result.shape.operation, result.shape.category, where, recordRemedy) + note)
}

// inspectDeployExecution scans each shell invocation of inspected for a recognized deploy shape. When filtered is
// true, a matched invocation only counts when it also carries one of markers; otherwise the scan continues to the
// next invocation instead of returning early, since a later invocation in the same command may carry a marked deploy.
// With a script reader behind the inspection, a bash or sh script call is judged by the commands the script runs; a
// script limit is an opaque invocation and keeps the gate.
func inspectDeployExecution(ctx context.Context, inspected toolcall.Inspection, markers []string, filtered bool) inspection {
	// first is the first recognized workflow-policy deployment. The scan goes on
	// past it, because a later destructive or opaque invocation in the same
	// command must keep the gate. first stays the reported match in that case,
	// so the message and the audit shape are the ones the command always had.
	var first inspection
	for index, invocation := range inspected.Policy.Invocations {
		// A refused invocation is a failed inspection: release-guardian denies with inspectionMessage unless a valid
		// record authorizes it, and then it writes the parse-limit audit line. A script line that cannot be read keeps
		// its origin; an inspection role without authority stays an error of the guard, with no origin.
		if refusal, refused := inspected.Refusal(ctx, index); refused {
			if !invocation.Inspectable {
				return settle(first, inspection{refusal: refusal, uninspectable: true, origin: invocation.Script})
			}
			return settle(first, inspection{err: errors.New(refusal.String())})
		}
		if invocation.Inspection != shellwords.NoInspection {
			continue
		}
		shape, ok, err := matchInvocation(invocation)
		if err != nil {
			return settle(first, inspection{err: err, origin: invocation.Script})
		}
		if !ok {
			continue
		}
		hit := inspection{shape: shape, match: true, origin: invocation.Script}
		if filtered {
			rule, matched := environmentRule(invocation, markers)
			if !matched {
				continue
			}
			hit.rule = rule
		}
		if destructiveOperations[strings.ToLower(shape.operation)] {
			return settle(first, hit)
		}
		if !first.match {
			first = hit
			first.droppable = true
		}
	}
	return first
}

// settle ends the scan at an invocation that keeps the gate. A workflow-policy
// match found earlier stays the reported match but stops being droppable.
// Otherwise found is the result.
func settle(first, found inspection) inspection {
	if first.match {
		first.droppable = false
		return first
	}
	return found
}

// environmentRuleLabel names the AGX_RELEASE_ENVIRONMENTS entry that
// authorized a matched deploy, or "" when the filter is unconfigured.
func environmentRuleLabel(filtered bool, rule int) string {
	if !filtered {
		return ""
	}
	if rule < 0 {
		return environmentsVar + "[non-literal]"
	}
	return fmt.Sprintf("%s[%d]", environmentsVar, rule)
}

func matchInvocation(invocation shellwords.Invocation) (deployShape, bool, error) {
	if len(invocation.Words) == 0 {
		return deployShape{}, false, nil
	}
	if invocation.Words[0].Substituted {
		return deployShape{}, false, errors.New("command-substituted executable is not literal")
	}
	executable := strings.ToLower(shellwords.ExecutableName(invocation.Words[0].Text))
	matcher, ok := deployMatchers[executable]
	if !ok {
		return deployShape{}, false, nil
	}
	return matcher(invocation, executable)
}

// deployMatcher reads one invocation of a command the guard matches by name. The executable is the lower-case name
// the invocation was dispatched on, for the matchers that serve several names.
type deployMatcher func(invocation shellwords.Invocation, executable string) (deployShape, bool, error)

// namedMatcher adapts a matcher that serves one command and so has no use for its name.
func namedMatcher(match func(shellwords.Invocation) (deployShape, bool, error)) deployMatcher {
	return func(invocation shellwords.Invocation, _ string) (deployShape, bool, error) {
		return match(invocation)
	}
}

// deployMatchers holds the matcher for every executable name the release guard recognizes. Each name must be
// Classified in argv, so the vocabulary names every command the guard dispatches on.
var deployMatchers = map[string]deployMatcher{
	"kubectl":                         namedMatcher(matchKubectl),
	"helm":                            namedMatcher(matchHelm),
	"terragrunt":                      matchOrchestrator,
	"helmfile":                        matchOrchestrator,
	"terraform":                       matchTerraform,
	"tofu":                            matchTerraform,
	"pulumi":                          namedMatcher(matchPulumi),
	"aws":                             namedMatcher(matchAWS),
	"az":                              namedMatcher(matchAzure),
	"new-azresourcegroupdeployment":   matchAzurePowerShell,
	"new-azdeployment":                matchAzurePowerShell,
	"new-azsubscriptiondeployment":    matchAzurePowerShell,
	"new-azmanagementgroupdeployment": matchAzurePowerShell,
	"new-aztenantdeployment":          matchAzurePowerShell,
	"gcloud":                          namedMatcher(matchGcloud),
	"vercel":                          namedMatcher(matchVercel),
}

// leadingCommandScan returns the words from the first operand on, or the refusal for the first word before it that
// cannot be read. A literal `--` ends the reading with no command. Each word is tested for being literal before its
// role, in order, so the refusal is the one the first unreadable word gives; a value word must be literal and
// non-empty. Because the first unreadable word decides, a reader never takes Scan.Problem alone. The refusal quotes
// the whole option word.
func leadingCommandScan(words []shellwords.Word, start int, spec argv.Spec) ([]shellwords.Word, string) {
	if start >= len(words) {
		return nil, ""
	}
	scan := spec.Scan(words[start:])
	for _, arg := range scan.Args {
		if arg.Role == argv.RoleValue {
			continue
		}
		if !arg.Word.Literal {
			return nil, "non-literal option or operation"
		}
		switch arg.Role {
		case argv.RoleEnd:
			return nil, ""
		case argv.RoleOperand:
			return words[start+arg.At:], ""
		case argv.RoleOption:
			if missingLiteralValue(scan, arg) {
				return nil, fmt.Sprintf("%s is missing its literal value", arg.Word.Text)
			}
		default:
			return nil, fmt.Sprintf("option %s is outside the frozen parser contract", arg.Word.Text)
		}
	}
	return nil, ""
}

// filteredPositionalsScan returns every operand, read past the first one, or the refusal for the first word that
// cannot be read. A literal `--` and every word after it are kept as they are. It tests each word as
// leadingCommandScan does.
func filteredPositionalsScan(words []shellwords.Word, start int, spec argv.Spec) ([]shellwords.Word, string) {
	if start >= len(words) {
		return nil, ""
	}
	var out []shellwords.Word
	scan := spec.Scan(words[start:])
	for _, arg := range scan.Args {
		if arg.Role == argv.RoleValue {
			continue
		}
		if !arg.Word.Literal {
			return nil, "non-literal option or operation"
		}
		switch arg.Role {
		case argv.RoleEnd:
			return append(out, words[start+arg.At:]...), ""
		case argv.RoleOperand:
			out = append(out, arg.Word)
		case argv.RoleOption:
			if missingLiteralValue(scan, arg) {
				return nil, fmt.Sprintf("%s is missing its literal value", arg.Word.Text)
			}
		default:
			return nil, fmt.Sprintf("option %s is outside the frozen parser contract", arg.Word.Text)
		}
	}
	return out, ""
}

// missingLiteralValue reports a value option whose separate value is absent, not literal, or empty. A value joined
// with `=` is accepted whatever it holds. A value option that ends the line is the scan's MissingValue problem at
// that word: every earlier problem would already have refused the line, so the first problem is this one.
func missingLiteralValue(scan argv.Scan, arg argv.Arg) bool {
	switch {
	case arg.Joined:
		return false
	case arg.ValueAt >= 0:
		return !arg.Value.Literal || arg.Value.Text == ""
	}
	return scan.Problem != nil && scan.Problem.Kind == argv.MissingValue && scan.Problem.At == arg.At
}

var terraformOptionRows = optionRows([]string{"-chdir"}, nil)

var terraformGrammar = argv.NewSpec(terraformOptionRows...).With(argv.FoldNames)

var terraformValueFlagsRows = optionRows([]string{
	"-backup", "-chdir", "-lock-timeout", "-out", "-parallelism", "-replace", "-state", "-state-out", "-target", "-var", "-var-file",
}, nil)

var terraformValueFlagsSpec = argv.NewSpec(terraformValueFlagsRows...)

func matchTerraform(invocation shellwords.Invocation, executable string) (deployShape, bool, error) {
	if hasHelpScan(invocation.Words[1:], false, invocation.Complete, terraformValueFlagsSpec) {
		return deployShape{}, false, nil
	}
	parts, reason := leadingCommandScan(invocation.Words, 1, terraformGrammar)
	if reason != "" {
		return deployShape{}, false, errors.New(executable + " " + reason)
	}
	if len(parts) == 0 {
		return deployShape{}, false, nil
	}
	verb := strings.ToLower(parts[0].Text)
	if hasHelpScan(parts[1:], false, invocation.Complete, terraformValueFlagsSpec) {
		return deployShape{}, false, nil
	}
	if verb != "apply" && verb != "destroy" {
		return deployShape{}, false, nil
	}
	return deployShape{executable + "_deploy", executable, verb}, true, nil
}

var pulumiOptionRows = optionRows(
	[]string{"--color", "--config", "--cwd", "--message", "--parallel", "--policy-pack", "--stack", "--target", "--tracing", "--verbose", "-v"},
	[]string{"--emoji", "--logflow", "--logtostderr", "--non-interactive", "--refresh"},
)

var pulumiGrammar = argv.NewSpec(pulumiOptionRows...).With(argv.FoldNames)

var pulumiValueFlagsRows = optionRows([]string{
	"--color", "--config", "--cwd", "--message", "--parallel", "--policy-pack", "--stack", "--target", "--tracing", "--verbose", "-v",
}, nil)

var pulumiValueFlagsSpec = argv.NewSpec(pulumiValueFlagsRows...)

func matchPulumi(invocation shellwords.Invocation) (deployShape, bool, error) {
	if hasHelpScan(invocation.Words[1:], false, invocation.Complete, pulumiValueFlagsSpec) {
		return deployShape{}, false, nil
	}
	parts, reason := leadingCommandScan(invocation.Words, 1, pulumiGrammar)
	if reason != "" {
		return deployShape{}, false, errors.New("pulumi " + reason)
	}
	if len(parts) == 0 {
		return deployShape{}, false, nil
	}
	verb := strings.ToLower(parts[0].Text)
	if hasHelpScan(parts[1:], false, invocation.Complete, pulumiValueFlagsSpec) || (verb != "up" && verb != "destroy") {
		return deployShape{}, false, nil
	}
	return deployShape{"pulumi_deploy", "pulumi", verb}, true, nil
}

var awsOptionRows = optionRows(
	[]string{"--ca-bundle", "--cli-connect-timeout", "--cli-read-timeout", "--color", "--endpoint-url", "--output", "--profile", "--query", "--region"},
	[]string{"--debug", "--no-cli-pager", "--no-sign-request", "--no-verify-ssl"},
)

var awsGrammar = argv.NewSpec(awsOptionRows...).With(argv.FoldNames)

var awsValueFlagsRows = optionRows([]string{
	"--ca-bundle", "--capabilities", "--change-set-name", "--change-set-type", "--cli-connect-timeout", "--cli-read-timeout", "--client-request-token", "--color", "--description", "--endpoint-url", "--include-nested-stacks", "--kms-key-id", "--notification-arns", "--on-failure", "--output", "--parameters", "--profile", "--query", "--region", "--resources-to-import", "--role-arn", "--rollback-configuration", "--s3-bucket", "--s3-prefix", "--stack-name", "--tags", "--template-file", "--timeout-in-minutes", "--use-previous-template",
}, nil)

var awsValueFlagsSpec = argv.NewSpec(awsValueFlagsRows...)

func matchAWS(invocation shellwords.Invocation) (deployShape, bool, error) {
	if hasHelpScan(invocation.Words[1:], false, invocation.Complete, awsValueFlagsSpec) {
		return deployShape{}, false, nil
	}
	parts, reason := leadingCommandScan(invocation.Words, 1, awsGrammar)
	if reason != "" {
		return deployShape{}, false, errors.New("aws " + reason)
	}
	if len(parts) < 1 || !strings.EqualFold(parts[0].Text, "cloudformation") {
		return deployShape{}, false, nil
	}
	operations, reason := leadingCommandScan(parts, 1, awsGrammar)
	if reason != "" {
		return deployShape{}, false, errors.New("aws cloudformation " + reason)
	}
	if len(operations) == 0 {
		return deployShape{}, false, nil
	}
	verb := strings.ToLower(operations[0].Text)
	args := operations[1:]
	if verb == "help" || hasTerminalHelpScan(args, invocation.Complete, awsValueFlagsSpec) {
		return deployShape{}, false, nil
	}
	if !names("deploy", "create-stack", "update-stack", "delete-stack", "execute-change-set")[verb] {
		return deployShape{}, false, nil
	}
	return deployShape{"cloudformation_deploy", "aws cloudformation", verb}, true, nil
}

var gcloudOptionRows = optionRows(
	[]string{"--account", "--billing-project", "--configuration", "--flags-file", "--flatten", "--format", "--impersonate-service-account", "--project", "--trace-token", "--verbosity"},
	[]string{"--log-http", "--quiet", "-q", "--user-output-enabled"},
)

var gcloudGrammar = argv.NewSpec(gcloudOptionRows...).With(argv.FoldNames)

var gcloudValueFlagsRows = optionRows([]string{
	"--account", "--billing-project", "--concurrency", "--configuration", "--cpu", "--flags-file", "--flatten", "--format", "--image", "--impersonate-service-account", "--ingress", "--memory", "--min", "--max", "--port", "--project", "--region", "--revision-suffix", "--service-account", "--set-env-vars", "--source", "--tag", "--timeout", "--trace-token", "--vpc-connector", "--verbosity",
}, nil)

var gcloudValueFlagsSpec = argv.NewSpec(gcloudValueFlagsRows...)

func matchGcloud(invocation shellwords.Invocation) (deployShape, bool, error) {
	if hasHelpScan(invocation.Words[1:], false, invocation.Complete, gcloudValueFlagsSpec) {
		return deployShape{}, false, nil
	}
	parts, reason := leadingCommandScan(invocation.Words, 1, gcloudGrammar)
	if reason != "" {
		return deployShape{}, false, errors.New("gcloud " + reason)
	}
	if len(parts) < 1 || !strings.EqualFold(parts[0].Text, "run") {
		return deployShape{}, false, nil
	}
	operations, reason := leadingCommandScan(parts, 1, gcloudGrammar)
	if reason != "" {
		return deployShape{}, false, errors.New("gcloud run " + reason)
	}
	if len(operations) < 1 || !strings.EqualFold(operations[0].Text, "deploy") {
		return deployShape{}, false, nil
	}
	if hasHelpScan(operations[1:], false, invocation.Complete, gcloudValueFlagsSpec) {
		return deployShape{}, false, nil
	}
	return deployShape{"gcloud_run_deploy", "gcloud run", "deploy"}, true, nil
}

// hasTerminalHelpScan reports a complete line whose only operand is a final `help`, after nothing but value options
// whose values are literal. Any other word, a non-literal word, and `--` each answer false.
func hasTerminalHelpScan(args []shellwords.Word, complete bool, spec argv.Spec) bool {
	if !complete {
		return false
	}
	for _, arg := range spec.Scan(args).Args {
		switch {
		case arg.Role == argv.RoleValue:
			continue
		case !arg.Word.Literal:
			return false
		case arg.Role == argv.RoleOption:
			if !arg.HasValue || !arg.Value.Literal {
				return false
			}
		case arg.Role == argv.RoleOperand && arg.Word.Text == "help" && arg.At == len(args)-1:
			return true
		default:
			return false
		}
	}
	return false
}

var vercelOptionRows = optionRows(
	[]string{"--build-env", "--cwd", "--env", "--git-branch", "--git-commit", "--git-metadata", "--local-config", "--meta", "--name", "--output", "--regions", "--scope", "--target", "--team", "--token"},
	[]string{"--confirm", "--force", "--help", "--local", "--no-clipboard", "--no-wait", "--prod", "--version", "-v"},
)

var vercelGrammar = argv.NewSpec(vercelOptionRows...).With(argv.FoldNames)

var vercelValueFlagsRows = optionRows([]string{
	"--build-env", "--cwd", "--env", "--git-branch", "--git-commit", "--git-metadata", "--local-config", "--meta", "--name", "--output", "--regions", "--scope", "--target", "--team", "--token",
}, nil)

var vercelValueFlagsSpec = argv.NewSpec(vercelValueFlagsRows...)

// vercelHelpRows are the vercel value options and Boolean flags the help question reads, both matched exactly.
var vercelHelpRows = optionRows(
	[]string{"--build-env", "--cwd", "--env", "--git-branch", "--git-commit", "--git-metadata", "--local-config", "--meta", "--name", "--output", "--regions", "--scope", "--target", "--team", "--token"},
	[]string{"--confirm", "--force", "--help", "--local", "--no-clipboard", "--no-wait", "--prod", "--version", "-v"},
)

var vercelHelpSpec = argv.NewSpec(vercelHelpRows...)

var vercelNonDeployCommands = names("alias", "build", "domains", "env", "help", "inspect", "link", "list", "logs", "ls", "project", "pull", "remove", "rollback", "teams", "whoami")

func matchVercel(invocation shellwords.Invocation) (deployShape, bool, error) {
	args := invocation.Words[1:]
	positionals, reason := filteredPositionalsScan(args, 0, vercelGrammar)
	if reason != "" {
		return deployShape{}, false, errors.New("vercel " + reason)
	}
	if len(positionals) > 0 && vercelNonDeployCommands[strings.ToLower(positionals[0].Text)] {
		return deployShape{}, false, nil
	}
	if hasHelpWithBoolsScan(args, invocation.Complete, vercelHelpSpec) {
		return deployShape{}, false, nil
	}
	prod, reason := vercelProductionScan(args)
	if reason != "" {
		return deployShape{}, false, errors.New(reason)
	}
	if !prod {
		return deployShape{}, false, nil
	}
	return deployShape{"vercel_production_deploy", "vercel", "deploy production"}, true, nil
}

// hasHelpWithBoolsScan reports a complete line that carries --help in any spelling, with nothing but the grammar's
// options and operands before `--`. Vercel's parser treats every Boolean occurrence as true, so `--help=false`
// still asks for help. A non-literal word, an unknown option, `--`, and a value option without a literal value each
// answer false.
func hasHelpWithBoolsScan(args []shellwords.Word, complete bool, spec argv.Spec) bool {
	if !complete {
		return false
	}
	scan := spec.Scan(args)
	seen := false
	for _, arg := range scan.Args {
		switch {
		case arg.Role == argv.RoleValue, arg.Role == argv.RoleOperand && arg.Word.Literal:
			continue
		case !arg.Word.Literal, arg.Role != argv.RoleOption:
			return false
		case arg.Name == "--help":
			seen = true
		case arg.ValueAt >= 0:
			if !arg.Value.Literal {
				return false
			}
		case !arg.Joined && valueMissing(scan, arg):
			return false
		}
	}
	return seen
}

// valueMissing reports a value option that ends the line. Scan records only its first problem, so the test holds
// only for a reader that has already answered at every earlier unknown or non-literal word.
func valueMissing(scan argv.Scan, arg argv.Arg) bool {
	return scan.Problem != nil && scan.Problem.Kind == argv.MissingValue && scan.Problem.At == arg.At
}

// vercelProductionScan reads whether a vercel line deploys to production, and the refusal for a line it cannot
// read. Every word before `--` is read by its text, literal or not, so a non-literal option still takes its value;
// only --prod and --target must themselves be literal. --prod in any spelling and --target production enable
// production, --no-prod disables it, and disagreeing occurrences are a conflict.
func vercelProductionScan(args []shellwords.Word) (bool, string) {
	var prodValues, targetValues []string
read:
	for _, arg := range vercelValueFlagsSpec.With(argv.ReadNonLiteral).Scan(args).Args {
		switch arg.Role {
		case argv.RoleEnd:
			break read
		case argv.RoleOption:
			// Only a separate value is tested: a joined one is part of its option word.
			if arg.Name != "--target" {
				if !arg.Joined && (!arg.HasValue || !arg.Value.Literal) {
					return false, "vercel option is missing its literal value"
				}
				continue
			}
			if !arg.Word.Literal {
				return false, "vercel --target is not literal"
			}
			if !arg.Joined && (!arg.HasValue || !arg.Value.Literal) {
				return false, "vercel --target is missing its literal value"
			}
			if strings.HasPrefix(arg.Value.Text, "-") {
				return false, "vercel --target has an invalid literal value"
			}
			targetValues = append(targetValues, strings.ToLower(arg.Value.Text))
		case argv.RoleUnknown:
			switch name, _, _ := strings.Cut(arg.Word.Text, "="); name {
			case "--prod":
				if !arg.Word.Literal {
					return false, "vercel --prod is not literal"
				}
				// Vercel's pinned arg 5 Boolean parser treats --prod=false as
				// the presence of the Boolean flag.
				prodValues = append(prodValues, "true")
			case "--no-prod":
				prodValues = append(prodValues, "false")
			}
		}
	}
	if conflicting(prodValues) || conflicting(targetValues) {
		return false, "vercel production flags conflict"
	}
	for _, value := range prodValues {
		if value == "true" {
			return true, ""
		}
	}
	for _, value := range targetValues {
		if value == "production" {
			return true, ""
		}
	}
	return false, ""
}

func conflicting(values []string) bool {
	for i := 1; i < len(values); i++ {
		if values[i] != values[0] {
			return true
		}
	}
	return false
}

// kubectlValueFlagsRows are the value flags that may occur after a command verb. Their operands are data even when
// they contain a preview or help spelling. --dry-run stays a value row, so a bare --dry-run still takes the next word.
var kubectlValueFlagsRows = optionRows([]string{
	"--as", "--as-group", "--cache-dir", "--certificate-authority", "--client-certificate", "--client-key", "--cluster", "--context", "--kubeconfig", "--namespace", "-n", "--password", "--profile", "--profile-output", "--request-timeout", "--server", "-s", "--tls-server-name", "--token", "--user", "--username", "--v",
	"-f", "--filename", "-k", "--kustomize", "-o", "--output", "--selector", "-l", "--field-selector", "--resource-version", "--timeout", "--field-manager", "--subresource", "--prune-whitelist", "--replicas", "-r", "--containers", "--from", "--from-env-file", "--from-file", "--from-literal", "--image", "--port", "--type", "--cert", "--key", "--docker-email", "--docker-password", "--docker-server", "--docker-username", "--raw", "--validate", "--keys", "--prefix", "--revision", "--template", "--dry-run",
}, nil)

var kubectlValueFlagsSpec = argv.NewSpec(kubectlValueFlagsRows...)

var kubectlOptionRows = optionRows(
	[]string{"--as", "--as-group", "--cache-dir", "--certificate-authority", "--client-certificate", "--client-key", "--cluster", "--context", "--kubeconfig", "--namespace", "-n", "--password", "--profile", "--profile-output", "--request-timeout", "--server", "-s", "--tls-server-name", "--token", "--user", "--username", "--v"},
	[]string{"--disable-compression", "--insecure-skip-tls-verify", "--match-server-version"},
)

var kubectlGrammar = argv.NewSpec(kubectlOptionRows...).With(argv.FoldNames)

func matchKubectl(invocation shellwords.Invocation) (deployShape, bool, error) {
	if hasHelpScan(invocation.Words[1:], true, invocation.Complete, kubectlValueFlagsSpec) {
		return deployShape{}, false, nil
	}
	parts, reason := leadingCommandScan(invocation.Words, 1, kubectlGrammar)
	if reason != "" {
		return deployShape{}, false, errors.New("kubectl " + reason)
	}
	if len(parts) == 0 {
		return deployShape{}, false, nil
	}
	verb := strings.ToLower(parts[0].Text)
	args := parts[1:]
	if hasHelpScan(args, true, invocation.Complete, kubectlValueFlagsSpec) {
		return deployShape{}, false, nil
	}
	operation := verb
	mutating := names("apply", "create", "delete", "patch", "replace", "scale")
	if verb == "set" || verb == "rollout" {
		nested, nestedReason := leadingCommandScan(parts, 1, kubectlGrammar)
		if nestedReason != "" {
			return deployShape{}, false, errors.New("kubectl " + nestedReason)
		}
		if len(nested) == 0 || !nested[0].Literal {
			return deployShape{}, false, errors.New("kubectl " + verb + " is missing a literal subcommand")
		}
		operation += " " + strings.ToLower(nested[0].Text)
		args = nested[1:]
		mutating = names("set image", "set env", "rollout restart", "rollout undo")
	}
	if !mutating[operation] {
		return deployShape{}, false, nil
	}
	if operation == "apply" && invocation.Complete && !helpConflictScan(args, true, kubectlValueFlagsSpec) && exactValueScan(args, "--dry-run", "client", kubectlValueFlagsSpec) {
		return deployShape{}, false, nil
	}
	return deployShape{"kubernetes_deploy", "kubectl", operation}, true, nil
}

// helmValueFlagsRows keeps --dry-run a value row, so a bare --dry-run still takes the next word.
var helmValueFlagsRows = optionRows([]string{
	"--burst-limit", "--kube-apiserver", "--kube-as-group", "--kube-as-user", "--kube-ca-file", "--kube-context", "--kube-tls-server-name", "--kube-token", "--kubeconfig", "--namespace", "-n", "--qps", "--registry-config", "--repository-cache", "--repository-config",
	"--ca-file", "--cert-file", "--content-cache", "--description", "--history-max", "--key-file", "--keyring", "--labels", "--name-template", "--output-dir", "--password", "--post-renderer", "--post-renderer-args", "--release-name", "--repo", "--repository", "--set", "--set-file", "--set-json", "--set-literal", "--set-string", "--timeout", "--username", "--values", "--version", "-f", "-l", "-o", "--dry-run",
}, nil)

var helmValueFlagsSpec = argv.NewSpec(helmValueFlagsRows...)

var helmOptionRows = optionRows(
	[]string{"--burst-limit", "--kube-apiserver", "--kube-as-group", "--kube-as-user", "--kube-ca-file", "--kube-context", "--kube-tls-server-name", "--kube-token", "--kubeconfig", "--namespace", "-n", "--qps", "--registry-config", "--repository-cache", "--repository-config"},
	[]string{"--debug", "--kube-insecure-skip-tls-verify"},
)

var helmGrammar = argv.NewSpec(helmOptionRows...).With(argv.FoldNames)

func matchHelm(invocation shellwords.Invocation) (deployShape, bool, error) {
	if hasHelpScan(invocation.Words[1:], true, invocation.Complete, helmValueFlagsSpec) {
		return deployShape{}, false, nil
	}
	parts, reason := leadingCommandScan(invocation.Words, 1, helmGrammar)
	if reason != "" {
		return deployShape{}, false, errors.New("helm " + reason)
	}
	if len(parts) == 0 {
		return deployShape{}, false, nil
	}
	verb := strings.ToLower(parts[0].Text)
	args := parts[1:]
	if hasHelpScan(args, true, invocation.Complete, helmValueFlagsSpec) {
		return deployShape{}, false, nil
	}
	if !names("install", "upgrade", "rollback", "uninstall")[verb] {
		return deployShape{}, false, nil
	}
	if (verb == "install" || verb == "upgrade") && invocation.Complete && !helpConflictScan(args, true, helmValueFlagsSpec) && exactValueScan(args, "--dry-run", "client", helmValueFlagsSpec) && exactBoolScan(args, "--hide-secret", true, helmValueFlagsSpec) {
		return deployShape{}, false, nil
	}
	return deployShape{"helm_deploy", "helm", verb}, true, nil
}

// hasHelpScan reports a complete line that asks for help before `--`: --help, `--help=true` in any case, or with
// short a bare -h, and no `--help=` other value or `-h=` beside it. A value option skips its value whatever that
// word is. A non-literal word, an unknown option, and a value option that ends the line each answer false.
func hasHelpScan(args []shellwords.Word, short, complete bool, spec argv.Spec) bool {
	if !complete {
		return false
	}
	seen, enabled := false, true
	for _, arg := range spec.Scan(args).Args {
		if arg.Role == argv.RoleValue {
			continue
		}
		if !arg.Word.Literal {
			return false
		}
		switch arg.Role {
		case argv.RoleEnd:
			return seen && enabled
		case argv.RoleOperand:
			continue
		case argv.RoleOption:
			// The grammar holds only value options, so an option without a value ends the line.
			if !arg.HasValue {
				return false
			}
			continue
		}
		name, value, joined := strings.Cut(arg.Word.Text, "=")
		switch {
		case name == "--help":
			seen = true
			if joined && strings.ToLower(value) != "true" {
				enabled = false
			}
		case short && name == "-h":
			seen = true
			if joined {
				enabled = false
			}
		default:
			return false
		}
	}
	return seen && enabled
}

// helpConflictScan reports help asked for both on and off before `--`. Every word is read by its text, literal or
// not, so a non-literal option still takes its value. A value option that ends the line answers false.
func helpConflictScan(args []shellwords.Word, short bool, spec argv.Spec) bool {
	seenTrue, seenFalse := false, false
read:
	for _, arg := range spec.With(argv.ReadNonLiteral).Scan(args).Args {
		switch arg.Role {
		case argv.RoleEnd:
			break read
		case argv.RoleOption:
			if !arg.HasValue {
				return false
			}
		case argv.RoleUnknown:
			name, value, joined := strings.Cut(arg.Word.Text, "=")
			switch {
			case name == "--help":
				if !joined || strings.EqualFold(value, "true") {
					seenTrue = true
				} else {
					seenFalse = true
				}
			case short && name == "-h":
				if joined {
					seenFalse = true
				} else {
					seenTrue = true
				}
			}
		}
	}
	return seenTrue && seenFalse
}

// exactValueScan reports that flag occurs before `--` and every occurrence is literal with a literal value equal to
// wanted in any case. flag must be a value row of spec. Every word is read by its text, literal or not, so a
// non-literal option still takes its value. A value option that ends the line answers false.
func exactValueScan(args []shellwords.Word, flag, wanted string, spec argv.Spec) bool {
	seen := false
read:
	for _, arg := range spec.With(argv.ReadNonLiteral).Scan(args).Args {
		switch {
		case arg.Role == argv.RoleEnd:
			break read
		case arg.Role != argv.RoleOption:
			continue
		case arg.Name == flag:
			if !arg.Word.Literal || !arg.HasValue || !arg.Value.Literal || !strings.EqualFold(arg.Value.Text, wanted) {
				return false
			}
			seen = true
		case !arg.HasValue:
			return false
		}
	}
	return seen
}

// exactBoolScan reports that flag or its --no- negative occurs before `--` and every occurrence is literal and
// reads wanted, with a joined value of true or false in any case. Neither name may be a row of spec. Every word is
// read by its text, literal or not, so a non-literal option still takes its value. A value option that ends the
// line answers false.
func exactBoolScan(args []shellwords.Word, flag string, wanted bool, spec argv.Spec) bool {
	seen := false
	negative := "--no-" + strings.TrimPrefix(flag, "--")
read:
	for _, arg := range spec.With(argv.ReadNonLiteral).Scan(args).Args {
		switch arg.Role {
		case argv.RoleEnd:
			break read
		case argv.RoleOption:
			if !arg.HasValue {
				return false
			}
			continue
		case argv.RoleValue, argv.RoleOperand:
			continue
		}
		name, value, joined := strings.Cut(arg.Word.Text, "=")
		if name != flag && name != negative {
			continue
		}
		if !arg.Word.Literal {
			return false
		}
		got := name != negative
		if joined {
			switch strings.ToLower(value) {
			case "true":
				got = true
			case "false":
				got = false
			default:
				return false
			}
		}
		if got != wanted {
			return false
		}
		seen = true
	}
	return seen
}

// optionRows builds a grammar's rows from its value options and its flags, in that order.
func optionRows(values, flags []string) []argv.Option {
	rows := make([]argv.Option, 0, len(values)+len(flags))
	for _, name := range values {
		rows = append(rows, argv.Option{Name: name, Arity: argv.ArityValue})
	}
	for _, name := range flags {
		rows = append(rows, argv.Option{Name: name, Arity: argv.ArityFlag})
	}
	return rows
}

func names(values ...string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, value := range values {
		out[value] = true
	}
	return out
}
