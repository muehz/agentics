package flow

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"
)

// verifyForm names the documented shape a verify row's evidence takes.
type verifyForm string

const (
	// formCommit is commit=<sha> with patch-id=<id>, or commit=<sha> alone when
	// that commit is a merge; a parse by shape cannot tell, so the merge test
	// is left to callers that can read the repository.
	formCommit verifyForm = "commit"
	// formIsolated is base=<sha> head=<sha> integrated=<sha> patch-id=<id>.
	formIsolated verifyForm = "isolated"
	// formFailure is cause=<word> alone, a failure with no commit to name.
	formFailure verifyForm = "failure"
	// formBranch is branch=<branch> patch-id=<id> on a failed fix.
	formBranch verifyForm = "branch"
	// formNoCommit is the red no-commit form, which quotes a failing run.
	formNoCommit verifyForm = "no-commit"
	// formIntegrated is commit=<sha>, optionally with patch-id=<id>, on a
	// wave, group, or final target.
	formIntegrated verifyForm = "integrated"
)

// verifyEvidence is a verify cell parsed into its form and values, so that
// callers resolving or auditing commits read the values without parsing again.
// A value the form does not carry is empty.
type verifyEvidence struct {
	form       verifyForm
	commit     string
	patchID    string
	base       string
	head       string
	integrated string
	branch     string
	cause      string
}

const (
	shaShape     = "a commit id of 7 to 64 lowercase hex"
	patchIDShape = "a patch-id of 40 or 64 lowercase hex"
	wordShape    = "a word of a-z 0-9 -"
	sliceShape   = "a comma-separated list of S<n> ids"

	commitFormText   = "commit=<sha> patch-id=<id>; commit=<sha> on a merge commit"
	isolatedFormText = "base=<sha> head=<sha> integrated=<sha> patch-id=<id>"
	failureFormText  = "cause=<word>"
	branchFormText   = "branch=<branch> patch-id=<id>"
)

var (
	evidencePatchID = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`^([0-9a-f]{40}|[0-9a-f]{64})$`) })
	evidenceWord    = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`^[a-z0-9-]+$`) })
)

// verifyClass is the class of a verify target, which decides the forms its evidence may take.
func verifyClass(target string) string {
	switch {
	case target == "final", strings.HasPrefix(target, "wave-"), strings.HasPrefix(target, "group-"):
		return "integrated"
	// The same target test as evidenceCell's quoted test, so a red row's no-commit cell and its class agree.
	case strings.HasPrefix(target, "red-"):
		return "red"
	case target == "fix":
		return "fix"
	}
	return "task"
}

// verifyKeys lists the keys any form of the class carries, whatever the status.
func verifyKeys(class string) string {
	switch class {
	case "integrated":
		return "commit patch-id"
	case "red":
		return "commit patch-id"
	case "fix":
		return "commit patch-id branch"
	}
	return "commit patch-id base head integrated"
}

// verifyForms describes the forms the class takes with the status, for the refusal that names them.
func verifyForms(class, status string) string {
	forms := []string{commitFormText}
	switch class {
	case "integrated":
		forms = []string{"commit=<sha>", "commit=<sha> patch-id=<id>"}
	case "red":
		forms = append(forms, noCommitShape)
	case "fix":
		if status == "failed" {
			forms = append(forms, failureFormText, branchFormText)
		}
	default:
		forms = append(forms, isolatedFormText)
		if status == "failed" {
			forms = append(forms, failureFormText)
		}
	}
	return strings.Join(forms, "; ")
}

