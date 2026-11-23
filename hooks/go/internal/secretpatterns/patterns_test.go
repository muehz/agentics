package secretpatterns

import (
	"strings"
	"testing"
)

// rowCount pins the size of the detection table. A dropped row fails loudly
// here; a deliberate change edits this number in the same commit as the
// table, so the change is visible in review.
const rowCount = 33

// TestTableShape holds All() to the properties both hooks rely on: every entry
// carries a unique name, a severity from the closed set, a non-empty source,
// and a compiled pattern; and the split PGP literal resolves to the whole
// marker.
func TestTableShape(t *testing.T) {
	if len(All()) != rowCount {
		t.Fatalf("All() has %d entries, rowCount pins %d — update the pin in the same change", len(All()), rowCount)
	}
	severities := map[string]bool{"critical": true, "high": true, "medium": true}
	names := map[string]bool{}
	for i, p := range All() {
		if p.Name == "" || p.Source == "" || p.RE == nil {
			t.Errorf("row %d: name, pattern source, and compiled pattern are all required", i)
			continue
		}
		if !severities[p.Severity] {
			t.Errorf("row %d (%s): severity %q is not one of critical, high, medium", i, p.Name, p.Severity)
		}
		if names[p.Name] {
			t.Errorf("row %d repeats the name %q", i, p.Name)
		}
		names[p.Name] = true
		if p.Name == "PGP_PRIVATE_BLOCK" && !strings.HasSuffix(p.Source, "PRIVATE KEY BLOCK-----") {
			t.Errorf("PGP_PRIVATE_BLOCK source does not carry the whole marker: %q", p.Source)
		}
	}
	if !names["PGP_PRIVATE_BLOCK"] {
		t.Error("PGP_PRIVATE_BLOCK is missing from the table")
	}
}

// TestMarkerOnlyRows pins which rules match a fixed marker rather than the
// secret. secrets-guardian never lets an edit's old text excuse a marker
// match (#671): an equal PEM header says nothing about the key after it. A
// new marker-shaped rule must be added here and flagged in the same change.
func TestMarkerOnlyRows(t *testing.T) {
	want := map[string]bool{"GCP_SERVICE_ACCOUNT": true, "PRIVATE_KEY": true, "PGP_PRIVATE_BLOCK": true}
	for _, p := range All() {
		if p.MarkerOnly != want[p.Name] {
			t.Errorf("%s: MarkerOnly = %v, want %v", p.Name, p.MarkerOnly, want[p.Name])
		}
	}
}

func findNamed(name, input string) []Match {
	for _, pattern := range All() {
		if pattern.Name == name {
			return FindAll(pattern, input)
		}
	}
	return nil
}

func TestGenericSecretAssignmentsAreCaseInsensitiveAndCaptureValue(t *testing.T) {
	value := strings.Join([]string{"Q7v9", "M2k4", "L6n8"}, "")
	for _, input := range []string{
		"PASSWORD='" + value + "'",
		`{"password":"` + value + `"}`,
		"Password\" : \"" + value + "\"",
	} {
		got := findNamed("GENERIC_SECRET", input)
		if len(got) != 1 {
			t.Fatalf("FindAll(%q) returned %d matches, want 1: %v", input, len(got), got)
		}
		if got[0].Secret != value {
			t.Errorf("captured secret = %q, want %q for %q", got[0].Secret, value, input)
		}
	}
	if got := findNamed("GENERIC_SECRET", "PASSWORD=abc"); len(got) != 0 {
		t.Errorf("short assignment produced %v, want no matches", got)
	}
}

func TestWholeValuePlaceholdersAndReferencesPass(t *testing.T) {
	cases := []struct {
		input string
		want  int
	}{
		{"password=placeholder", 0},
		{"password=replace-me", 0},
		{"password=${PASSWORD}", 0},
		{"password={{ secrets.PASSWORD }}", 0},
		{"password=${{ secrets.PASSWORD }}", 0},
		{"password={{x}}a", 0},
		{"password={{x}}ab", 0},
		{"password={{x}}abc", 1},
		{"password={{ secrets.PASSWORD }}suffix123", 1},
		{"password=${{ secrets.PASSWORD }}suffix123", 1},
	}
	for _, tc := range cases {
		got := findNamed("GENERIC_SECRET", tc.input)
		if len(got) != tc.want {
			t.Errorf("FindAll(%q) returned %v, want %d matches", tc.input, got, tc.want)
		}
	}
}

