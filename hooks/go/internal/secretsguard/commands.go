package secretsguard

import (
	"fmt"
	"slices"
	"strings"

	"github.com/muehz/agentics/hooks/go/internal/argv"
	"github.com/muehz/agentics/hooks/go/internal/shellwords"
)

type flagState uint8

const (
	flagAbsent flagState = iota
	flagTrue
	flagFalse
	flagUnknown
	flagConflict
)

// literalTexts preserves the parsed token text, including dynamic tokens.
// Policy helpers then distinguish literal values from unknown projections by
// the option shape, while retaining joined option identity (for example
// --raw=$MODE) instead of erasing the safety-relevant flag.
func literalTexts(words []shellwords.Word) []string {
	texts := make([]string, len(words))
	for i, word := range words {
		texts[i] = word.Text
	}
	return texts
}

func containsWord(words []string, values ...string) bool {
	return slices.ContainsFunc(words, func(word string) bool {
		return slices.ContainsFunc(values, func(value string) bool {
			return strings.EqualFold(word, value)
		})
	})
}

func sequenceIndex(words []string, sequence ...string) int {
	for i := 0; i+len(sequence) <= len(words); i++ {
		matched := true
		for j, want := range sequence {
			if !strings.EqualFold(words[i+j], want) {
				matched = false
				break
			}
		}
		if matched {
			return i
		}
	}
	return -1
}

func firstOperation(words []string, operations ...string) (string, int) {
	for i := 1; i < len(words); i++ {
		for _, operation := range operations {
			if strings.EqualFold(words[i], operation) {
				return operation, i
			}
		}
	}
	return "", -1
}

func helpProbe(executable string, words []shellwords.Word) bool {
	if executable == "kubectl" {
		return standaloneHelpProbeScan(words, kubectlSecretGrammar, "--help", "-h")
	}
	if executable == "helm" {
		return standaloneHelpProbeScan(words, helmSecretGrammar, "--help", "-h")
	}
	// A help word after the first `--` is an operand, never a help request.
	probe := literalTexts(words)
	if end := slices.Index(probe, "--"); end >= 0 {
		probe = probe[:end]
	}
	if executable == "aws" {
		return awsHelpProbe(probe)
	}
	return containsWord(probe, "--help")
}

func awsHelpProbe(words []string) bool {
	if len(words) <= 1 || !strings.EqualFold(words[len(words)-1], "help") {
		return false
	}
	if len(words) == 2 {
		return true
	}
	previous := words[len(words)-2]
	if !strings.HasPrefix(previous, "-") {
		return true
	}
	// A final help token after a known boolean option is the AWS help
	// subcommand. An option with an unknown or value-taking shape may have
	// consumed that token, so it cannot grant the help exemption.
	for _, boolean := range []string{
		"--debug", "--no-verify-ssl", "--no-paginate", "--no-sign-request",
		"--no-cli-pager", "--cli-auto-prompt", "--no-cli-auto-prompt",
		"--color", "--no-color",
	} {
		if strings.EqualFold(previous, boolean) {
			return true
		}
	}
	return false
}

type kubernetesResourceKind uint8

const (
	kubernetesResourceUnknown kubernetesResourceKind = iota
	kubernetesResourceNonSecret
	kubernetesResourceSecret
)

type kubernetesRawKind uint8

const (
	kubernetesRawAbsent kubernetesRawKind = iota
	kubernetesRawMetadata
	kubernetesRawSecret
	kubernetesRawUnknown
)

