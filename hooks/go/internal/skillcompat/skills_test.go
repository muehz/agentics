package skillcompat

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hosttest"
)

// The list files of the repository's skills, relative to the package directory, which is where go test runs.
const (
	knownDenialsFile = "testdata/known-denials.tsv"
	policyOffFile    = "testdata/policy-off.tsv"
)

// sweepFix closes the failure of a test that checks the skills against a list. It is three lines with the
// instruction last, because a gate that runs the package shows only the tail of its output. It names every way out,
// because the list is not where a denial the README does not state as a rule is settled.
const sweepFix = "Known-denial entry: block now allowed or gone, remove it; block edited and still denied for the " +
	"same stated rule, replace its digest with the one printed.\n" +
	"New denial: respell the command, or add an entry only if hooks/README.md states the denial as a rule. " +
	"Policy-off problem: fix " + policyOffFile + ".\n" +
	"To fix: change " + knownDenialsFile + ", " + policyOffFile + " or the skill as above, then rerun go test with " +
	"-count=1; the test cache does not see skills/ or hooks/bin/."

// skillsRoot returns the directory of the repository's skills.
func skillsRoot(t testing.TB) string {
	t.Helper()
	return filepath.Join(hosttest.Checkout(t), "skills")
}

// readSkillLists returns the entries of the two list files of the repository's skills.
func readSkillLists() ([]listEntry, []policyOffEntry, error) {
	denialsText, err := os.ReadFile(filepath.FromSlash(knownDenialsFile))
	if err != nil {
		return nil, nil, err
	}
	denials, err := readList(knownDenialsFile, string(denialsText))
	if err != nil {
		return nil, nil, err
	}
	policyOffText, err := os.ReadFile(filepath.FromSlash(policyOffFile))
	if err != nil {
		return nil, nil, err
	}
	policyOff, err := readPolicyOff(policyOffFile, string(policyOffText))
	if err != nil {
		return nil, nil, err
	}
	return denials, policyOff, nil
}

// checkSkillTree runs every fenced block of the Markdown under root through the built hook binary and returns
// checkTree's problems against the two list files and the headings of the repository's hook README. root is a
// parameter so that the guards of the sweep can be tested on a directory that is not the skills. It is an error
// when the README cannot be read or yields no heading, when a list file does not read cleanly, and when checkTree
// gives one; nothing is run then. A rule an entry names that is not among the headings is one of the problems, so
// the headings the list needs are not repeated here.
//
// Run it with -count=1. The skills and hook README lie outside this module. Cached results can miss changes to the
// Markdown or to the source used by the test's build subprocess.
func checkSkillTree(t *testing.T, root string) ([]string, error) {
	t.Helper()
	readmePath := filepath.Join(hosttest.Checkout(t), "hooks", "README.md")
	readme, err := os.ReadFile(readmePath)
	if err != nil {
		return nil, err
	}
	headings := headingSet(string(readme))
	if len(headings) == 0 {
		return nil, fmt.Errorf("%s holds no heading a list entry could name as its rule", readmePath)
	}
	denials, policyOff, err := readSkillLists()
	if err != nil {
		return nil, err
	}
	return checkTree(t, root, denials, policyOff, headings)
}

// unmatchedPolicyOff returns one problem for every policy-off entry that matches no block, in the words the
// known-denial matcher uses for it, so that the entries can be checked without running every block.
func unmatchedPolicyOff(blocks []block, entries []policyOffEntry) []string {
	present := map[blockKey]bool{}
	for _, b := range blocks {
		present[blockKey{b.path, b.tool, bodyDigest(b.body)}] = true
	}
	var problems []string
	for _, entry := range entries {
		if !present[entry.blockKey()] {
			problems = append(problems, fmt.Sprintf("%s: policy-off list line %d: the entry for %s block %s with the key %s matches no block",
				entry.path, entry.line, entry.tool, entry.digest, entry.key))
		}
	}
	return problems
}

// TestSkillsPassTheCommittedHook is the sweep of the repository's skills. Its inputs lie outside this module, so it
// reports a change to them only when run with -count=1; checkSkillTree says why.
func TestSkillsPassTheCommittedHook(t *testing.T) {
	problems, err := checkSkillTree(t, skillsRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) != 0 {
		t.Fatalf("the skills and the lists in testdata disagree in %d place(s):\n%s\n%s",
			len(problems), strings.Join(problems, "\n"), sweepFix)
	}
}

func TestSkillsSweepNeedsMarkdown(t *testing.T) {
	root := t.TempDir()
	problems, err := checkSkillTree(t, root)
	if err == nil || !strings.Contains(err.Error(), "no .md file under "+root) {
		t.Errorf("got %v and the error %v, want an error that says %s holds no .md file", problems, err, root)
	}
}

// TestPolicyOffBlocks checks the policy-off list on its own, without the sweep's run of every block: each entry
// must match a block of the skills, and that block must be denied under default policy and allowed in a repository
// that sets the entry's key to false, which is two hook processes per entry. The list is empty today, so the test
// starts no process and only holds the file to the reader's rules; the first entry will exercise both checks. Like
// the sweep, it sees a changed skill only with -count=1.
func TestPolicyOffBlocks(t *testing.T) {
	_, entries, err := readSkillLists()
	if err != nil {
		t.Fatal(err)
	}
	blocks, err := walkNonEmpty(skillsRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	problems := slices.Concat(unmatchedPolicyOff(blocks, entries), checkPolicyOff(t, blocks, entries))
	if len(problems) != 0 {
		t.Fatalf("the skills and %s disagree in %d place(s):\n%s\n%s",
			policyOffFile, len(problems), strings.Join(problems, "\n"), sweepFix)
	}
}

// TestPolicyKeysInSkillText finds nothing to judge today: no skill writes a policy key as an inline code span, so
// the scan passes without comparing a key. It is here for the first skill that names one; that the scanner reports
// an unknown key is proven by the fixture tests of policyKeySpans and walkPolicyKeySpans.
func TestPolicyKeysInSkillText(t *testing.T) {
	problems, err := walkPolicyKeySpans(skillsRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) != 0 {
		t.Errorf("the skills name %d policy key(s) the hook does not read:\n%s", len(problems), strings.Join(problems, "\n"))
	}
}
