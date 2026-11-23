package commandguard

import (
	"strings"

	"github.com/muehz/agentics/hooks/go/internal/shellwords"
)

// npmCommand is one reading of an npm invocation: the word npm would run as its subcommand and the effective help and
// dry-run state of the flags around it.
type npmCommand struct {
	// subcommand is the word npm runs as its subcommand, or the zero Word when every word is an option, an option
	// value, or an end marker.
	subcommand shellwords.Word
	// found reports a literal subcommand. False covers both a missing and a non-literal subcommand, and neither is
	// reported as a publication, because nothing proves which subcommand runs.
	found  bool
	help   booleanFlagState
	dryRun booleanFlagState
}

// readNpm reads an npm invocation in one pass that visits each word once. The pass feeds two readings that must stay
// apart, because they consume words differently and the guard's verdict depends on both:
//
//   - locating the subcommand canonicalises a boolean token before it tests the fixed aliases and --browser, consumes
//     the separated true, false, or non-literal value a boolean configuration flag takes (--no-help and --json
//     included), skips --, and reads a comment word as a candidate subcommand;
//   - reading the flags tests --color, the fixed aliases, and --browser before it canonicalises, consumes only the
//     values of --color, --browser, value flags, --dry-run, the help flags, and unknown options, treats -- before the
//     subcommand as disabling every later flag and -- after it as the end, and stops at a comment word.
//
// So in `npm --json W publish` with a non-literal W whose text is --dry-run, locating consumes W while the flag reading
// sees --dry-run; the invocation is a preview, as it has been. Neither reading is loosened to match the other.
func readNpm(words []shellwords.Word) npmCommand {
	var cmd npmCommand
	index := 0
	locateSkip := false
	flagSkip, flagsDisabled, flagsEnded := false, false, false
	for i := 1; i < len(words); i++ {
		if index == 0 {
			if locateSkip {
				locateSkip = false
			} else if skip, here := npmLocateStep(words, i); here {
				index = i
				cmd.subcommand, cmd.found = words[i], words[i].Literal
			} else {
				locateSkip = skip
			}
		}
		if !flagsEnded {
			switch {
			case flagSkip:
				flagSkip = false
			case i == index:
			default:
				flagSkip, flagsEnded = cmd.flagStep(words, i, index == 0, &flagsDisabled)
			}
		}
		if index > 0 && flagsEnded {
			break
		}
	}
	return cmd
}

// npmLocateStep reads word i while the subcommand is still unknown. It reports whether the word is the subcommand, or
// else whether the word after it is an option value to skip.
func npmLocateStep(words []shellwords.Word, i int) (skipNext, subcommand bool) {
	text := words[i].Text
	if text == "--" {
		return false, false
	}
	if canonical, ok := canonicalNpmBooleanToken(text); ok {
		text = canonical
	}
	hasNext := i+1 < len(words)
	switch {
	case npmFixedAlias(text):
		return false, false
	case text == "--browser":
		return hasNext && npmBrowserConsumesNext(words[i+1]), false
	case consumesNpmValue(text):
		return !strings.Contains(text, "=") && hasNext, false
	case npmBooleanConfigFlag(text):
		return hasNext && npmSeparatedBooleanValue(words[i+1]), false
	case npmUnknownOption(text) && hasNext && npmSeparatedBooleanValue(words[i+1]):
		return true, false
	case text == "--color":
		return hasNext && npmSeparatedColorValue(words[i+1]), false
	case strings.HasPrefix(text, "-"):
		return false, false
	}
	return false, true
}

