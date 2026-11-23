package secretpatterns

import "regexp"

// upstreamCommit pins the only Gitleaks source revision adapted by this package.
const upstreamCommit = "4fb43823ef3d152d239e92d7d5cb04783b548062"

type upstreamRow struct {
	id          string
	source      string
	secretGroup int
	entropy     float64
	keywords    []string
	allowlist   []string
}

var upstreamRows = []upstreamRow{
	{
		id:          "aws-access-token",
		source:      `\b((?:A3T[A-Z0-9]|AKIA|ASIA|ABIA|ACCA)[A-Z2-7]{16})\b`,
		secretGroup: 1,
		entropy:     3,
		keywords:    []string{"a3t", "akia", "asia", "abia", "acca"},
		// EXAMPLE is upstream's rule-local allowlist. ^AKIA is Agentics-local:
		// the unchanged legacy AWS_ACCESS_KEY row owns AKIA so one token cannot
		// produce two findings while ASIA and the other upstream prefixes remain.
		allowlist: []string{`.+EXAMPLE$`, `^AKIA`},
	},
	{
		id:          "openai-api-key",
		source:      `\b(sk-(?:proj|svcacct|admin)-(?:[A-Za-z0-9_-]{74}|[A-Za-z0-9_-]{58})T3BlbkFJ(?:[A-Za-z0-9_-]{74}|[A-Za-z0-9_-]{58})\b|sk-[a-zA-Z0-9]{20}T3BlbkFJ[a-zA-Z0-9]{20})(?:[\x60'"\s;]|\\[nr]|$)`,
		secretGroup: 1,
		entropy:     3,
		keywords:    []string{"t3blbkfj"},
	},
	{
		id:          "anthropic-api-key",
		source:      `\b(sk-ant-api03-[a-zA-Z0-9_\-]{93}AA)(?:[\x60'"\s;]|\\[nr]|$)`,
		secretGroup: 1,
		keywords:    []string{"sk-ant-api03"},
	},
	{
		id:          "anthropic-admin-api-key",
		source:      `\b(sk-ant-admin01-[a-zA-Z0-9_\-]{93}AA)(?:[\x60'"\s;]|\\[nr]|$)`,
		secretGroup: 1,
		keywords:    []string{"sk-ant-admin01"},
	},
	{
		id:       "gitlab-pat",
		source:   `glpat-[\w-]{20}`,
		entropy:  3,
		keywords: []string{"glpat-"},
	},
	{
		id:       "gitlab-pat-routable",
		source:   `\bglpat-[0-9a-zA-Z_-]{27,300}\.[0-9a-z]{2}[0-9a-z]{7}\b`,
		entropy:  4,
		keywords: []string{"glpat-"},
	},
	{
		id:       "hashicorp-tf-api-token",
		source:   `(?i)[a-z0-9]{14}\.(?-i:atlasv1)\.[a-z0-9\-_=]{60,70}`,
		entropy:  3.5,
		keywords: []string{"atlasv1"},
	},
	{
		id:       "age-secret-key",
		source:   `AGE-SECRET-KEY-1[QPZRY9X8GF2TVDW0S3JN54KHCE6MUA7L]{58}`,
		keywords: []string{"age-secret-key-1"},
	},
}

func mustUpstream(id, name, severity string) Pattern {
	for _, row := range upstreamRows {
		if row.id != id {
			continue
		}
		allowlist := make([]*regexp.Regexp, len(row.allowlist))
		for i, source := range row.allowlist {
			allowlist[i] = regexp.MustCompile(source)
		}
		return Pattern{
			Name: name, Severity: severity, Source: row.source,
			RE: regexp.MustCompile(row.source), SecretGroup: row.secretGroup,
			Keywords: row.keywords, Entropy: row.entropy, Allowlist: allowlist,
			UpstreamID: row.id, UpstreamCommit: upstreamCommit,
		}
	}
	panic("secretpatterns: missing pinned upstream rule " + id)
}