// helmSecretOptionRows is the helm grammar the secret-printing rule reads:
// every option whose next word is its value, so that a value is never taken
// for a help, dry-run, or hide-secret word, and the two options the rule asks
// about. The table stays with this guard, because a wider helm table would
// change which words the rule skips.
var helmSecretOptionRows = []argv.Option{
	{Name: "--ca-file", Arity: argv.ArityValue}, {Name: "--cert-file", Arity: argv.ArityValue},
	{Name: "--content-cache", Arity: argv.ArityValue}, {Name: "--description", Arity: argv.ArityValue},
	{Name: "--history-max", Arity: argv.ArityValue}, {Name: "--key-file", Arity: argv.ArityValue},
	{Name: "--kube-context", Arity: argv.ArityValue}, {Name: "--kubeconfig", Arity: argv.ArityValue},
	{Name: "--labels", Arity: argv.ArityValue}, {Name: "--name-template", Arity: argv.ArityValue},
	{Name: "--namespace", Arity: argv.ArityValue}, {Name: "-n", Arity: argv.ArityValue},
	{Name: "--output-dir", Arity: argv.ArityValue}, {Name: "--password", Arity: argv.ArityValue},
	{Name: "--post-renderer", Arity: argv.ArityValue}, {Name: "--repo", Arity: argv.ArityValue},
	{Name: "--repository", Arity: argv.ArityValue}, {Name: "--timeout", Arity: argv.ArityValue},
	{Name: "--username", Arity: argv.ArityValue}, {Name: "--values", Arity: argv.ArityValue},
	{Name: "-f", Arity: argv.ArityValue}, {Name: "--version", Arity: argv.ArityValue},
	{Name: "--set", Arity: argv.ArityValue}, {Name: "--set-string", Arity: argv.ArityValue},
	{Name: "--set-file", Arity: argv.ArityValue}, {Name: "--set-json", Arity: argv.ArityValue},
	{Name: "--set-literal", Arity: argv.ArityValue}, {Name: "--keyring", Arity: argv.ArityValue},
	{Name: "-l", Arity: argv.ArityValue}, {Name: "-o", Arity: argv.ArityValue},
	{Name: "--post-renderer-args", Arity: argv.ArityValue}, {Name: "--kube-apiserver", Arity: argv.ArityValue},
	{Name: "--kube-as-group", Arity: argv.ArityValue}, {Name: "--kube-as-user", Arity: argv.ArityValue},
	{Name: "--kube-ca-file", Arity: argv.ArityValue}, {Name: "--kube-tls-server-name", Arity: argv.ArityValue},
	{Name: "--kube-token", Arity: argv.ArityValue}, {Name: "--qps", Arity: argv.ArityValue},
	{Name: "--registry-config", Arity: argv.ArityValue}, {Name: "--repository-cache", Arity: argv.ArityValue},
	{Name: "--repository-config", Arity: argv.ArityValue},

	// A bare --dry-run means client, so only these words may be its value;
	// any other next word leaves the mode unknown.
	{Name: "--dry-run", Arity: argv.ArityOptional, Spellings: []string{"client", "server", "true", "false"}},
	// The hide-secret question needs a row, because a name with no row reads
	// as unknown rather than absent.
	{Name: "--hide-secret", Arity: argv.ArityFlag},
}

// helmSecretGrammar matches names in any letter case and reads a word from an
// expansion by its text, because the rule has always lower-cased every word and
// never tested Literal; skipping such a word would make the rule looser.
var helmSecretGrammar = argv.NewSpec(helmSecretOptionRows...).With(argv.FoldNames, argv.ReadNonLiteral)

// kubectlSecretOptionRows holds the inherited kubectl options that take a
// value and may occur around get, so that a value such as a context name is
// never read as the verb or the resource type. `-L` and `-l` fold to one row.
// The table stays with this guard.
var kubectlSecretOptionRows = []argv.Option{
	{Name: "--as", Arity: argv.ArityValue}, {Name: "--as-group", Arity: argv.ArityValue},
	{Name: "--cache-dir", Arity: argv.ArityValue}, {Name: "--as-uid", Arity: argv.ArityValue},
	{Name: "--as-user-extra", Arity: argv.ArityValue}, {Name: "--certificate-authority", Arity: argv.ArityValue},
	{Name: "--client-certificate", Arity: argv.ArityValue}, {Name: "--client-key", Arity: argv.ArityValue},
	{Name: "--cluster", Arity: argv.ArityValue}, {Name: "--context", Arity: argv.ArityValue},
	{Name: "--kubeconfig", Arity: argv.ArityValue}, {Name: "--kuberc", Arity: argv.ArityValue},
	{Name: "--namespace", Arity: argv.ArityValue}, {Name: "-n", Arity: argv.ArityValue},
	{Name: "--password", Arity: argv.ArityValue}, {Name: "--profile", Arity: argv.ArityValue},
	{Name: "--profile-output", Arity: argv.ArityValue}, {Name: "--proxy-url", Arity: argv.ArityValue},
	{Name: "--request-timeout", Arity: argv.ArityValue}, {Name: "--server", Arity: argv.ArityValue},
	{Name: "-s", Arity: argv.ArityValue}, {Name: "--tls-server-name", Arity: argv.ArityValue},
	{Name: "--token", Arity: argv.ArityValue}, {Name: "--user", Arity: argv.ArityValue},
	{Name: "--username", Arity: argv.ArityValue}, {Name: "--v", Arity: argv.ArityValue},
	{Name: "-v", Arity: argv.ArityValue}, {Name: "-o", Arity: argv.ArityValue},
	{Name: "--output", Arity: argv.ArityValue}, {Name: "-l", Arity: argv.ArityValue},
	{Name: "--selector", Arity: argv.ArityValue}, {Name: "--field-selector", Arity: argv.ArityValue},
	{Name: "--chunk-size", Arity: argv.ArityValue}, {Name: "-f", Arity: argv.ArityValue},
	{Name: "--filename", Arity: argv.ArityValue}, {Name: "-k", Arity: argv.ArityValue},
	{Name: "--kustomize", Arity: argv.ArityValue}, {Name: "-L", Arity: argv.ArityValue},
	{Name: "--label-columns", Arity: argv.ArityValue}, {Name: "--sort-by", Arity: argv.ArityValue},
	{Name: "--subresource", Arity: argv.ArityValue}, {Name: "--template", Arity: argv.ArityValue},
	{Name: "--raw", Arity: argv.ArityValue},
}