// parseVerify parses the evidence of a verify row by shape alone. It does not
// accept auto or auto:<sha>, which the writer resolves into the commit form
// before storing, so a stored auto cell reads as invalid. The error is a
// *cellError whose reason names what the cell breaks.
func parseVerify(target, status, evidence string) (verifyEvidence, error) {
	fail := func(reason string) (verifyEvidence, error) {
		return verifyEvidence{}, invalidCell("evidence", evidence, reason)
	}
	class := verifyClass(target)
	if class == "red" && strings.HasPrefix(evidence, "no-commit") {
		if err := validateNoCommit(evidence); err != nil {
			return verifyEvidence{}, err
		}
		return verifyEvidence{form: formNoCommit}, nil
	}
	tokens := strings.Fields(evidence)
	// Each rule is checked over the whole cell before the next, so a cell that
	// breaks several rules always reports the same one.
	for _, token := range tokens {
		if !evidencePair().MatchString(token) {
			return fail(token + " is not a key=value pair")
		}
	}
	values := make(map[string]string, len(tokens))
	for _, token := range tokens {
		key, value, _ := strings.Cut(token, "=")
		if _, seen := values[key]; seen {
			return fail("key " + key + " repeats")
		}
		values[key] = value
	}
	allowed := verifyKeys(class) + " cause slice"
	for _, token := range tokens {
		if key, _, _ := strings.Cut(token, "="); !inSet(key, allowed) {
			return fail("key " + key + " belongs to no " + class + " form")
		}
	}
	for _, token := range tokens {
		key, value, _ := strings.Cut(token, "=")
		shape := ""
		switch key {
		case "commit", "base", "head", "integrated":
			if !evidenceCommitID().MatchString(value) {
				shape = shaShape
			}
		case "patch-id":
			if !evidencePatchID().MatchString(value) {
				shape = patchIDShape
			}
		case "slice":
			if !evidenceSlices().MatchString(value) {
				shape = sliceShape
			}
		case "cause":
			if !evidenceWord().MatchString(value) {
				shape = wordShape
			}
		}
		if shape != "" {
			return fail(token + " is not " + shape)
		}
	}
	if len(tokens) == 0 {
		return fail("evidence is empty")
	}
	parsed := verifyEvidence{
		commit:     values["commit"],
		patchID:    values["patch-id"],
		base:       values["base"],
		head:       values["head"],
		integrated: values["integrated"],
		branch:     values["branch"],
		cause:      values["cause"],
	}
	var own []string
	for _, token := range tokens {
		if key, _, _ := strings.Cut(token, "="); key != "cause" && key != "slice" {
			own = append(own, key)
		}
	}
	has := func(keys ...string) bool {
		if len(own) != len(keys) {
			return false
		}
		for _, key := range keys {
			if _, ok := values[key]; !ok {
				return false
			}
		}
		return true
	}
	switch {
	case class == "integrated" && (has("commit") || has("commit", "patch-id")):
		parsed.form = formIntegrated
	case class != "integrated" && (has("commit") || has("commit", "patch-id")):
		parsed.form = formCommit
	case class == "task" && has("base", "head", "integrated", "patch-id"):
		parsed.form = formIsolated
	case (class == "task" || class == "fix") && status == "failed" && len(own) == 0 && parsed.cause != "":
		parsed.form = formFailure
	case class == "fix" && status == "failed" && has("branch", "patch-id"):
		parsed.form = formBranch
	default:
		return fail(class + " " + status + " evidence is one of: " + verifyForms(class, status))
	}
	return parsed, nil
}

// evidenceToken is one whitespace-separated word of an evidence cell, cut at its
// first "=". A word with no "=" is a key alone, with pair false.
type evidenceToken struct {
	key, value string
	pair       bool
}

// evidenceCell is column 8 of a state row, read once. It answers two kinds of
// question, and which kind a reader asks is part of the grammar it keeps.
//
// tokens is strings.Fields(text) once, each word cut at its first "=" and the
// whole word being the key when it holds none (pair false). quoted is event
// verify, a target starting "red-", and text starting "no-commit", whether or
// not the cell is a valid no-commit cell: such a cell quotes a failing run, and
// the words of the quote are not evidence.
//
// The token questions pairs, has, first, values, and list answer nothing for a
// quoted cell, and has, first, values, and list read only the words with pair
// set, by their exact key. asWritten gives the cell with quoted cleared, for the
// one reader that reads the words of a quoted cell.
//
// The text questions read text as written, quoted or not, and find a key
// anywhere in it, even inside a longer key or a value. They end a value at an
// ASCII space, not at any word break. approved reads the whole-cell prefix.
type evidenceCell struct {
	text   string
	quoted bool
	tokens []evidenceToken
}

