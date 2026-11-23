package flow

import (
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestPlanScopeTasks(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		files  map[string]string
		ok     bool
		tasks  []planTask
		slices map[string][]string
	}{
		{
			name: "role-labelled line with a line range and a parenthetical",
			files: map[string]string{"plan.md": "# Plan\n\n### Task 1: One\n\n**Files:**\n\n" +
				"- Modify: `hooks/go/a.go:12-40` (the parser)\n- Test: `hooks/go/a_test.go:7`\n\n**Acceptance Criteria:**\n\n- [ ] `not/a/file.go`\n"},
			ok:    true,
			tasks: []planTask{{id: "1", paths: []string{"hooks/go/a.go", "hooks/go/a_test.go"}}},
		},
		{
			name: "multi-span line and indented continuation line",
			files: map[string]string{"plan.md": "### Task 2: Two\n\n**Files:**\n\n" +
				"- Modify: `a.ts`, `b.ts` and `c.ts`\n  plus `d.ts` on a continuation\n- Create: `e.ts`\n"},
			ok:    true,
			tasks: []planTask{{id: "2", paths: []string{"a.ts", "b.ts", "c.ts", "d.ts", "e.ts"}}},
		},
		{
			name: "files list on the heading line wrapping onto an unindented line",
			files: map[string]string{"plan.md": "### Task 5: Five\n\n**ID:** task-5 **Dependencies:** none\n\n" +
				"**Files:** Modify `x/one.ts`, `x/two.ts`,\n`x/three.ts`. Reference `y/ref.go`.\n\n**Acceptance Criteria:**\n\n- [ ] `outside.ts`\n"},
			ok:    true,
			tasks: []planTask{{id: "task-5", paths: []string{"x/one.ts", "x/two.ts", "x/three.ts", "y/ref.go"}}},
		},
		{
			name: "move line, bare range span, none, directory, glob and brace set",
			files: map[string]string{"plan.md": "### Task s1-t01 Move things\n\n**Files:**\n\n" +
				"- Move: `old/a.go` -> `new/a.go`\n- Modify: `b.go` `:65-89`\n- Modify: none.\n" +
				"- Create: `dir/`, `src/*.ts`, `src/{a,b}.ts`, `  spaced.go  `\n"},
			ok:    true,
			tasks: []planTask{{id: "s1-t01", paths: []string{"old/a.go", "new/a.go", "b.go", "dir/", "src/*.ts", "src/{a,b}.ts", "spaced.go"}}},
		},
		{
			name:  "a task with no files item has no paths",
			files: map[string]string{"plan.md": "### Task 3: Three\n\nNo files here, `not/a/path.go`.\n"},
			ok:    true,
			tasks: []planTask{{id: "3"}},
		},
		{
			name:  "the next heading ends the files block",
			files: map[string]string{"plan.md": "### Task 1: One\n\n**Files:** `a.go`\n### Task 2: Two\n\n**Files:**\n- Modify: `b.go`\n#### Steps\n`c.go`\n"},
			ok:    true,
			tasks: []planTask{
				{id: "1", paths: []string{"a.go"}},
				{id: "2", paths: []string{"b.go"}},
			},
		},
		{
			name: "a fenced task heading and files line are ignored",
			files: map[string]string{"plan.md": "### Task 1: Real\n\n**Files:**\n\n- Modify: `real.go`\n\n" +
				"```md\n### Task 9: Quoted\n\n**Files:**\n\n- Modify: `quoted.go`\n```\n"},
			ok:    true,
			tasks: []planTask{{id: "1", paths: []string{"real.go"}}},
		},
		{
			name: "slices, tasks before the first slice and a resetting heading",
			files: map[string]string{"plan.md": "# Plan\n\n## Tasks\n\n### Task 0: Before\n\n" +
				"## S1\n\n### Task s1-t01: A\n\n### Task s1-t02: B\n\n" +
				"## S2: Second\n\n### Task s2-t01: C\n\n" +
				"## Standing orders\n\n### Task 7: After\n"},
			ok: true,
			tasks: []planTask{
				{id: "0"},
				{id: "s1-t01", slice: "S1"},
				{id: "s1-t02", slice: "S1"},
				{id: "s2-t01", slice: "S2"},
				{id: "7"},
			},
			slices: map[string][]string{"S1": {"s1-t01", "s1-t02"}, "S2": {"s2-t01"}},
		},
		{
			name:  "the brief task with a files item",
			files: map[string]string{"brief.md": "# Brief\n\n- **Files:** not here `x.go`\n\n## Task\n\nDo it.\n\n- **Files:** `a.go`, `b/`\n  and `c.go`\n\n## Decisions\n\n- `d.go`\n"},
			ok:    true,
			tasks: []planTask{{id: "brief", paths: []string{"a.go", "b/", "c.go"}}},
		},
		{
			name:  "the brief task without a files item",
			files: map[string]string{"brief.md": "# Brief\n\n## Task\n\nSplit `validate.yml`.\n"},
			ok:    true,
			tasks: []planTask{{id: "brief"}},
		},
		{
			name: "a plan beside a brief ignores the brief",
			files: map[string]string{
				"plan.md":  "### Task 1: One\n\n**Files:** `p.go`\n",
				"brief.md": "## Task\n\n- **Files:** `b.go`\n",
			},
			ok:    true,
			tasks: []planTask{{id: "1", paths: []string{"p.go"}}},
		},
		{
			name: "a plan with no task heading is not ok, even beside a brief",
			files: map[string]string{
				"plan.md":  "# Plan\n\n```\n### Task 1: Fenced\n```\n",
				"brief.md": "## Task\n\n- **Files:** `b.go`\n",
			},
		},
		{
			name:  "no plan and no brief",
			files: map[string]string{"spec.md": "# Spec\n"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			for name, body := range tc.files {
				writeFile(t, dir, name, body)
			}
			scope, ok := readPlanScope(dir, func(string, string) bool { return true })
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if !ok {
				return
			}
			if !reflect.DeepEqual(scope.tasks, tc.tasks) {
				t.Errorf("tasks\n got %+v\nwant %+v", scope.tasks, tc.tasks)
			}
			want := tc.slices
			if want == nil {
				want = map[string][]string{}
			}
			if !maps.EqualFunc(scope.slices, want, slices.Equal) {
				t.Errorf("slices %v, want %v", scope.slices, want)
			}
		})
	}
}

