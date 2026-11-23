package flow

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

// The parse is by shape, so these ids need not name a commit in any repository.
var (
	sha40 = strings.Repeat("a", 40)
	pid40 = strings.Repeat("b", 40)
	pid64 = strings.Repeat("c", 64)
	sha64 = strings.Repeat("d", 64)
)

func TestParseVerify(t *testing.T) {
	t.Parallel()
	isolated := "base=abc1234 head=" + sha40 + " integrated=" + sha64 + " patch-id=" + pid40
	for _, tc := range []struct {
		target, status, evidence string
		want                     verifyEvidence
	}{
		{"task-1", "ok", "commit=abc1234 patch-id=" + pid40, verifyEvidence{form: formCommit, commit: "abc1234", patchID: pid40}},
		{"task-1", "ok", "commit=" + sha64 + " patch-id=" + pid64, verifyEvidence{form: formCommit, commit: sha64, patchID: pid64}},
		// A bare commit= is a merge commit's form; whether it names a merge is the resolver's question.
		{"task-1", "ok", "commit=" + sha40, verifyEvidence{form: formCommit, commit: sha40}},
		{"red-task-1", "failed", "commit=" + sha40, verifyEvidence{form: formCommit, commit: sha40}},
		{"fix", "ok", "commit=" + sha40, verifyEvidence{form: formCommit, commit: sha40}},
		{"task-1", "ok", isolated, verifyEvidence{form: formIsolated, base: "abc1234", head: sha40, integrated: sha64, patchID: pid40}},
		{"task-1", "failed", "commit=abc1234 patch-id=" + pid40 + " cause=fix-attempt", verifyEvidence{form: formCommit, commit: "abc1234", patchID: pid40, cause: "fix-attempt"}},
		{"task-1", "ok", "commit=abc1234 patch-id=" + pid40 + " slice=S1", verifyEvidence{form: formCommit, commit: "abc1234", patchID: pid40}},
		{"task-1", "ok", "commit=abc1234 patch-id=" + pid40 + " slice=S1,S2", verifyEvidence{form: formCommit, commit: "abc1234", patchID: pid40}},
		{"wave-2", "ok", "commit=" + sha40, verifyEvidence{form: formIntegrated, commit: sha40}},
		{"wave-2", "ok", "commit=" + sha40 + " patch-id=" + pid40, verifyEvidence{form: formIntegrated, commit: sha40, patchID: pid40}},
		{"group-g1", "ok", "commit=" + sha40, verifyEvidence{form: formIntegrated, commit: sha40}},
		{"final", "ok", "commit=" + sha40, verifyEvidence{form: formIntegrated, commit: sha40}},
		{"red-fix", "failed", "commit=abc1234 patch-id=" + pid40, verifyEvidence{form: formCommit, commit: "abc1234", patchID: pid40}},
		{"red-task-1", "failed", "no-commit exit=1 cmd=go test out=FAIL", verifyEvidence{form: formNoCommit}},
		{"task-1", "failed", "cause=repair", verifyEvidence{form: formFailure, cause: "repair"}},
		{"fix", "failed", "cause=context", verifyEvidence{form: formFailure, cause: "context"}},
		{"fix", "failed", "branch=fix/q patch-id=" + pid40, verifyEvidence{form: formBranch, branch: "fix/q", patchID: pid40}},
	} {
		got, err := parseVerify(tc.target, tc.status, tc.evidence)
		if err != nil || got != tc.want {
			t.Errorf("parseVerify(%q, %q, %q) = %+v, %v; want %+v", tc.target, tc.status, tc.evidence, got, err, tc.want)
		}
	}
}

