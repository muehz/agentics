package shellwords

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func hasExecutable(res Result, name string) bool {
	for _, inv := range res.Invocations {
		if len(inv.Words) > 0 && ExecutableName(inv.Words[0].Text) == name {
			return true
		}
	}
	return false
}

func hasNonLiteralCommand(res Result) bool {
	for _, inv := range res.Invocations {
		if len(inv.Words) > 0 && !inv.Words[0].Literal {
			return true
		}
	}
	return false
}

func firstRefused(res Result) *Invocation {
	for i := range res.Invocations {
		if !res.Invocations[i].Inspectable {
			return &res.Invocations[i]
		}
	}
	return nil
}

func findInvocation(res Result, words ...string) *Invocation {
	for i := range res.Invocations {
		inv := &res.Invocations[i]
		if len(inv.Words) != len(words) {
			continue
		}
		match := true
		for j, w := range words {
			if inv.Words[j].Text != w {
				match = false
				break
			}
		}
		if match {
			return inv
		}
	}
	return nil
}

func assertValidInvariants(t *testing.T, cmd string, res Result) {
	t.Helper()
	if err := validateFuzzParseResult(res); err != nil {
		t.Fatalf("%q: invariant failure: %v\nresult: %#v", cmd, err, res)
	}
}

func TestCasePatternSuppressesFalsePositives(t *testing.T) {
	cases := []struct {
		name      string
		cmd       string
		wantExecs []string
		forbid    []string
		check     func(t *testing.T, res Result)
	}{
		{
			name:      "quoted variable pattern",
			cmd:       `case "$x" in "$pat") echo ok ;; esac`,
			wantExecs: []string{"case", "echo", "esac"},
		},
		{
			name:      "glob with variable pattern",
			cmd:       `case "$x" in *"$nl"*) echo ok ;; esac`,
			wantExecs: []string{"case", "echo", "esac"},
		},
		{
			name:      "bare dot alternation pattern",
			cmd:       `case "$x" in .|..) echo ok ;; esac`,
			wantExecs: []string{"case", "echo", "esac"},
			forbid:    []string{".", ".."},
		},
		{
			name:      "leading paren bare dot alternation",
			cmd:       `case "$x" in (.|..) echo ok ;; esac`,
			wantExecs: []string{"case", "echo", "esac"},
			forbid:    []string{".", ".."},
		},
		{
			name:      "extglob pattern",
			cmd:       `case "$x" in @(a|b)) echo ok ;; esac`,
			wantExecs: []string{"case", "echo", "esac"},
			forbid:    []string{"a", "b"},
		},
		{
			name:      "brace pattern",
			cmd:       `case "$x" in {a,b}) echo ok ;; esac`,
			wantExecs: []string{"case", "echo", "esac"},
			forbid:    []string{"a,b", "{a,b}"},
		},
		{
			name:      "double-quoted parameter expansion in subject and pattern",
			cmd:       `case "${var}" in "${pat}") rm -rf / ;; esac`,
			wantExecs: []string{"case", "rm", "esac"},
		},
		{
			name:      "case inside parameter expansion inside double quotes",
			cmd:       `echo "${X:-$(case "$a" in b) rm -rf / ;; esac)}"`,
			wantExecs: []string{"echo", "case", "rm", "esac"},
		},
		{
			name:      "param expansion with closing paren inside subshell case pattern",
			cmd:       `(case "$x" in ${var%)} ) rm -rf / ;; esac)`,
			wantExecs: []string{"case", "rm", "esac"},
		},
		{
			name:      "double semicolon inside parameter expansion in arm body",
			cmd:       `case "$x" in a) echo ${y:-foo;;bar} ;; "$var") echo ok ;; esac`,
			wantExecs: []string{"case", "echo", "esac"},
		},
		{
			name:      "esac inside parameter expansion inside brace group",
			cmd:       `{ case "$x" in a) ${y:-; esac} ;; "$var") echo ok ;; esac; }`,
			wantExecs: []string{"case", "echo", "esac"},
		},
		{
			name:      "esac attached after parameter expansion close inside brace group",
			cmd:       `{ case "$x" in a) ${y:-; }esac ;; "$var") echo ok ;; esac; }`,
			wantExecs: []string{"case", "echo", "esac"},
		},
		{
			name:      "function keyword block inside case arm inside function",
			cmd:       `f() { case "$x" in a) function inner { echo ok; } ;; "$var") echo ok ;; esac; }`,
			wantExecs: []string{"case", "echo", "esac"},
		},
		{
			name:      "nested case with brace subject inside function keyword block",
			cmd:       `f() { case "$x" in a) function inner { case {a,b} in "$var") echo ok ;; esac; } ;; "$var") echo ok ;; esac; }`,
			wantExecs: []string{"case", "echo", "esac"},
		},
		{
			name:      "nested case with literal closing brace pattern inside function keyword block",
			cmd:       `f() { case "$x" in a) function inner { case "$y" in }) echo ok ;; esac; } ;; "$var") echo ok ;; esac; }`,
			wantExecs: []string{"case", "echo", "esac"},
		},
		{
			name:      "brace subject inside brace group",
			cmd:       `{ case {a,b} in "$var") echo ok ;; esac; }`,
			wantExecs: []string{"case", "echo", "esac"},
		},
		{
			name:      "pattern named case after newline",
			cmd:       "case \"$x\" in\ncase ) echo ok ;; esac",
			wantExecs: []string{"case", "echo", "esac"},
		},
		{
			name:      "pattern alternation with process substitution",
			cmd:       `case "$x" in "$var" | <(echo hi)) echo ok ;; esac`,
			wantExecs: []string{"case", "echo", "esac"},
		},
		{
			name:      "if then function keyword wrapping case",
			cmd:       `if true; then function foo { case "$x" in "$var") echo ok ;; esac; }; fi`,
			wantExecs: []string{"true", "echo", "esac", "fi"},
		},
		{
			name:      "nested case closed with esac;;",
			cmd:       `case "$a" in x) case "$b" in y) echo 1 ;; esac;; "$var") echo 2 ;; esac`,
			wantExecs: []string{"case", "echo", "esac"},
		},
		{
			name:      "nested case closed with esac;&",
			cmd:       `case "$a" in x) case "$b" in y) echo 1 ;; esac;& "$var") echo 2 ;; esac`,
			wantExecs: []string{"case", "echo", "esac"},
		},
		{
			name:      "nested case closed with esac;;&",
			cmd:       `case "$a" in x) case "$b" in y) echo 1 ;; esac;;& "$var") echo 2 ;; esac`,
			wantExecs: []string{"case", "echo", "esac"},
		},
		{
			name:      "in followed directly by opening paren",
			cmd:       `case "$x" in("$pat") echo ok ;; esac`,
			wantExecs: []string{"case", "echo", "esac"},
		},
		{
			name:      "pipe-and before double semicolon resets boundary",
			cmd:       `case "$x" in a) echo ok |& ;; b) echo 2 ;; esac`,
			wantExecs: []string{"case", "echo", "esac"},
			check: func(t *testing.T, res Result) {
				inv := findInvocation(res, "echo", "2")
				if inv == nil {
					t.Fatalf("missing echo 2 invocation in %#v", res.Invocations)
				}
				if inv.Before != Sequence || inv.PipelineInputEdge != 0 {
					t.Fatalf("echo 2 got Before=%v PipelineInputEdge=%d, want Sequence and 0", inv.Before, inv.PipelineInputEdge)
				}
			},
		},
		{
			name:      "background before double semicolon resets boundary",
			cmd:       `case "$x" in a) echo ok & ;; esac`,
			wantExecs: []string{"case", "echo", "esac"},
			check: func(t *testing.T, res Result) {
				inv := findInvocation(res, "esac")
				if inv == nil {
					t.Fatalf("missing esac invocation in %#v", res.Invocations)
				}
				if inv.Before != Sequence || inv.PipelineInputEdge != 0 {
					t.Fatalf("esac got Before=%v PipelineInputEdge=%d, want Sequence and 0", inv.Before, inv.PipelineInputEdge)
				}
			},
		},
		{
			name:      "stray closing paren breaks esa and c tokens",
			cmd:       `case "$x" in a) esa)c; echo ok ;; esac`,
			wantExecs: []string{"case", "esa", "c", "echo", "esac"},
		},
		{
			name:      "empty braces before double semicolon do not leak bracedWord to next arm",
			cmd:       `case "$x" in a) {} ;; b) echo ok ;; esac`,
			wantExecs: []string{"case", "echo", "esac"},
			check: func(t *testing.T, res Result) {
				if findInvocation(res, "echo", "ok") == nil {
					t.Fatalf("missing echo ok invocation in %#v", res.Invocations)
				}
				edge := 0
				for _, seg := range splitSegments(`case "$x" in a) {} ;; b) echo ok ;; esac`, POSIX, &edge) {
					if seg.bracedWord {
						t.Fatalf("segment %q leaked bracedWord=true across case arm boundary", seg.raw)
					}
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := Parse(tc.cmd, POSIX)
			assertValidInvariants(t, tc.cmd, res)
			for _, inv := range res.Invocations {
				if !inv.Inspectable {
					t.Errorf("%q: unexpected refusal %q on %#v", tc.cmd, inv.Reason.String(), inv)
				}
			}
			for _, exe := range tc.wantExecs {
				if !hasExecutable(res, exe) {
					t.Errorf("%q: missing expected executable %q in %#v", tc.cmd, exe, res.Invocations)
				}
			}
			for _, bad := range tc.forbid {
				if hasExecutable(res, bad) {
					t.Errorf("%q: forbidden pattern executable %q leaked in %#v", tc.cmd, bad, res.Invocations)
				}
			}
			// Verify no pattern variable leaked as a non-literal command word
			// (except the explicit `${y:-...}` arm body tests where `${y:-...}` is the command itself).
			if !strings.Contains(tc.cmd, "${y:-;") && hasNonLiteralCommand(res) {
				t.Errorf("%q: unexpected non-literal command word in %#v", tc.cmd, res.Invocations)
			}
			if tc.check != nil {
				tc.check(t, res)
			}
		})
	}
}

func TestCasePatternScriptModeNoFalsePositives(t *testing.T) {
	scripts := []struct {
		name string
		body string
	}{
		{"var-pattern", "case \"$1\" in\n  \"$pat\") echo ok ;;\n  *\"$nl\"*) echo nl ;;\n  .|..) echo dot ;;\n  (.|..) echo pdot ;;\n  @(a|b)) echo ext ;;\n  {a,b}) echo brace ;;\nesac\n"},
		{"nested-case", "case \"$1\" in\n  x) case \"$2\" in y) echo 1 ;; esac;;\n  \"$var\") echo 2 ;;\nesac\n"},
		{"case-pattern-word", "case \"$1\" in\n  case ) echo ok ;;\nesac\n"},
	}
	for _, sc := range scripts {
		t.Run(sc.name, func(t *testing.T) {
			res := parseScript("bash /ws/t.sh arg", fakeReader{"/ws/t.sh": {Text: sc.body}})
			assertValidInvariants(t, sc.body, res)
			if ref := firstRefused(res); ref != nil {
				t.Fatalf("%s: unexpected script refusal %q on raw %q", sc.name, ref.Reason.String(), ref.Raw)
			}
		})
	}
}

func TestCaseHeaderMultilineRetainsSingleControlSegment(t *testing.T) {
	cmd := "case # c1\n\"$x\" # c2\nin\n\"$pat\") echo ok ;;\nesac"
	res := Parse(cmd, POSIX)
	assertValidInvariants(t, cmd, res)
	if len(res.Invocations) != 3 {
		t.Fatalf("got %d invocations, want 3 (case header, echo, esac): %#v", len(res.Invocations), res.Invocations)
	}
	hdr := res.Invocations[0]
	if len(hdr.Words) != 3 || hdr.Words[0].Text != "case" || !hdr.Words[0].Literal || hdr.Words[1].Text != "$x" || hdr.Words[2].Text != "in" {
		t.Fatalf("unexpected header words: %#v", hdr.Words)
	}
	if res.Invocations[1].Words[0].Text != "echo" || res.Invocations[2].Words[0].Text != "esac" {
		t.Fatalf("unexpected body/closer invocations: %#v", res.Invocations)
	}
}

func TestCasePatternFailClosedAndSecurityExtraction(t *testing.T) {
	t.Run("command substitution inside pattern", func(t *testing.T) {
		cmd := `case "$x" in $(rm -rf /)) echo ok ;; esac`
		res := Parse(cmd, POSIX)
		assertValidInvariants(t, cmd, res)
		if !hasExecutable(res, "rm") || !hasExecutable(res, "echo") {
			t.Fatalf("missing rm or echo: %#v", res.Invocations)
		}
		if hasNonLiteralCommand(res) {
			t.Fatalf("pattern substitution text leaked as non-literal command: %#v", res.Invocations)
		}
	})

	t.Run("command substitution in pattern resets arm body boundary and parent", func(t *testing.T) {
		cmd := `case "$x" in $(echo 1)) echo 2 ;; esac`
		res := Parse(cmd, POSIX)
		assertValidInvariants(t, cmd, res)
		inv := findInvocation(res, "echo", "2")
		if inv == nil {
			t.Fatalf("missing echo 2 invocation in %#v", res.Invocations)
		}
		if inv.Before != Sequence || inv.PipelineInputEdge != 0 || inv.Parent != -1 {
			t.Fatalf("echo 2 got Before=%v PipelineInputEdge=%d Parent=%d, want Sequence, 0, and -1", inv.Before, inv.PipelineInputEdge, inv.Parent)
		}
	})

	t.Run("command substitution in subject keeps Parent link", func(t *testing.T) {
		cmd := `case "$(rm -rf /)" in a) echo ok ;; esac`
		res := Parse(cmd, POSIX)
		assertValidInvariants(t, cmd, res)
		caseIdx, rmIdx := -1, -1
		for i, inv := range res.Invocations {
			if inv.Words[0].Text == "case" {
				caseIdx = i
			}
			if inv.Words[0].Text == "rm" {
				rmIdx = i
			}
		}
		if caseIdx < 0 || rmIdx < 0 {
			t.Fatalf("missing case or rm: %#v", res.Invocations)
		}
		if res.Invocations[rmIdx].Parent != caseIdx {
			t.Fatalf("rm Parent = %d, want %d (case header)", res.Invocations[rmIdx].Parent, caseIdx)
		}
	})

	t.Run("case header ending at EOF with in keeps subject substitution", func(t *testing.T) {
		cmd := `case "$(rm -rf /)" in`
		res := Parse(cmd, POSIX)
		assertValidInvariants(t, cmd, res)
		if !hasExecutable(res, "case") || !hasExecutable(res, "rm") {
			t.Fatalf("missing case or rm: %#v", res.Invocations)
		}
		caseIdx, rmIdx := -1, -1
		for i, inv := range res.Invocations {
			if inv.Words[0].Text == "case" {
				caseIdx = i
			}
			if inv.Words[0].Text == "rm" {
				rmIdx = i
			}
		}
		if res.Invocations[rmIdx].Parent != caseIdx {
			t.Fatalf("rm Parent = %d, want %d", res.Invocations[rmIdx].Parent, caseIdx)
		}
	})

	t.Run("esac pipe into unnamed writer refused by script concurrency", func(t *testing.T) {
		files := fakeReader{"/ws/x.sh": {Text: "echo hi\n"}}

		// Assert directly on the pipeline writer rather than firstRefused: the bash call inside
		// a compound command is itself refused before expansion, while git pull is refused with
		// RulePredecessorMayWrite only when case/esac control-segment emission and pipe linking
		// tie the pipeline through concurrencyJudge's writesUnnamedFiles check.
		unnamedCmd := `case "$x" in a) bash ./x.sh ;; esac | git pull`
		unnamedRes := parseScript(unnamedCmd, files)
		assertValidInvariants(t, unnamedCmd, unnamedRes)
		gitInv := findInvocation(unnamedRes, "git", "pull")
		if gitInv == nil {
			t.Fatalf("%q: missing git pull invocation in %#v", unnamedCmd, unnamedRes.Invocations)
		}
		if gitInv.Inspectable || gitInv.Reason.Rule() != RulePredecessorMayWrite {
			t.Fatalf("%q: git pull got Inspectable=%v Reason=%q, want refused with RulePredecessorMayWrite", unnamedCmd, gitInv.Inspectable, gitInv.Reason.String())
		}

		// Positive control: a named-target writer in the same pipeline stays inspectable.
		safeCmd := `case "$x" in a) bash ./x.sh ;; esac | tee run.log`
		safeRes := parseScript(safeCmd, files)
		assertValidInvariants(t, safeCmd, safeRes)
		teeInv := findInvocation(safeRes, "tee", "run.log")
		if teeInv == nil {
			t.Fatalf("%q: missing tee run.log invocation in %#v", safeCmd, safeRes.Invocations)
		}
		if !teeInv.Inspectable {
			t.Fatalf("%q: unexpected refusal %q on tee run.log", safeCmd, teeInv.Reason.String())
		}

		// Positive control: the same unnamed writer after a sequence separator stays inspectable.
		seqCmd := `case "$x" in a) bash ./x.sh ;; esac; git pull`
		seqRes := parseScript(seqCmd, files)
		assertValidInvariants(t, seqCmd, seqRes)
		seqGit := findInvocation(seqRes, "git", "pull")
		if seqGit == nil {
			t.Fatalf("%q: missing git pull invocation in %#v", seqCmd, seqRes.Invocations)
		}
		if !seqGit.Inspectable {
			t.Fatalf("%q: unexpected refusal %q on sequential git pull", seqCmd, seqGit.Reason.String())
		}
	})

	t.Run("non-group brace close keeps wordStart false so hash does not swallow rm", func(t *testing.T) {
		cmd := `f() { case "$x" in a) function inner { echo ok; }#; }; rm -rf / ;; "$var") echo ok ;; esac; }`
		res := Parse(cmd, POSIX)
		assertValidInvariants(t, cmd, res)
		if !hasExecutable(res, "rm") {
			t.Fatalf("rm -rf / was swallowed after non-group brace close: %#v", res.Invocations)
		}
	})

	t.Run("hash inside extglob does not swallow rm", func(t *testing.T) {
		for _, cmd := range []string{
			`case "$x" in @(a #)) rm -rf / ;; esac`,
			`case "$x" in @(#)) rm -rf / ;; esac`,
		} {
			res := Parse(cmd, POSIX)
			assertValidInvariants(t, cmd, res)
			if !hasExecutable(res, "rm") {
				t.Fatalf("%q: rm -rf / was swallowed by hash in extglob: %#v", cmd, res.Invocations)
			}
		}
	})

	t.Run("malformed pattern aborts fail closed and extract rm", func(t *testing.T) {
		for _, cmd := range []string{
			`case "$x" in a "rm" -rf /`,
			`case "$x" in .; rm -rf /`,
			`case "$x" in foo(rm -rf /)`,
			"case \"$x\" in @(\nrm -rf /",
		} {
			res := Parse(cmd, POSIX)
			assertValidInvariants(t, cmd, res)
			if !hasExecutable(res, "rm") {
				t.Fatalf("%q: failed to extract rm after pattern abort: %#v", cmd, res.Invocations)
			}
		}
	})

	t.Run("pipe before double semicolon followed by multi-word abort stays valid", func(t *testing.T) {
		cmd := `case "$x" in a) echo ok | ;; b c`
		res := Parse(cmd, POSIX)
		assertValidInvariants(t, cmd, res)
		inv := findInvocation(res, "c")
		if inv == nil {
			t.Fatalf("expected trailing c after multi-word abort: %#v", res.Invocations)
		}
		if inv.Before != Sequence || inv.PipelineInputEdge != 0 {
			t.Fatalf("c got Before=%v PipelineInputEdge=%d, want Sequence and 0", inv.Before, inv.PipelineInputEdge)
		}
	})

	t.Run("non-in word aborts header", func(t *testing.T) {
		cmd := `case "$x" foo`
		res := Parse(cmd, POSIX)
		assertValidInvariants(t, cmd, res)
		if len(res.Invocations) != 1 || res.Invocations[0].Words[0].Text != "case" || len(res.Invocations[0].Words) != 3 {
			t.Fatalf("unexpected parse for %q: %#v", cmd, res.Invocations)
		}
	})

	t.Run("process substitution in pattern extracts inner command", func(t *testing.T) {
		cmd := `case "$x" in "$var" | <(echo hi)) echo ok ;; esac`
		res := Parse(cmd, POSIX)
		assertValidInvariants(t, cmd, res)
		echoCount := 0
		for _, inv := range res.Invocations {
			if len(inv.Words) > 0 && inv.Words[0].Text == "echo" {
				echoCount++
			}
		}
		if echoCount != 2 {
			t.Fatalf("expected 2 echo invocations (from process subst and arm body), got %d: %#v", echoCount, res.Invocations)
		}
	})
}

func TestCasePatternUnterminatedAndANSIRefusals(t *testing.T) {
	t.Run("unclosed parameter expansion after substitution in pattern", func(t *testing.T) {
		cmd := `case "$x" in $(echo ok)${y`
		res := Parse(cmd, POSIX)
		assertValidInvariants(t, cmd, res)
		if !hasExecutable(res, "echo") {
			t.Fatalf("missing echo from substitution: %#v", res.Invocations)
		}
		ref := firstRefused(res)
		if ref == nil || ref.Reason.String() != "unterminated quoted word" {
			t.Fatalf("expected unterminated refusal, got %#v", res.Invocations)
		}
	})

	t.Run("unclosed parameter expansion swallowing rm refused in Parse and ParseScripts", func(t *testing.T) {
		cmd := `case "$x" in ${unclosed; rm -rf /`
		typed := Parse(cmd, POSIX)
		assertValidInvariants(t, cmd, typed)
		ref := firstRefused(typed)
		if ref == nil || ref.Reason.String() != "unterminated quoted word" {
			t.Fatalf("Parse(%q): expected unterminated refusal, got %#v", cmd, typed.Invocations)
		}

		scripted := parseScript("bash /ws/u.sh", fakeReader{"/ws/u.sh": {Text: cmd + "\n"}})
		assertValidInvariants(t, cmd, scripted)
		sref := firstRefused(scripted)
		if sref == nil || !strings.HasPrefix(sref.Reason.String(), "script-opaque:unsupported-syntax") {
			t.Fatalf("ParseScripts(%q): expected script-opaque:unsupported-syntax refusal, got %#v", cmd, scripted.Invocations)
		}
	})

	t.Run("unterminated single quote and ANSI-C escapes", func(t *testing.T) {
		cases := []struct {
			cmd  string
			want string
		}{
			{`case "$x" in 'unclosed`, "unterminated quoted word"},
			{`case "$x" in $'unclosed`, "unterminated ANSI-C quoted word"},
			{`case "$x" in $'unclosed\'`, "unterminated ANSI-C quoted word"},
			{`case $'\cK'`, "ANSI-C control escape is outside the literal parser contract"},
		}
		for _, tc := range cases {
			res := Parse(tc.cmd, POSIX)
			assertValidInvariants(t, tc.cmd, res)
			ref := firstRefused(res)
			if ref == nil || ref.Reason.String() != tc.want {
				t.Errorf("%q: got refusal %#v, want %q", tc.cmd, ref, tc.want)
			}
		}
	})

	t.Run("stripped prefix tokens preserve unterminated refusals in Parse and ParseScripts", func(t *testing.T) {
		cases := []struct {
			cmd  string
			want string
		}{
			{`case "$1" in --flag='unclosed`, "unterminated quoted word"},
			{`case "$1" in --flag=${unclosed`, "unterminated quoted word"},
			{`case "$1" in --flag=$'unclosed`, "unterminated ANSI-C quoted word"},
			{`case "$1" in A='unclosed; rm -rf /`, "unterminated quoted word"},
			{`case "$1" in A=${unclosed;rm`, "unterminated quoted word"},
		}
		for _, tc := range cases {
			t.Run(tc.cmd, func(t *testing.T) {
				parsed := Parse(tc.cmd, POSIX)
				assertValidInvariants(t, tc.cmd, parsed)
				ref := firstRefused(parsed)
				if ref == nil || len(ref.Words) == 0 || ref.Reason.String() != tc.want {
					t.Fatalf("Parse(%q): got refusal %#v, want non-empty words with %q", tc.cmd, ref, tc.want)
				}

				direct := ParseScripts(context.Background(), tc.cmd, POSIX, ScriptOptions{Reader: fakeReader{}, Context: provenPOSIX, Cwd: "/ws"})
				assertValidInvariants(t, tc.cmd, direct)
				dref := firstRefused(direct)
				if dref == nil || len(dref.Words) == 0 || dref.Reason.String() != tc.want {
					t.Fatalf("ParseScripts(%q): got refusal %#v, want non-empty words with %q", tc.cmd, dref, tc.want)
				}

				scripted := ParseScripts(context.Background(), "bash /ws/u.sh", POSIX, ScriptOptions{
					Reader:  fakeReader{"/ws/u.sh": {Text: tc.cmd + "\n"}},
					Context: provenPOSIX,
					Cwd:     "/ws",
				})
				assertValidInvariants(t, tc.cmd, scripted)
				sref := firstRefused(scripted)
				if sref == nil || len(sref.Words) == 0 || !strings.HasPrefix(sref.Reason.String(), "script-opaque:unsupported-syntax") {
					t.Fatalf("ParseScripts(script %q): got refusal %#v, want non-empty words with script-opaque:unsupported-syntax", tc.cmd, sref)
				}
			})
		}

		// Positive control: a non-refused bare assignment stays invocation-free.
		bare := Parse(`A=1`, POSIX)
		assertValidInvariants(t, `A=1`, bare)
		if len(bare.Invocations) != 0 || len(bare.BareAssignments) != 1 {
			t.Fatalf("Parse(A=1): got %d invocations and %d bare assignments, want 0 and 1", len(bare.Invocations), len(bare.BareAssignments))
		}
	})
}

func TestCasePatternRealScriptsSmoke(t *testing.T) {
	repoRoot := filepath.Join("..", "..", "..", "..")
	scripts := []struct {
		name string
		path string
	}{
		{"planning-protection.sh", filepath.Join(repoRoot, "skills", "scaffold", "scripts", "planning-protection.sh")},
		{"find-polluter.sh", filepath.Join(repoRoot, "skills", "diagnose", "references", "find-polluter.sh")},
		{"flow.sh", filepath.Join("testdata", "flow.sh")},
	}
	for _, sc := range scripts {
		t.Run(sc.name, func(t *testing.T) {
			raw, err := os.ReadFile(sc.path)
			if err != nil {
				t.Fatalf("read %s: %v", sc.path, err)
			}
			text := string(raw)
			res := parseScript("bash /ws/"+sc.name, fakeReader{"/ws/" + sc.name: {Text: text}})
			assertValidInvariants(t, sc.name, res)
			for _, inv := range res.Invocations {
				if !inv.Inspectable {
					t.Errorf("%s: unexpected refusal %q on raw %q", sc.name, inv.Reason.String(), inv.Raw)
				}
			}
		})
	}

	t.Run("planning-protection with bare dots and variable newline pattern", func(t *testing.T) {
		p := filepath.Join(repoRoot, "skills", "scaffold", "scripts", "planning-protection.sh")
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		modified := strings.Replace(
			string(raw),
			"[.] | [.][.] | [.]/* | [.][.]/* | */[.] | */[.][.] | */[.]/* | */[.][.]/*",
			". | .. | ./* | ../* | */. | */.. | */./* | */../* | *\"$nl\"*",
			1,
		)
		res := parseScript("bash /ws/pp.sh", fakeReader{"/ws/pp.sh": {Text: modified}})
		assertValidInvariants(t, "pp.sh", res)
		for _, inv := range res.Invocations {
			if !inv.Inspectable {
				t.Errorf("modified planning-protection.sh: unexpected refusal %q on raw %q", inv.Reason.String(), inv.Raw)
			}
		}
	})
}

func TestCasePatternPowerShellUnaffected(t *testing.T) {
	cmd := `case "$x" in "$pat") Write-Output ok ;; esac`
	res := Parse(cmd, PowerShell)
	assertValidInvariants(t, cmd, res)
	if !hasExecutable(res, "Write-Output") {
		t.Fatalf("PowerShell parse lost Write-Output: %#v", res.Invocations)
	}
}