// evidenceOf reads the evidence cell text of a row with the given event and target.
func evidenceOf(event, target, text string) evidenceCell {
	fields := strings.Fields(text)
	tokens := make([]evidenceToken, len(fields))
	for i, field := range fields {
		key, value, pair := strings.Cut(field, "=")
		tokens[i] = evidenceToken{key: key, value: value, pair: pair}
	}
	return evidenceCell{
		text:   text,
		quoted: event == "verify" && strings.HasPrefix(target, "red-") && strings.HasPrefix(text, "no-commit"),
		tokens: tokens,
	}
}

// cellOf reads the evidence cell of a state row.
func cellOf(row stateRow) evidenceCell { return evidenceOf(row[3], row[4], row[7]) }

// asWritten returns the cell with quoted cleared, so its token questions
// answer over the words of a quoted cell.
func (c evidenceCell) asWritten() evidenceCell {
	c.quoted = false
	return c
}

// pairs returns every token, or nothing for a quoted cell.
func (c evidenceCell) pairs() []evidenceToken {
	if c.quoted {
		return nil
	}
	return c.tokens
}

// has reports whether a token with pair set has the key, even one with no value.
func (c evidenceCell) has(key string) bool {
	_, ok := c.find(key)
	return ok
}

// first returns the value of the first token with pair set and the key, or ""
// when there is none.
func (c evidenceCell) first(key string) string {
	value, _ := c.find(key)
	return value
}

// find returns the value of the first token with pair set and the key.
func (c evidenceCell) find(key string) (string, bool) {
	if c.quoted {
		return "", false
	}
	for _, token := range c.tokens {
		if token.pair && token.key == key {
			return token.value, true
		}
	}
	return "", false
}

// values returns the value of every token with pair set and the key, in order.
func (c evidenceCell) values(key string) []string {
	if c.quoted {
		return nil
	}
	var values []string
	for _, token := range c.tokens {
		if token.pair && token.key == key {
			values = append(values, token.value)
		}
	}
	return values
}

// list returns first(key) split on commas, with the empty parts dropped.
func (c evidenceCell) list(key string) []string {
	return strings.FieldsFunc(c.first(key), func(r rune) bool { return r == ',' })
}

// textRun returns the run of accepted bytes after the first "key=" in the text
// that has one, skipping an occurrence followed by none.
func (c evidenceCell) textRun(key string, accept func(byte) bool) string {
	text, key := c.text, key+"="
	for offset := 0; offset < len(text); {
		at := strings.Index(text[offset:], key)
		if at < 0 {
			break
		}
		start := offset + at + len(key)
		end := start
		for end < len(text) && accept(text[end]) {
			end++
		}
		if end > start {
			return text[start:end]
		}
		offset = start
	}
	return ""
}

// textTimestamp returns the first 20 bytes after a "key=" in the text that read
// as a UTC timestamp, YYYY-MM-DDTHH:MM:SSZ, or "".
func (c evidenceCell) textTimestamp(key string) string {
	text, key := c.text, key+"="
	for offset := 0; offset < len(text); {
		at := strings.Index(text[offset:], key)
		if at < 0 {
			break
		}
		start := offset + at + len(key)
		offset = start
		if len(text)-start < 20 {
			continue
		}
		candidate := text[start : start+20]
		valid := true
		for i := 0; i < 20; i++ {
			switch i {
			case 4, 7:
				valid = valid && candidate[i] == '-'
			case 10:
				valid = valid && candidate[i] == 'T'
			case 13, 16:
				valid = valid && candidate[i] == ':'
			case 19:
				valid = valid && candidate[i] == 'Z'
			default:
				valid = valid && asciiDigit(candidate[i])
			}
		}
		if valid {
			return candidate
		}
	}
	return ""
}

// textHasValue reports whether some "key=" in the text is followed by a
// non-space byte.
func (c evidenceCell) textHasValue(key string) bool {
	text, key := c.text, key+"="
	for offset := 0; offset < len(text); {
		at := strings.Index(text[offset:], key)
		if at < 0 {
			return false
		}
		start := offset + at + len(key)
		if start < len(text) && text[start] != ' ' {
			return true
		}
		offset = start
	}
	return false
}

