// The read direction's classifier: which filesystem paths are credential
// material, what category each belongs to, and what to suggest instead.
package secretsguard

import (
	"os"
	"path"
	"strings"

	"github.com/muehz/agentics/hooks/go/internal/shellwords"
)

// severity tiers. Both deny in block mode; the label is for the message and
// the audit trail, matching how command-guardian reports.
const (
	sevCritical = "critical"
	sevHigh     = "high"
)

// finding is one credential path a tool call would have read.
type finding struct {
	category string
	severity string
	path     string
	// origin is the script line the path was read from, nil for a typed path. The message then names the line, since
	// the path is script content.
	origin *shellwords.ScriptOrigin
}

// credential is the category and severity a matched name carries.
type credential struct {
	category string
	severity string
}

// templateSuffixes mark a file as a template rather than a credential.
// `.env.example` is the single most common file in this shape and blocking it
// would make the guard useless in ordinary work — it is committed on purpose,
// holds placeholder values, and gets read constantly during setup.
var templateSuffixes = []string{
	".example", ".sample", ".template", ".dist", ".tmpl", ".defaults",
}

// keyExtensions are extensions whose files are private key material often
// enough to be worth a default block. `.pub` is excluded everywhere: a public
// key is not a secret.
var keyExtensions = map[string]bool{
	".pem": true, ".key": true, ".p12": true, ".pfx": true,
	".jks": true, ".keystore": true, ".ppk": true,
}

// sshKeyNames are private key basenames that appear outside ~/.ssh often
// enough to match on the name alone.
var sshKeyNames = map[string]bool{
	"id_rsa": true, "id_dsa": true, "id_ecdsa": true,
	"id_ed25519": true, "id_ed25519_sk": true, "id_ecdsa_sk": true,
}

// exactNames match on basename alone, wherever the file lives.
var exactNames = map[string]credential{
	".netrc":                               {"netrc", sevHigh},
	"_netrc":                               {"netrc", sevHigh},
	".npmrc":                               {"registry_credentials", sevHigh},
	".pypirc":                              {"registry_credentials", sevHigh},
	".git-credentials":                     {"git_credentials", sevCritical},
	"credentials":                          {"cloud_credentials", sevCritical},
	"terraform.tfvars":                     {"terraform_state", sevHigh},
	".pgpass":                              {"database_credentials", sevCritical},
	".my.cnf":                              {"database_credentials", sevCritical},
	".vault-token":                         {"vault_token", sevCritical},
	"application_default_credentials.json": {"cloud_credentials", sevCritical},
}

var explicitSuffixes = []struct {
	suffix string
	credential
}{
	{"/.terraform.d/credentials.tfrc.json", credential{"terraform_credentials", sevCritical}},
	{"/.config/gh/hosts.yml", credential{"git_credentials", sevCritical}},
	{"/sops/age/keys.txt", credential{"sops_age_identity", sevCritical}},
}

// dirSegments match when the path passes through one of these directories.
// The surrounding separators are part of the match so `.sshfoo/` cannot hit.
var dirSegments = []struct {
	segment string
	credential
}{
	{"/.gnupg/", credential{"gpg_keyring", sevCritical}},
	{"/.aws/", credential{"cloud_credentials", sevCritical}},
	{"/.azure/", credential{"cloud_credentials", sevCritical}},
	{"/.config/gcloud/", credential{"cloud_credentials", sevCritical}},
	{"/.docker/", credential{"registry_credentials", sevHigh}},
	{"/.kube/", credential{"kubeconfig", sevCritical}},
}

// credentialStore reports the credential whose store directory the probe
// passes through. The probe must start with a separator, and a directory
// probe ends with one, so `.sshfoo/` cannot hit.
func credentialStore(probe string) (credential, bool) {
	for _, segment := range dirSegments {
		if strings.Contains(probe, segment.segment) {
			return segment.credential, true
		}
	}
	return credential{}, false
}

// patternCandidates are the names a glob pattern is tried against when it
// cannot be expanded, such as the path of `git show rev:pattern`. The order
// picks the finding when a pattern matches several names, so a new name is
// appended rather than inserted. `.env` and `terraform.tfstate` stand for the
// two code rules that have a conventional name: dotenv files and Terraform
// state.
var patternCandidates = []string{
	".env", ".pgpass", ".my.cnf", ".vault-token", ".npmrc", ".pypirc",
	".netrc", "_netrc", ".git-credentials", "credentials", "terraform.tfvars",
	"terraform.tfstate", "id_rsa", "id_dsa", "id_ecdsa", "id_ed25519",
	"application_default_credentials.json", "credentials.tfrc.json", "hosts.yml", "keys.txt",
	"id_ed25519_sk", "id_ecdsa_sk",
}

// classifyPattern reports whether basePattern, a base-name glob under dir,
// could name a credential: the first of patternCandidates it matches that
// classifies as one under dir. The finding's path is that probe. A malformed
// pattern matches nothing.
func classifyPattern(dir, basePattern string) (finding, bool) {
	for _, name := range patternCandidates {
		matched, err := path.Match(basePattern, name)
		if err != nil || !matched {
			continue
		}
		probe := name
		if dir != "." {
			probe = dir + "/" + name
		}
		if result, ok := classify(probe); ok {
			return result, true
		}
	}
	return finding{}, false
}

var hostCredentialOverrideNames = []string{
	"PGPASSFILE", "KUBECONFIG", "GOOGLE_APPLICATION_CREDENTIALS",
	"AWS_SHARED_CREDENTIALS_FILE", "SOPS_AGE_KEY_FILE",
}