func TestPlanScopeDivergences(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFile(t, dir, "plan.md", "### Task 1: One\n\n**Files:** `p.go`\n\n## Divergences\n\n"+
		"- recorded open `plan/open.go:3-9` and `plan/two/`\n"+
		"- unrecorded `plan/unrecorded.go`\n"+
		"- recorded reconciled `plan/done.go`. Reconciled: 2026-11-12.\n")
	writeFile(t, dir, "spec.md", "## Divergences\n\n- spec `spec/a.go` `:4`\n")
	writeFile(t, dir, "intent.md", "## Divergences\n\n- intent `intent/a.go`\n")
	writeFile(t, dir, "brief.md", "## Divergences\n\n- brief `brief/a.go`\n")
	writeFile(t, dir, "notes.md", "## Divergences\n\n- notes `notes/a.go`\n")
	recordedSet := map[string]bool{
		"plan.md " + sha256Hex("recorded open `plan/open.go:3-9` and `plan/two/`\n"): true,
		"plan.md " + sha256Hex("recorded reconciled `plan/done.go`.\n"):              true,
		"spec.md " + sha256Hex("spec `spec/a.go` `:4`\n"):                            true,
		"brief.md " + sha256Hex("brief `brief/a.go`\n"):                              true,
		// The same entry text under another artifact does not record this one.
		"plan.md " + sha256Hex("intent `intent/a.go`\n"): true,
		"notes.md " + sha256Hex("notes `notes/a.go`\n"):  true,
	}
	scope, ok := readPlanScope(dir, func(artifact, hash string) bool { return recordedSet[artifact+" "+hash] })
	if !ok {
		t.Fatal("ok = false")
	}
	got := slices.Clone(scope.divergencePaths)
	slices.Sort(got)
	want := []string{"brief/a.go", "plan/done.go", "plan/open.go", "plan/two/", "spec/a.go"}
	if !slices.Equal(got, want) {
		t.Errorf("divergence paths %v, want %v", got, want)
	}
}