// kubectlSecretGrammar reads names in any case and expansions by their text,
// for the same reason as helmSecretGrammar.
var kubectlSecretGrammar = argv.NewSpec(kubectlSecretOptionRows...).With(argv.FoldNames, argv.ReadNonLiteral)

// noOptionGrammar has no rows, so it consumes no value and only marks the
// first `--`. The readers that test word text themselves use it.
var noOptionGrammar = argv.NewSpec().With(argv.ReadNonLiteral)

// wordsBeforeDoubleDash cuts at the first word whose text is `--` before any
// scan: a value option must not carry the reading past that word by taking it
// as its value.
func wordsBeforeDoubleDash(words []argv.Word) []argv.Word {
	for i, word := range words {
		if word.Text == "--" {
			return words[:i]
		}
	}
	return words
}

// argsBeforeEnd returns one arg per word before the first `--`.
func argsBeforeEnd(words []argv.Word) []argv.Arg {
	scan := noOptionGrammar.Scan(words)
	if end := scan.End(); end >= 0 {
		return scan.Args[:end]
	}
	return scan.Args
}

// scanPastEnds reads words with grammar and goes on reading options after each
// `--`, because kubectl's get index and resource rules skip that word rather
// than stop at it. The scan holds no state from one word to the next except
// the end of options, and a `--` is never a row, so reading each stretch that
// ends in a `--` on its own gives the same args as rescanning the rest after
// every end, in linear time. Each At and ValueAt counts from words[0].
func scanPastEnds(grammar argv.Spec, words []argv.Word) []argv.Arg {
	args := make([]argv.Arg, 0, len(words))
	start := 0
	for i := 0; i <= len(words); i++ {
		if i < len(words) && words[i].Text != "--" {
			continue
		}
		stop := min(i+1, len(words))
		for _, arg := range grammar.Scan(words[start:stop]).Args {
			arg.At += start
			if arg.ValueAt >= 0 {
				arg.ValueAt += start
			}
			args = append(args, arg)
		}
		start = stop
	}
	return args
}

func flagStateOf(state argv.FlagState) flagState {
	switch state {
	case argv.FlagTrue:
		return flagTrue
	case argv.FlagFalse:
		return flagFalse
	case argv.FlagUnknown:
		return flagUnknown
	case argv.FlagConflict:
		return flagConflict
	}
	return flagAbsent
}

// optionPresentScan reports whether a word before the first `--` names one of
// the options, alone or with `=value`, in any letter case. It skips no value,
// so a word that is only an option's value still counts.
func optionPresentScan(words []argv.Word, names ...string) bool {
	for _, arg := range argsBeforeEnd(words) {
		word := arg.Word.Text
		for _, name := range names {
			if strings.EqualFold(word, name) || strings.HasPrefix(strings.ToLower(word), strings.ToLower(name)+"=") {
				return true
			}
		}
	}
	return false
}

// standaloneHelpProbeScan reports whether a help word stands on its own before
// the first `--`, after the executable. A word that grammar reads as an
// option's value is no help word, and an unknown option takes nothing, so a
// help word after it still counts.
func standaloneHelpProbeScan(words []argv.Word, grammar argv.Spec, helpWords ...string) bool {
	words = wordsBeforeDoubleDash(words)
	if len(words) < 2 {
		return false
	}
	for _, arg := range grammar.Scan(words[1:]).Args {
		if arg.Role == argv.RoleOption || arg.Role == argv.RoleValue {
			continue
		}
		if slices.Contains(helpWords, strings.ToLower(arg.Word.Text)) {
			return true
		}
	}
	return false
}

