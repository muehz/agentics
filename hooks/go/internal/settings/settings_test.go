package settings

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// fixture builds a home directory and a repository with an optional nested
// working directory, and returns their paths.
type fixture struct {
	home string
	repo string
	cwd  string
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	root := t.TempDir()
	f := fixture{home: filepath.Join(root, "home"), repo: filepath.Join(root, "repo")}
	f.cwd = filepath.Join(f.repo, "pkg", "inner")
	for _, dir := range []string{f.home, filepath.Join(f.repo, ".git"), f.cwd} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

func write(t *testing.T, root, body string) string {
	t.Helper()
	dir := filepath.Join(root, dirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, fileName)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func wantOnly(t *testing.T, res Result, off ...Key) {
	t.Helper()
	for _, k := range Keys {
		want := !slices.Contains(off, k)
		if got := res.Policy.Enabled(k); got != want {
			t.Errorf("%s = %v, want %v", k, got, want)
		}
	}
}

func TestMissingFilesApplyEveryDefault(t *testing.T) {
	f := newFixture(t)
	res := ResolveIn(f.cwd, f.home)
	if len(res.Invalid) != 0 {
		t.Fatalf("missing files reported invalid: %+v", res.Invalid)
	}
	wantOnly(t, res)
}

func TestZeroPolicyEnablesEveryKey(t *testing.T) {
	var p Policy
	for _, k := range Keys {
		if !p.Enabled(k) {
			t.Fatalf("zero Policy disabled %s", k)
		}
	}
}

func TestEachKeyCanBeTurnedOff(t *testing.T) {
	for _, k := range Keys {
		t.Run(string(k), func(t *testing.T) {
			f := newFixture(t)
			name := strings.TrimPrefix(string(k), "policy.")
			write(t, f.repo, `{"version":1,"policy":{"`+name+`":false}}`)
			wantOnly(t, ResolveIn(f.cwd, f.home), k)
		})
	}
}

func TestMinimalAndEmptyPolicyObjectsAreValid(t *testing.T) {
	for _, body := range []string{`{"version":1}`, `{"version":1,"policy":{}}`, " \n{\"version\":1}\n"} {
		f := newFixture(t)
		write(t, f.repo, body)
		res := ResolveIn(f.cwd, f.home)
		if len(res.Invalid) != 0 {
			t.Fatalf("%q reported invalid: %+v", body, res.Invalid)
		}
		wantOnly(t, res)
	}
}

func TestPrecedenceAndInheritance(t *testing.T) {
	f := newFixture(t)
	write(t, f.home, `{"version":1,"policy":{"draft_prs":false,"block_package_publish":false}}`)
	write(t, f.repo, `{"version":1,"policy":{"draft_prs":true}}`)
	res := ResolveIn(f.cwd, f.home)
	wantOnly(t, res, BlockPackagePublish)

	write(t, f.repo, `{"version":1,"policy":{"block_package_publish":true,"command_reformulation":false}}`)
	wantOnly(t, ResolveIn(f.cwd, f.home), DraftPRs, CommandReformulation)
}

func TestInvalidFileClasses(t *testing.T) {
	cases := []struct{ name, body, class string }{
		{"empty", ``, ClassNotJSON},
		{"blank", " \n", ClassNotJSON},
		{"truncated", `{"version":1`, ClassNotJSON},
		{"comment", `{"version":1 /* x */}`, ClassNotJSON},
		{"bom", "\xEF\xBB\xBF{\"version\":1}", ClassNotJSON},
		{"invalid utf8", "{\"version\":1}\xff", ClassNotJSON},
		{"array root", `[1]`, ClassBadType},
		{"string root", `"x"`, ClassBadType},
		{"trailing value", `{"version":1}{}`, ClassTrailingData},
		{"trailing text", `{"version":1} nope`, ClassTrailingData},
		{"duplicate top", `{"version":1,"version":1}`, ClassDuplicateKey},
		{"duplicate policy", `{"version":1,"policy":{},"policy":{}}`, ClassDuplicateKey},
		{"duplicate leaf", `{"version":1,"policy":{"draft_prs":false,"draft_prs":true}}`, ClassDuplicateKey},
		{"unknown top", `{"version":1,"extra":true}`, ClassUnknownKey},
		{"unknown leaf", `{"version":1,"policy":{"merge":false}}`, ClassUnknownKey},
		{"safety leaf", `{"version":1,"policy":{"destructive_sql":false}}`, ClassUnknownKey},
		{"flow section", `{"version":1,"flow":{"stop":true}}`, ClassUnknownKey},
		{"missing version", `{"policy":{}}`, ClassBadVersion},
		{"version two", `{"version":2}`, ClassBadVersion},
		{"version float", `{"version":1.0}`, ClassBadVersion},
		{"version string", `{"version":"1"}`, ClassBadType},
		{"null policy", `{"version":1,"policy":null}`, ClassBadType},
		{"array policy", `{"version":1,"policy":[]}`, ClassBadType},
		{"null leaf", `{"version":1,"policy":{"draft_prs":null}}`, ClassBadType},
		{"string leaf", `{"version":1,"policy":{"draft_prs":"false"}}`, ClassBadType},
		{"number leaf", `{"version":1,"policy":{"draft_prs":0}}`, ClassBadType},
		{"object leaf", `{"version":1,"policy":{"draft_prs":{}}}`, ClassBadType},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFixture(t)
			path := write(t, f.repo, c.body)
			res := ResolveIn(f.cwd, f.home)
			if len(res.Invalid) != 1 || res.Invalid[0].Class != c.class || res.Invalid[0].Path != path {
				t.Fatalf("Invalid = %+v, want one %s for %s", res.Invalid, c.class, path)
			}
			wantOnly(t, res)
		})
	}
}

