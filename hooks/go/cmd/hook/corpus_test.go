package main

// The behavior corpus: the binary's observable contract, recorded.
//
// testdata/corpus/entries.json is a hand-authored table. Each entry names a
// hook, its argv, and what each surface receives on stdin; it may set
// environment variables and describe a fixture directory the hook runs against.
// Every entry runs through run() — dispatch, normalize, handler, validate,
// render — on every surface it applies to, and (exit, stdout, stderr) is
// compared byte for byte with testdata/corpus/recorded/<surface>.json.
//
// A second recorded set runs the same entries as the grouped pre-tool event,
// minus the few that test a single check's own argv, and compares them with
// testdata/corpus/recorded/<surface>.pre-tool.json.
//
// The recorded files are written only by
//
//	go test ./cmd/hook -run 'TestBehaviorCorpus$' -update
//	go test ./cmd/hook -run '^TestBehaviorCorpusGrouped$' -update
//
// (the first command writes the single-check set, the second the grouped
// set) and never by hand. A deliberate behavior change is therefore a
// reviewed diff in a recorded file, next to the code change that caused it.
//
// Two tokens are expanded at run time so the table stays portable and
// writable: {{FIXTURE}} is the entry's fixture directory (JSON-escaped
// inside stdin, raw in env, and normalized back to the token in outputs),
// and {{AWS_KEY}} is an AWS-key-shaped string — a literal one in a committed
// file would be blocked by the secrets guardian this corpus exercises.
//
// stdin forms, per surface or shared:
//
//	"@pre-tool-use.json"  a file under hooks/go/surfaces/<surface>/payloads/
//	"$post"               a named template from the table's "templates" map
//	{...}                 a JSON object, sent compact
//	""                    a literal string (here: empty stdin)

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hosttest"
	"github.com/muehz/agentics/hooks/go/internal/surface"
)

var update = flag.Bool("update", false, "rewrite testdata/corpus/recorded/*.json from the current implementation")

const (
	corpusEntriesPath = "testdata/corpus/entries.json"
	corpusRecordedDir = "testdata/corpus/recorded"
	corpusPayloadsDir = "../../surfaces" // + /<surface>/payloads/<file>
	fixtureToken      = "{{FIXTURE}}"
	awsKeyToken       = "{{AWS_KEY}}"
)

// corpusSurfaces is every surface; each has its own recorded file.
var corpusSurfaces = surface.All()

type corpusTable struct {
	Templates map[string]map[string]json.RawMessage `json:"templates"`
	Entries   []corpusEntry                         `json:"entries"`
}

type corpusEntry struct {
	Name    string            `json:"name"`
	Hook    string            `json:"hook"`
	Argv    []string          `json:"argv"`
	Stdin   json.RawMessage   `json:"stdin"`
	Env     map[string]string `json:"env,omitempty"`
	Fixture *corpusFixture    `json:"fixture,omitempty"`
	Only    []string          `json:"only,omitempty"`
}

// corpusFixture describes a directory the entry's cwd points at.
//
//	kind: "repo"    git init + one baseline commit of fixture.txt
//	      "unborn"  git init, no commits
//	      "plain"   a directory that is not a repository
//	      "missing" a path that is not created
//	corrupt_index overwrites .git/index after staging
type corpusFixture struct {
	Kind             string            `json:"kind"`
	Files            map[string]string `json:"files,omitempty"`
	Stage            []string          `json:"stage,omitempty"`
	DropStagedObject string            `json:"drop_staged_object,omitempty"`
	// CorruptIndex overwrites .git/index after staging, so every git
	// enumeration a hook runs fails — the portable trigger for an
	// enumeration-failure outcome.
	CorruptIndex bool `json:"corrupt_index,omitempty"`
}

type corpusResult struct {
	Exit   int    `json:"exit"`
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
}

func (e corpusEntry) appliesTo(surface surface.ID) bool {
	if len(e.Only) == 0 {
		return true
	}
	for _, s := range e.Only {
		if s == string(surface) {
			return true
		}
	}
	return false
}

func TestBehaviorCorpus(t *testing.T) {
	table := loadCorpusTable(t)
	for _, surface := range corpusSurfaces {
		t.Run(string(surface), func(t *testing.T) {
			recordedPath := filepath.Join(corpusRecordedDir, string(surface)+".json")
			var recorded map[string]corpusResult
			if !*update {
				recorded = readRecorded(t, recordedPath)
			}
			got := map[string]corpusResult{}
			for _, entry := range table.Entries {
				if !entry.appliesTo(surface) {
					continue
				}
				t.Run(entry.Name, func(t *testing.T) {
					result := runCorpusEntry(t, table, surface, entry)
					got[entry.Name] = result
					if *update {
						return
					}
					want, ok := recorded[entry.Name]
					if !ok {
						t.Fatalf("no recorded output for %s/%s — run: go test ./cmd/hook -run 'TestBehaviorCorpus$' -update", surface, entry.Name)
					}
					if result != want {
						t.Errorf("%s/%s diverged from the recorded output\n got: exit=%d\n      stdout=%q\n      stderr=%q\nwant: exit=%d\n      stdout=%q\n      stderr=%q",
							surface, entry.Name, result.Exit, result.Stdout, result.Stderr, want.Exit, want.Stdout, want.Stderr)
					}
				})
			}
			if *update {
				writeRecorded(t, recordedPath, got)
				return
			}
			for name := range recorded {
				if _, live := got[name]; !live {
					t.Errorf("recorded %s/%s has no entry in %s — stale; run -update", surface, name, corpusEntriesPath)
				}
			}
		})
	}
}

