package releaseguard

import (
	"os"
	"strings"
	"testing"
)

func TestEnvironmentsUnsetIsNotConfigured(t *testing.T) {
	unsetEnv(t, environmentsVar)
	markers, configured, err := environments()
	if configured || err != nil || markers != nil {
		t.Fatalf("markers=%v configured=%v err=%v, want unconfigured with no markers", markers, configured, err)
	}
}

func TestEnvironmentsEmptyOrBlankNamesNoEnvironment(t *testing.T) {
	for _, raw := range []string{"", " , ", ","} {
		t.Run(raw, func(t *testing.T) {
			t.Setenv(environmentsVar, raw)
			markers, configured, err := environments()
			if !configured || markers != nil {
				t.Fatalf("markers=%v configured=%v, want configured with no markers", markers, configured)
			}
			if err == nil || err.Error() != "release-guardian: "+environmentsVar+" is set but names no environment" {
				t.Fatalf("err = %v, want the names-no-environment error", err)
			}
		})
	}
}

func TestEnvironmentsControlCharacterDenies(t *testing.T) {
	t.Setenv(environmentsVar, "prod\x01,staging")
	markers, configured, err := environments()
	if !configured || markers != nil {
		t.Fatalf("markers=%v configured=%v, want configured with no markers", markers, configured)
	}
	if err == nil || err.Error() != "release-guardian: "+environmentsVar+" holds a control character" {
		t.Fatalf("err = %v, want the control-character error", err)
	}
}

func TestEnvironmentsDELIsAControlCharacter(t *testing.T) {
	t.Setenv(environmentsVar, "prod\x7f")
	_, _, err := environments()
	if err == nil || !strings.Contains(err.Error(), "control character") {
		t.Fatalf("err = %v, want the control-character error", err)
	}
}

func TestEnvironmentsParsesTrimmedMarkers(t *testing.T) {
	t.Setenv(environmentsVar, " prod-sub ,production.tfvars,, customer-facing ")
	markers, configured, err := environments()
	if err != nil || !configured {
		t.Fatalf("markers=%v configured=%v err=%v", markers, configured, err)
	}
	want := []string{"prod-sub", "production.tfvars", "customer-facing"}
	if len(markers) != len(want) {
		t.Fatalf("markers = %v, want %v", markers, want)
	}
	for i, m := range want {
		if markers[i] != m {
			t.Fatalf("markers[%d] = %q, want %q", i, markers[i], m)
		}
	}
}

// unsetEnv clears a variable that t.Setenv cannot express by itself: t.Setenv
// can only set a value, so this sets it first (to register the restore) and
// then unsets it directly, restoring through t.Cleanup.
func unsetEnv(t *testing.T, key string) {
	t.Helper()
	t.Setenv(key, "")
	os.Unsetenv(key)
	t.Cleanup(func() { os.Unsetenv(key) })
}
