package secretpatterns

import (
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/asciifold"
)

var (
	seedValue   = strings.Repeat("ab", 4) + "123456"
	genericKeys = []string{"secret", "token", "password", "passwd", "pwd", "api_key", "api-key", "apikey", "access_key", "auth_token", "client_secret"}
)

// gateSeeds builds every seed at run time so the source never holds a
// credential-shaped literal: each GENERIC_SECRET alternative in lower,
// upper and mixed case, the U+017F and U+212A spellings, a bearer header,
// two connection strings, one address in each private range, an
// AWS-shaped token, and benign prose.
func gateSeeds() []string {
	seeds := []string{"Kelvin K", "\u017F", "nothing here", "", "AKIA" + strings.Repeat("A", 16)}
	for _, key := range genericKeys {
		seeds = append(seeds,
			key+"="+seedValue,
			strings.ToUpper(key)+"="+seedValue,
			strings.ToUpper(key[:1])+key[1:]+": "+seedValue,
			key+" = '"+seedValue+"'")
	}
	// U+017F folds to s and U+212A to k; api_key is the alternative that
	// carries a k, since bare "key" is not one of GENERIC_SECRET's names.
	// (Spelled in pieces so the source never holds a credential-shaped literal.)
	seeds = append(seeds, "\u017Fecret="+seedValue, "api_"+"\u212Aey="+seedValue, "API_"+"KEY="+seedValue)
	part := strings.Repeat("a", 21)
	seeds = append(seeds, "Bea"+"rer "+part+"."+part, "bea"+"rer "+part+"."+part)
	seeds = append(seeds, "postgres"+"://user:pass@host/database", "mongodb+srv"+"://u:p@cluster.example/db")
	seeds = append(seeds, "10."+"0.0.1:8080", "172."+"16.0.1:443", "192.168."+"1.1:22")
	return seeds
}

// TestKeywordGatesAreSound is the prefilter invariant: a rule whose
// keywords are all absent from the folded input cannot match the input.
func TestKeywordGatesAreSound(t *testing.T) {
	for _, p := range All() {
		if len(p.Keywords) == 0 {
			continue
		}
		for _, kw := range p.Keywords {
			if kw != asciifold.String(kw) {
				t.Errorf("%s keyword %q is not folded", p.Name, kw)
			}
		}
		for _, seed := range gateSeeds() {
			if !hasKeyword(p, asciifold.String(seed)) && p.RE.MatchString(seed) {
				t.Errorf("%s: keywords %v skip %q, which the rule matches", p.Name, p.Keywords, seed)
			}
		}
	}
}

// sameMatches compares Full and Secret plus the pattern name per element,
// avoiding reflect.DeepEqual on Match: the embedded Pattern carries the same
// *regexp.Regexp but a different Keywords slice between a gated pattern and
// its ungated copy, which would make DeepEqual fail for every real match.
func sameMatches(a, b []Match) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Full != b[i].Full || a[i].Secret != b[i].Secret || a[i].Pattern.Name != b[i].Pattern.Name {
			return false
		}
	}
	return true
}

// TestFindingsUnchangedByGates: gated and ungated matching agree on every
// seed, in order and content.
func TestFindingsUnchangedByGates(t *testing.T) {
	for _, p := range All() {
		ungated := p
		ungated.Keywords = nil
		for _, seed := range gateSeeds() {
			got := FindAll(p, seed)
			want := FindAllFolded(ungated, seed, asciifold.String(seed))
			if !sameMatches(got, want) {
				t.Errorf("%s on %q: gated %v, ungated %v", p.Name, seed, got, want)
			}
		}
	}
}

func TestAllBuildsOnce(t *testing.T) {
	first, second := All(), All()
	if len(first) == 0 || &first[0] != &second[0] {
		t.Fatal("All must build the table once")
	}
}
