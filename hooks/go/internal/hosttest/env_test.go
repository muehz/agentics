package hosttest

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hookprovenance"
)

func TestScrubGuardEnvBlanksAndUnsets(t *testing.T) {
	for _, name := range GuardEnv {
		t.Setenv(name, "x")
	}
	ScrubGuardEnv(t)
	for _, name := range GuardEnv {
		value, set := os.LookupEnv(name)
		if unsetWhenEmpty[name] {
			if set {
				t.Errorf("%s is set to %q, want it unset", name, value)
			}
			continue
		}
		if value != "" {
			t.Errorf("%s = %q, want empty", name, value)
		}
	}
	for _, name := range []string{"GIT_PAGER", "AGX_RELEASE_ENVIRONMENTS", "AGX_RELEASE_RECORD_SHA256"} {
		if !unsetWhenEmpty[name] {
			t.Errorf("%s is not truly unset by the scrub", name)
		}
	}
}

func TestGuardEnvHoldsTheCorpusRosterAndTheShellOptions(t *testing.T) {
	want := []string{
		"GIT_PAGER",
		"SKIP_COMMAND_GUARD", "COMMAND_GUARD_MODE", "COMMAND_GUARD_LOG_DIR", "COMMAND_GUARD_ALLOWLIST",
		"SKIP_SECRETS_GUARD", "SECRETS_GUARD_MODE", "SECRETS_GUARD_ALLOWLIST",
		"RIPGREP_CONFIG_PATH",
		"PGPASSFILE", "KUBECONFIG", "GOOGLE_APPLICATION_CREDENTIALS",
		"AWS_SHARED_CREDENTIALS_FILE", "SOPS_AGE_KEY_FILE",
		"AGX_RELEASE_RECORD", "AGX_RELEASE_RECORD_SHA256", "AGX_RELEASE_ENVIRONMENTS", "RELEASE_GUARD_LOG_DIR",
		"CLAUDE_PLUGIN_ROOT", "CLAUDE_PLUGIN_DATA", "COPILOT_PLUGIN_ROOT", "COPILOT_PLUGIN_DATA",
		"PLUGIN_ROOT", "PLUGIN_DATA", "AGX_VAULT", "AGX_SOURCE_DIGEST",
		"CDPATH", "BASHOPTS", "GLOBIGNORE",
	}
	for _, name := range want {
		if !slices.Contains(GuardEnv, name) {
			t.Errorf("GuardEnv lacks %s", name)
		}
	}
}

func TestIsolateHomePointsBothVariablesAtOneEmptyDirectory(t *testing.T) {
	home := IsolateHome(t)
	if got := os.Getenv("HOME"); got != home {
		t.Fatalf("HOME = %q, want %q", got, home)
	}
	if got := os.Getenv("USERPROFILE"); got != home {
		t.Fatalf("USERPROFILE = %q, want %q", got, home)
	}
	entries, err := os.ReadDir(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("home %q holds %d entries, want none", home, len(entries))
	}
}

// Not parallel: it changes process-wide variables, restored by t.Setenv.
func TestMainWithIsolatedHomePinsGoCacheAndSwapsHome(t *testing.T) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		t.Skipf("no user cache directory: %v", err)
	}
	for _, name := range []string{"GOCACHE", "GOENV", "GOPATH", "HOME", "USERPROFILE"} {
		t.Setenv(name, os.Getenv(name))
	}
	os.Unsetenv("GOCACHE")
	realHome := os.Getenv("HOME")

	var runHome string
	code := mainWithIsolatedHome(func() int {
		if got, want := os.Getenv("GOCACHE"), filepath.Join(cacheDir, "go-build"); got != want {
			t.Errorf("GOCACHE = %q during the run, want %q", got, want)
		}
		runHome = os.Getenv("HOME")
		if runHome == "" || runHome == realHome {
			t.Errorf("HOME = %q during the run, want a new directory", runHome)
		}
		if got := os.Getenv("USERPROFILE"); got != runHome {
			t.Errorf("USERPROFILE = %q during the run, want %q", got, runHome)
		}
		if entries, err := os.ReadDir(runHome); err != nil || len(entries) != 0 {
			t.Errorf("run home %q: %d entries, err %v; want an empty directory", runHome, len(entries), err)
		}
		return 7
	})
	if code != 7 {
		t.Fatalf("exit code = %d, want the run's 7", code)
	}
	if _, err := os.Stat(runHome); !os.IsNotExist(err) {
		t.Fatalf("run home %q still exists after the run (err %v)", runHome, err)
	}
}

func shippedModule(t *testing.T) string {
	t.Helper()
	return filepath.Join(Checkout(t), "hooks", "go")
}

func TestGuardEnvCoversShippedReads(t *testing.T) {
	problems, err := rosterProblems(shippedModule(t), GuardEnv)
	if err != nil {
		t.Fatal(err)
	}
	for _, problem := range problems {
		t.Error(problem)
	}

	t.Run("a name missing from the roster fails", func(t *testing.T) {
		roster := slices.DeleteFunc(slices.Clone(GuardEnv), func(name string) bool { return name == "CDPATH" })
		problems, err := rosterProblems(shippedModule(t), roster)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.ContainsFunc(problems, func(p string) bool { return strings.Contains(p, "CDPATH") }) {
			t.Fatalf("problems %q do not name CDPATH", problems)
		}
	})
}

