package secretpatterns

import (
	"regexp"
	"strings"
	"testing"
)

type upstreamTestRule struct {
	id       string
	name     string
	severity string
}

var upstreamTestRules = []upstreamTestRule{
	{id: "aws-access-token", name: "AWS_ACCESS_TOKEN_GITLEAKS", severity: "critical"},
	{id: "openai-api-key", name: "OPENAI_API_KEY", severity: "critical"},
	{id: "anthropic-api-key", name: "ANTHROPIC_API_KEY", severity: "critical"},
	{id: "anthropic-admin-api-key", name: "ANTHROPIC_ADMIN_API_KEY", severity: "critical"},
	{id: "gitlab-pat", name: "GITLAB_PAT", severity: "critical"},
	{id: "gitlab-pat-routable", name: "GITLAB_PAT_ROUTABLE", severity: "critical"},
	{id: "hashicorp-tf-api-token", name: "HASHICORP_TF_API_TOKEN", severity: "critical"},
	{id: "age-secret-key", name: "AGE_SECRET_KEY", severity: "critical"},
}

func upstreamForTest(t *testing.T, rule upstreamTestRule) Pattern {
	t.Helper()
	return mustUpstream(rule.id, rule.name, rule.severity)
}

func TestPinnedUpstreamRules(t *testing.T) {
	positives := map[string]string{
		"aws-access-token":        "ASIA" + "ABCDEFGHJKLMNPQ2",
		"openai-api-key":          "sk-" + "Ab3dEf5hIj7lMn9pQr2t" + "T3BlbkFJ" + "Uv4xYz6Bcd8Fgh2Jkl4N",
		"anthropic-api-key":       "sk-ant-api03-" + strings.Repeat("Ab3_", 23) + "Z" + "AA",
		"anthropic-admin-api-key": "sk-ant-admin01-" + strings.Repeat("Cd5-", 23) + "Y" + "AA",
		"gitlab-pat":              "glpat-" + "Ab3_defGh5-Jk7Lm9Nop",
		"gitlab-pat-routable":     "glpat-" + "Ab3_defGh5-Jk7Lm9NopQr2StUv4.xy5z7ab3c",
		"hashicorp-tf-api-token":  "ab3def5gh7jk9m.atlasv1." + strings.Repeat("Ab3_defGh5-Jk7Lm9NopQr2StUv4Xy6Z", 2),
		"age-secret-key":          "AGE-SECRET-KEY-1" + strings.Repeat("QPZRY9X8GF2TVDW0S3JN54KHCE6MUA7L", 2)[:58],
	}
	for _, rule := range upstreamTestRules {
		rule := rule
		t.Run(rule.id+" positive", func(t *testing.T) {
			pattern := upstreamForTest(t, rule)
			if pattern.UpstreamCommit != upstreamCommit {
				t.Fatalf("commit = %q, want %q", pattern.UpstreamCommit, upstreamCommit)
			}
			if len(pattern.Keywords) == 0 {
				t.Fatal("at least one keyword is required")
			}
			matches := FindAll(pattern, positives[rule.id])
			if len(matches) != 1 {
				t.Fatalf("matches = %d, want 1", len(matches))
			}
			if matches[0].Secret == "" {
				t.Fatal("match did not retain a captured secret")
			}
			keywordBlocked := pattern
			keywordBlocked.Keywords = []string{"keyword-that-is-not-present"}
			if matches := FindAll(keywordBlocked, positives[rule.id]); len(matches) != 0 {
				t.Fatalf("keyword gate returned %d match(es) for an absent keyword", len(matches))
			}
		})
	}
}

