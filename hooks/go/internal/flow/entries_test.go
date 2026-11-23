package flow

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestSectionEntries(t *testing.T) {
	t.Parallel()
	type want struct {
		text       string
		reconciled bool
	}
	cases := []struct {
		name, body, heading string
		want                []want
	}{
		{
			name:    "numbered list",
			body:    "# Plan\n\n## Standing orders\n\n1. one\n2. two\n10) ten\n",
			heading: "## Standing orders",
			want:    []want{{"one\n", false}, {"two\n", false}, {"ten\n", false}},
		},
		{
			name:    "bullet list",
			body:    "## Divergences\n\n- dash\n* star\n+ plus\n",
			heading: "## Divergences",
			want:    []want{{"dash\n", false}, {"star\n", false}, {"plus\n", false}},
		},
		{
			name:    "continuation lines and blank lines inside an item",
			body:    "## Divergences\n\n- first\n  wrapped\n\n  second paragraph\n\n\n- next\n",
			heading: "## Divergences",
			want:    []want{{"first\n  wrapped\n\n  second paragraph\n", false}, {"next\n", false}},
		},
		{
			name:    "nested list and indented fence",
			body:    "## Divergences\n\n- parent\n  - child\n\t- tab child\n  ```\n  code\n  ```\n- sibling\n",
			heading: "## Divergences",
			want:    []want{{"parent\n  - child\n\t- tab child\n  ```\n  code\n  ```\n", false}, {"sibling\n", false}},
		},
		{
			name:    "a rule line ends the item and belongs to no entry",
			body:    "## Divergences\n\nintro prose\n- one\n---\n  stray indented\n- two\n### Child\n- three\n",
			heading: "## Divergences",
			want:    []want{{"one\n", false}, {"two\n", false}, {"three\n", false}},
		},
		{
			name:    "a following heading with no blank line ends the section",
			body:    "## Divergences\n- one\n## After\n- outside\n",
			heading: "## Divergences",
			want:    []want{{"one\n", false}},
		},
		{
			name:    "a heading inside a fenced code block opens and closes no section",
			body:    "```md\n## Divergences\n- quoted\n```\n## Divergences\n- one\n  ~~~\n## After\n  ~~~\n- two\n",
			heading: "## Divergences",
			want:    []want{{"one\n  ~~~\n", false}, {"two\n", false}},
		},
		{
			name:    "every occurrence of the heading counts in file order",
			body:    "## Divergences\n- one\n## Other\n- outside\n## Divergences \t\r\n- two\n",
			heading: "## Divergences",
			want:    []want{{"one\n", false}, {"two\n", false}},
		},
		{
			name:    "mark on the last line",
			body:    "## Divergences\n\n- one\n  done. Reconciled: 2026-11-12.\n- two Reconciled:\t 2026-11-12.\n",
			heading: "## Divergences",
			want:    []want{{"one\n  done.\n", true}, {"two\n", true}},
		},
		{
			name:    "mark with the date on its own continuation line",
			body:    "## Divergences\n\n- one. Reconciled:\n  2026-11-12.  \r\n\n- two\n  Reconciled:\n  2026-11-12.\n",
			heading: "## Divergences",
			want:    []want{{"one.\n", true}, {"two\n", true}},
		},
		{
			name:    "a mark that is not at the very end stays",
			body:    "## Divergences\n\n- one Reconciled: 2026-11-12. still open\n- two Reconciled: 2026-11.\n",
			heading: "## Divergences",
			want:    []want{{"one Reconciled: 2026-11-12. still open\n", false}, {"two Reconciled: 2026-11.\n", false}},
		},
		{
			name:    "markers need a space and column zero",
			body:    "## Divergences\n\n-no space\n1.no space\n - indented\n- real\n",
			heading: "## Divergences",
			want:    []want{{"real\n", false}},
		},
		{
			name:    "the marker goes once and from the first line only",
			body:    "## Standing orders\n\n1. 2. starts like a marker\n   - continuation keeps its marker\n",
			heading: "## Standing orders",
			want:    []want{{"2. starts like a marker\n   - continuation keeps its marker\n", false}},
		},
		{
			// A known consequence of hashing without the marker: one order row records both.
			name:    "entries with the same text after the marker share a hash",
			body:    "## Standing orders\n\n1. same\n- same\n",
			heading: "## Standing orders",
			want:    []want{{"same\n", false}, {"same\n", false}},
		},
		{
			name:    "no section",
			body:    "# Plan\n\n- item\n",
			heading: "## Divergences",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := sectionEntries([]byte(tc.body), tc.heading)
			var expected []sectionEntry
			for i, w := range tc.want {
				expected = append(expected, sectionEntry{number: i + 1, hash: sha256Hex(w.text), reconciled: w.reconciled, text: strings.TrimSuffix(w.text, "\n")})
			}
			if !slices.Equal(got, expected) {
				t.Errorf("entries\n got %+v\nwant %+v", got, expected)
			}
		})
	}
}

func TestArtifactEntriesNeedsARegularFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if _, ok := artifactEntries(dir, "plan.md", "## Divergences"); ok {
		t.Error("a missing artifact was read")
	}
	if err := os.Mkdir(filepath.Join(dir, "spec.md"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, ok := artifactEntries(dir, "spec.md", "## Divergences"); ok {
		t.Error("a directory was read as an artifact")
	}
	writeFile(t, dir, "brief.md", "## Divergences\n\n- one\n")
	got, ok := artifactEntries(dir, "brief.md", "## Divergences")
	if want := []sectionEntry{{number: 1, hash: sha256Hex("one\n"), text: "one"}}; !ok || !slices.Equal(got, want) {
		t.Errorf("brief.md entries %+v %v, want %+v", got, ok, want)
	}
}
