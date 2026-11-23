package argv

import (
	"sort"
	"testing"
)

// vocabularyRows states every name the vocabulary holds with all seven attributes written out, so a changed row shows
// as a test diff. The rows copy the rosters the guards and the script proof read today; the package cannot import
// them, so another test compares the two.
var vocabularyRows = []struct {
	name string
	want Command
}{
	// The deploy and release commands the release guard matches by name.
	{"kubectl", Command{Classified: true, Shadow: true, VerbCLI: true}},
	{"helm", Command{Classified: true, Shadow: true, VerbCLI: true}},
	{"terragrunt", Command{Classified: true, Shadow: true, VerbCLI: true}},
	{"helmfile", Command{Classified: true, Shadow: true, VerbCLI: true}},
	{"terraform", Command{Classified: true, Shadow: true, VerbCLI: true}},
	{"tofu", Command{Classified: true, Shadow: true, VerbCLI: true}},
	{"pulumi", Command{Classified: true, Shadow: true, VerbCLI: true}},
	{"aws", Command{Classified: true, Shadow: true, VerbCLI: true}},
	{"az", Command{Classified: true, Shadow: true, VerbCLI: true}},
	{"new-azresourcegroupdeployment", Command{Classified: true, Shadow: true}},
	{"new-azdeployment", Command{Classified: true, Shadow: true}},
	{"new-azsubscriptiondeployment", Command{Classified: true, Shadow: true}},
	{"new-azmanagementgroupdeployment", Command{Classified: true, Shadow: true}},
	{"new-aztenantdeployment", Command{Classified: true, Shadow: true}},
	{"gcloud", Command{Classified: true, Shadow: true, VerbCLI: true}},
	{"vercel", Command{Classified: true, Shadow: true, VerbCLI: true}},

	// The commands the secrets guard treats as reading a file, and git, which it dispatches on to read a revision.
	{"cat", Command{Classified: true, Shadow: true}},
	{"bat", Command{Classified: true, Shadow: true}},
	{"less", Command{Classified: true, Shadow: true}},
	{"more", Command{Classified: true, Shadow: true}},
	{"head", Command{Classified: true, Shadow: true}},
	{"tail", Command{Classified: true, Shadow: true}},
	{"strings", Command{Classified: true, Shadow: true}},
	{"xxd", Command{Classified: true, Shadow: true}},
	{"od", Command{Classified: true, Shadow: true}},
	{"hexdump", Command{Classified: true, Shadow: true}},
	{"base64", Command{Classified: true, Shadow: true}},
	{"cp", Command{Classified: true, Shadow: true}},
	{"mv", Command{Classified: true, Shadow: true}},
	{"scp", Command{Classified: true, Shadow: true}},
	{"rsync", Command{Classified: true, Shadow: true}},
	{"sed", Command{Classified: true, Shadow: true}},
	{"awk", Command{Classified: true, Shadow: true}},
	{"grep", Command{Classified: true, Shadow: true}},
	{"rg", Command{Classified: true, Shadow: true}},
	{"ag", Command{Classified: true, Shadow: true}},
	{"jq", Command{Classified: true, Shadow: true}},
	{"yq", Command{Classified: true, Shadow: true}},
	{"source", Command{Classified: true, Shadow: true, Evaluator: true}},
	{".", Command{Classified: true, Shadow: true, Evaluator: true}},
	{"dd", Command{Classified: true, Shadow: true, ProvenOperands: true}},
	{"get-content", Command{Classified: true, Shadow: true}},
	{"gc", Command{Classified: true, Shadow: true}},
	{"type", Command{Classified: true, Shadow: true}},
	{"select-string", Command{Classified: true, Shadow: true}},
	{"sls", Command{Classified: true, Shadow: true}},
	{"copy-item", Command{Classified: true, Shadow: true}},
	{"copy", Command{Classified: true, Shadow: true}},
	{"format-hex", Command{Classified: true, Shadow: true}},
	{"git", Command{Classified: true, Shadow: true, VerbCLI: true, ProvenOperands: true}},

	// The commands the command guard checks for writing, deleting or moving a settings file or a Git hook.
	{"tee", Command{Classified: true, Shadow: true}},
	{"rm", Command{Classified: true, Shadow: true, ProvenOperands: true}},
	{"del", Command{Classified: true, Shadow: true, PowerShellProvenOperands: true}},
	{"erase", Command{Classified: true, Shadow: true, PowerShellProvenOperands: true}},
	{"unlink", Command{Classified: true, Shadow: true}},
	{"truncate", Command{Classified: true, Shadow: true}},
	{"shred", Command{Classified: true, Shadow: true}},
	{"move", Command{Classified: true, Shadow: true}},
	{"move-item", Command{Classified: true, Shadow: true}},
	{"mi", Command{Classified: true, Shadow: true}},
	{"remove-item", Command{Classified: true, Shadow: true, PowerShellProvenOperands: true}},
	{"ri", Command{Classified: true, Shadow: true, PowerShellProvenOperands: true}},
	{"rd", Command{Classified: true, Shadow: true, PowerShellProvenOperands: true}},
	{"rmdir", Command{Classified: true, Shadow: true, PowerShellProvenOperands: true}},
	{"set-content", Command{Classified: true, Shadow: true}},
	{"sc", Command{Classified: true, Shadow: true}},
	{"add-content", Command{Classified: true, Shadow: true}},
	{"ac", Command{Classified: true, Shadow: true}},
	{"clear-content", Command{Classified: true, Shadow: true}},
	{"clc", Command{Classified: true, Shadow: true}},
	{"out-file", Command{Classified: true, Shadow: true}},
	{"rename-item", Command{Classified: true, Shadow: true}},
	{"rni", Command{Classified: true, Shadow: true}},
	{"ren", Command{Classified: true, Shadow: true}},
	{"new-item", Command{Classified: true, Shadow: true}},
	{"ni", Command{Classified: true, Shadow: true}},
	{"cpi", Command{Classified: true, Shadow: true}},
	{"set-item", Command{Classified: true, Shadow: true}},
	{"si", Command{Classified: true, Shadow: true}},
	{"install", Command{Classified: true, Shadow: true}},
	{"ln", Command{Classified: true, Shadow: true}},
	{":", Command{Classified: true}},

	// The commands the command guard reads for SQL text and for publication.
	{"delete", Command{Classified: true}},
	{"psql", Command{Classified: true, Shadow: true}},
	{"mysql", Command{Classified: true, Shadow: true}},
	{"mariadb", Command{Classified: true, Shadow: true}},
	{"sqlcmd", Command{Classified: true, Shadow: true}},
	{"sqlite3", Command{Classified: true, Shadow: true}},
	{"gh", Command{Classified: true, Shadow: true, VerbCLI: true}},
	{"npm", Command{Classified: true, Shadow: true, VerbCLI: true}},
	{"dotnet", Command{Classified: true, Shadow: true, VerbCLI: true}},
	{"nuget", Command{Classified: true, Shadow: true, VerbCLI: true}},

	// The downloaders and evaluators the command guard pairs across a pipe.
	{"curl", Command{Classified: true, Shadow: true}},
	{"wget", Command{Classified: true, Shadow: true}},
	{"invoke-webrequest", Command{Classified: true, Shadow: true}},
	{"iwr", Command{Classified: true, Shadow: true}},
	{"invoke-restmethod", Command{Classified: true, Shadow: true}},
	{"irm", Command{Classified: true, Shadow: true}},
	{"invoke-expression", Command{Classified: true, Shadow: true, Evaluator: true}},
	{"iex", Command{Classified: true, Shadow: true, Evaluator: true}},
	{"invoke-command", Command{Classified: true, Shadow: true, Evaluator: true}},
	{"icm", Command{Classified: true, Shadow: true, Evaluator: true}},
	{"start-process", Command{Classified: true, Shadow: true}},
	{"saps", Command{Classified: true, Shadow: true}},
	{"start", Command{Classified: true, Shadow: true}},
	{"import-module", Command{Classified: true, Shadow: true}},
	{"ipmo", Command{Classified: true, Shadow: true}},
	{"add-type", Command{Classified: true, Shadow: true}},
	{"new-module", Command{Classified: true, Shadow: true}},

	// The shells. The parser unwraps only the POSIX shells and the PowerShell hosts, and the script proof never inspects
	// a call to another shell. The command guard reads sh, bash and zsh as consumers of a download.
	{"sh", Command{Classified: true, Shadow: true, Shell: PosixShell}},
	{"bash", Command{Classified: true, Shadow: true, Shell: PosixShell}},
	{"zsh", Command{Classified: true, Shell: OtherShell}},
	{"dash", Command{Shadow: true, Shell: PosixShell}},
	{"ksh", Command{Shadow: true, Shell: PosixShell}},
	{"fish", Command{Shell: OtherShell}},
	{"csh", Command{Shell: OtherShell}},
	{"tcsh", Command{Shell: OtherShell}},
	{"pwsh", Command{Shadow: true, Shell: PowerShellHost}},
	{"powershell", Command{Shadow: true, Shell: PowerShellHost}},

	// The names only the script proof reads: functions a script may not shadow, commands whose first argument
	// selects the operation, and commands that run text they are handed.
	{"chmod", Command{Shadow: true, ProvenOperands: true}},
	{"chown", Command{Shadow: true}},
	{"mkfs", Command{Shadow: true}},
	{"docker", Command{Shadow: true, VerbCLI: true}},
	{"ssh", Command{Shadow: true, Evaluator: true}},
	{"find", Command{Shadow: true}},
	{"sort", Command{Shadow: true}},
	{"uniq", Command{Shadow: true}},
	{"wc", Command{Shadow: true}},
	{"mkdir", Command{Shadow: true}},
	{"sudo", Command{Shadow: true}},
	{"doas", Command{Shadow: true}},
	{"env", Command{Shadow: true}},
	{"nohup", Command{Shadow: true}},
	{"time", Command{Shadow: true}},
	{"timeout", Command{Shadow: true}},
	{"nice", Command{Shadow: true}},
	{"ionice", Command{Shadow: true}},
	{"stdbuf", Command{Shadow: true}},
	{"setsid", Command{Shadow: true}},
	{"flock", Command{Shadow: true}},
	{"xargs", Command{Shadow: true, Evaluator: true}},
	{"parallel", Command{Shadow: true, Evaluator: true}},
	{"watch", Command{Shadow: true, Evaluator: true}},
	{"echo", Command{Shadow: true}},
	{"printf", Command{Shadow: true}},
	{"mktemp", Command{Shadow: true}},
	{"test", Command{Shadow: true}},
	{"[", Command{Shadow: true}},
	{"cd", Command{Shadow: true}},
	{"eval", Command{Shadow: true, Evaluator: true}},
	{"builtin", Command{Shadow: true}},
	{"command", Command{Shadow: true}},
	{"exec", Command{Shadow: true}},
	{"read", Command{Shadow: true}},
	{"set", Command{Shadow: true}},
	{"unset", Command{Shadow: true}},
	{"hash", Command{Shadow: true}},
	{"enable", Command{Shadow: true}},
	{"export", Command{Shadow: true}},
	{"declare", Command{Shadow: true}},
	{"typeset", Command{Shadow: true}},
	{"local", Command{Shadow: true}},
	{"readonly", Command{Shadow: true}},
	{"alias", Command{Shadow: true}},
	{"unalias", Command{Shadow: true}},
	{"trap", Command{Shadow: true}},
	{"pushd", Command{Shadow: true}},
	{"popd", Command{Shadow: true}},
	{"let", Command{Shadow: true}},
	{"shopt", Command{Shadow: true}},
	{"mapfile", Command{Shadow: true}},
	{"readarray", Command{Shadow: true}},
	{"npx", Command{VerbCLI: true}},
	{"pnpm", Command{VerbCLI: true}},
	{"yarn", Command{VerbCLI: true}},
	{"cargo", Command{VerbCLI: true}},
	{"go", Command{VerbCLI: true}},
	{"make", Command{VerbCLI: true}},
	{"pip", Command{VerbCLI: true}},
}