// flagStep reads word i, which is not the subcommand, into the help and dry-run state. beforeSubcommand is true while
// no subcommand has been located at or before i. It reports whether the next word is an option value to skip and
// whether the flag reading has ended.
func (c *npmCommand) flagStep(words []shellwords.Word, i int, beforeSubcommand bool, disabled *bool) (skipNext, end bool) {
	token := words[i].Text
	if isPosixCommentWord(words[i]) {
		return false, true
	}
	if token == "--" {
		if beforeSubcommand {
			*disabled = true
			return false, false
		}
		return false, true
	}
	if *disabled {
		return false, false
	}
	hasNext := i+1 < len(words)
	if token == "--color" {
		return hasNext && npmSeparatedColorValue(words[i+1]), false
	}
	if strings.HasPrefix(token, "--color=") || npmFixedAlias(token) {
		return false, false
	}
	if token == "--browser" {
		return hasNext && npmBrowserConsumesNext(words[i+1]), false
	}
	if canonical, ok := canonicalNpmBooleanToken(token); ok {
		token = canonical
	}
	if consumesNpmValue(token) {
		return !strings.Contains(token, "=") && hasNext, false
	}
	if token == "--dry-run" {
		return observeNpmSeparatedBoolean(&c.dryRun, words, i), false
	}
	if npmHelpFlag(token) {
		if strings.HasPrefix(token, "--no-") {
			observeBooleanFlag(&c.help, token, "--help", "--no-help", "--no-usage")
			return false, false
		}
		return observeNpmSeparatedBoolean(&c.help, words, i), false
	}
	if npmUnknownOption(token) {
		return hasNext && npmSeparatedBooleanValue(words[i+1]), false
	}
	if observeBooleanFlag(&c.help, token, "--help") || observeBooleanFlag(&c.help, token, "--usage") ||
		observeBooleanFlag(&c.help, token, "-?") || observeBooleanFlag(&c.help, token, "-H") || observeBooleanFlag(&c.help, token, "-h") {
		return false, false
	}
	observeBooleanFlag(&c.dryRun, token, "--dry-run", "--no-dry-run")
	return false, false
}

// observeNpmSeparatedBoolean records a bare boolean flag at word i. A following true, false, or non-literal word is
// its value and is consumed; any other value is invalid, and without one the flag is true.
func observeNpmSeparatedBoolean(state *booleanFlagState, words []shellwords.Word, i int) (skipNext bool) {
	if i+1 >= len(words) || !npmSeparatedBooleanValue(words[i+1]) {
		state.trueSeen = true
		return false
	}
	switch words[i+1].Text {
	case "true":
		state.trueSeen = true
	case "false":
		state.falseSeen = true
	default:
		state.invalidSeen = true
	}
	return true
}

func npmSeparatedBooleanValue(word shellwords.Word) bool {
	return word.Text == "true" || word.Text == "false" || !word.Literal
}

func npmHelpFlag(token string) bool {
	canonical, ok := canonicalNpmBooleanToken(token)
	return ok && (canonical == "--usage" || strings.HasPrefix(canonical, "--no-usage"))
}

func npmSeparatedColorValue(word shellwords.Word) bool {
	return word.Text == "always" || word.Text == "true" || word.Text == "false" || !word.Literal
}

func npmBrowserConsumesNext(word shellwords.Word) bool {
	if !word.Literal {
		return true
	}
	if word.Text == "true" || word.Text == "false" || word.Text == "always" {
		return true
	}
	return word.Text != "publish"
}

func npmBooleanConfigFlag(token string) bool {
	if npmBooleanAlias(token) {
		return true
	}
	if _, ok := npmBooleanShortFlags[token]; ok {
		return true
	}
	if _, ok := npmBooleanConfigFlags[token]; ok {
		return true
	}
	if npmShortClusterIsBoolean(token) {
		return true
	}
	return npmLongOptionKind(token) == npmOptionBoolean
}

var npmBooleanShortFlags = map[string]struct{}{
	"-a": {}, "-f": {}, "-g": {}, "-l": {}, "-p": {}, "-S": {}, "-B": {}, "-D": {}, "-E": {}, "-O": {}, "-P": {},
	"-?": {}, "-H": {}, "-h": {}, "-v": {}, "-y": {},
}

