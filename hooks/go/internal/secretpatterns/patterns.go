// Package secretpatterns owns the ordered detector used by secrets-guardian.
package secretpatterns

import (
	"math"
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/muehz/agentics/hooks/go/internal/asciifold"
)

// PlaceholderPolicy controls whether a detector can exempt an entire
// captured value as a documentation or configuration stand-in.
type PlaceholderPolicy uint8

const (
	// PlaceholderNone applies no placeholder exemption.
	PlaceholderNone PlaceholderPolicy = iota
	// PlaceholderWholeValue exempts only a complete captured placeholder or
	// a complete environment, user-config or template reference.
	PlaceholderWholeValue
)

// Pattern is one stable detector row. UpstreamID and UpstreamCommit are empty
// for Agentics-native rules. SecretGroup zero means the whole regexp match.
type Pattern struct {
	Name, Severity, Source string
	RE                     *regexp.Regexp
	SecretGroup            int
	Keywords               []string
	Entropy                float64
	Allowlist              []*regexp.Regexp
	Placeholder            PlaceholderPolicy
	Advisory               bool
	// MarkerOnly marks a rule whose match is a fixed marker (a PEM header,
	// a service-account type field) rather than the secret itself. Two
	// matches of such a rule are equal whatever key material follows, so a
	// caller must never treat an equal match as the same secret.
	MarkerOnly     bool
	UpstreamID     string
	UpstreamCommit string
}

// Match preserves both the display match and the secret capture used for
// placeholder, entropy, and rule-local allowlist decisions.
type Match struct {
	Pattern Pattern
	Full    string
	Secret  string
}

func legacy(name, severity, source string) Pattern {
	return Pattern{Name: name, Severity: severity, Source: source, RE: regexp.MustCompilePOSIX(source)}
}

func withAllowlist(pattern Pattern, sources ...string) Pattern {
	pattern.Allowlist = make([]*regexp.Regexp, len(sources))
	for i, source := range sources {
		pattern.Allowlist[i] = regexp.MustCompile(source)
	}
	return pattern
}

func goPattern(name, severity, source string, secretGroup int, placeholder PlaceholderPolicy) Pattern {
	return Pattern{
		Name: name, Severity: severity, Source: source, RE: regexp.MustCompile(source),
		SecretGroup: secretGroup, Placeholder: placeholder,
	}
}

var (
	allOnce sync.Once
	allRows []Pattern
)

// All returns the pattern-major table, built once on first use so a hook
// that never scans content does not compile it. AWS_ACCESS_KEY keeps its
// exact legacy shape and table position, including low-entropy AKIA
// values. The separately named pinned row adds ASIA and the other
// upstream prefixes without narrowing it.
func All() []Pattern {
	allOnce.Do(func() { allRows = buildAll() })
	return allRows
}