// TestGenericSecretCodeExpression pins #636: a keyword-named variable
// assigned from a code expression is not a secret, while every config-shaped
// or literal value keeps its finding. Inputs are assembled from pieces so this
// source holds no line the secrets-guardian would deny.
func TestGenericSecretCodeExpression(t *testing.T) {
	kw := "to" + "ken"
	field := strings.ToUpper(kw[:1]) + kw[1:]
	ident := "canon" + "ical"
	chain := "resp.data." + kw
	for _, input := range []string{
		"\t\t\t" + kw + " = " + ident,
		kw + " = " + ident + " // trailing comment",
		kw + "=" + "getPass" + "word()",
		kw + " = " + chain,
		kw + "=" + chain,
		kw + "=" + ident + ";",
		"f(" + kw + "=" + ident + ")",
		"\t" + field + ": " + ident + ",",
		kw + ": " + chain,
		"`" + kw + "=" + ident + "`",
		kw + " = some_" + ident,
	} {
		t.Run("exempt "+input, func(t *testing.T) {
			if got := findNamed("GENERIC_SECRET", input); len(got) != 0 {
				t.Errorf("FindAll(%q) = %v, want no match", input, got)
			}
		})
	}
	for _, tc := range []struct{ input, value string }{
		{strings.ToUpper(kw) + "=" + ident, ident},
		{kw + ": " + ident, ident},
		{kw + ":" + ident, ident},
		{kw + " = '" + ident + "'", ident},
		{kw + " = \"" + ident + "\"", ident},
		{kw + " = " + ident + "7", ident + "7"},
		{kw + " = C" + ident, "C" + ident},
		{kw + " = " + ident + "/x", ident + "/x"},
		{kw + " = " + ident + "+x", ident + "+x"},
		{kw + " = " + ident + "@2024", ident},
		{kw + " = " + ident + "#Tr0ub", ident},
		{kw + "=" + chain + "!x", chain},
		{kw + " = \"" + chain + "\"", chain},
		{kw + ": '" + ident + "',", ident},
		{kw + " = _" + ident, "_" + ident},
		{kw + " = resp.data7." + kw, "resp.data7." + kw},
	} {
		t.Run("reported "+tc.input, func(t *testing.T) {
			got := findNamed("GENERIC_SECRET", tc.input)
			if len(got) != 1 || got[0].Secret != tc.value {
				t.Errorf("FindAll(%q) = %v, want one match capturing %q", tc.input, got, tc.value)
			}
		})
	}
}

// TestGenericSecretEnvironmentAccessor pins the closed list of environment
// accessors: a keyword-named variable that reads its value through one of
// them, unquoted and directly followed by the accessor's own bracket, is not
// a finding. Every near miss stays one, because the exemption must not widen
// into "any identifier followed by a bracket". Inputs are assembled from
// pieces so this source holds no line the secrets-guardian would deny.
func TestGenericSecretEnvironmentAccessor(t *testing.T) {
	kw := "to" + "ken"
	assign := kw + " = "
	accessors := []struct{ name, open string }{
		{"os.environ", "["},
		{"env::var", "("},
		{"std::env::var", "("},
		{"System.getenv", "("},
		{"Environment.GetEnvironmentVariable", "("},
		{"ENV.fetch", "("},
	}
	exempt := func(t *testing.T, input string) {
		t.Helper()
		if got := findNamed("GENERIC_SECRET", input); len(got) != 0 {
			t.Errorf("FindAll(%q) = %v, want no match", input, got)
		}
	}
	reported := func(t *testing.T, input, value string) {
		t.Helper()
		got := findNamed("GENERIC_SECRET", input)
		if len(got) != 1 || got[0].Secret != value {
			t.Errorf("FindAll(%q) = %v, want one match capturing %q", input, got, value)
		}
	}

	for _, accessor := range accessors {
		name, open := accessor.name, accessor.open
		t.Run("exempt "+name+open, func(t *testing.T) {
			exempt(t, assign+name+open)
			exempt(t, assign+name+open+`"HOME"`)
			exempt(t, kw+"="+name+open+`"HOME"`)
			exempt(t, "\t"+kw+": "+name+open+`"HOME"`)
		})
		// A quote before the value makes it a string literal, whatever byte
		// follows. The two opening-quote forms have the bracket directly
		// after the name, so only the quote check keeps them findings.
		for _, quote := range []string{`"`, `'`} {
			t.Run("reported wrapped in "+quote+" "+name, func(t *testing.T) {
				reported(t, assign+quote+name+quote+open, name)
			})
			t.Run("reported opened by "+quote+" "+name, func(t *testing.T) {
				reported(t, assign+quote+name+open, name)
			})
		}
		if name == "os.environ" {
			continue
		}
		t.Run("reported without its bracket "+name, func(t *testing.T) {
			reported(t, assign+name+" ", name)
			reported(t, assign+name+";", name)
			reported(t, assign+name, name)
			reported(t, assign+name+".x(", name+".x")
		})
		t.Run("reported with the other bracket "+name, func(t *testing.T) {
			reported(t, assign+name+"[", name)
		})
	}

	// The table is matched whole and case-sensitively: a longer name that
	// ends with an accessor, or an accessor in another case, is some other
	// identifier even with the right bracket after it. Each form here is
	// one the code-expression filter does not allow, by its colons or its
	// uppercase first letter, so only the exact lookup keeps it a finding.
	for _, value := range []string{
		"x" + "env::var",
		"x" + "std::env::var",
		"ENV::VAR",
		"Env.fetch",
	} {
		t.Run("reported near miss "+value, func(t *testing.T) {
			reported(t, assign+value+"(", value)
		})
	}

	// These three are allowed by the code-expression filter, not by the
	// accessor list, and must stay allowed through it.
	t.Run("exempt as a code expression", func(t *testing.T) {
		exempt(t, assign+"os.environ(")
		exempt(t, assign+"os.environ")
		exempt(t, assign+"os.environ.copy")
	})
}

