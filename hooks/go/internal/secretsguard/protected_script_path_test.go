package secretsguard

import "testing"

// The script policy is the credential classifier and nothing else, so a path the guard calls a credential is refused
// as a script and an ordinary script name is not.
func TestProtectedScriptPathFollowsTheCredentialClassifier(t *testing.T) {
	for _, p := range []string{
		"/ws/.env.sh", "/ws/.env", "/home/u/.aws/x.sh", "/home/u/.ssh/id_rsa", "C:/Users/u/.aws/credentials",
		"/ws/server.pem", `C:\Users\u\.aws\x.sh`, "/ws/.ENV.sh",
	} {
		if reason := ProtectedScriptPath(p); reason == "" {
			t.Errorf("%q should be protected", p)
		}
		if _, ok := classify(p); !ok {
			t.Errorf("%q: the classifier disagrees with the fixture", p)
		}
	}
	for _, p := range []string{"/ws/ok.sh", "/ws/env.sh", "/home/u/scripts/build.sh", ""} {
		if reason := ProtectedScriptPath(p); reason != "" {
			t.Errorf("%q refused: %s", p, reason)
		}
	}
}