// booleanFlagScan reads a boolean option and its bare negative before the
// first `--`, skipping no value.
func booleanFlagScan(words []argv.Word, positive, negative string) flagState {
	rows := []argv.Option{{Name: positive, Arity: argv.ArityFlag}}
	if negative != "" {
		rows = append(rows, argv.Option{Name: negative, Arity: argv.ArityFlag})
	}
	grammar := argv.NewSpec(rows...).With(argv.FoldNames, argv.ReadNonLiteral)
	return booleanFlagWithValuesScan(words, positive, negative, grammar)
}

// booleanFlagWithValuesScan reads positive before the first `--` under
// grammar, which must hold a row for it. An unreadable value reads unknown,
// and true beside false reads conflict; the caller treats both as true. Only a
// bare negative reads false: a negative with a value is not a spelling the
// rule knows, and it is ignored, as it always was.
func booleanFlagWithValuesScan(words []argv.Word, positive, negative string, grammar argv.Spec) flagState {
	scan := grammar.Scan(wordsBeforeDoubleDash(words))
	state := flagStateOf(scan.Flag(positive, ""))
	if negative == "" || state == flagUnknown || state == flagConflict {
		return state
	}
	for _, arg := range scan.Args {
		if arg.Role != argv.RoleOption || arg.Name != negative || arg.HasValue {
			continue
		}
		if state == flagTrue {
			return flagConflict
		}
		state = flagFalse
	}
	return state
}

// helmDryRunScan reads helm's --dry-run before the first `--`. A bare
// --dry-run as the last word is true; one followed by a word that is not a
// mode reads unknown, because that word may have been meant as the mode.
func helmDryRunScan(words []argv.Word) flagState {
	return flagStateOf(helmSecretGrammar.Scan(wordsBeforeDoubleDash(words)).Flag("--dry-run", ""))
}

// outputModeScan reads kubectl's output mode before the first `--`. A
// separate -o or --output matches only in that spelling, while the joined
// forms match in any case, so `-O yaml` stays unread as it always was. A
// missing, empty, dashed, or expanded mode, or two different modes, read as
// present and invalid.
func outputModeScan(words []argv.Word) (present bool, value string, valid bool) {
	args := argsBeforeEnd(words)
	for i := 0; i < len(args); i++ {
		word := args[i].Word.Text
		var candidate string
		switch {
		case word == "-o" || word == "--output":
			present = true
			if i+1 >= len(args) || args[i+1].Word.Text == "" || strings.HasPrefix(args[i+1].Word.Text, "-") {
				return true, "", false
			}
			candidate = args[i+1].Word.Text
			i++
		case strings.HasPrefix(strings.ToLower(word), "--output="):
			present = true
			candidate = word[len("--output="):]
		case strings.HasPrefix(strings.ToLower(word), "-o="):
			present = true
			candidate = word[len("-o="):]
		case len(word) > 2 && strings.HasPrefix(strings.ToLower(word), "-o"):
			present = true
			candidate = word[2:]
		default:
			continue
		}
		candidate = strings.ToLower(candidate)
		if candidate == "" || strings.Contains(candidate, "$") {
			return true, "", false
		}
		if value != "" && value != candidate {
			return true, "", false
		}
		value = candidate
	}
	return present, value, true
}

// kubernetesRawReadScan classifies every --raw endpoint after the executable,
// past any `--` too, since kubectl still reads one there. An endpoint from an
// expansion, or a --raw with no endpoint, is unknown.
func kubernetesRawReadScan(words []argv.Word) kubernetesRawKind {
	result := kubernetesRawAbsent
	if len(words) < 2 {
		return result
	}
	args := noOptionGrammar.Scan(words[1:]).Args
	for i := 0; i < len(args); i++ {
		text := strings.ToLower(args[i].Word.Text)
		var target string
		switch {
		case text == "--raw":
			if i+1 >= len(args) || !args[i+1].Word.Literal {
				return kubernetesRawUnknown
			}
			target = args[i+1].Word.Text
			i++
		case strings.HasPrefix(text, "--raw="):
			if !args[i].Word.Literal {
				return kubernetesRawUnknown
			}
			target = args[i].Word.Text[len("--raw="):]
		default:
			continue
		}
		if target == "" {
			result = kubernetesRawUnknown
			continue
		}
		clean := strings.ToLower(strings.SplitN(target, "?", 2)[0])
		for _, segment := range strings.Split(clean, "/") {
			if segment == "secret" || segment == "secrets" {
				return kubernetesRawSecret
			}
		}
		parts := strings.Split(strings.Trim(clean, "/"), "/")
		if clean == "/version" || (len(parts) <= 2 && parts[0] == "api") ||
			(len(parts) <= 3 && parts[0] == "apis") {
			if result == kubernetesRawAbsent {
				result = kubernetesRawMetadata
			}
			continue
		}
		result = kubernetesRawUnknown
	}
	return result
}