func TestInvalidFileDiscardsEveryOverrideInBothLayers(t *testing.T) {
	f := newFixture(t)
	write(t, f.home, `{"version":1,"policy":{"draft_prs":false}}`)
	write(t, f.repo, `{"version":1,"policy":{"unknown":true}}`)
	res := ResolveIn(f.cwd, f.home)
	wantOnly(t, res)

	f = newFixture(t)
	write(t, f.home, `{"version":1,"oops":1}`)
	write(t, f.repo, `{"version":1,"policy":{"draft_prs":false}}`)
	res = ResolveIn(f.cwd, f.home)
	wantOnly(t, res)
	if len(res.Invalid) != 1 || res.Invalid[0].Path != res.UserPath {
		t.Fatalf("Invalid = %+v, want the user file only", res.Invalid)
	}
}

func TestBothInvalidAreBothReported(t *testing.T) {
	f := newFixture(t)
	write(t, f.home, `{`)
	write(t, f.repo, `{"version":2}`)
	if got := len(ResolveIn(f.cwd, f.home).Invalid); got != 2 {
		t.Fatalf("reported %d invalid files, want 2", got)
	}
}

func TestUnreadableShapesAreInvalid(t *testing.T) {
	f := newFixture(t)
	path := filepath.Join(f.repo, dirName, fileName)
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	res := ResolveIn(f.cwd, f.home)
	if len(res.Invalid) != 1 || res.Invalid[0].Class != ClassNotAFile {
		t.Fatalf("a directory in place of the file: Invalid = %+v", res.Invalid)
	}

	f = newFixture(t)
	big := `{"version":1,"policy":{}}` + strings.Repeat(" ", maxFileBytes)
	write(t, f.repo, big)
	res = ResolveIn(f.cwd, f.home)
	if len(res.Invalid) != 1 || res.Invalid[0].Class != ClassTooLarge {
		t.Fatalf("an oversized file: Invalid = %+v", res.Invalid)
	}

	f = newFixture(t)
	exact := `{"version":1}` + strings.Repeat(" ", maxFileBytes-len(`{"version":1}`))
	write(t, f.repo, exact)
	if res = ResolveIn(f.cwd, f.home); len(res.Invalid) != 0 {
		t.Fatalf("a file of exactly the limit was rejected: %+v", res.Invalid)
	}
}