// textLast returns what follows the last "key=" in the text, up to the next
// ASCII space, or "" when the text has none.
func (c evidenceCell) textLast(key string) string {
	key += "="
	at := strings.LastIndex(c.text, key)
	if at < 0 {
		return ""
	}
	value := c.text[at+len(key):]
	if end := strings.IndexByte(value, ' '); end >= 0 {
		value = value[:end]
	}
	return value
}

// approved returns the hash rule and hash a row's approval was written under,
// read from the start of the whole cell: "text" and the rest of the cell for
// text=<hash> with a non-empty hash, else "blob" and the rest of the cell for
// blob=<hash>. Any other cell, a bare blob among them, is "blob" with no hash.
func (c evidenceCell) approved() (key, hash string) {
	if hash, ok := strings.CutPrefix(c.text, "text="); ok && hash != "" {
		return "text", hash
	}
	if hash, ok := strings.CutPrefix(c.text, "blob="); ok {
		return "blob", hash
	}
	return "blob", ""
}

func validateNoCommit(evidence string) error {
	fail := func(reason string) error { return invalidCell("evidence", evidence, reason) }
	if utf8.RuneCountInString(evidence) > noCommitLimit {
		return fail(fmt.Sprintf("a no-commit cell is at most %d characters", noCommitLimit))
	}
	var exit, command string
	rest, ok := strings.CutPrefix(evidence, "no-commit exit=")
	if ok {
		exit, rest, ok = strings.Cut(rest, " cmd=")
	}
	if ok {
		// The command may hold spaces and pairs of its own, so it ends at the first " out=".
		command, _, ok = strings.Cut(rest, " out=")
	}
	// A command can fail without printing anything, so the excerpt may be empty.
	if !ok || command == "" {
		return fail("no-commit evidence is: " + noCommitShape)
	}
	// A run that exited 0 did not fail, so it is no red evidence. PowerShell
	// reports a crashed process as a negative status.
	if !noCommitExit().MatchString(exit) {
		return fail("exit must be a non-zero integer exit status")
	}
	return nil
}