func TestParseVerifyRefuses(t *testing.T) {
	t.Parallel()
	const (
		taskForms  = "commit=<sha> patch-id=<id>; commit=<sha> on a merge commit; base=<sha> head=<sha> integrated=<sha> patch-id=<id>"
		taskOK     = "task ok evidence is one of: " + taskForms
		taskFailed = "task failed evidence is one of: " + taskForms + "; cause=<word>"
		fixOK      = "fix ok evidence is one of: commit=<sha> patch-id=<id>; commit=<sha> on a merge commit"
		waveFailed = "integrated failed evidence is one of: commit=<sha>; commit=<sha> patch-id=<id>"
	)
	isolated := "base=abc1234 head=" + sha40 + " integrated=" + sha64 + " patch-id=" + pid40
	for _, tc := range []struct{ target, status, evidence, reason string }{
		// The writer takes auto and auto:<sha> before the parser; a stored one is no form.
		{"task-1", "ok", "auto", "auto is not a key=value pair"},
		{"task-1", "ok", "auto:abc1234", "auto:abc1234 is not a key=value pair"},
		{"task-1", "ok", "", "evidence is empty"},
		{"task-1", "ok", "patch-id=abc", "patch-id=abc is not a patch-id of 40 or 64 lowercase hex"},
		{"task-1", "ok", "commit=xyz", "commit=xyz is not a commit id of 7 to 64 lowercase hex"},
		{"task-1", "ok", "commit=abc1234 patch-id=" + strings.ToUpper(pid40), "patch-id=" + strings.ToUpper(pid40) + " is not a patch-id of 40 or 64 lowercase hex"},
		{"task-1", "ok", "patch-id=" + pid40 + "0", "patch-id=" + pid40 + "0 is not a patch-id of 40 or 64 lowercase hex"},
		{"task-1", "ok", "base=xyz head=" + sha40 + " integrated=" + sha40 + " patch-id=" + pid40, "base=xyz is not a commit id of 7 to 64 lowercase hex"},
		{"task-1", "failed", "cause=Repair", "cause=Repair is not a word of a-z 0-9 -"},
		{"task-1", "ok", "commit=abc1234 patch-id=" + pid40 + " slice=S1,not-a-slice", "slice=S1,not-a-slice is not a comma-separated list of S<n> ids"},
		{"task-1", "failed", "cause=repair slice=S0", "slice=S0 is not a comma-separated list of S<n> ids"},
		{"task-1", "failed", "cause=repair slice=S1,", "slice=S1, is not a comma-separated list of S<n> ids"},
		{"task-1", "ok", "patch-id=" + pid40, taskOK},
		{"task-1", "ok", "cause=repair", taskOK},
		{"task-1", "failed", "slice=S1", taskFailed},
		{"task-1", "ok", "commit=abc1234 head=" + sha40, taskOK},
		{"fix", "ok", "branch=b patch-id=" + pid40, fixOK},
		{"task-1", "failed", "branch=b patch-id=" + pid40, "key branch belongs to no task form"},
		{"wave-1", "ok", isolated, "key base belongs to no integrated form"},
		{"red-task-1", "failed", isolated, "key base belongs to no red form"},
		{"wave-1", "failed", "cause=repair", waveFailed},
		{"red-fix", "failed", "commit=0123abc patch-id=0123abc", "patch-id=0123abc is not a patch-id of 40 or 64 lowercase hex"},
		{"red-fix", "failed", "none", "none is not a key=value pair"},
		// The no-commit form belongs to a red target; elsewhere it is no pair.
		{"task-1", "failed", "no-commit", "no-commit is not a key=value pair"},
		{"redo", "failed", "no-commit exit=0", "no-commit is not a key=value pair"},
		// A cell that breaks several rules reports the first in a fixed order.
		{"task-1", "ok", "note=x commit=a commit=b none", "none is not a key=value pair"},
		{"task-1", "ok", "note=x commit=xyz commit=b", "key commit repeats"},
		{"task-1", "ok", "commit=xyz note=x", "key note belongs to no task form"},
		{"task-1", "ok", "patch-id=abc cause=repair", "patch-id=abc is not a patch-id of 40 or 64 lowercase hex"},
	} {
		_, err := parseVerify(tc.target, tc.status, tc.evidence)
		assertCellError(t, err, "flow: invalid evidence: '"+tc.evidence+"' -- "+tc.reason)
	}
}