func loadCorpusTable(t testing.TB) corpusTable {
	t.Helper()
	raw, err := os.ReadFile(corpusEntriesPath)
	if err != nil {
		t.Fatalf("reading %s: %v", corpusEntriesPath, err)
	}
	var table corpusTable
	if err := json.Unmarshal(raw, &table); err != nil {
		t.Fatalf("parsing %s: %v", corpusEntriesPath, err)
	}
	seen := map[string]bool{}
	for _, e := range table.Entries {
		if e.Name == "" || e.Hook == "" {
			t.Fatalf("%s: every entry needs a name and a hook (got %+v)", corpusEntriesPath, e)
		}
		if seen[e.Name] {
			t.Fatalf("%s: duplicate entry name %q", corpusEntriesPath, e.Name)
		}
		seen[e.Name] = true
		if e.Argv == nil {
			t.Fatalf("%s: entry %q has no argv (use [] for none)", corpusEntriesPath, e.Name)
		}
		for _, surface := range corpusSurfaces {
			if e.appliesTo(surface) {
				resolveStdin(t, table, surface, e, "") // fails loudly on a missing surface or fixture
			}
		}
	}
	return table
}

func readRecorded(t *testing.T, path string) map[string]corpusResult {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("no recorded outputs at %s — run: go test ./cmd/hook -run 'TestBehaviorCorpus$' -update", path)
	}
	var recorded map[string]corpusResult
	if err := json.Unmarshal(raw, &recorded); err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	return recorded
}

