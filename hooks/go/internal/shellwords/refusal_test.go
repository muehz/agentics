package shellwords

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hosttest"
)

const leakMarker = "leakmarkerS8"

// UnmarshalText reads a rule name back. It exists only in test builds, so production code cannot turn arbitrary text
// into a rule.
func (r *Rule) UnmarshalText(text []byte) error {
	if len(text) == 0 {
		*r = RuleNone
		return nil
	}
	for rule := ruleFirst; rule <= ruleLast; rule++ {
		if rule.String() == string(text) {
			*r = rule
			return nil
		}
	}
	return fmt.Errorf("unknown script rule %q", text)
}

// UnmarshalText reads a construct name back, in test builds only.
func (c *construct) UnmarshalText(text []byte) error {
	if len(text) == 0 {
		*c = constructNone
		return nil
	}
	for k := constructFirst; k <= constructLast; k++ {
		if k.String() == string(text) {
			*c = k
			return nil
		}
	}
	return fmt.Errorf("unknown script construct %q", text)
}

// formPatterns matches each form template in full, the most specific template first, so that the timeout and flock
// option texts are not read as the generic wrapper option.
var formPatterns = func() []struct {
	kind formKind
	re   *regexp.Regexp
} {
	var out []struct {
		kind formKind
		re   *regexp.Regexp
	}
	for k := formFirst; k <= formLast; k++ {
		quoted := regexp.QuoteMeta(formTemplates[k])
		quoted = strings.Replace(quoted, "%d", strconv.Itoa(maxWrapperDepth), 1)
		quoted = strings.Replace(quoted, "%s", "(.+?)", 1)
		quoted = strings.Replace(quoted, "%s", "(.+)", 1)
		out = append(out, struct {
			kind formKind
			re   *regexp.Regexp
		}{k, regexp.MustCompile("^" + quoted + "$")})
	}
	sort.SliceStable(out, func(i, j int) bool { return len(formTemplates[out[i].kind]) > len(formTemplates[out[j].kind]) })
	return out
}()

// UnmarshalText parses rendered text back into its closed kind, in test builds only, so golden files that hold today's
// text can be decoded. Text that no kind renders is an error.
func (r *Refusal) UnmarshalText(text []byte) error {
	s := string(text)
	if s == "" {
		*r = Refusal{}
		return nil
	}
	for k := cancelFirst; k <= cancelLast; k++ {
		if cancelTexts[k] == s {
			*r = Refusal{cancel: k}
			return nil
		}
	}
	for g := guardFirst; g <= guardLast; g++ {
		if g.String() == s {
			*r = GuardRefusal(g)
			return nil
		}
	}
	if strings.HasPrefix(s, "script-") {
		head, label, located := strings.Cut(s, " in ")
		var origin *ScriptOrigin
		if located {
			origin = parseLabel(label)
		}
		name, cons, opaque := strings.Cut(head, ":")
		var rule Rule
		if err := rule.UnmarshalText([]byte(name)); err != nil {
			return err
		}
		if opaque != (rule == RuleOpaque) {
			return fmt.Errorf("refusal %q: a construct goes with script-opaque only", s)
		}
		if opaque {
			var c construct
			if err := c.UnmarshalText([]byte(cons)); err != nil {
				return err
			}
			*r = opaqueRefusal(c, origin)
		} else {
			*r = ruleRefusal(rule, origin)
		}
		return nil
	}
	for _, p := range formPatterns {
		if m := p.re.FindStringSubmatch(s); m != nil {
			*r = formRefusal(p.kind, m[1:]...)
			return nil
		}
	}
	return fmt.Errorf("no refusal kind renders %q", s)
}

// parseLabel reverses ScriptOrigin.label for paths that hold no " (via " and no trailing ":<digits>" of their own.
func parseLabel(label string) *ScriptOrigin {
	o := &ScriptOrigin{}
	if i := strings.LastIndex(label, " (via "); i >= 0 && strings.HasSuffix(label, ")") {
		o.Chain = strings.Split(label[i+len(" (via "):len(label)-1], " > ")
		label = label[:i]
	}
	if i := strings.LastIndex(label, ":"); i >= 0 {
		if n, err := strconv.Atoi(label[i+1:]); err == nil && n > 0 {
			o.Line, label = n, label[:i]
		}
	}
	o.Path = label
	return o
}