// cellProbe is the answer of every key question for one probe key.
type cellProbe struct {
	key      string
	has      bool
	first    string
	values   []string
	list     []string
	lower    string // textRun(key, asciiLower)
	digit    string // textRun(key, asciiDigit)
	hasValue bool
	last     string
}

func words(w ...string) []string { return w }

func pair(key, value string) evidenceToken { return evidenceToken{key: key, value: value, pair: true} }
func bare(word string) evidenceToken       { return evidenceToken{key: word} }

// TestEvidenceCellAnswers pins every question of the cell on rows that fit no
// documented form, since the readers it replaces must agree on those too.
// It is not parallel: AllocsPerRun cannot run in a parallel test.
func TestEvidenceCellAnswers(t *testing.T) {
	absent := cellProbe{key: "missing"}
	big := strings.TrimSuffix(strings.Repeat("k=vvvvvvv ", 10000), " ")
	for _, tc := range []struct {
		name, event, target, text string
		quoted                    bool
		tokens                    []evidenceToken
		probes                    []cellProbe
	}{
		{"plain", "verify", "task-1", "commit=abc1234 patch-id=bb", false,
			[]evidenceToken{pair("commit", "abc1234"), pair("patch-id", "bb")},
			[]cellProbe{
				{"commit", true, "abc1234", words("abc1234"), words("abc1234"), "abc", "", true, "abc1234"},
				{"patch-id", true, "bb", words("bb"), words("bb"), "bb", "", true, "bb"},
				absent,
			}},
		{"repeated keys", "review", "task-1", "kind=ab kind=cd kind= slice=1,,2 slice=9", false,
			[]evidenceToken{pair("kind", "ab"), pair("kind", "cd"), pair("kind", ""), pair("slice", "1,,2"), pair("slice", "9")},
			[]cellProbe{
				{"kind", true, "ab", words("ab", "cd", ""), words("ab"), "ab", "", true, ""},
				{"slice", true, "1,,2", words("1,,2", "9"), words("1", "2"), "", "1", true, "9"},
			}},
		{"empty values", "review", "task-1", "a= b=1 c=", false,
			[]evidenceToken{pair("a", ""), pair("b", "1"), pair("c", "")},
			[]cellProbe{
				{"a", true, "", words(""), nil, "", "", false, ""},
				{"b", true, "1", words("1"), words("1"), "", "1", true, "1"},
				{"c", true, "", words(""), nil, "", "", false, ""},
			}},
		{"bare words", "review", "task-1", "bare k=v =x", false,
			[]evidenceToken{bare("bare"), pair("k", "v"), pair("", "x")},
			[]cellProbe{
				{"bare", false, "", nil, nil, "", "", false, ""},
				{"k", true, "v", words("v"), words("v"), "v", "", true, "v"},
			}},
		{"longer keys", "review", "task-1", "xkind=ab prefixcommit=cd", false,
			[]evidenceToken{pair("xkind", "ab"), pair("prefixcommit", "cd")},
			[]cellProbe{
				{"kind", false, "", nil, nil, "ab", "", true, "ab"},
				{"commit", false, "", nil, nil, "cd", "", true, "cd"},
			}},
		{"longer key before the key", "review", "task-1", "xkind=ab kind=ef", false,
			[]evidenceToken{pair("xkind", "ab"), pair("kind", "ef")},
			[]cellProbe{{"kind", true, "ef", words("ef"), words("ef"), "ab", "", true, "ef"}}},
		{"key inside a value", "review", "task-1", "note=a=b k=v", false,
			[]evidenceToken{pair("note", "a=b"), pair("k", "v")},
			[]cellProbe{
				{"note", true, "a=b", words("a=b"), words("a=b"), "a", "", true, "a=b"},
				{"a", false, "", nil, nil, "b", "", true, "b"},
			}},
		{"quoted cell holding every key", "verify", "red-task-1",
			"no-commit exit=1 cmd=go test commit=abc entry=e slice=S1,S2 out=x dispatched=2026-11-22T10:20:30Z", true,
			[]evidenceToken{bare("no-commit"), pair("exit", "1"), pair("cmd", "go"), bare("test"), pair("commit", "abc"), pair("entry", "e"),
				pair("slice", "S1,S2"), pair("out", "x"), pair("dispatched", "2026-11-22T10:20:30Z")},
			[]cellProbe{
				{"commit", false, "", nil, nil, "abc", "", true, "abc"},
				{"entry", false, "", nil, nil, "e", "", true, "e"},
				{"slice", false, "", nil, nil, "", "", true, "S1,S2"},
				{"exit", false, "", nil, nil, "", "1", true, "1"},
				{"cmd", false, "", nil, nil, "go", "", true, "go"},
				{"test", false, "", nil, nil, "", "", false, ""},
			}},
		{"quoted cell, no other words", "verify", "red-fix", "no-commit", true,
			[]evidenceToken{bare("no-commit")}, []cellProbe{absent}},
		{"quoted cell glued to a pair", "verify", "red-fix", "no-commitk=v", true,
			[]evidenceToken{pair("no-commitk", "v")}, []cellProbe{{"k", false, "", nil, nil, "v", "", true, "v"}}},
		{"words split by unicode space", "review", "task-1", "a=1 b=2\u0085c=3\vd=4  e=5", false,
			[]evidenceToken{pair("a", "1"), pair("b", "2"), pair("c", "3"), pair("d", "4"), pair("e", "5")},
			[]cellProbe{
				{"a", true, "1", words("1"), words("1"), "", "1", true, "1"},
				{"b", true, "2", words("2"), words("2"), "", "2", true, "2\u0085c=3\vd=4"},
				{"c", true, "3", words("3"), words("3"), "", "3", true, "3\vd=4"},
				{"d", true, "4", words("4"), words("4"), "", "4", true, "4"},
				{"e", true, "5", words("5"), words("5"), "", "5", true, "5"},
			}},
		{"empty", "review", "task-1", "", false, []evidenceToken{}, []cellProbe{absent}},
		{"only space", "review", "task-1", "  \u0085 \v", false, []evidenceToken{}, []cellProbe{absent}},
		{"about 100 KB", "review", "task-1", big, false, slices.Repeat([]evidenceToken{pair("k", "vvvvvvv")}, 10000),
			[]cellProbe{{"k", true, "vvvvvvv", slices.Repeat([]string{"vvvvvvv"}, 10000), words("vvvvvvv"), "vvvvvvv", "", true, "vvvvvvv"}}},
	} {
		cell := evidenceOf(tc.event, tc.target, tc.text)
		if cell.text != tc.text || cell.quoted != tc.quoted || !slices.Equal(cell.tokens, tc.tokens) {
			t.Errorf("%s: evidenceOf = {text %q, quoted %v, tokens %v}; want quoted %v, tokens %v", tc.name, cell.text, cell.quoted, cell.tokens, tc.quoted, tc.tokens)
		}
		wantPairs := tc.tokens
		if tc.quoted {
			wantPairs = nil
		}
		if got := cell.pairs(); !slices.Equal(got, wantPairs) {
			t.Errorf("%s: pairs() = %v; want %v", tc.name, got, wantPairs)
		}
		// The one reader that reads the tokens of a quoted cell asks the cell as written.
		written, unquoted := cell.asWritten(), evidenceOf("review", "task-1", tc.text)
		if !reflect.DeepEqual(written, unquoted) {
			t.Errorf("%s: asWritten() = %+v; want the unquoted reading %+v", tc.name, written, unquoted)
		}
		for _, probe := range tc.probes {
			got := cellProbe{probe.key, cell.has(probe.key), cell.first(probe.key), cell.values(probe.key), cell.list(probe.key),
				cell.textRun(probe.key, asciiLower), cell.textRun(probe.key, asciiDigit), cell.textHasValue(probe.key), cell.textLast(probe.key)}
			if got.has != probe.has || got.first != probe.first || !slices.Equal(got.values, probe.values) || !slices.Equal(got.list, probe.list) ||
				got.lower != probe.lower || got.digit != probe.digit || got.hasValue != probe.hasValue || got.last != probe.last {
				t.Errorf("%s: key %q answers %+v; want %+v", tc.name, probe.key, got, probe)
			}
		}
	}

	if cell := evidenceOf("verify", "red-task-1", "no-commit exit=1 commit=abc"); !cell.asWritten().has("commit") || cell.has("commit") || cell.asWritten().quoted {
		t.Errorf("asWritten() of a quoted cell must answer its tokens with quoted cleared, and the cell itself must answer none: %+v", cell.asWritten())
	}

	// A cell is split by strings.Fields once, and a question over it allocates nothing.
	cell := evidenceOf("review", "task-1", big)
	if got := testing.AllocsPerRun(5, func() { cell = evidenceOf("review", "task-1", big) }); got > 2 {
		t.Errorf("evidenceOf on a 100 KB cell made %v allocations; want at most 2 (one field slice, one token slice)", got)
	}
	if got := testing.AllocsPerRun(5, func() { cell.has("missing"); cell.first("missing") }); got != 0 {
		t.Errorf("has and first on a 100 KB cell made %v allocations; want 0", got)
	}

	row := stateRow{"t", "b", "p", "verify", "red-task-1", "failed", "w", "no-commit exit=1 commit=abc", "n"}
	if got, want := cellOf(row), evidenceOf("verify", "red-task-1", "no-commit exit=1 commit=abc"); !reflect.DeepEqual(got, want) {
		t.Errorf("cellOf(row) = %+v; want %+v", got, want)
	}
}