// classifiedOnlyNames are the rows added only so that a name a guard dispatches on is Classified. Classified drives no
// decision, so such a row sets no other attribute.
var classifiedOnlyNames = []string{":"}

func TestVocabulary(t *testing.T) {
	t.Parallel()
	for _, name := range classifiedOnlyNames {
		if got := Lookup(name); got != (Command{Classified: true}) {
			t.Errorf("Lookup(%q) = %+v, want Command{Classified: true} and nothing else", name, got)
		}
	}
	for _, row := range vocabularyRows {
		if row.want == (Command{}) {
			t.Errorf("row %q has every attribute false; a name with no attribute does not belong in the table", row.name)
		}
		if got := Lookup(row.name); got != row.want {
			t.Errorf("Lookup(%q) = %+v, want %+v", row.name, got, row.want)
		}
	}
}

func TestVocabularyUnknownNames(t *testing.T) {
	t.Parallel()
	// Lookup folds no case and strips no path or extension: callers pass the lower-cased executable name.
	for _, name := range []string{"", "RM", "Git", "rm.exe", "/bin/rm", "unlisted-command", "gitt", "kubectl "} {
		if got := Lookup(name); got != (Command{}) {
			t.Errorf("Lookup(%q) = %+v, want the zero Command", name, got)
		}
	}
}

