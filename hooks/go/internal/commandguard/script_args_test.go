package commandguard

import (
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/shellwords"
)

func TestProtectedCallerArgumentMatchesWholeWords(t *testing.T) {
	for arg, want := range map[string]bool{
		"/": true, "~": true, "--no-verify": true, "--force": true, ".git": true, "a/.git/config": true, ".env": true,
		"x" + string(rune(92)) + ".env.local": true, ".ENV": true,
		"src/a.go": false, "main": false, "msg2": false, "moved .env to backup": false, "use --force": false, "": false,
		"--forced": false, "-n": true, "-an": true, "-nm": true, "--no-verif": true, "--no-v": true, "-m": false, "-a": false, "--force-with-lease": false, "//": true, "~/": true, "C:/": true, ".": false, "..": false, "./": false, ".gitignore": false,
	} {
		if got := protectedCallerArgument(arg); got != want {
			t.Errorf("%q: got %v, want %v", arg, got, want)
		}
	}
}

func TestCheckScriptCallerArgumentsUsesCallerWordsAndSkipsScriptsWithoutParameters(t *testing.T) {
	prose := []string{"moved .env to backup"}
	for name, tc := range map[string]struct {
		call shellwords.ScriptCall
		want bool
	}{
		"quoted use, prose":      {shellwords.ScriptCall{Path: "/ws/p.sh", Args: prose, ConsumesParams: true}, false},
		"unquoted use, prose":    {shellwords.ScriptCall{Path: "/ws/p.sh", Args: prose, ConsumesParams: true, UnquotedParams: true}, true},
		"no parameters":          {shellwords.ScriptCall{Path: "/ws/p.sh", Args: []string{".env"}}, false},
		"whole word":             {shellwords.ScriptCall{Path: "/ws/p.sh", Args: []string{".env"}, ConsumesParams: true}, true},
		"pattern fails closed":   {shellwords.ScriptCall{Path: "/ws/p.sh", Args: []string{"*.txt"}, ConsumesParams: true, PatternArgs: true}, true},
		"pattern, no parameters": {shellwords.ScriptCall{Path: "/ws/p.sh", Args: []string{"*.txt"}, PatternArgs: true}, false},
	} {
		var shown []string
		checkScriptCallerArguments([]shellwords.ScriptCall{tc.call}, func(r rule, match, suggestion string) {
			shown = append(shown, match+"|"+suggestion)
		})
		if (len(shown) == 1) != tc.want || len(shown) > 1 {
			t.Errorf("%s: got %v, want denied=%v", name, shown, tc.want)
		}
		for _, s := range shown {
			if strings.Contains(s, "backup") || strings.Contains(s, ".env") || !strings.Contains(s, "/ws/p.sh") {
				t.Errorf("%s: the report must name the script and not the argument: %q", name, s)
			}
		}
	}
}