func TestEvidenceCellQuoted(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		event, target, text string
		want                bool
	}{
		{"verify", "red-task-1", "no-commit", true},
		{"verify", "red-fix", "no-commitk=v", true},
		{"verify", "red-", "no-commit exit=1", true},
		{"verify", "red-task-1", "no-commit garbage", true},
		{"verify", "red-task-1", "", false},
		{"verify", "task-1", "no-commit exit=1", false},
		{"verify", "redo", "no-commit", false},
		{"review", "red-task-1", "no-commit exit=1", false},
		{"verify", "red-task-1", " no-commit", false},
		{"verify", "red-task-1", "No-commit", false},
		{"verify", "red-task-1", "commit=a no-commit", false},
	} {
		if got := evidenceOf(tc.event, tc.target, tc.text).quoted; got != tc.want {
			t.Errorf("evidenceOf(%q, %q, %q).quoted = %v; want %v", tc.event, tc.target, tc.text, got, tc.want)
		}
	}
}

func TestEvidenceCellApproved(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ text, key, hash string }{
		{"text=abc", "text", "abc"},
		// The hash is the rest of the cell, as the status line reads it.
		{"text=abc blob=def", "text", "abc blob=def"},
		{"text=", "blob", ""},
		{"text=x", "text", "x"},
		{"blob=abc", "blob", "abc"},
		{"blob=", "blob", ""},
		{"blob", "blob", ""},
		{"blob=abc text=def", "blob", "abc text=def"},
		{"", "blob", ""},
		{"unrecorded", "blob", ""},
		{" text=abc", "blob", ""},
		{"x text=abc", "blob", ""},
		{"xtext=abc", "blob", ""},
		{"no-commit text=abc", "blob", ""},
	} {
		key, hash := evidenceOf("approve", "plan.md", tc.text).approved()
		if key != tc.key || hash != tc.hash {
			t.Errorf("approved() of %q = %q, %q; want %q, %q", tc.text, key, hash, tc.key, tc.hash)
		}
	}
	// The approval is the whole-cell prefix, so a quoted cell is read as written.
	if key, hash := evidenceOf("verify", "red-task-1", "no-commit").approved(); key != "blob" || hash != "" {
		t.Errorf("approved() of a quoted cell = %q, %q; want blob with no hash", key, hash)
	}
}