func buildAll() []Pattern {
	return []Pattern{
		withAllowlist(legacy("AWS_ACCESS_KEY", "critical", `AKIA[0-9A-Z]{16}`), `.+EXAMPLE$`),
		legacy("AWS_SECRET_KEY", "critical", `aws_secret_access_key[[:space:]]*[:=][[:space:]]*['"]?[A-Za-z0-9/+=]{40}`),
		markerOnly(legacy("GCP_SERVICE_ACCOUNT", "critical", `"type"[[:space:]]*:[[:space:]]*"service_account"`)),
		legacy("GCP_API_KEY", "high", `AIza[0-9A-Za-z_-]{35}`),
		legacy("AZURE_CLIENT_SECRET", "critical", `azure[_-]?client[_-]?secret[[:space:]]*[:=][[:space:]]*['"]?[A-Za-z0-9_~.-]{34,}`),
		legacy("GITHUB_PAT", "critical", `ghp_[0-9A-Za-z]{36}`),
		legacy("GITHUB_OAUTH", "critical", `gho_[0-9A-Za-z]{36}`),
		legacy("GITHUB_APP_TOKEN", "critical", `ghs_[0-9A-Za-z]{36}`),
		legacy("GITHUB_REFRESH_TOKEN", "critical", `ghr_[0-9A-Za-z]{36}`),
		legacy("GITHUB_FINE_GRAINED_PAT", "critical", `github_pat_[0-9A-Za-z_]{82}`),
		markerOnly(legacy("PRIVATE_KEY", "critical", `-----BEGIN (RSA |EC |OPENSSH |DSA |PGP )?PRIVATE KEY-----`)),
		markerOnly(legacy("PGP_PRIVATE_BLOCK", "critical", `-----BEGIN PGP PRIVATE`+` KEY BLOCK-----`)),
		keyed(goPattern("GENERIC_SECRET", "high", `(?i)(secret|token|password|passwd|pwd|api[_-]?key|apikey|access[_-]?key|auth[_-]?token|client[_-]?secret)[[:space:]]*['"]?[[:space:]]*[:=][[:space:]]*['"]?(\$\{\{[^\r\n}]+\}\}[A-Za-z0-9_/+=~.$:{}-]*|\{\{[^\r\n}]+\}\}[A-Za-z0-9_/+=~.$:{}-]*|[A-Za-z0-9_/+=~.$:{}-]{8,})`, 2, PlaceholderWholeValue), "secret", "token", "passw", "pwd", "key"),
		keyed(legacy("CONNECTION_STRING", "high", `(mongodb(\+srv)?|postgres(ql)?|mysql|redis|amqp|mssql)://[^[:space:]'"]{10,}`), "://"),
		keyed(legacy("BEARER_TOKEN", "medium", `[Bb]earer[[:space:]]+[A-Za-z0-9_-]{20,}\.[A-Za-z0-9_-]{20,}`), "earer"),
		legacy("SLACK_TOKEN", "high", `xox[baprs]-[0-9]{10,}-[0-9A-Za-z-]+`),
		legacy("SLACK_WEBHOOK", "high", `https://hooks\.slack\.com/services/T[0-9A-Z]{8,}/B[0-9A-Z]{8,}/[0-9A-Za-z]{24}`),
		legacy("DISCORD_TOKEN", "high", `[MN][A-Za-z0-9]{23,}\.[A-Za-z0-9_-]{6}\.[A-Za-z0-9_-]{27,}`),
		legacy("TWILIO_API_KEY", "high", `SK[0-9a-fA-F]{32}`),
		legacy("SENDGRID_API_KEY", "high", `SG\.[0-9A-Za-z_-]{22}\.[0-9A-Za-z_-]{43}`),
		legacy("STRIPE_SECRET_KEY", "critical", `sk_live_[0-9A-Za-z]{24,}`),
		legacy("STRIPE_RESTRICTED_KEY", "high", `rk_live_[0-9A-Za-z]{24,}`),
		legacy("NPM_TOKEN", "high", `npm_[0-9A-Za-z]{36}`),
		legacy("JWT_TOKEN", "medium", `eyJ[A-Za-z0-9_-]{10,}\.eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}`),
		{
			Name: "INTERNAL_IP_PORT", Severity: "medium", Advisory: true,
			Source:   `(^|[^.0-9])(10\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}|172\.(1[6-9]|2[0-9]|3[01])\.[0-9]{1,3}\.[0-9]{1,3}|192\.168\.[0-9]{1,3}\.[0-9]{1,3}):[0-9]{2,5}([^0-9]|$)`,
			RE:       regexp.MustCompilePOSIX(`(^|[^.0-9])(10\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}|172\.(1[6-9]|2[0-9]|3[01])\.[0-9]{1,3}\.[0-9]{1,3}|192\.168\.[0-9]{1,3}\.[0-9]{1,3}):[0-9]{2,5}([^0-9]|$)`),
			Keywords: []string{"10.", "172.", "192.168."},
		},
		mustUpstream("aws-access-token", "AWS_ACCESS_TOKEN_GITLEAKS", "critical"),
		mustUpstream("openai-api-key", "OPENAI_API_KEY", "critical"),
		mustUpstream("anthropic-api-key", "ANTHROPIC_API_KEY", "critical"),
		mustUpstream("anthropic-admin-api-key", "ANTHROPIC_ADMIN_API_KEY", "critical"),
		mustUpstream("gitlab-pat", "GITLAB_PAT", "critical"),
		mustUpstream("gitlab-pat-routable", "GITLAB_PAT_ROUTABLE", "critical"),
		mustUpstream("hashicorp-tf-api-token", "HASHICORP_TF_API_TOKEN", "critical"),
		mustUpstream("age-secret-key", "AGE_SECRET_KEY", "critical"),
	}
}

// markerOnly flags a rule whose match is a fixed marker rather than the
// secret; see Pattern.MarkerOnly.
func markerOnly(pattern Pattern) Pattern {
	pattern.MarkerOnly = true
	return pattern
}

// keyed attaches the prefilter keywords a legacy rule's regexp requires;
// TestKeywordGatesAreSound proves each is a literal every match contains.
func keyed(pattern Pattern, keywords ...string) Pattern {
	pattern.Keywords = keywords
	return pattern
}

// templateOperand is one dotted identifier inside a template reference,
// optionally opened by the Go/Helm template cursor: .Values.x or $.Values.x.
const templateOperand = `(?:\$?\.)?[A-Za-z_][A-Za-z0-9_.-]*`