func TestPlanScopeBacktickSpans(t *testing.T) {
	t.Parallel()
	cases := []struct {
		line string
		want []string
	}{
		{"`a` and `b`", []string{"a", "b"}},
		{"``a`b`` then `c`", []string{"a`b", "c"}},
		{"unclosed `a and ``b", nil},
		{"`a and ``b`", []string{"a and ``b"}},
		{"```", nil},
		{"`x.go:10` `y.go:1-2` `z.go:a-b` `:5`", []string{"x.go", "y.go", "z.go:a-b"}},
	}
	for _, tc := range cases {
		var got []string
		for _, s := range backtickSpans(tc.line) {
			if p, ok := normalizeScopePath(s); ok {
				got = append(got, p)
			}
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("%q: got %q, want %q", tc.line, got, tc.want)
		}
	}
}

// TestPlanScopeCorpus reads every committed work folder, so a reader change
// that cannot parse a real plan fails here rather than in an audit.
func TestPlanScopeCorpus(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..", "..", "..", "docs", "specs")
	if _, err := os.Stat(root); err != nil {
		t.Skipf("no docs/specs beside this module: %v", err)
	}
	plans := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || (d.Name() != "plan.md" && d.Name() != "brief.md") {
			return nil
		}
		dir := filepath.Dir(path)
		scope, ok := readPlanScope(dir, func(string, string) bool { return true })
		if d.Name() != "plan.md" {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !hasUnfencedTaskHeading(body) {
			return nil
		}
		plans++
		if !ok {
			t.Errorf("%s: a plan with a task heading was not read", path)
			return nil
		}
		for _, task := range scope.tasks {
			if task.id == "" {
				t.Errorf("%s: a task has no id", path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if plans == 0 {
		t.Error("the corpus held no plan with a task heading")
	}
}

func hasUnfencedTaskHeading(body []byte) bool {
	for _, l := range artifactLines(body) {
		if !l.fenced && strings.HasPrefix(string(l.text), "### Task") {
			return true
		}
	}
	return false
}

// A brace set expands into each alternative within the expansion, set and
// length bounds; a path past one is read as written, so a branch cannot hold
// the audit with the product of many sets, a deep run of single sets, or a
// long path.
func TestListedInBoundsBraceExpansion(t *testing.T) {
	t.Parallel()
	sets := func(n int) string { return "p/" + strings.Repeat("{a,b}", n) }
	cases := []struct {
		name, path string
		listed     string
		want       bool
	}{
		{"one set", "brace/y.txt", "brace/{x,y}.txt", true},
		{"at the bound", "p/" + strings.Repeat("b", 8), sets(8), true},
		{"past the bound", "p/" + strings.Repeat("b", 9), sets(9), false},
		{"past the bound, as written", sets(9), sets(9), true},
		{"far past the bound", "p/" + strings.Repeat("b", 64), sets(64), false},
		// Sets of one alternative add no alternatives but deepen the expansion.
		{"single sets at the bound", "p/" + strings.Repeat("a", 64), "p/" + strings.Repeat("{a}", 64), true},
		{"single sets past the bound", "p/" + strings.Repeat("a", 65), "p/" + strings.Repeat("{a}", 65), false},
		{"many single sets", "p/" + strings.Repeat("a", 100000), "p/" + strings.Repeat("{a}", 100000), false},
		{"path past the length bound", "p/" + strings.Repeat("a", 4096) + "b", "p/" + strings.Repeat("a", 4096) + "{b,c}", false},
	}
	for _, c := range cases {
		if got := listedIn(c.path, []string{c.listed}); got != c.want {
			t.Errorf("%s: listedIn(%q) = %v, want %v", c.name, c.path, got, c.want)
		}
	}
}
