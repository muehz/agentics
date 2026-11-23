package commandguard

import (
	"strings"
	"testing"
)

// referenceNpmMapHit scans every short flag for an exact match and every
// config flag for an exact or --flag=value match; npmMapHit must return the
// same answer for every token.
func referenceNpmMapHit(token string) bool {
	for flag := range npmValueShortFlags {
		if token == flag {
			return true
		}
	}
	for flag := range npmValueConfigFlags {
		if token == flag || strings.HasPrefix(token, flag+"=") {
			return true
		}
	}
	return false
}

func TestFlagMapKeysHaveNoEquals(t *testing.T) {
	for name, m := range map[string]map[string]struct{}{
		"npmValueShortFlags":  npmValueShortFlags,
		"npmValueConfigFlags": npmValueConfigFlags,
	} {
		for key := range m {
			if strings.Contains(key, "=") {
				t.Errorf("%s key %q contains '='; the Cut lookup is exact only without it", name, key)
			}
		}
	}
}

func TestNpmValueFlagLookupMatchesRangeScan(t *testing.T) {
	tokens := []string{"", "=", "-", "--", "-c", "-cx", "-c=", "-C=1", "--registry", "--registry=", "--registry=x=y",
		"--REGISTRY=x", "--registr", "--registryx", "publish", "--no-registry=x"}
	for key := range npmValueConfigFlags {
		tokens = append(tokens, key, key+"=", key+"=v", key+"x", key[:len(key)-1], key+"==")
	}
	for key := range npmValueShortFlags {
		tokens = append(tokens, key, key+"=", key+"v")
	}
	for _, token := range tokens {
		if got, want := npmMapHit(token), referenceNpmMapHit(token); got != want {
			t.Errorf("npmMapHit(%q) = %v, want %v", token, got, want)
		}
	}
}