var (
	environmentReference = regexp.MustCompile(`^\$(?:[A-Za-z_][A-Za-z0-9_]*|\{[A-Za-z_][A-Za-z0-9_]*\})$`)

	// The MCPB manifest format passes a user-configured value into a
	// server's environment only through this reference, so a manifest cannot
	// avoid writing it beside a key-named member. It names where the value
	// comes from and holds none. Only a whole value is accepted: with any
	// other character around it the reference is part of a literal.
	userConfigReference = regexp.MustCompile(`^\$\{user_config\.[A-Za-z_][A-Za-z0-9_]*\}$`)

	// A template reference is one operand or an || chain of them; a quoted
	// or other literal operand is a value and stays a candidate (#699).
	templateReference = regexp.MustCompile(`^(?:\{\{|\$\{\{)[ \t]*` + templateOperand +
		`(?:[ \t]*\|\|[ \t]*` + templateOperand + `)*[ \t]*\}\}$`)
)

var placeholderValues = map[string]bool{
	"example": true, "placeholder": true, "changeme": true, "todo": true,
	"fixme": true, "dummy": true, "fake": true, "sample": true,
	"your_key": true, "your_token": true, "replace_me": true,
}

// IsPlaceholder accepts only an entire captured value: a stand-in word, an
// environment reference, a user-config reference (the braced dollar form
// naming one member of user_config), or a template reference. Each names
// where a value comes from or marks where one belongs, and holds none; the
// same text inside a longer value is part of a literal and is not accepted.
// Provider rules leave Placeholder at PlaceholderNone and therefore never
// call this exemption.
func IsPlaceholder(candidate string) bool {
	value := strings.TrimSpace(candidate)
	if len(value) >= 2 && ((value[0] == '\'' && value[len(value)-1] == '\'') ||
		(value[0] == '"' && value[len(value)-1] == '"')) {
		value = value[1 : len(value)-1]
	}
	normalized := strings.ReplaceAll(strings.ToLower(value), "-", "_")
	return placeholderValues[normalized] || environmentReference.MatchString(value) ||
		userConfigReference.MatchString(value) || templateReference.MatchString(value)
}

// FindAll folds the input itself; callers that scan many patterns over one
// input fold once and call FindAllFolded.
func FindAll(pattern Pattern, input string) []Match {
	if len(pattern.Keywords) == 0 {
		return FindAllFolded(pattern, input, "")
	}
	return FindAllFolded(pattern, input, asciifold.String(input))
}

// FindAllFolded applies every rule-local decision in one place. folded is
// asciifold.String(input), or empty when the pattern has no keywords.
// Callers retain pattern-major and line-major loops and use this function
// for the innermost match iteration, preserving existing observable ordering.
func FindAllFolded(pattern Pattern, input, folded string) []Match {
	if !hasKeyword(pattern, folded) {
		return nil
	}
	indices := pattern.RE.FindAllStringSubmatchIndex(input, -1)
	matches := make([]Match, 0, len(indices))
	for _, index := range indices {
		full := input[index[0]:index[1]]
		secret := full
		if pattern.SecretGroup > 0 {
			pair := pattern.SecretGroup * 2
			if pair+1 >= len(index) || index[pair] < 0 || index[pair+1] < 0 {
				continue
			}
			secret = input[index[pair]:index[pair+1]]
		}
		if pattern.Entropy > 0 && shannonEntropy(secret) <= pattern.Entropy {
			continue
		}
		if matchesAny(pattern.Allowlist, secret) {
			continue
		}
		if pattern.Placeholder == PlaceholderWholeValue && IsPlaceholder(secret) {
			continue
		}
		// The legacy generic assignment detector keeps its eight-byte
		// minimum for every value form. A complete template/reference may be
		// shorter and is already exempted above; a non-placeholder suffix is
		// a candidate only once the complete capture reaches eight characters.
		if pattern.Name == "GENERIC_SECRET" && utf8.RuneCountInString(secret) < 8 {
			continue
		}
		// #636: a keyword-named variable assigned from a code expression is
		// not a value. Only unquoted, code-shaped right-hand sides with a
		// code cue qualify; see isCodeExpression.
		if pattern.Name == "GENERIC_SECRET" && isCodeExpression(input, index) {
			continue
		}
		// A read through a listed environment accessor names where the
		// value comes from and holds none itself; see isEnvironmentAccessor.
		if pattern.Name == "GENERIC_SECRET" && isEnvironmentAccessor(input, index) {
			continue
		}
		matches = append(matches, Match{Pattern: pattern, Full: full, Secret: secret})
	}
	return matches
}