func validateEvidence(event, target, status, evidence string) error {
	// Status reads an entry= pair as the hash of an entry the helper read from
	// the artifact, so a pair typed by the caller could record an entry nobody
	// read. An order row's evidence is auto, checked below. A no-commit cell
	// quotes a command and its output, so an entry= there is text, not a pair.
	if event != "order" && evidenceOf(event, target, evidence).has("entry") {
		return invalidCell("evidence", evidence, "entry= is written by --entry only; pass --entry <n|last>")
	}
	keys, optional, label := "", "", event
	switch event {
	case "review":
		if !inSet(status, "ok failed") {
			return invalidCell("status", status, "a review row is ok or failed")
		}
		keys, optional = "kind dispatched round blocked", "rev by commit"
	case "escape":
		if status != "failed" {
			return invalidCell("status", status, "an escape row is failed")
		}
		keys, optional = "found-by severity review", "word"
	case "approve":
		// A declined approval answers no and approves no content, so it has nothing to point at.
		if status == "declined" && evidence != "none" {
			return invalidCell("evidence", evidence, "a declined approve row has evidence none")
		}
		return nil
	case "verify":
		// The writer resolves auto and auto:<sha> into the commit form before it
		// stores the row, so they are taken here and refused by the parser on read.
		if evidence == "auto" || strings.HasPrefix(evidence, "auto:") {
			return nil
		}
		_, err := parseVerify(target, status, evidence)
		return err
	case "commit":
		if !inSet(target, commitKinds) {
			return invalidCell("target", target, "a commit row names one of: "+commitKinds)
		}
		if status != "ok" {
			return invalidCell("status", status, "a commit row is ok")
		}
		// The row exists to name a real commit, so neither HEAD by default nor a typed id is taken.
		if sha, named := strings.CutPrefix(evidence, "auto:"); !named || sha == "" {
			return invalidCell("evidence", evidence, "commit evidence is auto:<sha>, naming the commit")
		}
		return nil
	case "order":
		if !evidenceRound().MatchString(target) {
			return invalidCell("target", target, "an order row names the standing order's position, a positive integer")
		}
		if status != "ok" {
			return invalidCell("status", status, "an order row is ok")
		}
		// The row binds the entry's text as it stands, so only the hash the helper computes is taken.
		if evidence != "auto" {
			return invalidCell("evidence", evidence, "order evidence is auto, stored as the entry hash")
		}
		return nil
	// validateEvidence also checks the target of these two, since it names the waived gate or the demoted wave.
	case "waive":
		if err := parseWaive(target); err != nil {
			return invalidCell("target", target, err.Error())
		}
		return nil
	case "demote":
		if err := parseDemote(target); err != nil {
			return invalidCell("target", target, err.Error())
		}
		return nil
	case "drop":
		if status != "cancelled" {
			return invalidCell("status", status, "a drop row is cancelled")
		}
		return nil
	case "note":
		switch {
		case target == "slices":
			keys = "slices final"
		case noteSliceTarget().MatchString(target):
			keys = "issue"
		case noteLaneTarget().MatchString(target):
			keys = "merged"
		case target == "policy":
			keys = "skill version"
		case target != "coverage":
			return invalidCell("target", target, "a note row names one of: "+noteTargets)
		}
		if !inSet(status, "ok failed") {
			return invalidCell("status", status, "a note row is ok or failed")
		}
		if keys == "" {
			return nil
		}
		label = "note " + target
	default:
		return nil
	}
	required := strings.Fields(keys)
	allowed, shape := keys, label+" evidence is exactly: "+keys
	if optional != "" {
		allowed, shape = keys+" "+optional, label+" evidence is: "+keys+", and optionally: "+optional
	}
	most := len(strings.Fields(allowed))
	fail := func(reason string) error { return invalidCell("evidence", evidence, reason) }
	if strings.Contains(evidence, "  ") {
		return fail("an empty pair; split key=value pairs with one space")
	}
	pairs := strings.Split(evidence, " ")
	if len(pairs) < len(required) || len(pairs) > most {
		return fail(shape)
	}
	seen := make(map[string]bool, most)
	for _, pair := range pairs {
		if !evidencePair().MatchString(pair) {
			return fail("'" + pair + "' is not a key=value pair")
		}
		key, value, _ := strings.Cut(pair, "=")
		if !inSet(key, allowed) {
			return fail("unknown key " + key + "; expected: " + allowed)
		}
		if seen[key] {
			return fail("key " + key + " repeats")
		}
		seen[key] = true
		switch key {
		case "kind":
			if !inSet(value, reviewKinds) {
				return fail("kind must be one of: " + reviewKinds)
			}
		case "review":
			if !inSet(value, escapeKinds) {
				return fail("review must be one of: " + escapeKinds)
			}
		case "rev":
			if !evidenceRound().MatchString(value) {
				return fail("rev must be a positive integer")
			}
		case "by":
			if !inSet(value, "independent self") {
				return fail("by must be one of: independent self")
			}
		case "commit":
			if !evidenceCommitID().MatchString(value) {
				return fail("commit must be a commit id")
			}
		case "dispatched":
			if !evidenceTimestamp().MatchString(value) {
				return fail("dispatched must be an ISO 8601 UTC ts like 2026-10-30T10:00:00Z")
			}
		case "round":
			if !evidenceRound().MatchString(value) {
				return fail("round must be a positive integer")
			}
		case "blocked":
			if !evidenceBlocked().MatchString(value) {
				return fail("blocked must be a whole number of seconds")
			}
		case "found-by":
			if !inSet(value, "gate ci copilot person") {
				return fail("found-by must be one of: gate ci copilot person")
			}
		case "severity":
			if !inSet(value, "critical important") {
				return fail("severity must be one of: critical important")
			}
		case "slices":
			if !evidenceSlices().MatchString(value) {
				return fail("slices must be a comma-separated list of S<n> ids")
			}
		case "final":
			if !inSet(value, "yes no") {
				return fail("final must be one of: yes no")
			}
		case "issue":
			if !evidenceIssue().MatchString(value) {
				return fail("issue must be #<n> or <owner>/<repo>#<n>")
			}
		case "merged":
			if !evidenceCommitID().MatchString(value) {
				return fail("merged must be a commit id")
			}
		}
	}
	// An optional pair can fill the count, so each required key is looked for by name.
	for _, key := range required {
		if !seen[key] {
			return fail(shape)
		}
	}
	return nil
}