// allRules lists every named rule from the first to the last.
func allRules() []Rule {
	var out []Rule
	for r := ruleFirst; r <= ruleLast; r++ {
		out = append(out, r)
	}
	return out
}

// readmeSection returns the lines of hooks/README.md under the heading, up to the next heading.
func readmeSection(t *testing.T, heading string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(hosttest.Checkout(t), "hooks", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	start := slices.Index(lines, heading)
	if start < 0 {
		t.Fatalf("hooks/README.md has no heading %q", heading)
	}
	var out []string
	for _, line := range lines[start+1:] {
		if strings.HasPrefix(line, "#") {
			break
		}
		out = append(out, line)
	}
	return out
}

// tableRows returns the cells of the first table in lines, without the header and separator rows.
func tableRows(lines []string) [][]string {
	var rows [][]string
	seen := false
	for _, line := range lines {
		if !strings.HasPrefix(line, "|") {
			if seen {
				break
			}
			continue
		}
		seen = true
		cells := strings.Split(strings.Trim(line, "|"), "|")
		for i := range cells {
			cells[i] = strings.TrimSpace(cells[i])
		}
		rows = append(rows, cells)
	}
	if len(rows) < 2 {
		return nil
	}
	return rows[2:]
}

func TestRuleTableIsClosed(t *testing.T) {
	rows := tableRows(readmeSection(t, "### Command-guardian refusal reasons"))
	if len(rows) == 0 {
		t.Fatal("the refusal reasons table has no rows")
	}
	rules := allRules()
	if len(rules) != 22 {
		t.Fatalf("got %d named rules, want 22", len(rules))
	}
	seen := map[string]bool{}
	for _, rule := range rules {
		name := rule.String()
		if !strings.HasPrefix(name, "script-") {
			t.Errorf("rule %d name %q does not start with script-", rule, name)
		}
		if seen[name] {
			t.Errorf("rule name %q is not unique", name)
		}
		seen[name] = true
		if rule.Hint() == "" {
			t.Errorf("rule %s has no hint", name)
		}
		cell := "`" + name + "`"
		if rule == RuleOpaque {
			cell = "`script-opaque:<construct>`"
		}
		found := false
		for _, row := range rows {
			if len(row) >= 2 && strings.Contains(row[0], cell) {
				found = true
				if row[1] != rule.Hint() {
					t.Errorf("rule %s: README remedy %q, hint %q", name, row[1], rule.Hint())
				}
			}
		}
		if !found {
			t.Errorf("rule %s has no row in the README refusal reasons table", name)
		}
	}
	if RuleNone.String() != "" || RuleNone.Hint() != "" {
		t.Errorf("RuleNone renders %q with hint %q, want both empty", RuleNone.String(), RuleNone.Hint())
	}
	if RuleOpaque.String() != "script-opaque" {
		t.Errorf("RuleOpaque renders %q", RuleOpaque.String())
	}
	for _, rule := range rules {
		var back Rule
		text, err := rule.MarshalText()
		if err != nil || back.UnmarshalText(text) != nil || back != rule {
			t.Errorf("rule %s does not round-trip: %q, %v, %v", rule, text, err, back)
		}
	}
	var bogus Rule
	if bogus.UnmarshalText([]byte("script-nope")) == nil {
		t.Error("an unknown rule name was accepted")
	}
}

func TestConstructVocabularyMatchesReadme(t *testing.T) {
	var readme []string
	for _, row := range tableRows(readmeSection(t, "### Command-guardian inside the script")) {
		readme = append(readme, strings.Trim(row[0], "`"))
	}
	var ours []string
	for c := constructFirst; c <= constructLast; c++ {
		ours = append(ours, c.String())
		var back construct
		if err := back.UnmarshalText([]byte(c.String())); err != nil || back != c {
			t.Errorf("construct %q does not round-trip: %v", c, err)
		}
	}
	if len(ours) != 24 {
		t.Errorf("got %d constructs, want 24", len(ours))
	}
	sort.Strings(readme)
	sort.Strings(ours)
	if !slices.Equal(readme, ours) {
		t.Errorf("constructs differ from the README:\n got %q\nwant %q", ours, readme)
	}
	if constructNone.String() != "" {
		t.Errorf("constructNone renders %q", constructNone.String())
	}
	var bogus construct
	if bogus.UnmarshalText([]byte("nope")) == nil {
		t.Error("an unknown construct name was accepted")
	}
}

// sampleForms is one refusal per form kind, with a sample executable and word.
func sampleForms(word string) map[formKind]Refusal {
	return map[formKind]Refusal{
		formShellContract:      formRefusal(formShellContract, "bash"),
		formPowerShellContract: formRefusal(formPowerShellContract, "pwsh"),
		formEnvName:            formRefusal(formEnvName, "-u"),
		formWrapperOption:      formRefusal(formWrapperOption, "sudo", word),
		formWrapperValue:       formRefusal(formWrapperValue, "sudo", "-u"),
		formNoChild:            formRefusal(formNoChild, "sudo"),
		formXargsChild:         formRefusal(formXargsChild),
		formTimeoutDuration:    formRefusal(formTimeoutDuration),
		formTimeoutOption:      formRefusal(formTimeoutOption, word),
		formFlockOption:        formRefusal(formFlockOption, word),
		formChildNotLiteral:    formRefusal(formChildNotLiteral, "nohup"),
		formCommandString:      formRefusal(formCommandString, "bash"),
		formWrapperDepth:       formRefusal(formWrapperDepth, "time"),
		formUnterminated:       formRefusal(formUnterminated),
		formRedirection:        formRefusal(formRedirection, ">"),
		formANSIControl:        formRefusal(formANSIControl),
		formANSIByte:           formRefusal(formANSIByte),
		formANSIUnterminated:   formRefusal(formANSIUnterminated),
	}
}

func TestRefusalRendersTodaysText(t *testing.T) {
	origin := &ScriptOrigin{Path: "/ws/t.sh", Line: 3, Chain: []string{"/ws/a.sh"}}
	forms := sampleForms("--bogus")
	rows := []struct {
		name string
		r    Refusal
		want string
	}{
		{"zero", Refusal{}, ""},
		{"unreadable bare", ruleRefusal(RuleUnreadable, nil), "script-unreadable"},
		{"unreadable located", ruleRefusal(RuleUnreadable, origin), "script-unreadable in /ws/t.sh:3 (via /ws/a.sh)"},
		{"located later", ruleRefusal(RuleDepth, nil).locatedAt(&ScriptOrigin{Path: "/ws/t.sh", Line: 4}), "script-depth in /ws/t.sh:4"},
		{"opaque eval", opaqueRefusal(constructEval, &ScriptOrigin{Path: "/ws/t.sh", Line: 2}), "script-opaque:eval in /ws/t.sh:2"},
		{"opaque bare", opaqueRefusal(constructHeredocDelimiter, nil), "script-opaque:heredoc-delimiter"},
		{"caller argument", CallerArgumentRefusal(ScriptCall{Path: "/ws/t.sh"}), "script-caller-argument in /ws/t.sh"},
		{"control character", ruleRefusal(RuleUnreadable, &ScriptOrigin{Path: "/ws/a\x1b[31m.sh", Chain: []string{"/ws/\u202eb.sh"}}), "script-unreadable in /ws/a?[31m.sh (via /ws/?b.sh)"},
		{"shell contract", forms[formShellContract], "bash requires exactly -c or -lc and a literal command string"},
		{"powershell contract", forms[formPowerShellContract], "pwsh requires optional PowerShell flags, then -File <literal .ps1> or -Command and one literal token"},
		{"env name", forms[formEnvName], "env -u requires a literal name"},
		{"wrapper option", forms[formWrapperOption], "sudo option --bogus is outside the literal wrapper contract"},
		{"wrapper value", forms[formWrapperValue], "sudo -u requires a literal value"},
		{"no child", forms[formNoChild], "sudo has no child executable"},
		{"xargs child", forms[formXargsChild], "xargs has no literal child executable"},
		{"timeout duration", forms[formTimeoutDuration], "timeout requires a literal duration before the command"},
		{"timeout option", forms[formTimeoutOption], "timeout option --bogus after the duration is outside the literal wrapper contract"},
		{"flock option", forms[formFlockOption], "flock option --bogus after the lock file is outside the literal wrapper contract"},
		{"child not literal", forms[formChildNotLiteral], "nohup child executable is not literal"},
		{"command string", forms[formCommandString], "bash command string is not literal"},
		{"wrapper depth", forms[formWrapperDepth], "wrapper depth exceeds 4 at time"},
		{"unterminated", forms[formUnterminated], "unterminated quoted word"},
		{"redirection", forms[formRedirection], "redirection > has no target"},
		{"ansi control", forms[formANSIControl], "ANSI-C control escape is outside the literal parser contract"},
		{"ansi byte", forms[formANSIByte], "ANSI-C non-ASCII or NUL escape is outside the literal parser contract"},
		{"ansi unterminated", forms[formANSIUnterminated], "unterminated ANSI-C quoted word"},
		{"shell canceled", cancelRefusal(POSIX), "shell inspection canceled"},
		{"powershell canceled", cancelRefusal(PowerShell), "PowerShell environment inspection canceled"},
		{"guard authority", GuardRefusal(GuardBashAuthorityUnavailable), "Bash inspection authority unavailable"},
		{"guard syntax operand", GuardRefusal(GuardBashSyntaxOperandUnavailable), "Bash syntax file read operand unavailable"},
		{"guard search canceled", GuardRefusal(GuardSearchCanceled), "search argument inspection canceled"},
		{"guard bash canceled", GuardRefusal(GuardBashCanceled), "Bash inspection canceled"},
		{"guard brace", GuardRefusal(GuardSearchBraceExpansion), "unsupported search brace expansion"},
	}
	for _, row := range rows {
		if got := row.r.String(); got != row.want {
			t.Errorf("%s: String() = %q, want %q", row.name, got, row.want)
		}
		if got := fmt.Sprintf("%s", row.r); got != row.want {
			t.Errorf("%s: %%s = %q, want %q", row.name, got, row.want)
		}
		if got, err := row.r.MarshalText(); err != nil || string(got) != row.want {
			t.Errorf("%s: MarshalText() = %q, %v", row.name, got, err)
		}
		if row.r.Refused() != (row.want != "") {
			t.Errorf("%s: Refused() = %v", row.name, row.r.Refused())
		}
		remedy, ok := row.r.Remedy()
		if row.r.isNamedRule() {
			want := row.r.Rule().Hint() + " (" + row.want + ")"
			if !ok || remedy != want || row.r.Hint() != row.r.Rule().Hint() || row.r.Hint() == "" {
				t.Errorf("%s: Remedy() = %q, %v; want %q, true", row.name, remedy, ok, want)
			}
		} else if ok || remedy != "" || row.r.Hint() != "" {
			t.Errorf("%s: Remedy() = %q, %v and Hint() = %q; want no remedy", row.name, remedy, ok, row.r.Hint())
		}
		var back Refusal
		if err := back.UnmarshalText([]byte(row.want)); err != nil || back.String() != row.want {
			t.Errorf("%s: %q does not round-trip: %v, %q", row.name, row.want, err, back.String())
		}
	}

	// A remedy pinned in full, so the composition is not only checked against itself.
	remedy, ok := CallerArgumentRefusal(ScriptCall{Path: "/ws/t.sh"}).Remedy()
	if want := hintCallerIn + " (script-caller-argument in /ws/t.sh)"; !ok || remedy != want {
		t.Errorf("caller-argument Remedy() = %q, %v; want %q", remedy, ok, want)
	}

	// The origin is a copy in both directions, and only a named rule carries one.
	r := ruleRefusal(RuleUnreadable, origin)
	origin.Chain[0], origin.Path = "/ws/changed.sh", "/ws/changed.sh"
	got := r.Origin()
	if got == nil || got.Path != "/ws/t.sh" || got.Line != 3 || !slices.Equal(got.Chain, []string{"/ws/a.sh"}) {
		t.Fatalf("Origin() = %+v, want the location at construction", got)
	}
	got.Chain[0] = "/ws/x.sh"
	if r.Origin().Chain[0] != "/ws/a.sh" {
		t.Error("Origin() shares its chain with the refusal")
	}
	if ruleRefusal(RuleUnreadable, nil).Origin() != nil {
		t.Error("a bare rule reports an origin")
	}
	if forms[formNoChild].locatedAt(&ScriptOrigin{Path: "/ws/t.sh"}).Origin() != nil {
		t.Error("a form kind took a location it does not render")
	}
	if ruleRefusal(RuleNone, origin).Refused() {
		t.Error("ruleRefusal(RuleNone) refuses")
	}
	if c := opaqueRefusal(constructEval, nil).Construct(); c != "eval" {
		t.Errorf("Construct() = %q, want eval", c)
	}
	if c := ruleRefusal(RuleUnreadable, nil).Construct(); c != "" {
		t.Errorf("Construct() on a non-opaque rule = %q", c)
	}

	var bad Refusal
	for _, text := range []string{"script-nope", "script-opaque:nope", "script-unreadable:eval", "something else", "sudo option"} {
		if bad.UnmarshalText([]byte(text)) == nil {
			t.Errorf("UnmarshalText accepted %q", text)
		}
	}
}

func TestRefusalGuardZeroAndUnknown(t *testing.T) {
	if r := GuardRefusal(GuardNone); r.Refused() || r.String() != "" || r != (Refusal{}) {
		t.Errorf("GuardRefusal(GuardNone) = %+v, want the zero refusal", r)
	}
	if GuardNone.String() != "" {
		t.Errorf("GuardNone renders %q", GuardNone.String())
	}
	if text, err := GuardNone.MarshalText(); err != nil || len(text) != 0 {
		t.Errorf("GuardNone.MarshalText() = %q, %v", text, err)
	}
	defer func() {
		msg := fmt.Sprint(recover())
		if !strings.Contains(msg, "GuardReason(99)") {
			t.Errorf("GuardRefusal(99) panic = %q, want it to name the value", msg)
		}
	}()
	GuardRefusal(GuardReason(99))
}

// everyKind builds one refusal of every kind, with the marker in every input its constructor takes except the script
// and chain paths, which are accepted content.
func everyKind() []Refusal {
	origin := &ScriptOrigin{Path: "/ws/t.sh", Line: 7, Chain: []string{"/ws/a.sh"}}
	var out []Refusal
	for _, rule := range allRules() {
		if rule != RuleOpaque {
			out = append(out, ruleRefusal(rule, origin), ruleRefusal(rule, nil))
		}
	}
	for c := constructFirst; c <= constructLast; c++ {
		out = append(out, opaqueRefusal(c, origin))
	}
	out = append(out, CallerArgumentRefusal(ScriptCall{Path: "/ws/t.sh", Args: []string{leakMarker}}))
	for k := formFirst; k <= formLast; k++ {
		args := make([]string, formArity(k))
		for i := range args {
			args[i] = leakMarker
		}
		out = append(out, formRefusal(k, args...))
	}
	out = append(out, cancelRefusal(POSIX), cancelRefusal(PowerShell))
	for g := guardFirst; g <= guardLast; g++ {
		out = append(out, GuardRefusal(g))
	}
	return out
}

func TestRefusalKindsAreContentFree(t *testing.T) {
	forms, canceled := 0, 0
	for _, r := range everyKind() {
		remedy, _ := r.Remedy()
		if r.ContentFree() {
			if strings.Contains(r.String(), leakMarker) || strings.Contains(remedy, leakMarker) {
				t.Errorf("content-free kind %q renders the marker", r.String())
			}
		} else {
			forms++
			if r.form == formNone {
				t.Errorf("%q reports content but is not a form kind", r.String())
			}
		}
		if r.form != formNone && r.ContentFree() {
			t.Errorf("form kind %q reports content-free", r.String())
		}
		if r.Canceled() {
			canceled++
			switch {
			case r.cancel != cancelNone, r.guard == GuardSearchCanceled, r.guard == GuardBashCanceled:
			default:
				t.Errorf("%q reports canceled", r.String())
			}
		}
	}
	if forms != 18 {
		t.Errorf("%d kinds report content, want the 18 form kinds", forms)
	}
	if canceled != 4 {
		t.Errorf("%d kinds report canceled, want 4", canceled)
	}
}

// TestRefusalFormsQuoteOnlyClassifiedWords parses commands that produce each form kind today and requires the exact
// text the kind renders. The marker sits in every position the kind does not quote: option values, the child
// command, the -c string, and the quoted body.
func TestRefusalFormsQuoteOnlyClassifiedWords(t *testing.T) {
	m := leakMarker
	rows := []struct {
		command string
		dialect Dialect
		kind    formKind
		args    []string
		want    string
	}{
		{"bash -x " + m, POSIX, formShellContract, []string{"bash"}, "bash requires exactly -c or -lc and a literal command string"},
		{"sh " + m, POSIX, formShellContract, []string{"sh"}, "sh requires exactly -c or -lc and a literal command string"},
		{"pwsh -File " + m, POSIX, formPowerShellContract, []string{"pwsh"}, "pwsh requires optional PowerShell flags, then -File <literal .ps1> or -Command and one literal token"},
		{"powershell -Command " + m + " " + m, POSIX, formPowerShellContract, []string{"powershell"}, "powershell requires optional PowerShell flags, then -File <literal .ps1> or -Command and one literal token"},
		{"env -u \"$" + m + "\" x", POSIX, formEnvName, []string{"-u"}, "env -u requires a literal name"},
		{"env --unset", POSIX, formEnvName, []string{"--unset"}, "env --unset requires a literal name"},
		{"sudo --bogus " + m, POSIX, formWrapperOption, []string{"sudo", "--bogus"}, "sudo option --bogus is outside the literal wrapper contract"},
		{"env --foo=bar " + m, POSIX, formWrapperOption, []string{"env", "--foo=bar"}, "env option --foo=bar is outside the literal wrapper contract"},
		{"sudo -uleakvalue " + m, POSIX, formWrapperOption, []string{"sudo", "-uleakvalue"}, "sudo option -uleakvalue is outside the literal wrapper contract"},
		{"sudo -$(leakvalue) " + m, POSIX, formWrapperOption, []string{"sudo", "-$(leakvalue)"}, "sudo option -$(leakvalue) is outside the literal wrapper contract"},
		{"sudo -u \"$" + m + "\" x", POSIX, formWrapperValue, []string{"sudo", "-u"}, "sudo -u requires a literal value"},
		{"flock -w", POSIX, formWrapperValue, []string{"flock", "-w"}, "flock -w requires a literal value"},
		{"env FOO=" + m, POSIX, formNoChild, []string{"env"}, "env has no child executable"},
		{"xargs -n " + m, POSIX, formXargsChild, nil, "xargs has no literal child executable"},
		{"timeout " + m + " x", POSIX, formTimeoutDuration, nil, "timeout requires a literal duration before the command"},
		{"timeout 10 -s " + m, POSIX, formTimeoutOption, []string{"-s"}, "timeout option -s after the duration is outside the literal wrapper contract"},
		{"timeout 10 -sleakvalue " + m, POSIX, formTimeoutOption, []string{"-sleakvalue"}, "timeout option -sleakvalue after the duration is outside the literal wrapper contract"},
		{"flock /tmp/l -c " + m, POSIX, formFlockOption, []string{"-c"}, "flock option -c after the lock file is outside the literal wrapper contract"},
		{"flock /tmp/l -cleakvalue " + m, POSIX, formFlockOption, []string{"-cleakvalue"}, "flock option -cleakvalue after the lock file is outside the literal wrapper contract"},
		{"nohup \"$" + m + "\" " + m, POSIX, formChildNotLiteral, []string{"nohup"}, "nohup child executable is not literal"},
		{"bash -c \"$" + m + "\"", POSIX, formCommandString, []string{"bash"}, "bash command string is not literal"},
		{"sudo nice nohup env time " + m, POSIX, formWrapperDepth, []string{"time"}, "wrapper depth exceeds 4 at time"},
		{"echo '" + m, POSIX, formUnterminated, nil, "unterminated quoted word"},
		{"echo " + m + " >", POSIX, formRedirection, []string{">"}, "redirection > has no target"},
		{"echo " + m + " >>", POSIX, formRedirection, []string{">>"}, "redirection >> has no target"},
		{"echo $'" + m + "\\cK'", POSIX, formANSIControl, nil, "ANSI-C control escape is outside the literal parser contract"},
		{"echo $'" + m + "\\0'", POSIX, formANSIByte, nil, "ANSI-C non-ASCII or NUL escape is outside the literal parser contract"},
		{"echo $'" + m + "\\'", POSIX, formANSIUnterminated, nil, "unterminated ANSI-C quoted word"},
	}
	seen := map[formKind]bool{}
	for _, row := range rows {
		r := formRefusal(row.kind, row.args...)
		if got := r.String(); got != row.want {
			t.Errorf("%q: the kind renders %q, want %q", row.command, got, row.want)
		}
		if strings.Contains(row.want, m) {
			t.Errorf("%q: the expected text %q quotes the marker", row.command, row.want)
		}
		var reasons []string
		for _, inv := range Parse(row.command, row.dialect).Invocations {
			reasons = append(reasons, inv.Reason.String())
		}
		if !slices.Contains(reasons, row.want) {
			t.Errorf("%q: parse reasons %q do not hold %q", row.command, reasons, row.want)
		}
		seen[row.kind] = true
	}
	for k := formFirst; k <= formLast; k++ {
		if !seen[k] {
			t.Errorf("form kind %q has no row", formTemplates[k])
		}
	}
}

func TestRefusalMarshalsAsText(t *testing.T) {
	for _, row := range []struct {
		r    Refusal
		want string
	}{
		{Refusal{}, `{"Reason":""}`},
		{formRefusal(formRedirection, ">"), `{"Reason":"redirection ` + `\` + `u003e has no target"}`},
	} {
		got, err := json.Marshal(struct{ Reason Refusal }{row.r})
		if err != nil || string(got) != row.want {
			t.Errorf("json.Marshal = %s, %v; want %s", got, err, row.want)
		}
		var back struct{ Reason Refusal }
		if err := json.Unmarshal(got, &back); err != nil || back.Reason.String() != row.r.String() {
			t.Errorf("json.Unmarshal(%s) = %q, %v", got, back.Reason.String(), err)
		}
	}

	// Every reason the parse golden file holds decodes into a kind and encodes back to the same bytes.
	data, err := os.ReadFile(filepath.Join("testdata", "parse_golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	var golden []struct {
		Result struct {
			Invocations []struct{ Reason json.RawMessage }
		} `json:"result"`
	}
	if err := json.Unmarshal(data, &golden); err != nil {
		t.Fatal(err)
	}
	refused := 0
	for _, entry := range golden {
		for _, inv := range entry.Result.Invocations {
			var r Refusal
			if err := json.Unmarshal(inv.Reason, &r); err != nil {
				t.Errorf("golden reason %s: %v", inv.Reason, err)
				continue
			}
			if got, _ := json.Marshal(r); string(got) != string(inv.Reason) {
				t.Errorf("golden reason %s re-encodes as %s", inv.Reason, got)
			}
			if r.Refused() {
				refused++
			}
		}
	}
	if refused == 0 {
		t.Error("the golden file holds no refusal, so the round trip proves nothing")
	}
}