func TestEvidenceCellTimestamp(t *testing.T) {
	t.Parallel()
	const stamp = "2026-11-22T10:20:30Z"
	for _, tc := range []struct{ name, text, key, want string }{
		{"valid", "dispatched=" + stamp, "dispatched", stamp},
		{"after other words", "a=1 dispatched=" + stamp + " b=2", "dispatched", stamp},
		{"second occurrence", "dispatched=bad dispatched=" + stamp, "dispatched", stamp},
		{"short", "dispatched=2026-11-22T10:20:3", "dispatched", ""},
		{"short then valid", "dispatched=2026-11-22T10:20:3 dispatched=" + stamp, "dispatched", stamp},
		{"wrong separator", "dispatched=2026-11-22 10:20:30Z", "dispatched", ""},
		{"lower-case zone", "dispatched=2026-11-22T10:20:30z", "dispatched", ""},
		{"letter for a digit", "dispatched=2026-1a-22T10:20:30Z", "dispatched", ""},
		{"key inside a longer key", "xdispatched=" + stamp, "dispatched", stamp},
		{"characters after", "dispatched=" + stamp + "junk", "dispatched", stamp},
		{"no key", stamp, "dispatched", ""},
		{"another key", "started=" + stamp + " dispatched=bad", "started", stamp},
		{"empty", "", "dispatched", ""},
	} {
		if got := evidenceOf("review", "task-1", tc.text).textTimestamp(tc.key); got != tc.want {
			t.Errorf("%s: textTimestamp(%q) of %q = %q; want %q", tc.name, tc.key, tc.text, got, tc.want)
		}
	}
	// A quoted cell is read as written.
	if got := evidenceOf("verify", "red-task-1", "no-commit cmd=x dispatched="+stamp).textTimestamp("dispatched"); got != stamp {
		t.Errorf("textTimestamp of a quoted cell = %q; want %q", got, stamp)
	}
}

