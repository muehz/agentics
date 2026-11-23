package secretsguard

import (
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/shellwords"
)

func TestCallerArgumentReasonJudgesCredentialPathsAsWholeWords(t *testing.T) {
	const wantDenied = "secrets-guardian: Do not pass a protected target, a check-bypass flag or an unquoted pattern as an argument to a script that uses its parameters; quote a pattern, and run the command itself when it needs a protected target. (script-caller-argument in /ws/p.sh)"
	prose := []string{"moved .env to backup"}
	for name, tc := range map[string]struct {
		call shellwords.ScriptCall
		want bool
	}{
		"dotenv":               {shellwords.ScriptCall{Path: "/ws/p.sh", Args: []string{".env"}, ConsumesParams: true}, true},
		"aws credentials":      {shellwords.ScriptCall{Path: "/ws/p.sh", Args: []string{"~/.aws/credentials"}, ConsumesParams: true}, true},
		"ordinary file":        {shellwords.ScriptCall{Path: "/ws/p.sh", Args: []string{"src/a.go"}, ConsumesParams: true}, false},
		"no parameters":        {shellwords.ScriptCall{Path: "/ws/p.sh", Args: []string{".env"}}, false},
		"quoted use, prose":    {shellwords.ScriptCall{Path: "/ws/p.sh", Args: prose, ConsumesParams: true}, false},
		"unquoted use, prose":  {shellwords.ScriptCall{Path: "/ws/p.sh", Args: prose, ConsumesParams: true, UnquotedParams: true}, true},
		"pattern fails closed": {shellwords.ScriptCall{Path: "/ws/p.sh", Args: []string{".e*"}, ConsumesParams: true, PatternArgs: true}, true},
	} {
		reason, denied := callerArgumentReason([]shellwords.ScriptCall{tc.call}, nil)
		if denied != tc.want {
			t.Errorf("%s: denied=%v, want %v", name, denied, tc.want)
		}
		if denied && (reason != wantDenied || strings.Contains(reason, "backup") || strings.Contains(reason, ".env")) {
			t.Errorf("%s: reason %q, want %q", name, reason, wantDenied)
		}
	}
}