func TestVocabularyIsClosed(t *testing.T) {
	t.Parallel()
	listed := map[string]bool{}
	for _, row := range vocabularyRows {
		if listed[row.name] {
			t.Errorf("row %q is listed twice", row.name)
		}
		listed[row.name] = true
		if _, ok := vocabulary[row.name]; !ok {
			t.Errorf("the test lists %q but the table lacks it", row.name)
		}
	}
	var extra []string
	for name := range vocabulary {
		if !listed[name] {
			extra = append(extra, name)
		}
	}
	sort.Strings(extra)
	for _, name := range extra {
		t.Errorf("the table holds %q but the test does not list it", name)
	}
}

func TestPowerShellProvenOperandsSeparation(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"remove-item", "ri", "del", "erase", "rd", "rmdir"} {
		cmd := Lookup(name)
		if cmd.ProvenOperands || !cmd.PowerShellProvenOperands {
			t.Errorf("Lookup(%q) = %+v, want ProvenOperands=false and PowerShellProvenOperands=true", name, cmd)
		}
	}
	for _, name := range []string{"rm", "git", "dd", "chmod"} {
		cmd := Lookup(name)
		if !cmd.ProvenOperands || cmd.PowerShellProvenOperands {
			t.Errorf("Lookup(%q) = %+v, want ProvenOperands=true and PowerShellProvenOperands=false", name, cmd)
		}
	}
}