func TestDanglingSymlinkIsInvalidNotAbsent(t *testing.T) {
	f := newFixture(t)
	write(t, f.home, `{"version":1,"policy":{"draft_prs":false}}`)
	dir := filepath.Join(f.repo, dirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, fileName)
	if err := os.Symlink(filepath.Join(f.repo, "missing.json"), path); err != nil {
		t.Skipf("symlinks are unavailable here: %v", err)
	}
	res := ResolveIn(f.cwd, f.home)
	if len(res.Invalid) != 1 || res.Invalid[0].Class != ClassUnreadable || res.Invalid[0].Path != path {
		t.Fatalf("Invalid = %+v, want one %s for %s", res.Invalid, ClassUnreadable, path)
	}
	wantOnly(t, res)
}

func TestDiscovery(t *testing.T) {
	t.Run("nearest git marker is the root", func(t *testing.T) {
		f := newFixture(t)
		nested := filepath.Join(f.repo, "sub")
		if err := os.MkdirAll(filepath.Join(nested, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
		write(t, f.repo, `{"version":1,"policy":{"draft_prs":false}}`)
		res := ResolveIn(nested, f.home)
		wantOnly(t, res)
		if want := filepath.Join(nested, dirName, fileName); res.RepoPath != want {
			t.Fatalf("RepoPath = %q, want %q", res.RepoPath, want)
		}
	})
	t.Run("a linked worktree uses its own file", func(t *testing.T) {
		f := newFixture(t)
		linked := filepath.Join(filepath.Dir(f.repo), "linked")
		if err := os.MkdirAll(linked, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(linked, ".git"), []byte("gitdir: elsewhere\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		write(t, f.repo, `{"version":1,"policy":{"draft_prs":false}}`)
		write(t, linked, `{"version":1,"policy":{"block_package_publish":false}}`)
		wantOnly(t, ResolveIn(linked, f.home), BlockPackagePublish)
		wantOnly(t, ResolveIn(f.cwd, f.home), DraftPRs)
	})
	t.Run("outside a repository only the user file applies", func(t *testing.T) {
		root := t.TempDir()
		home := filepath.Join(root, "home")
		plain := filepath.Join(root, "plain")
		for _, d := range []string{home, plain} {
			if err := os.MkdirAll(d, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		write(t, plain, `{"version":1,"policy":{"draft_prs":false}}`)
		write(t, home, `{"version":1,"policy":{"command_reformulation":false}}`)
		res := ResolveIn(plain, home)
		wantOnly(t, res, CommandReformulation)
		if res.RepoPath != "" {
			t.Fatalf("RepoPath = %q, want empty", res.RepoPath)
		}
	})
	t.Run("a relative cwd has no repository file", func(t *testing.T) {
		f := newFixture(t)
		write(t, f.repo, `{"version":1,"policy":{"draft_prs":false}}`)
		res := ResolveIn("pkg/inner", f.home)
		wantOnly(t, res)
		if res.RepoPath != "" {
			t.Fatalf("RepoPath = %q, want empty", res.RepoPath)
		}
	})
	t.Run("an empty cwd uses the process working directory", func(t *testing.T) {
		f := newFixture(t)
		write(t, f.repo, `{"version":1,"policy":{"draft_prs":false}}`)
		t.Chdir(f.cwd)
		wantOnly(t, ResolveIn("", f.home), DraftPRs)
	})
	t.Run("no home directory means no user file", func(t *testing.T) {
		f := newFixture(t)
		res := ResolveIn(f.cwd, "")
		if res.UserPath != "" || len(res.Invalid) != 0 {
			t.Fatalf("UserPath = %q, Invalid = %+v", res.UserPath, res.Invalid)
		}
	})
}

func TestHomeThatIsTheRepositoryRootReadsTheFileOnce(t *testing.T) {
	t.Run("valid file applies once", func(t *testing.T) {
		f := newFixture(t)
		if err := os.MkdirAll(filepath.Join(f.home, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
		write(t, f.home, `{"version":1,"policy":{"draft_prs":false}}`)
		res := ResolveIn(f.home, f.home)
		if res.RepoPath != res.UserPath || res.UserPath == "" {
			t.Fatalf("RepoPath = %q, UserPath = %q, want the same non-empty path", res.RepoPath, res.UserPath)
		}
		if len(res.Invalid) != 0 {
			t.Fatalf("valid file reported invalid: %+v", res.Invalid)
		}
		wantOnly(t, res, DraftPRs)
	})
	t.Run("invalid file is reported once", func(t *testing.T) {
		f := newFixture(t)
		if err := os.MkdirAll(filepath.Join(f.home, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
		path := write(t, f.home, `{"version":1,"oops":1}`)
		res := ResolveIn(f.home, f.home)
		if len(res.Invalid) != 1 || res.Invalid[0].Path != path {
			t.Fatalf("Invalid = %+v, want one entry for %s", res.Invalid, path)
		}
		if got := strings.Count(Note(res, []Key{DraftPRs}), "ignored invalid"); got != 1 {
			t.Fatalf("Note names the invalid file %d times, want 1", got)
		}
	})
}

func TestRepairTakesEffectOnTheNextCall(t *testing.T) {
	f := newFixture(t)
	write(t, f.repo, `{"version":1,`)
	if len(ResolveIn(f.cwd, f.home).Invalid) != 1 {
		t.Fatal("broken file was not reported")
	}
	write(t, f.repo, `{"version":1,"policy":{"draft_prs":false}}`)
	res := ResolveIn(f.cwd, f.home)
	if len(res.Invalid) != 0 {
		t.Fatalf("repaired file still reported: %+v", res.Invalid)
	}
	wantOnly(t, res, DraftPRs)
}

func TestResolveUsesTheHomeDirectoryVariable(t *testing.T) {
	f := newFixture(t)
	write(t, f.home, `{"version":1,"policy":{"draft_prs":false}}`)
	t.Setenv("HOME", f.home)
	t.Setenv("USERPROFILE", f.home)
	wantOnly(t, Resolve(f.cwd), DraftPRs)
}

func TestNote(t *testing.T) {
	res := Result{Policy: Defaults(), RepoPath: "/r/.agentics/hooks.json"}
	got := Note(res, []Key{CommandReformulation, DraftPRs})
	want := "Settings: policy.draft_prs, policy.command_reformulation. To allow this, ask the user to set each to false in /r/.agentics/hooks.json or ~/.agentics/hooks.json; an agent must not edit those files."
	if got != want {
		t.Fatalf("Note =\n%q\nwant\n%q", got, want)
	}
	single := Note(Result{}, []Key{DraftPRs})
	if !strings.HasPrefix(single, "Setting: policy.draft_prs.") || !strings.Contains(single, "ask the user to set it to false in .agentics/hooks.json in the repository root or ~/.agentics/hooks.json") {
		t.Fatalf("single-key note = %q", single)
	}
	if Note(res, nil) != "" {
		t.Fatal("a note with no keys must be empty")
	}
	bad := Note(Result{Invalid: []Invalid{{Path: "/h/.agentics/hooks.json", Class: ClassUnknownKey}}}, []Key{DraftPRs})
	if !strings.HasSuffix(bad, "\nAgentics hook settings: ignored invalid /h/.agentics/hooks.json (unknown-key); every policy default applies.") {
		t.Fatalf("invalid line missing: %q", bad)
	}
}

func TestNoteNeverQuotesFileContents(t *testing.T) {
	f := newFixture(t)
	write(t, f.repo, `{"version":1,"policy":{"SECRET_MARKER":true}}`)
	res := ResolveIn(f.cwd, f.home)
	if note := Note(res, []Key{DraftPRs}); strings.Contains(note, "SECRET_MARKER") {
		t.Fatalf("note quoted file contents: %q", note)
	}
}