var npmValueShortFlags = map[string]struct{}{"-c": {}, "-L": {}, "-C": {}, "-m": {}, "-w": {}}

var npmFixedShorthandAliases = map[string]struct{}{
	"d": {}, "dd": {}, "ddd": {}, "quiet": {}, "q": {}, "s": {}, "silent": {}, "verbose": {},
}

var npmValueShorthandAliases = map[string]struct{}{"enjoy-by": {}, "reg": {}, "--enjoy-by": {}, "--reg": {}}

var npmBooleanShorthandAliases = map[string]struct{}{
	"desc": {}, "help": {}, "local": {}, "n": {}, "no": {}, "porcelain": {}, "readonly": {}, "iwr": {}, "ws": {},
	"--desc": {}, "--help": {}, "--local": {}, "--no": {}, "--porcelain": {}, "--readonly": {}, "--iwr": {}, "--ws": {},
}

// npmBooleanConfigFlags is pinned to npm 11's Boolean and nullable-Boolean
// configuration definitions. It is used only to consume a separated value
// before subcommand selection; no npm configuration is loaded at runtime.
var npmBooleanConfigFlags = map[string]struct{}{
	"--all": {}, "--allow-same-version": {}, "--audit": {}, "--bin-links": {},
	"--bypass-2fa": {}, "--commit-hooks": {}, "--dangerously-allow-all-scripts": {}, "--description": {}, "--dev": {},
	"--diff-ignore-all-space": {}, "--diff-name-only": {}, "--diff-no-prefix": {}, "--diff-text": {}, "--dry-run": {},
	"--engine-strict": {}, "--expect-results": {}, "--force": {}, "--foreground-scripts": {}, "--format-package-lock": {},
	"--fund": {}, "--git-tag-version": {}, "--global": {}, "--global-style": {}, "--if-present": {}, "--ignore-scripts": {},
	"--include-staged": {}, "--include-workspace-root": {}, "--include-attestations": {}, "--init-private": {}, "--install-links": {},
	"--json": {}, "--legacy-bundling": {}, "--legacy-peer-deps": {}, "--link": {}, "--long": {}, "--offline": {},
	"--omit-lockfile-registry-resolved": {}, "--optional": {}, "--package-lock": {}, "--package-lock-only": {}, "--parseable": {},
	"--allow-scripts-pending": {}, "--allow-scripts-pin": {}, "--prefer-dedupe": {}, "--prefer-offline": {}, "--prefer-online": {},
	"--production": {}, "--progress": {}, "--provenance": {}, "--read-only": {}, "--rebuild-bundle": {}, "--save": {},
	"--save-bundle": {}, "--save-dev": {}, "--save-exact": {}, "--save-optional": {}, "--save-peer": {}, "--save-prod": {},
	"--packages-all": {}, "--shrinkwrap": {}, "--sign-git-commit": {}, "--sign-git-tag": {}, "--strict-peer-deps": {},
	"--strict-allow-scripts": {}, "--strict-ssl": {}, "--timing": {}, "--unicode": {}, "--update-notifier": {}, "--usage": {},
	"--version": {}, "--versions": {}, "--workspaces": {}, "--workspaces-update": {}, "--yes": {}, "--help": {},
	"--no-color": {}, "--no-dry-run": {}, "--no-global": {}, "--no-yes": {},
}

// npmMapHit reports whether token is a value-taking npm flag, bare or in
// --flag=value form. No key contains '=' (TestFlagMapKeysHaveNoEquals), so a
// Cut and one lookup match the prefix scan over every key exactly.
func npmMapHit(token string) bool {
	if _, ok := npmValueShortFlags[token]; ok {
		return true
	}
	if _, ok := npmValueConfigFlags[token]; ok {
		return true
	}
	if name, _, found := strings.Cut(token, "="); found {
		if _, ok := npmValueConfigFlags[name]; ok {
			return true
		}
	}
	return false
}