func TestPinnedUpstreamSourceIdentityAndMetadata(t *testing.T) {
	if len(upstreamRows) != len(upstreamTestRules) {
		t.Fatalf("pinned rows = %d, want %d", len(upstreamRows), len(upstreamTestRules))
	}
	for _, rule := range upstreamTestRules {
		rule := rule
		t.Run(rule.id, func(t *testing.T) {
			pattern := upstreamForTest(t, rule)
			if pattern.Name != rule.name || pattern.Severity != rule.severity {
				t.Fatalf("metadata = (%q, %q), want (%q, %q)", pattern.Name, pattern.Severity, rule.name, rule.severity)
			}
			if pattern.UpstreamID != rule.id {
				t.Fatalf("upstream ID = %q, want %q", pattern.UpstreamID, rule.id)
			}
			if pattern.Source == "" || pattern.RE == nil {
				t.Fatal("source and compiled expression are required")
			}
			if pattern.UpstreamCommit != upstreamCommit {
				t.Fatalf("commit = %q, want %q", pattern.UpstreamCommit, upstreamCommit)
			}
		})
	}
}

func TestPinnedUpstreamEntropyBoundariesAndAllowlists(t *testing.T) {
	lowEntropy := map[string]string{
		"aws-access-token":       "ASIA" + strings.Repeat("A", 16),
		"gitlab-pat":             "glpat-" + strings.Repeat("a", 20),
		"gitlab-pat-routable":    "glpat-" + strings.Repeat("a", 27) + "." + strings.Repeat("a", 9),
		"hashicorp-tf-api-token": strings.Repeat("a", 14) + ".atlasv1." + strings.Repeat("a", 60),
	}
	for _, rule := range upstreamTestRules {
		value, exists := lowEntropy[rule.id]
		if !exists {
			continue
		}
		rule := rule
		t.Run(rule.id+" entropy boundary", func(t *testing.T) {
			if matches := FindAll(upstreamForTest(t, rule), value); len(matches) != 0 {
				t.Fatalf("low-entropy lookalike produced %d match(es)", len(matches))
			}
		})
	}

	if matches := FindAll(upstreamForTest(t, upstreamTestRules[0]), "ASIA"+"ABCDEFGH2EXAMPLE"); len(matches) != 0 {
		t.Fatal("AWS rule-local EXAMPLE allowlist did not apply")
	}
	if matches := FindAll(upstreamForTest(t, upstreamTestRules[7]), "age1publicrecipient"); len(matches) != 0 {
		t.Fatal("public age recipient matched private identity rule")
	}
}

func TestEntropyThresholdUsesCapturedSecretAtBoundary(t *testing.T) {
	pattern := Pattern{
		Name:        "synthetic-entropy",
		Source:      `prefix([A-Z]+)suffix`,
		RE:          regexp.MustCompile(`prefix([A-Z]+)suffix`),
		SecretGroup: 1,
		Entropy:     3,
	}

	for _, tc := range []struct {
		name  string
		input string
		want  int
	}{
		{name: "below", input: "prefixAAAAAAAAAAAsuffix", want: 0},
		{name: "at", input: "prefixABCDEFGHsuffix", want: 0},
		{name: "above", input: "prefixABCDEFGHJsuffix", want: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			matches := FindAll(pattern, tc.input)
			if len(matches) != tc.want {
				t.Fatalf("matches = %d, want %d", len(matches), tc.want)
			}
			if tc.want == 1 && matches[0].Secret != strings.TrimSuffix(strings.TrimPrefix(tc.input, "prefix"), "suffix") {
				t.Fatalf("captured secret = %q, want capture from %q", matches[0].Secret, tc.input)
			}
		})
	}
}

func TestPinnedUpstreamCapturesSecretBeforeTrailingDelimiter(t *testing.T) {
	rule := upstreamTestRules[1]
	pattern := upstreamForTest(t, rule)
	value := "sk-" + "Ab3dEf5hIj7lMn9pQr2t" + "T3BlbkFJ" + "Uv4xYz6Bcd8Fgh2Jkl4N"
	for _, input := range []string{value + "'", value + "\n"} {
		matches := FindAll(pattern, input)
		if len(matches) != 1 {
			t.Fatalf("input %q produced %d matches, want 1", input, len(matches))
		}
		if matches[0].Secret != value {
			t.Fatalf("captured secret = %q, want %q", matches[0].Secret, value)
		}
		if matches[0].Full == matches[0].Secret {
			t.Fatalf("full match %q did not retain trailing delimiter separately", matches[0].Full)
		}
	}
}