// kubernetesGetIndexScan returns the index in words of a leading get verb.
// An unknown option, or a value option with no value, before the first
// operand makes the verb ambiguous: the index is -1 and ambiguous is true.
// Every kubectl row takes a value, so an option arg without one is missing it.
func kubernetesGetIndexScan(words []argv.Word) (index int, ambiguous bool) {
	if len(words) < 2 {
		return -1, false
	}
	for _, arg := range scanPastEnds(kubectlSecretGrammar, words[1:]) {
		switch arg.Role {
		case argv.RoleEnd, argv.RoleValue:
			continue
		case argv.RoleOption:
			if !arg.HasValue {
				return -1, true
			}
			continue
		case argv.RoleUnknown:
			return -1, true
		}
		if strings.EqualFold(arg.Word.Text, "get") {
			return arg.At + 1, false
		}
		return -1, false
	}
	return -1, false
}

// kubernetesSecretResourceScan classifies the resource operand after the get
// at getIndex. An unknown option may be boolean or may take the next word, and
// a value option may lack its value, so either before the resource leaves it
// unknown; so does a resource from an expansion. Unknown never authorizes
// structured output.
func kubernetesSecretResourceScan(words []argv.Word, getIndex int) kubernetesResourceKind {
	for _, arg := range scanPastEnds(kubectlSecretGrammar, words[getIndex+1:]) {
		switch arg.Role {
		case argv.RoleEnd, argv.RoleValue:
			continue
		case argv.RoleOption:
			if !arg.HasValue {
				return kubernetesResourceUnknown
			}
			continue
		case argv.RoleUnknown:
			return kubernetesResourceUnknown
		}
		if !arg.Word.Literal {
			return kubernetesResourceUnknown
		}
		lower := strings.ToLower(arg.Word.Text)
		for _, resource := range strings.Split(lower, ",") {
			resourceType := strings.SplitN(resource, "/", 2)[0]
			resourceBase := strings.SplitN(resourceType, ".", 2)[0]
			if resourceBase == "secret" || resourceBase == "secrets" {
				return kubernetesResourceSecret
			}
		}
		if strings.HasPrefix(lower, "$") {
			return kubernetesResourceUnknown
		}
		return kubernetesResourceNonSecret
	}
	return kubernetesResourceUnknown
}

func terraformOutputHasName(words []string, operationIndex int) bool {
	afterDoubleDash := false
	for _, word := range words[operationIndex+1:] {
		if word == "--" {
			afterDoubleDash = true
			continue
		}
		if word == "" {
			continue
		}
		if afterDoubleDash || !strings.HasPrefix(word, "-") {
			return true
		}
	}
	return false
}

// secretCommands are the executable names commandSecretReason has a branch for. Each name must be Classified in argv,
// so the vocabulary names every command the guard dispatches on, and a name is added here before its branch.
var secretCommands = map[string]bool{
	"helm": true, "aws": true, "kubectl": true, "terraform": true, "tofu": true,
}