func TestValidateEvidence(t *testing.T) {
	t.Parallel()
	const review = "kind=quality dispatched=2026-11-10T03:59:13Z round=1 blocked=0"
	const escape = "found-by=gate severity=important review=quality"
	const reviewShape = "review evidence is: kind dispatched round blocked, and optionally: rev by commit"
	const escapeShape = "escape evidence is: found-by severity review, and optionally: word"
	for _, tc := range []struct{ event, status, evidence string }{
		{"review", "ok", review}, {"review", "failed", review}, {"escape", "failed", escape},
		{"review", "ok", "blocked=999999999999999999999999999 round=999999999999999999999999999 dispatched=2099-99-99T99:99:99Z kind=plan"},
		{"create", "pending", "not structured"},
		{"review", "ok", review + " rev=3 by=independent commit=0123abc"},
		{"review", "ok", "by=self " + review + " commit=" + strings.Repeat("a", 64)},
		{"escape", "failed", escape + " word=blocker"},
		{"approve", "declined", "none"}, {"approve", "ok", "blob=0123abc"}, {"approve", "failed", "see the thread"},
	} {
		if err := validateEvidence(tc.event, "task-1", tc.status, tc.evidence); err != nil {
			t.Errorf("valid %+v: %v", tc, err)
		}
	}
	for _, kind := range strings.Fields(reviewKinds) {
		if err := validateEvidence("review", "task-1", "ok", strings.Replace(review, "quality", kind, 1)); err != nil {
			t.Error(err)
		}
	}
	for _, kind := range strings.Fields(escapeKinds) {
		if err := validateEvidence("escape", "task-1", "failed", strings.Replace(escape, "quality", kind, 1)); err != nil {
			t.Error(err)
		}
	}
	for _, by := range []string{"gate", "ci", "copilot", "person"} {
		if err := validateEvidence("escape", "task-1", "failed", strings.Replace(escape, "gate", by, 1)); err != nil {
			t.Error(err)
		}
	}
	if err := validateEvidence("escape", "task-1", "failed", strings.Replace(escape, "important", "critical", 1)); err != nil {
		t.Error(err)
	}
	for _, tc := range []struct{ event, status, evidence, field, reason string }{
		{"review", "pending", review, "status", "a review row is ok or failed"},
		{"escape", "ok", escape, "status", "an escape row is failed"},
		{"review", "ok", strings.Replace(review, " ", "  ", 1), "evidence", "an empty pair; split key=value pairs with one space"},
		{"review", "ok", "kind=quality", "evidence", reviewShape},
		{"review", "ok", review + " rev=1 by=self commit=0123abc rev=2", "evidence", reviewShape},
		{"review", "ok", strings.Replace(review, "blocked=0", "rev=1", 1), "evidence", reviewShape},
		{"escape", "failed", "found-by=gate severity=important", "evidence", escapeShape},
		{"escape", "failed", escape + " word=a word=b", "evidence", escapeShape},
		{"escape", "failed", strings.Replace(escape, "review=quality", "word=a", 1), "evidence", escapeShape},
		{"escape", "failed", escape + " extra=x", "evidence", "unknown key extra; expected: found-by severity review word"},
		{"review", "ok", strings.Replace(review, "kind=quality", "kind:quality", 1), "evidence", "'kind:quality' is not a key=value pair"},
		{"review", "ok", strings.Replace(review, "kind=quality", "kind=", 1), "evidence", "'kind=' is not a key=value pair"},
		{"review", "ok", strings.Replace(review, "kind=quality", "Kind=quality", 1), "evidence", "'Kind=quality' is not a key=value pair"},
		{"review", "ok", strings.Replace(review, "kind=quality", "other=quality", 1), "evidence", "unknown key other; expected: kind dispatched round blocked rev by commit"},
		{"review", "ok", strings.Replace(review, "blocked=0", "kind=plan", 1), "evidence", "key kind repeats"},
		{"review", "ok", strings.Replace(review, "quality", "bogus", 1), "evidence", "kind must be one of: plan packet quality integration spec"},
		{"escape", "failed", strings.Replace(escape, "quality", "bogus", 1), "evidence", "review must be one of: plan packet quality integration"},
		{"review", "ok", strings.Replace(review, "2026-11-10T03:59:13Z", "1999-11-10T03:59:13Z", 1), "evidence", "dispatched must be an ISO 8601 UTC ts like 2026-10-30T10:00:00Z"},
		{"review", "ok", strings.Replace(review, "round=1", "round=0", 1), "evidence", "round must be a positive integer"},
		{"review", "ok", strings.Replace(review, "blocked=0", "blocked=01", 1), "evidence", "blocked must be a whole number of seconds"},
		{"escape", "failed", strings.Replace(escape, "found-by=gate", "found-by=agent", 1), "evidence", "found-by must be one of: gate ci copilot person"},
		{"escape", "failed", strings.Replace(escape, "important", "minor", 1), "evidence", "severity must be one of: critical important"},
		{"escape", "failed", strings.Replace(escape, "quality", "spec", 1), "evidence", "review must be one of: plan packet quality integration"},
		{"review", "ok", review + " rev=0", "evidence", "rev must be a positive integer"},
		{"review", "ok", review + " by=other", "evidence", "by must be one of: independent self"},
		{"review", "ok", review + " commit=0123ab", "evidence", "commit must be a commit id"},
		{"review", "ok", review + " commit=0123ABC", "evidence", "commit must be a commit id"},
		{"review", "ok", review + " commit=" + strings.Repeat("a", 65), "evidence", "commit must be a commit id"},
		{"approve", "declined", "later", "evidence", "a declined approve row has evidence none"},
	} {
		value := tc.evidence
		if tc.field == "status" {
			value = tc.status
		}
		assertCellError(t, validateEvidence(tc.event, "task-1", tc.status, tc.evidence), "flow: invalid "+tc.field+": '"+value+"' -- "+tc.reason)
	}
	// Exercise the caller so validation cannot quietly stop invoking the evidence rules.
	v := validCellValues()
	v[1] = "review"
	v[5] = "none"
	_, err := validateCells(v)
	assertCellError(t, err, "flow: invalid evidence: 'none' -- "+reviewShape)
}