// TestGenericSecretUserConfigReference pins the manifest form in which a
// key-named JSON member takes its whole value from a user-config reference:
// the member names where the value comes from and holds none. Anything after
// the reference makes the capture a value again. The member is assembled from
// pieces so this source holds no line the secrets-guardian would deny.
func TestGenericSecretUserConfigReference(t *testing.T) {
	name := "api" + "_key"
	reference := "${user_config." + name + "}"
	member := func(value string) string {
		return `"` + name + `"` + `: "` + value + `"`
	}
	if got := findNamed("GENERIC_SECRET", member(reference)); len(got) != 0 {
		t.Errorf("FindAll(%q) = %v, want no match", member(reference), got)
	}
	suffixed := reference + "abcd" + "1234"
	got := findNamed("GENERIC_SECRET", member(suffixed))
	if len(got) != 1 || got[0].Secret != suffixed {
		t.Errorf("FindAll(%q) = %v, want one match capturing %q", member(suffixed), got, suffixed)
	}
}

func TestProviderMatchesIgnoreGenericPlaceholderPolicy(t *testing.T) {
	value := "ghp_" + "AbCdEfGhIjKlMnOpQrStUvWxYz0123456xxx"
	got := findNamed("GITHUB_PAT", value)
	if len(got) != 1 {
		t.Fatalf("provider-shaped value produced %d matches, want 1: %v", len(got), got)
	}
}