// commandSecretReason returns a denial only for the frozen R06/R07 command
// families. It never searches substrings or evaluates JMESPath/templates.
func commandSecretReason(invocation shellwords.Invocation) (string, bool) {
	if !invocation.Inspectable {
		return "secrets-guardian could not safely inspect a recognized command wrapper: " + invocation.Reason.String(), true
	}
	if len(invocation.Words) == 0 || !invocation.Words[0].Literal {
		return "", false
	}
	words := literalTexts(invocation.Words)
	executable := strings.ToLower(shellwords.ExecutableName(words[0]))
	if !secretCommands[executable] {
		return "", false
	}
	if invocation.Complete && helpProbe(executable, invocation.Words) {
		return "", false
	}

	switch executable {
	case "helm":
		operation, _ := firstOperation(words, "install", "upgrade")
		if operation != "" {
			dryRun := helmDryRunScan(invocation.Words)
			hideSecret := booleanFlagWithValuesScan(invocation.Words, "--hide-secret", "", helmSecretGrammar)
			if dryRun == flagTrue || dryRun == flagUnknown || dryRun == flagConflict {
				if hideSecret != flagTrue || !invocation.Complete {
					return fmt.Sprintf("helm %s dry-run can print Kubernetes Secret data; add --hide-secret=true", operation), true
				}
			}
		}

	case "aws":
		if sequenceIndex(words, "secretsmanager", "get-secret-value") >= 0 ||
			sequenceIndex(words, "secretsmanager", "batch-get-secret-value") >= 0 {
			return "AWS Secrets Manager value reads print secret material; use describe-secret or list-secrets for metadata", true
		}
		if sequenceIndex(words, "ssm", "get-parameter") >= 0 ||
			sequenceIndex(words, "ssm", "get-parameters") >= 0 ||
			sequenceIndex(words, "ssm", "get-parameters-by-path") >= 0 {
			decryption := booleanFlagScan(invocation.Words, "--with-decryption", "--no-with-decryption")
			if !invocation.Complete || decryption == flagTrue || decryption == flagUnknown || decryption == flagConflict ||
				optionPresentScan(invocation.Words, "--cli-input-json", "--cli-input-yaml") {
				return "AWS SSM decrypted parameter reads print secret material; use describe-parameters for metadata", true
			}
		}

	case "kubectl":
		if configIndex := sequenceIndex(words, "config", "view"); configIndex >= 0 {
			if !invocation.Complete {
				return "kubectl config view has dynamically appended arguments; omit dynamic execution and use the redacted form", true
			}
			raw := booleanFlagScan(invocation.Words, "--raw", "")
			if raw == flagTrue || raw == flagUnknown || raw == flagConflict {
				return "kubectl config view --raw prints credential material; omit --raw for redacted output", true
			}
		}
		switch kubernetesRawReadScan(invocation.Words) {
		case kubernetesRawSecret:
			return "kubectl raw Secret API reads print credential material; use a metadata endpoint or -o name", true
		case kubernetesRawUnknown:
			return "kubectl raw output has an unknown endpoint; use a redacted metadata endpoint", true
		}
		getIndex, getAmbiguous := kubernetesGetIndexScan(invocation.Words)
		if getIndex < 0 && getAmbiguous && containsWord(words, "get") {
			present, mode, valid := outputModeScan(invocation.Words)
			if present && (!valid || (mode != "name" && mode != "wide")) {
				return "kubectl get has an ambiguous option before its resource; use -o name or the default redacted table", true
			}
		}
		if getIndex >= 0 {
			resourceKind := kubernetesSecretResourceScan(invocation.Words, getIndex)
			if resourceKind == kubernetesResourceUnknown {
				if !invocation.Complete {
					return "kubectl Secret read has dynamically appended arguments; use -o name or the default redacted table", true
				}
				present, mode, valid := outputModeScan(invocation.Words)
				if !present || (valid && (mode == "name" || mode == "wide")) {
					return "", false
				}
				return "kubectl secret read has an unknown resource or output mode; use -o name or the default redacted table", true
			}
			if resourceKind != kubernetesResourceSecret {
				return "", false
			}
			if !invocation.Complete {
				return "kubectl Secret read has dynamically appended arguments; use -o name or the default redacted table", true
			}
			present, mode, valid := outputModeScan(invocation.Words)
			if !present {
				return "", false
			}
			if !valid {
				return "kubectl secret read has an uninspectable output mode; use -o name or the default redacted table", true
			}
			if mode != "name" && mode != "wide" {
				return "kubectl structured Secret output can print data; use -o name or the default redacted table", true
			}
		}

	case "terraform", "tofu":
		operation, operationIndex := firstOperation(words, "output", "show", "state")
		switch operation {
		case "output":
			if !invocation.Complete || optionPresentScan(invocation.Words, "-json", "-raw") || terraformOutputHasName(words, operationIndex) {
				return executable + " output by name, -json, or -raw can print sensitive values; run unnamed `" + executable + " output` for redacted output", true
			}
		case "show":
			if !invocation.Complete || optionPresentScan(invocation.Words, "-json") {
				return executable + " show -json can print resolved sensitive values; use the human-readable redacted form", true
			}
		case "state":
			if !invocation.Complete {
				return executable + " state has dynamically appended arguments; use `" + executable + " state list` for structure", true
			}
			if operationIndex+1 < len(words) && strings.EqualFold(words[operationIndex+1], "pull") {
				return executable + " state pull prints the complete state; use `" + executable + " state list` for structure", true
			}
		}
	}
	return "", false
}
