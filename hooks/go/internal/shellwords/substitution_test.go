package shellwords

import (
	"reflect"
	"testing"
)

func TestUnquotedSubstitutionRetainsWords(t *testing.T) {
	for _, tc := range []struct {
		command string
		words   []string
		index   int
	}{
		{`terraform -chdir=$(pwd)/infra apply`, []string{"terraform", "-chdir=$(pwd)/infra", "apply"}, 1},
		{`kubectl --context $(kubectl config current-context) apply`, []string{"kubectl", "--context", "$(kubectl config current-context)", "apply"}, 2},
		{"kubectl `printf apply` -f prod.yaml", []string{"kubectl", "`printf apply`", "-f", "prod.yaml"}, 1},
		{`echo pre$(printf '%s' "$(date)")post`, []string{"echo", `pre$(printf '%s' "$(date)")post`}, 1},
	} {
		t.Run(tc.command, func(t *testing.T) {
			inv := Parse(tc.command, POSIX).Invocations
			if len(inv) < 2 || inv[0].Raw != tc.command || inv[1].Parent != 0 {
				t.Fatalf("lost enclosing command or child: %#v", inv)
			}
			var got []string
			for _, word := range inv[0].Words {
				got = append(got, word.Text)
			}
			if !reflect.DeepEqual(got, tc.words) || inv[0].Words[tc.index].Literal || !inv[0].Words[tc.index].Substituted {
				t.Fatalf("word boundaries or expansion provenance: %#v", inv[0].Words)
			}
		})
	}
}

func TestANSICLiteralProvenance(t *testing.T) {
	for _, tc := range []struct{ command, want string }{
		{`echo $'it\'s; $(kubectl apply)'`, "it's; $(kubectl apply)"},
		{`echo $'a\nb\tc'`, "a\nb\tc"},
		{`echo $'\x6b\165\u0062\U00000065ctl'`, "kubectl"},
		{`echo $'\a\b\e\E\f\r\v\\\"\?\z'`, "\a\b\x1b\x1b\f\r\v\\\"?\\z"},
	} {
		t.Run(tc.command, func(t *testing.T) {
			inv := Parse(tc.command, POSIX).Invocations
			if len(inv) != 1 || !inv[0].Inspectable || len(inv[0].Words) != 2 {
				t.Fatalf("quoted data became syntax: %#v", inv)
			}
			word := inv[0].Words[1]
			if word.Text != tc.want || !word.Literal || !word.Quoted || word.Substituted {
				t.Fatalf("word = %#v, want literal %q", word, tc.want)
			}
		})
	}
}