func writeRecorded(t *testing.T, path string, results map[string]corpusResult) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("creating %s: %v", filepath.Dir(path), err)
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(results); err != nil {
		t.Fatalf("encoding %s: %v", path, err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

// runCorpusEntry executes one entry on one surface in a scrubbed
// environment and returns its normalized result.
func runCorpusEntry(t *testing.T, table corpusTable, surface surface.ID, e corpusEntry) corpusResult {
	t.Helper()
	argv, stdin, fixture := prepareCorpusEntry(t, table, surface, e)

	var stdout, stderr bytes.Buffer
	code := run(argv, strings.NewReader(stdin), &stdout, &stderr)
	return corpusResult{
		Exit:   code,
		Stdout: normalizeOutput(stdout.String(), fixture),
		Stderr: normalizeOutput(stderr.String(), fixture),
	}
}

// prepareCorpusEntry scrubs the environment, applies the entry's own env,
// builds its fixture, and resolves its stdin — everything run() needs except
// the call itself. The latency baseline reuses it so a benchmark measures the
// same inputs the corpus records.
func prepareCorpusEntry(t testing.TB, table corpusTable, surface surface.ID, e corpusEntry) (argv []string, stdin, fixture string) {
	t.Helper()
	hosttest.ScrubGuardEnv(t)
	t.Setenv("LC_ALL", "C")
	t.Setenv("LANG", "C")
	scratch := t.TempDir()
	t.Setenv("COMMAND_GUARD_LOG_DIR", filepath.Join(scratch, "logs"))
	t.Setenv("RELEASE_GUARD_LOG_DIR", filepath.Join(scratch, "logs"))
	// No git config outside the fixture may influence a scan.
	emptyConfig := filepath.Join(scratch, "gitconfig")
	if err := os.WriteFile(emptyConfig, nil, 0o644); err != nil {
		t.Fatalf("writing %s: %v", emptyConfig, err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", emptyConfig)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")

	if e.Fixture != nil {
		fixture = buildFixture(t, scratch, *e.Fixture)
	}
	for key, value := range e.Env {
		t.Setenv(key, expandRaw(value, fixture))
	}
	stdin = resolveStdin(t, table, surface, e, fixture)
	argv = append([]string{string(surface), e.Hook}, e.Argv...)
	return argv, stdin, fixture
}

// resolveStdin turns an entry's stdin field into the bytes run() reads.
// With fixture == "" it only validates (used at load time).
func resolveStdin(t testing.TB, table corpusTable, surface surface.ID, e corpusEntry, fixture string) string {
	t.Helper()
	raw := e.Stdin
	// A per-surface map: {"claude-code": ..., "copilot": ..., "codex": ...}
	if isSurfaceMap(raw) {
		var bySurface map[string]json.RawMessage
		if err := json.Unmarshal(raw, &bySurface); err != nil {
			t.Fatalf("entry %q: stdin map: %v", e.Name, err)
		}
		v, ok := bySurface[string(surface)]
		if !ok {
			t.Fatalf("entry %q applies to %s but its stdin names no %s form", e.Name, surface, surface)
		}
		raw = v
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		switch {
		case strings.HasPrefix(s, "@"):
			path := filepath.Join(corpusPayloadsDir, string(surface), "payloads", s[1:])
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("entry %q: fixture %s: %v", e.Name, path, err)
			}
			return expandJSON(string(data), fixture)
		case strings.HasPrefix(s, "$"):
			tmpl, ok := table.Templates[s[1:]]
			if !ok {
				t.Fatalf("entry %q: no template %q", e.Name, s[1:])
			}
			v, ok := tmpl[string(surface)]
			if !ok {
				t.Fatalf("entry %q: template %q has no %s form", e.Name, s[1:], surface)
			}
			return expandJSON(compactJSON(t, e.Name, v), fixture)
		default:
			return expandJSON(s, fixture)
		}
	}
	return expandJSON(compactJSON(t, e.Name, raw), fixture)
}

// isSurfaceMap reports whether raw is an object whose keys are all surfaces —
// the per-surface form — as opposed to a payload object sent to every surface.
func isSurfaceMap(raw json.RawMessage) bool {
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil || len(m) == 0 {
		return false
	}
	for k := range m {
		known := false
		for _, s := range corpusSurfaces {
			if k == string(s) {
				known = true
			}
		}
		if !known {
			return false
		}
	}
	return true
}

func compactJSON(t testing.TB, name string, raw json.RawMessage) string {
	t.Helper()
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		t.Fatalf("entry %q: stdin is not valid JSON: %v", name, err)
	}
	return buf.String()
}

func awsKey() string { return "AKIA" + strings.Repeat("0", 16) }

// expandRaw substitutes tokens for a value used outside JSON (env, files).
func expandRaw(s, fixture string) string {
	s = strings.ReplaceAll(s, awsKeyToken, awsKey())
	return strings.ReplaceAll(s, fixtureToken, fixture)
}

// expandJSON substitutes tokens inside a JSON document: the fixture path is
// JSON-escaped so a Windows path survives as a string value.
func expandJSON(s, fixture string) string {
	s = strings.ReplaceAll(s, awsKeyToken, awsKey())
	if fixture == "" {
		return s
	}
	escaped, _ := json.Marshal(fixture)
	return strings.ReplaceAll(s, fixtureToken, strings.Trim(string(escaped), `"`))
}

// normalizeOutput replaces every spelling of the fixture path with the
// token, so a recording is identical on every OS.
func normalizeOutput(s, fixture string) string {
	if fixture == "" {
		return s
	}
	escaped, _ := json.Marshal(fixture)
	for _, form := range []string{strings.Trim(string(escaped), `"`), fixture, filepath.ToSlash(fixture)} {
		if form != "" {
			s = strings.ReplaceAll(s, form, fixtureToken)
		}
	}
	return s
}

// buildFixture materializes one fixture manifest under scratch and returns
// its path. Files are written after the baseline commit, so in a "repo"
// fixture they are the working-tree changes against that commit.
func buildFixture(t testing.TB, scratch string, f corpusFixture) string {
	t.Helper()
	dir := filepath.Join(scratch, "fixture")
	switch f.Kind {
	case "missing":
		return dir
	case "plain":
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("creating %s: %v", dir, err)
		}
	case "unborn", "repo":
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("creating %s: %v", dir, err)
		}
		gitIn(t, dir, "init", "-q")
		gitIn(t, dir, "config", "user.email", "corpus@example.invalid")
		gitIn(t, dir, "config", "user.name", "Agentics Corpus")
		gitIn(t, dir, "config", "core.autocrlf", "false")
		if f.Kind == "repo" {
			writeFixtureFile(t, dir, "fixture.txt", "safe baseline\n")
			gitIn(t, dir, "add", "fixture.txt")
			gitIn(t, dir, "commit", "-qm", "baseline")
		}
	default:
		t.Fatalf("fixture kind %q is not one of repo, unborn, plain, missing", f.Kind)
	}
	for name, content := range f.Files {
		writeFixtureFile(t, dir, name, expandRaw(content, dir))
	}
	for _, name := range f.Stage {
		gitIn(t, dir, "add", "--", name)
	}
	if f.DropStagedObject != "" {
		id := strings.TrimSpace(gitIn(t, dir, "rev-parse", ":0:"+f.DropStagedObject))
		object := filepath.Join(dir, ".git", "objects", id[:2], id[2:])
		if err := os.Remove(object); err != nil {
			t.Fatalf("dropping staged object %s: %v", object, err)
		}
	}
	if f.CorruptIndex {
		index := filepath.Join(dir, ".git", "index")
		if err := os.WriteFile(index, []byte("not a git index"), 0o644); err != nil {
			t.Fatalf("corrupting %s: %v", index, err)
		}
	}
	return dir
}

func writeFixtureFile(t testing.TB, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("creating %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

// gitIn runs git in dir and fails the test on a non-zero exit.
func gitIn(t testing.TB, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return string(out)
}