func TestStagedDetectorContract(t *testing.T) {
	if len(All()) != 33 {
		t.Fatalf("final detector row count = %d, want 33", len(All()))
	}
	wantRegistered := []struct {
		id, name string
	}{
		{"aws-access-token", "AWS_ACCESS_TOKEN_GITLEAKS"},
		{"openai-api-key", "OPENAI_API_KEY"},
		{"anthropic-api-key", "ANTHROPIC_API_KEY"},
		{"anthropic-admin-api-key", "ANTHROPIC_ADMIN_API_KEY"},
		{"gitlab-pat", "GITLAB_PAT"},
		{"gitlab-pat-routable", "GITLAB_PAT_ROUTABLE"},
		{"hashicorp-tf-api-token", "HASHICORP_TF_API_TOKEN"},
		{"age-secret-key", "AGE_SECRET_KEY"},
	}
	for i, want := range wantRegistered {
		got := All()[len(All())-len(wantRegistered)+i]
		if got.Name != want.name || got.UpstreamID != want.id || got.UpstreamCommit != upstreamCommit {
			t.Fatalf("registered row %d = (%q, %q, %q), want (%q, %q, %q)", i, got.Name, got.UpstreamID, got.UpstreamCommit, want.name, want.id, upstreamCommit)
		}
	}
	registeredPositives := map[string]string{
		"aws-access-token":        "AS" + "IA" + "ABCDEFGHJKLMNPQ2",
		"openai-api-key":          "sk-" + "Ab3dEf5hIj7lMn9pQr2t" + "T3BlbkFJ" + "Uv4xYz6Bcd8Fgh2Jkl4N",
		"anthropic-api-key":       "sk-ant-api03-" + strings.Repeat("Ab3_", 23) + "Z" + "AA",
		"anthropic-admin-api-key": "sk-ant-admin01-" + strings.Repeat("Cd5-", 23) + "Y" + "AA",
		"gitlab-pat":              "glpat-" + "Ab3_defGh5-Jk7Lm9Nop",
		"gitlab-pat-routable":     "glpat-" + "Ab3_defGh5-Jk7Lm9NopQr2StUv4.xy5z7ab3c",
		"hashicorp-tf-api-token":  "ab3def5gh7jk9m.atlasv1." + strings.Repeat("Ab3_defGh5-Jk7Lm9NopQr2StUv4Xy6Z", 2),
		"age-secret-key":          "AGE-SECRET-KEY-1" + strings.Repeat("QPZRY9X8GF2TVDW0S3JN54KHCE6MUA7L", 2)[:58],
	}
	for i, want := range wantRegistered {
		if got := FindAll(All()[len(All())-len(wantRegistered)+i], registeredPositives[want.id]); len(got) != 1 {
			t.Fatalf("registered %s positive produced %d matches, want 1", want.id, len(got))
		}
	}
	if !matches("AWS_ACCESS_KEY", "AKIA"+"0000000000000000") {
		t.Fatal("legacy low-entropy AKIA coverage narrowed")
	}
	if !matches("AWS_ACCESS_TOKEN_GITLEAKS", "ASIA"+"ABCDEFGHIJKLMNOP") {
		t.Fatal("pinned AWS temporary token was not detected")
	}
	if matches("AWS_ACCESS_KEY", "AKIA"+"000000000EXAMPLE") ||
		matches("AWS_ACCESS_TOKEN_GITLEAKS", "AKIA"+"000000000EXAMPLE") {
		t.Fatal("pinned complete EXAMPLE value was not allowlisted")
	}
	value := strings.Join([]string{"Q7v9", "M2k4", "L6n8"}, "")
	if !matches("GENERIC_SECRET", `{"PASSWORD":"`+value+`"}`) {
		t.Fatal("quoted uppercase JSON assignment was not detected")
	}
	for _, input := range []string{
		`password={{ secrets.PASSWORD }}`,
		`password=${PASSWORD}`,
		`password=replace_me`,
	} {
		if matches("GENERIC_SECRET", input) {
			t.Fatalf("whole placeholder/reference was detected: %s", input)
		}
	}
	providerWithXXX := "ghp_" + "AbCdEfGhIjKlMnOpQrStUvWxYz0123456xxx"
	if !matches("GITHUB_PAT", providerWithXXX) {
		t.Fatal("provider-shaped token containing xxx was suppressed")
	}
}