// codeExpressionValue is an unquoted right-hand side that reads as code:
// dot-separated segments of letters and underscores, the first starting
// lowercase. A digit or any other value character keeps it a candidate.
var codeExpressionValue = regexp.MustCompile(`^[a-z][A-Za-z_]*(?:\.[A-Za-z_]+)*$`)

// isCodeExpression reports whether a GENERIC_SECRET match assigns a code
// expression rather than a value. index is the match's submatch index:
// group 1 is the keyword, group 2 the value. The value must be unquoted and
// code-shaped, with one cue: a following ( ; , ) or backtick, a dotted
// chain, or an = with whitespace on both sides. Any other byte after the
// value means the capture stopped inside a longer value, such as a password
// continuing with @ or #, so the match stays a finding. The colon operator
// has no spacing cue, so a YAML value at the end of a line stays a finding.
func isCodeExpression(input string, index []int) bool {
	value := input[index[4]:index[5]]
	if !codeExpressionValue.MatchString(value) {
		return false
	}
	operator := input[index[3]:index[4]]
	if opensQuote(operator) {
		return false
	}
	if end := index[5]; end < len(input) {
		if strings.IndexByte("(;,)`", input[end]) >= 0 {
			return true
		}
		if !isRegexpSpace(input[end]) {
			return false
		}
	}
	if strings.Contains(value, ".") {
		return true
	}
	at := strings.IndexAny(operator, ":=")
	return operator[at] == '=' && at > 0 && isRegexpSpace(operator[at-1]) &&
		at+1 < len(operator) && isRegexpSpace(operator[at+1])
}

// environmentAccessors maps each exempt environment accessor to the byte
// that must directly follow it: the bracket that opens its lookup. The list
// is closed on purpose. A general rule of "an identifier followed by a
// bracket" would also exempt an unquoted secret that happens to be followed
// by a bracket, so an accessor is exempt only when it is spelled here in
// full and followed by its own bracket.
var environmentAccessors = map[string]byte{
	"os.environ":                         '[',
	"env::var":                           '(',
	"std::env::var":                      '(',
	"System.getenv":                      '(',
	"Environment.GetEnvironmentVariable": '(',
	"ENV.fetch":                          '(',
}

// isEnvironmentAccessor reports whether a GENERIC_SECRET match assigns a
// lookup through one of environmentAccessors rather than a value. index is
// the match's submatch index, as in isCodeExpression. Three conditions must
// all hold. The captured value is exactly a listed accessor, so a longer
// value that merely starts with one stays a finding. No quote precedes it,
// because a quoted accessor name is a string literal whatever follows. The
// next byte is the accessor's own bracket, so a bare name, which reads
// nothing, gains no exemption here. This only adds allowed inputs; whatever
// isCodeExpression allows is decided there first.
func isEnvironmentAccessor(input string, index []int) bool {
	next, listed := environmentAccessors[input[index[4]:index[5]]]
	if !listed {
		return false
	}
	if opensQuote(input[index[3]:index[4]]) {
		return false
	}
	end := index[5]
	return end < len(input) && input[end] == next
}

// opensQuote reports whether the text between a GENERIC_SECRET match's
// keyword and its value ends with a quote. A value opened by either quote
// character is a string literal, so no code-shaped reading of it applies.
func opensQuote(operator string) bool {
	return strings.HasSuffix(operator, `'`) || strings.HasSuffix(operator, `"`)
}

// isRegexpSpace matches the bytes of the row's [[:space:]] class.
func isRegexpSpace(b byte) bool {
	return strings.IndexByte(" \t\n\v\f\r", b) >= 0
}

// hasKeyword is the prefilter: folded is the asciifold of the input, so a
// missing keyword proves the rule cannot match (TestKeywordGatesAreSound).
func hasKeyword(pattern Pattern, folded string) bool {
	if len(pattern.Keywords) == 0 {
		return true
	}
	for _, keyword := range pattern.Keywords {
		if strings.Contains(folded, keyword) {
			return true
		}
	}
	return false
}

func matchesAny(expressions []*regexp.Regexp, value string) bool {
	for _, expression := range expressions {
		if expression.MatchString(value) {
			return true
		}
	}
	return false
}

func shannonEntropy(value string) float64 {
	if value == "" {
		return 0
	}
	counts := make(map[rune]int)
	length := 0
	for _, char := range value {
		counts[char]++
		length++
	}
	entropy := 0.0
	for _, count := range counts {
		probability := float64(count) / float64(length)
		entropy -= probability * math.Log2(probability)
	}
	return entropy
}