func TestValidateNoCommitEvidence(t *testing.T) {
	t.Parallel()
	const cell = "no-commit exit=2 cmd=go test ./... out=FAIL"
	// sized is a valid cell of n characters; the filler is counted in characters, not bytes.
	sized := func(n int, filler string) string {
		return cell + strings.Repeat(filler, n-len(cell))
	}
	for _, tc := range []struct{ event, target, evidence string }{
		{"verify", "red-fix", cell}, {"verify", "red-task-1", cell},
		{"verify", "red-fix", "no-commit exit=130 cmd=a out=b out=c"},
		// PowerShell reports a crashed process as a negative exit status.
		{"verify", "red-task-1", "no-commit exit=-1 cmd=go test out=FAIL"},
		{"verify", "red-fix", "no-commit exit=-1073741819 cmd=go test out=FAIL"},
		// A command can fail without printing anything.
		{"verify", "red-fix", "no-commit exit=1 cmd=go test out="},
		{"verify", "red-fix", sized(500, "x")},
		{"verify", "red-fix", sized(500, "é")},
		// The form belongs to a verify row for a red target; a revert row's evidence is free.
		{"revert", "red-fix", "no-commit exit=0"},
	} {
		if err := validateEvidence(tc.event, tc.target, "failed", tc.evidence); err != nil {
			t.Errorf("valid %+v: %v", tc, err)
		}
	}
	const (
		shape   = "no-commit evidence is: no-commit exit=<n> cmd=<command> out=<excerpt>"
		badExit = "exit must be a non-zero integer exit status"
	)
	for _, tc := range []struct{ target, evidence, reason string }{
		{"red-fix", "no-commit", shape},
		{"red-fix", "no-commitment", shape},
		{"red-task-1", "no-commit exit=2 cmd=go test ./...", shape},
		{"red-fix", "no-commit exit=2 out=FAIL cmd=go test", shape},
		{"red-fix", "no-commit exit=2 cmd= out=FAIL", shape},
		{"red-fix", "no-commit exit=2 cmd= out=", shape},
		{"red-fix", "no-commit cmd=go test out=FAIL exit=2", shape},
		{"red-fix", "no-commit exit=0 cmd=go test out=FAIL", badExit},
		{"red-task-1", "no-commit exit=00 cmd=go test out=FAIL", badExit},
		{"red-fix", "no-commit exit=-0 cmd=go test out=FAIL", badExit},
		{"red-fix", "no-commit exit=+1 cmd=go test out=FAIL", badExit},
		{"red-fix", "no-commit exit=1x cmd=go test out=FAIL", badExit},
		{"red-fix", "no-commit exit=01 cmd=go test out=FAIL", badExit},
		{"red-fix", "no-commit exit=- cmd=go test out=FAIL", badExit},
		{"red-fix", "no-commit exit= cmd=go test out=FAIL", badExit},
		{"red-fix", sized(501, "x"), "a no-commit cell is at most 500 characters"},
		{"red-task-1", sized(501, "é"), "a no-commit cell is at most 500 characters"},
		// A red target takes the commit form besides the no-commit form, and nothing else.
		{"red-fix", "commit=0123abc patch-id=0123abc", "patch-id=0123abc is not a patch-id of 40 or 64 lowercase hex"},
		{"red-fix", "none", "none is not a key=value pair"},
		// Elsewhere the no-commit form is no form at all, so its first word is no pair.
		{"task-1", "no-commit", "no-commit is not a key=value pair"},
		{"redo", "no-commit exit=0", "no-commit is not a key=value pair"},
	} {
		assertCellError(t, validateEvidence("verify", tc.target, "failed", tc.evidence), "flow: invalid evidence: '"+tc.evidence+"' -- "+tc.reason)
	}
}