func TestScanReadsFailsClosedOnUnlistedIndirectReads(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		path := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.com/m\n\ngo 1.27\n")
	write("p/a.go", `package p

import osx "os"

const shared = "SHARED_NAME"

func A(name string) string {
	_ = osx.Getenv("LITERAL_NAME")
	_, _ = osx.LookupEnv(shared)
	return osx.Getenv(name)
}
`)
	write("p/b.go", `package p

import "os"

var getenv = os.Getenv
`)

	reads, problems, err := scanReads(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"LITERAL_NAME", "SHARED_NAME"} {
		if !slices.Contains(reads, want) {
			t.Errorf("reads %q lack %s", reads, want)
		}
	}
	for _, want := range []string{"p/a.go", "p/b.go"} {
		if !slices.ContainsFunc(problems, func(p string) bool { return strings.Contains(p, want) }) {
			t.Errorf("problems %q do not name the unlisted indirect read in %s", problems, want)
		}
	}
}

// variableReads lists the shipped files that read the environment through a
// name the scan cannot resolve, with the names each one reads. Every name is
// checked against the roster and must appear as a string literal in the
// file's package, so the list cannot drift from the code silently.
var variableReads = map[string][]string{
	// runtimeRoots hands each surface's pair of root variables, and the
	// vault override, to rootOrNil.
	"internal/provider/provider.go": {
		"CLAUDE_PLUGIN_ROOT", "CLAUDE_PLUGIN_DATA", "COPILOT_PLUGIN_ROOT", "COPILOT_PLUGIN_DATA",
		"PLUGIN_ROOT", "PLUGIN_DATA", "AGX_VAULT",
	},
	// hostCredentialOverridePaths reads each hostCredentialOverrideNames entry.
	"internal/secretsguard/paths.go": {
		"PGPASSFILE", "KUBECONFIG", "GOOGLE_APPLICATION_CREDENTIALS",
		"AWS_SHARED_CREDENTIALS_FILE", "SOPS_AGE_KEY_FILE",
	},
}

// moduleVariableReads lists the shipped files that read the environment
// through a name another package hands them, with the names each one reads.
// Every name is checked against the roster and must appear as a string
// literal in a shipped file of the module, so renaming or removing the name
// where it is spelled still fails the scan.
var moduleVariableReads = map[string][]string{
	// GuardLog.Dir reads EnvDir, which each guard spells in its own package:
	// commandguard/reporting.go and releaseguard/auditlog.go.
	"internal/hookutil/guardlog.go": {"COMMAND_GUARD_LOG_DIR", "RELEASE_GUARD_LOG_DIR"},
}

// valueReads lists the shipped files that pass os.Getenv or os.LookupEnv on
// as a value, with the names the receiver reads and whether those names
// belong in the guard roster.
var valueReads = map[string]struct {
	names   []string
	inGuard bool
}{
	// The flow subcommand receives os.Getenv. Its names carry workflow state,
	// not guard configuration, so GuardEnv does not hold them.
	"cmd/hook/main.go": {names: []string{"FLOW_SESSION", "CLAUDE_CODE_SESSION_ID", "FLOW_WORK_ROOT", "FLOW_STORAGE"}},
	// Each guard hands os.Getenv to guardmode.Control.Resolve, which reads its
	// kill switch and its mode override. Both are guard configuration.
	"internal/commandguard/commandguard.go": {names: []string{"SKIP_COMMAND_GUARD", "COMMAND_GUARD_MODE"}, inGuard: true},
	"internal/secretsguard/secretsguard.go": {names: []string{"SKIP_SECRETS_GUARD", "SECRETS_GUARD_MODE"}, inGuard: true},
}

// rosterProblems reports every shipped read the roster does not account for.
func rosterProblems(moduleDir string, roster []string) ([]string, error) {
	reads, problems, err := scanReads(moduleDir)
	if err != nil {
		return nil, err
	}
	for _, name := range reads {
		if !slices.Contains(roster, name) {
			problems = append(problems, fmt.Sprintf("a shipped file reads %s, which GuardEnv lacks", name))
		}
	}
	for file, entry := range valueReads {
		for _, name := range entry.names {
			if in := slices.Contains(roster, name); in != entry.inGuard {
				problems = append(problems, fmt.Sprintf("%s: %s in GuardEnv is %t, want %t", file, name, in, entry.inGuard))
			}
		}
	}
	sort.Strings(problems)
	return problems, nil
}