func consumesNpmValue(token string) bool {
	if npmValueAlias(token) {
		return true
	}
	if npmMapHit(token) {
		return true
	}
	if npmShortClusterHasValue(token) {
		return true
	}
	if npmLongOptionKind(token) == npmOptionValue {
		return true
	}
	return false
}

type npmOptionKind uint8

const (
	npmOptionUnknown npmOptionKind = iota
	npmOptionBoolean
	npmOptionValue
)

func npmValueAlias(token string) bool {
	base := token
	if equals := strings.IndexByte(base, '='); equals >= 0 {
		base = base[:equals]
	}
	if _, ok := npmValueShorthandAliases[base]; ok {
		return true
	}
	return false
}

func npmBooleanAlias(token string) bool {
	base := token
	if equals := strings.IndexByte(base, '='); equals >= 0 {
		base = base[:equals]
	}
	_, ok := npmBooleanShorthandAliases[base]
	return ok
}

func npmFixedAlias(token string) bool {
	_, ok := npmFixedShorthandAliases[token]
	return ok
}

func npmUnknownOption(token string) bool {
	if !strings.HasPrefix(token, "-") || token == "-" || token == "--" {
		return false
	}
	if npmFixedAlias(token) || npmBooleanConfigFlag(token) || consumesNpmValue(token) || token == "--browser" || strings.HasPrefix(token, "--color") {
		return false
	}
	return true
}

func npmShortClusterHasValue(token string) bool {
	if len(token) < 3 || token[0] != '-' || token[1] == '-' {
		return false
	}
	body := token[1:]
	for i := 0; i < len(body); i++ {
		flag := "-" + body[i:i+1]
		if _, ok := npmValueShortFlags[flag]; ok {
			return i == len(body)-1
		}
		if _, ok := npmBooleanShortFlags[flag]; !ok {
			return false
		}
	}
	return false
}

func npmShortClusterIsBoolean(token string) bool {
	if len(token) < 3 || token[0] != '-' || token[1] == '-' {
		return false
	}
	body := token[1:]
	for i := 0; i < len(body); i++ {
		if _, ok := npmBooleanShortFlags["-"+body[i:i+1]]; !ok {
			return false
		}
	}
	return true
}

func npmLongOptionKind(token string) npmOptionKind {
	base := token
	if equals := strings.IndexByte(base, '='); equals >= 0 {
		base = base[:equals]
	}
	if !strings.HasPrefix(base, "--") {
		return npmOptionUnknown
	}
	if strings.HasPrefix(base, "--no-") {
		if npmLongOptionKind("--"+strings.TrimPrefix(base, "--no-")) == npmOptionBoolean {
			return npmOptionBoolean
		}
		return npmOptionUnknown
	}
	if _, ok := npmBooleanConfigFlags[base]; ok {
		return npmOptionBoolean
	}
	if _, ok := npmValueConfigFlags[base]; ok {
		return npmOptionValue
	}
	var candidate npmOptionKind
	count := 0
	for flag := range npmBooleanConfigFlags {
		if strings.HasPrefix(flag, base) {
			candidate = npmOptionBoolean
			count++
		}
	}
	for flag := range npmValueConfigFlags {
		if strings.HasPrefix(flag, base) {
			candidate = npmOptionValue
			count++
		}
	}
	if count == 1 {
		return candidate
	}
	return npmOptionUnknown
}

func npmResolvedLongName(base string, want npmOptionKind) string {
	if !strings.HasPrefix(base, "--") {
		return ""
	}
	if want == npmOptionBoolean {
		if _, ok := npmBooleanConfigFlags[base]; ok {
			return base
		}
	} else if want == npmOptionValue {
		if _, ok := npmValueConfigFlags[base]; ok {
			return base
		}
	}
	resolved := ""
	count := 0
	if want == npmOptionBoolean {
		for flag := range npmBooleanConfigFlags {
			if strings.HasPrefix(flag, base) {
				resolved, count = flag, count+1
			}
		}
	} else if want == npmOptionValue {
		for flag := range npmValueConfigFlags {
			if strings.HasPrefix(flag, base) {
				resolved, count = flag, count+1
			}
		}
	}
	if count == 1 {
		return resolved
	}
	return ""
}