func normalizeCredentialPath(raw string) string {
	value := strings.TrimSpace(raw)
	if len(value) >= 2 && ((value[0] == '\'' && value[len(value)-1] == '\'') ||
		(value[0] == '"' && value[len(value)-1] == '"')) {
		value = value[1 : len(value)-1]
	}
	value = strings.ReplaceAll(value, "\\", "/")
	return strings.ToLower(path.Clean(value))
}

func stripTemplateSuffix(base string) (string, bool) {
	for _, suffix := range templateSuffixes {
		if strings.HasSuffix(base, suffix) {
			return strings.TrimSuffix(base, suffix), true
		}
	}
	return base, false
}

func normalizedOverridePaths(values []string) map[string]bool {
	overrides := make(map[string]bool)
	for _, value := range values {
		if normalized := normalizeCredentialPath(value); normalized != "" && normalized != "." {
			overrides[normalized] = true
		}
	}
	return overrides
}

// classify reports the credential category of one path, or ok=false when the
// path is not credential material.
//
// Matching is case-insensitive. Credential filenames are conventional and
// lowercase, so a case variant is far more likely to be an evasion attempt
// than a legitimate different file — and on macOS and Windows the filesystem
// would resolve them to the same file anyway.
func classify(raw string) (finding, bool) {
	return classifyWithOverrides(raw, hostCredentialOverridePaths())
}

// ProtectedScriptPath names why a path must never be read as a script, or returns "". It is the credential classifier
// and no other: a script the guards would refuse to cat is not read to be inspected either.
func ProtectedScriptPath(p string) string {
	if _, ok := classify(p); ok {
		return "credential path"
	}
	return ""
}

func hostCredentialOverridePaths() []string {
	paths := make([]string, 0, len(hostCredentialOverrideNames))
	for _, name := range hostCredentialOverrideNames {
		if value := os.Getenv(name); value != "" {
			paths = append(paths, value)
		}
	}
	return paths
}

func classifyWithOverrides(raw string, overrideValues []string) (finding, bool) {
	if strings.TrimSpace(raw) == "" {
		return finding{}, false
	}
	norm := normalizeCredentialPath(raw)
	if normalizedOverridePaths(overrideValues)[norm] {
		return finding{"credential_path_override", sevCritical, raw, nil}, true
	}

	base := path.Base(norm)
	underlyingBase, isTemplate := stripTemplateSuffix(base)
	underlyingPath := strings.TrimSuffix(norm, base) + underlyingBase

	if hit, ok := exactNames[underlyingBase]; ok {
		return finding{hit.category, hit.severity, raw, nil}, true
	}
	for _, explicit := range explicitSuffixes {
		if strings.HasSuffix("/"+strings.TrimPrefix(underlyingPath, "/"), explicit.suffix) {
			return finding{explicit.category, explicit.severity, raw, nil}, true
		}
	}
	if !strings.HasSuffix(underlyingBase, ".pub") && keyExtensions[path.Ext(underlyingBase)] {
		return finding{"private_key_file", sevCritical, raw, nil}, true
	}
	if !strings.HasSuffix(underlyingBase, ".pub") && sshKeyNames[underlyingBase] {
		return finding{"ssh_private_key", sevCritical, raw, nil}, true
	}
	probe := "/" + strings.TrimPrefix(norm, "/")
	if store, ok := credentialStore(probe); ok {
		return finding{store.category, store.severity, raw, nil}, true
	}
	if isTemplate {
		return finding{}, false
	}

	if base == ".env" || strings.HasPrefix(base, ".env.") {
		return finding{"dotenv", sevCritical, raw, nil}, true
	}
	if !strings.HasSuffix(base, ".pub") && strings.Contains("/"+strings.TrimPrefix(norm, "/"), "/.ssh/") && strings.HasPrefix(base, "id_") {
		return finding{"ssh_private_key", sevCritical, raw, nil}, true
	}
	if strings.HasSuffix(base, ".kubeconfig") {
		return finding{"kubeconfig", sevCritical, raw, nil}, true
	}
	if strings.HasSuffix(base, ".tfstate") || strings.HasSuffix(base, ".tfstate.backup") {
		return finding{"terraform_state", sevHigh, raw, nil}, true
	}

	return finding{}, false
}

// suggestionFor is the remediation line shown with a deny. It names an action
// the caller can actually take instead of the one that was blocked.
func suggestionFor(category string) string {
	switch category {
	case "dotenv":
		return "Read the committed template (.env.example) instead, or ask the user for the one value you need"
	case "ssh_private_key", "private_key_file", "gpg_keyring", "sops_age_identity":
		return "Private key material must never enter the transcript; use the public half, or the agent that holds the key"
	case "cloud_credentials", "kubeconfig", "credential_path_override":
		return "Use the provider CLI (aws/az/gcloud/kubectl), which reads the file itself without printing it"
	case "database_credentials":
		return "Use the database client, which reads its credential file without printing it"
	case "vault_token":
		return "Use the Vault CLI with the configured token; do not print the token file"
	case "terraform_credentials":
		return "Use Terraform or OpenTofu with the configured credentials; do not print the credentials file"
	case "registry_credentials", "netrc", "git_credentials":
		return "Ask the user to confirm the registry is configured; the token itself is not needed to answer that"
	case "terraform_state":
		return "Use `terraform output` without a name for redacted output, or `terraform state list` for structure"
	default:
		return "Ask the user for the specific value rather than reading the credential file"
	}
}