// scanReads parses the non-test files hookprovenance.SourceFiles returns
// under moduleDir, outside cmd/launcher, and returns the sorted names their
// os.Getenv and os.LookupEnv calls read: each literal argument, each
// same-package string constant, and each name variableReads,
// moduleVariableReads, or a guard entry of valueReads lists for a file it
// covers. Any other read is a problem, so a
// new indirect read cannot pass unnoticed.
func scanReads(moduleDir string) (reads, problems []string, err error) {
	files, err := hookprovenance.SourceFiles(moduleDir)
	if err != nil {
		return nil, nil, err
	}
	fset := token.NewFileSet()
	packages := map[string][]*ast.File{}
	names := map[*ast.File]string{}
	for _, rel := range files {
		if !strings.HasSuffix(rel, ".go") || strings.HasPrefix(rel, "cmd/launcher/") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(moduleDir, filepath.FromSlash(rel)), nil, 0)
		if err != nil {
			return nil, nil, err
		}
		packages[path.Dir(rel)] = append(packages[path.Dir(rel)], file)
		names[file] = rel
	}

	moduleLiterals := map[string]bool{}
	for _, pkgFiles := range packages {
		_, literals := packageStrings(pkgFiles)
		for name := range literals {
			moduleLiterals[name] = true
		}
	}

	found := map[string]bool{}
	for _, pkgFiles := range packages {
		constants, literals := packageStrings(pkgFiles)
		for _, file := range pkgFiles {
			rel := names[file]
			for _, name := range variableReads[rel] {
				found[name] = true
				if !literals[name] {
					problems = append(problems, fmt.Sprintf("%s is listed as reading %s, which its package never spells", rel, name))
				}
			}
			for _, name := range moduleVariableReads[rel] {
				found[name] = true
				if !moduleLiterals[name] {
					problems = append(problems, fmt.Sprintf("%s is listed as reading %s, which no shipped file spells", rel, name))
				}
			}
			if entry, ok := valueReads[rel]; ok && entry.inGuard {
				for _, name := range entry.names {
					found[name] = true
				}
			}
			problems = append(problems, scanFile(fset, rel, file, constants, found)...)
		}
	}
	for name := range found {
		reads = append(reads, name)
	}
	sort.Strings(reads)
	return reads, problems, nil
}

// packageStrings returns the package's string constants by name and every
// string literal it spells.
func packageStrings(files []*ast.File) (map[string]string, map[string]bool) {
	constants := map[string]string{}
	literals := map[string]bool{}
	for _, file := range files {
		ast.Inspect(file, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.BasicLit:
				if value, ok := stringLit(n); ok {
					literals[value] = true
				}
			case *ast.GenDecl:
				if n.Tok != token.CONST {
					return true
				}
				for _, spec := range n.Specs {
					spec := spec.(*ast.ValueSpec)
					for i, ident := range spec.Names {
						if i < len(spec.Values) {
							if lit, ok := spec.Values[i].(*ast.BasicLit); ok {
								if value, ok := stringLit(lit); ok {
									constants[ident.Name] = value
								}
							}
						}
					}
				}
			}
			return true
		})
	}
	return constants, literals
}

// scanFile records the names file's environment reads resolve to and returns
// a problem for each read it cannot resolve and its list does not cover.
func scanFile(fset *token.FileSet, rel string, file *ast.File, constants map[string]string, found map[string]bool) []string {
	osName := ""
	for _, spec := range file.Imports {
		if value, _ := strconv.Unquote(spec.Path.Value); value != "os" {
			continue
		}
		osName = "os"
		if spec.Name != nil {
			osName = spec.Name.Name
		}
	}
	if osName == "" {
		return nil
	}
	if osName == "." || osName == "_" {
		return []string{fmt.Sprintf("%s imports os as %q, which the scan cannot follow", rel, osName)}
	}
	isRead := func(expr ast.Expr) bool {
		sel, ok := expr.(*ast.SelectorExpr)
		if !ok {
			return false
		}
		pkg, ok := sel.X.(*ast.Ident)
		return ok && pkg.Name == osName && (sel.Sel.Name == "Getenv" || sel.Sel.Name == "LookupEnv")
	}

	var problems []string
	called := map[ast.Expr]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || !isRead(call.Fun) {
			return true
		}
		called[call.Fun] = true
		at := fmt.Sprintf("%s:%d", rel, fset.Position(call.Pos()).Line)
		if len(call.Args) == 1 {
			if lit, ok := call.Args[0].(*ast.BasicLit); ok {
				if value, ok := stringLit(lit); ok {
					found[value] = true
					return true
				}
			}
			if ident, ok := call.Args[0].(*ast.Ident); ok {
				if value, ok := constants[ident.Name]; ok {
					found[value] = true
					return true
				}
			}
		}
		_, listed := variableReads[rel]
		_, moduleListed := moduleVariableReads[rel]
		if !listed && !moduleListed {
			problems = append(problems, fmt.Sprintf("%s reads the environment through a name the scan cannot resolve; list the names in variableReads", at))
		}
		return true
	})
	ast.Inspect(file, func(n ast.Node) bool {
		expr, ok := n.(ast.Expr)
		if !ok || called[expr] || !isRead(expr) {
			return true
		}
		if _, listed := valueReads[rel]; !listed {
			problems = append(problems, fmt.Sprintf("%s:%d passes an environment reader on as a value; list the names it reads in valueReads", rel, fset.Position(expr.Pos()).Line))
		}
		return true
	})
	return problems
}

func stringLit(lit *ast.BasicLit) (string, bool) {
	if lit.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(lit.Value)
	return value, err == nil
}
