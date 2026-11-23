package flow

import (
	"strings"
	"testing"
)

// resolveRepo is a setup with a non-merge commit, recorded as "T" with its
// patch id "T-patch" and its tree "tree", and a merge commit "M" at HEAD. No
// flow folder exists, so a refused call must leave no state.tsv behind.
func resolveRepo(t *testing.T) *contractFixture {
	t.Helper()
	fx := emptyRepo(t)
	writeFile(t, fx.repo, "src/task.txt", "task\n")
	commitAll(t, fx.repo, "task")
	fx.values["T"] = gitOut(t, fx.repo, "rev-parse", "HEAD")
	fx.values["T-patch"] = patchIDOf(fx.repo, fx.values["T"])
	if fx.values["T-patch"] == "" {
		t.Fatal("the task commit has no patch id")
	}
	fx.values["tree"] = gitOut(t, fx.repo, "rev-parse", "HEAD^{tree}")
	git(t, fx.repo, "switch", "-q", "-c", "side")
	writeFile(t, fx.repo, "side.txt", "side\n")
	commitAll(t, fx.repo, "side")
	git(t, fx.repo, "switch", "-q", "main")
	writeFile(t, fx.repo, "main.txt", "main\n")
	commitAll(t, fx.repo, "main")
	git(t, fx.repo, "merge", "-q", "--no-ff", "--no-edit", "side")
	fx.values["M"] = gitOut(t, fx.repo, "rev-parse", "HEAD")
	return fx
}

// resolveCases are hand-typed verify rows whose commits flow log resolves
// against the fixture repository before it writes them.
func resolveCases() []contractCase {
	wrong := strings.Repeat("b", 40)
	absent := strings.Repeat("0", 39) + "1"
	// expand fills the placeholders of a cell or reason with fixture values.
	expand := func(fx *contractFixture, text string) string {
		return strings.NewReplacer(
			"<T>", fx.values["T"],
			"<T7>", fx.values["T"][:7],
			"<Tp>", fx.values["T-patch"],
			"<M>", fx.values["M"],
			"<tree>", fx.values["tree"],
			"<wrong>", wrong,
			"<absent>", absent,
		).Replace(text)
	}
	logArgs := func(target, status, evidence string) func(fx *contractFixture) []string {
		return func(fx *contractFixture) []string {
			return contractLog("execute", "verify", target, status, contractWhy, expand(fx, evidence), contractNext)
		}
	}
	written := func(name, target, status, evidence string) contractCase {
		return contractCase{
			name:  "resolve/" + name,
			setup: resolveRepo,
			steps: []contractStep{{argsOf: logArgs(target, status, evidence)}},
			check: rowsAre(func(fx *contractFixture) []string {
				return []string{cellsRow("execute", "verify", target, status, contractWhy, expand(fx, evidence), contractNext)}
			}),
		}
	}
	refused := func(name, target, status, evidence, reason string) contractCase {
		return contractCase{
			name:  "resolve/" + name,
			setup: resolveRepo,
			steps: []contractStep{{
				argsOf: logArgs(target, status, evidence),
				code:   2,
				check: func(t *testing.T, fx *contractFixture, res result) {
					t.Helper()
					want := "flow: invalid evidence: '" + expand(fx, evidence) + "' -- " + expand(fx, reason)
					checkStream(t, "stderr", res.stderr, []string{want}, nil, nil)
				},
			}},
			check: noStateFile,
		}
	}
	const (
		isolated   = "base=<T> head=<T> integrated=<M> patch-id=<wrong>"
		mismatch   = "patch-id=<wrong> does not match commit <T> (patch-id <Tp>)"
		needsPatch = "commit <T> is not a merge, so it needs patch-id="
		mergeHasNo = "commit <M> is a merge, which has no patch-id"
	)
	return []contractCase{
		written("a-commit-with-its-patch-id-is-written", "task-1", "ok", "commit=<T> patch-id=<Tp>"),
		written("a-commit-prefix-is-stored-as-typed", "task-1", "ok", "commit=<T7> patch-id=<Tp>"),
		written("a-merge-commit-alone-is-written", "task-1", "ok", "commit=<M>"),
		written("the-isolated-form-is-written-without-comparing-its-patch-id", "task-1", "ok", isolated),
		written("a-wave-commit-alone-is-written", "wave-1", "ok", "commit=<T>"),
		written("a-wave-commit-with-its-patch-id-is-written", "wave-1", "ok", "commit=<T> patch-id=<Tp>"),
		written("a-wave-merge-commit-is-written", "wave-1", "ok", "commit=<M>"),

		refused("a-wrong-patch-id-is-refused", "task-1", "ok", "commit=<T> patch-id=<wrong>", mismatch),
		refused("a-snapshot-hash-is-refused", "task-1", "ok", "commit=<absent> patch-id=<wrong>", "commit <absent> does not resolve to a commit"),
		refused("a-tree-id-is-refused", "task-1", "ok", "commit=<tree> patch-id=<wrong>", "commit <tree> does not resolve to a commit"),
		refused("a-bare-non-merge-commit-on-a-task-is-refused", "task-1", "ok", "commit=<T>", needsPatch),
		refused("a-bare-non-merge-commit-on-a-red-task-is-refused", "red-task-1", "failed", "commit=<T>", needsPatch),
		refused("a-bare-non-merge-commit-on-fix-is-refused", "fix", "ok", "commit=<T>", needsPatch),
		refused("a-patch-id-on-a-merge-commit-is-refused", "task-1", "ok", "commit=<M> patch-id=<wrong>", mergeHasNo),
		refused("an-unresolved-base-is-refused", "task-1", "ok", strings.Replace(isolated, "base=<T>", "base=<absent>", 1), "base <absent> does not resolve to a commit"),
		refused("an-unresolved-head-is-refused", "task-1", "ok", strings.Replace(isolated, "head=<T>", "head=<absent>", 1), "head <absent> does not resolve to a commit"),
		refused("an-unresolved-integrated-is-refused", "task-1", "ok", strings.Replace(isolated, "integrated=<M>", "integrated=<absent>", 1), "integrated <absent> does not resolve to a commit"),
		refused("a-wrong-patch-id-on-a-wave-is-refused", "wave-1", "ok", "commit=<T> patch-id=<wrong>", mismatch),
		refused("a-patch-id-on-a-wave-merge-commit-is-refused", "wave-1", "ok", "commit=<M> patch-id=<wrong>", mergeHasNo),
		refused("an-unresolved-wave-commit-is-refused", "wave-1", "ok", "commit=<absent>", "commit <absent> does not resolve to a commit"),
	}
}

func TestResolveContract(t *testing.T) {
	t.Parallel()
	for _, c := range resolveCases() {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			runContractCase(t, c)
		})
	}
}