// TestReferenceChain pins #699 (an || chain of template/secrets operands is
// a reference, not a candidate) and its #486 follow-up (a Go/Helm template
// action, opened by a `.` or `$.` cursor, is also a reference).
func TestReferenceChain(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  bool
	}{
		{"two-operand secrets chain", "${{ secrets.A || secrets.B }}", true},
		{"three-operand chain with github.token", "${{ secrets.A || secrets.B || github.token }}", true},
		{"double-brace two-operand chain", "{{ a.b || c }}", true},
		{"bare identifier operand is valid by design", "${{ abc123 }}", true},
		{"quoted operand is rejected", "${{ secrets.A || 'literal' }}", false},
		{"operand with a leading digit is rejected", "${{ secrets.A || 1abc }}", false},
		{"operand containing a colon is rejected", "${{ secrets.A || abc:def }}", false},
		{"operand containing a slash is rejected", "${{ secrets.A || abc/def }}", false},
		{"dangling || at the start is rejected", "${{ || secrets.A }}", false},
		{"dangling || at the end is rejected", "${{ secrets.A || }}", false},
		{"empty operand between || is rejected", "${{ secrets.A ||  || secrets.B }}", false},
		// #486: Go/Helm template actions
		{"helm dot operand", "{{ .Values.x }}", true},
		{"helm dollar-dot operand", "{{ $.Values.x }}", true},
		{"helm dot chain", "{{ .Values.a || .Values.b }}", true},
		{"mixed secrets and helm chain", "${{ secrets.A || .x }}", true},
		{"leading digit after the cursor is rejected", "{{ .5x }}", false},
		{"bare dot cursor is rejected", "{{ . }}", false},
		{"bare dollar cursor is rejected", "{{ $ }}", false},
		{"a pipeline stays a candidate", "{{ .Values.x | quote }}", false},
		{"a double dot is rejected", "{{ ..x }}", false},
		// A user-config reference is accepted only as the entire value, with
		// an identifier key and the exact lower-case prefix.
		{"user-config letter-first key", "${user_config.region}", true},
		{"user-config underscore-first key", "${user_config._region}", true},
		{"user-config key holding a digit", "${user_config.region2}", true},
		{"user-config in double quotes", `"${user_config.region}"`, true},
		{"user-config in single quotes", `'${user_config.region}'`, true},
		{"user-config with a suffix is rejected", "${user_config.region}abcd1234", false},
		{"user-config with a prefix is rejected", "abcd1234${user_config.region}", false},
		{"user-config with an empty key is rejected", "${user_config.}", false},
		{"user-config key holding a hyphen is rejected", "${user_config.my-region}", false},
		{"user-config key holding a dot is rejected", "${user_config.my.region}", false},
		{"user-config key starting with a digit is rejected", "${user_config.2region}", false},
		{"user-config upper-case prefix with a key is rejected", "${USER_CONFIG.region}", false},
		{"dotless user_config is a braced environment reference", "${user_config}", true},
		{"dotless USER_CONFIG is a braced environment reference", "${USER_CONFIG}", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsPlaceholder(tc.value); got != tc.want {
				t.Errorf("IsPlaceholder(%q) = %v, want %v", tc.value, got, tc.want)
			}
		})
	}

	t.Run("GENERIC_SECRET exempts a two-operand secrets chain", func(t *testing.T) {
		key := "GITHUB_MCP_SERVER_TOKEN"
		line := key + ": " + "${{ secrets.A || secrets.B }}"
		if got := findNamed("GENERIC_SECRET", line); len(got) != 0 {
			t.Errorf("FindAll(%q) = %v, want no match", line, got)
		}
	})
	t.Run("GENERIC_SECRET still reports a bare quoted literal", func(t *testing.T) {
		key := "GITHUB_MCP_SERVER_TOKEN"
		literal := strings.Join([]string{"Ab12", "Cd34", "Ef56", "Gh78"}, "")
		line := key + ": " + "'" + literal + "'"
		got := findNamed("GENERIC_SECRET", line)
		if len(got) != 1 || got[0].Secret != literal {
			t.Errorf("FindAll(%q) = %v, want one match capturing %q", line, got, literal)
		}
	})
	t.Run("GENERIC_SECRET reports a quoted literal inside a chain", func(t *testing.T) {
		key := "GITHUB_MCP_SERVER_TOKEN"
		literal := strings.Join([]string{"Ab12", "Cd34", "Ef56", "Gh78"}, "")
		chain := "${{ secrets.A || '" + literal + "' }}"
		line := key + ": " + chain
		got := findNamed("GENERIC_SECRET", line)
		if len(got) != 1 || got[0].Secret != chain {
			t.Errorf("FindAll(%q) = %v, want one match capturing %q", line, got, chain)
		}
	})

	// #486: Go/Helm template actions
	t.Run("GENERIC_SECRET exempts a Helm action value", func(t *testing.T) {
		key := "password"
		line := key + ": " + "{{ .Values.db.password }}"
		if got := findNamed("GENERIC_SECRET", line); len(got) != 0 {
			t.Errorf("FindAll(%q) = %v, want no match", line, got)
		}
	})
	t.Run("GENERIC_SECRET still reports a quoted literal for a password key", func(t *testing.T) {
		key := "password"
		literal := strings.Join([]string{"Zz98", "Yy76", "Xx54", "Ww32"}, "")
		line := key + ": " + "'" + literal + "'"
		got := findNamed("GENERIC_SECRET", line)
		if len(got) != 1 || got[0].Secret != literal {
			t.Errorf("FindAll(%q) = %v, want one match capturing %q", line, got, literal)
		}
	})
}

func matches(name, input string) bool {
	for _, pattern := range All() {
		if pattern.Name == name && len(FindAll(pattern, input)) > 0 {
			return true
		}
	}
	return false
}