func canonicalNpmBooleanToken(token string) (string, bool) {
	base, value := token, ""
	if equals := strings.IndexByte(base, '='); equals >= 0 {
		value = base[equals:]
		base = base[:equals]
	}
	switch base {
	case "help", "--help", "-?", "-H", "-h":
		return "--usage" + value, true
	case "--no-help":
		return "--no-usage" + value, true
	}
	negative := strings.HasPrefix(base, "--no-")
	lookup := base
	if negative {
		lookup = "--" + strings.TrimPrefix(base, "--no-")
	}
	resolved := npmResolvedLongName(lookup, npmOptionBoolean)
	if resolved == "" {
		return "", false
	}
	if negative {
		resolved = "--no-" + strings.TrimPrefix(resolved, "--")
	}
	return resolved + value, true
}

// npmValueConfigFlags is pinned to npm 11's non-Boolean configuration
// definitions. This is lexical metadata only: the hook never imports npm or
// reads user configuration while deciding whether a publish is allowed.
var npmValueConfigFlags = map[string]struct{}{
	"--_auth": {}, "--access": {}, "--allow-directory": {}, "--allow-file": {}, "--allow-git": {}, "--allow-remote": {},
	"--allow-scripts": {}, "--also": {}, "--audit-level": {}, "--auth-type": {}, "--before": {}, "--ca": {},
	"--cache": {}, "--cache-max": {}, "--cache-min": {}, "--cafile": {}, "--call": {}, "--cert": {}, "--cidr": {},
	"--cpu": {}, "--depth": {}, "--diff": {}, "--diff-dst-prefix": {}, "--diff-src-prefix": {}, "--diff-unified": {},
	"--editor": {}, "--expect-result-count": {}, "--expires": {}, "--fetch-retries": {}, "--fetch-retry-factor": {},
	"--fetch-retry-maxtimeout": {}, "--fetch-retry-mintimeout": {}, "--fetch-timeout": {}, "--git": {}, "--globalconfig": {},
	"--heading": {}, "--https-proxy": {}, "--include": {}, "--init-author-email": {}, "--init-author-name": {},
	"--init-author-url": {}, "--init-license": {}, "--init-module": {}, "--init-type": {}, "--init-version": {},
	"--init.author.email": {}, "--init.author.name": {}, "--init.author.url": {}, "--init.license": {}, "--init.module": {},
	"--init.version": {}, "--install-strategy": {}, "--key": {}, "--libc": {}, "--local-address": {}, "--location": {},
	"--lockfile-version": {}, "--loglevel": {}, "--logs-dir": {}, "--logs-max": {}, "--maxsockets": {}, "--message": {}, "--min-release-age": {},
	"--min-release-age-exclude": {}, "--node-gyp": {}, "--node-options": {}, "--noproxy": {}, "--omit": {}, "--only": {},
	"--orgs": {}, "--os": {}, "--otp": {}, "--package": {}, "--pack-destination": {}, "--packages": {}, "--prefix": {},
	"--preid": {}, "--provenance-file": {}, "--proxy": {}, "--registry": {}, "--replace-registry-host": {}, "--save-prefix": {},
	"--sbom-format": {}, "--sbom-type": {}, "--scope": {}, "--scopes": {}, "--packages-and-scopes-permission": {},
	"--orgs-permission": {}, "--password": {}, "--token-description": {}, "--name": {}, "--script-shell": {}, "--searchexclude": {},
	"--searchlimit": {}, "--searchopts": {}, "--searchstaleness": {}, "--shell": {}, "--tag": {}, "--tag-version-prefix": {},
	"--umask": {}, "--user-agent": {}, "--userconfig": {}, "--viewer": {}, "--which": {}, "--workspace": {},
}
