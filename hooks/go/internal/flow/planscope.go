package flow

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// briefTaskID names the single task of a work folder that has a brief and no
// plan, so a check over tasks treats both kinds of folder alike.
const briefTaskID = "brief"

var (
	sliceHeading = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`^## ([Ss][0-9]+)(?:[ \t:]|$)`) })
	anyHeading   = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`^#{1,6}(?:[ \t]|$)`) })
	// A path cited with a line range names the file, not the lines, so the
	// range is no part of the path a change is matched against.
	lineRange = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`:[0-9]+(?:-[0-9]+)?$`) })
)

// planTask is one task of a work folder: its ID, the slice it belongs to
// ("" when none), and every path its Files block names, as written.
type planTask struct {
	id    string
	slice string
	paths []string
}

// planScope is what a work folder declares in scope: its tasks, the task IDs
// of each slice in file order, and the paths named by divergence entries that
// a row records.
type planScope struct {
	tasks           []planTask
	slices          map[string][]string
	divergencePaths []string
}

// readPlanScope reads the tasks of the work folder dir from plan.md, or from
// brief.md when there is no plan, and the paths of its recorded divergence
// entries. recorded reports whether a row records the entry of artifact with
// hash; it decides for reconciled entries too, since a reconciled entry still
// approved the change it describes. ok is false when there is neither a plan
// nor a brief, or the plan names no task, so a caller cannot mistake an
// unreadable folder for one that declares nothing in scope.
func readPlanScope(dir string, recorded func(artifact, hash string) bool) (scope planScope, ok bool) {
	if body, exists, err := readRegular(dir, "plan.md"); exists {
		if err != nil {
			return planScope{}, false
		}
		scope.tasks, scope.slices = planTasks(body)
		if len(scope.tasks) == 0 {
			return planScope{}, false
		}
	} else if body, exists, err := readRegular(dir, "brief.md"); exists {
		if err != nil {
			return planScope{}, false
		}
		scope.tasks = []planTask{{id: briefTaskID, paths: briefFiles(body)}}
		scope.slices = map[string][]string{}
	} else {
		return planScope{}, false
	}
	for _, artifact := range textArtifacts {
		entries, _ := artifactEntries(dir, artifact, divergencesHeading)
		for _, entry := range entries {
			if !recorded(artifact, entry.hash) {
				continue
			}
			for line := range strings.SplitSeq(entry.text, "\n") {
				scope.divergencePaths = append(scope.divergencePaths, linePaths(line)...)
			}
		}
	}
	return scope, true
}

// readRegular reads artifact in dir; exists is false when it is not a regular
// file, so a directory of that name counts as absent.
func readRegular(dir, artifact string) (body []byte, exists bool, err error) {
	file := filepath.Join(dir, artifact)
	if info, err := os.Stat(file); err != nil || !info.Mode().IsRegular() {
		return nil, false, nil
	}
	body, err = os.ReadFile(file)
	return body, true, err
}

// scopeLines returns body's lines outside fenced code blocks, trimmed as
// sectionEntries trims them, so a quoted example of a plan declares nothing.
func scopeLines(body []byte) []string {
	var lines []string
	for _, l := range artifactLines(body) {
		if !l.fenced {
			lines = append(lines, string(l.text))
		}
	}
	return lines
}

// planTasks returns the tasks of a plan in file order and the task IDs of
// each slice.
func planTasks(body []byte) ([]planTask, map[string][]string) {
	var tasks []planTask
	bySlice := map[string][]string{}
	slice := ""
	current := -1
	idSeen := false
	var files filesBlock
	for _, line := range scopeLines(body) {
		switch {
		case strings.HasPrefix(line, "## "):
			slice = ""
			if m := sliceHeading().FindStringSubmatch(line); m != nil {
				slice = m[1]
			}
			current = -1
		case strings.HasPrefix(line, "### Task"):
			tasks = append(tasks, planTask{id: headingTaskID(line), slice: slice})
			current, idSeen = len(tasks)-1, false
		case strings.HasPrefix(line, "### ") || strings.HasPrefix(line, "# ") || line == "#" || line == "##" || line == "###":
			current = -1
		}
		paths := files.step(line)
		if current < 0 {
			continue
		}
		tasks[current].paths = append(tasks[current].paths, paths...)
		if _, after, found := strings.Cut(line, "**ID:**"); found && !idSeen {
			if fields := strings.Fields(after); len(fields) > 0 {
				tasks[current].id, idSeen = fields[0], true
			}
		}
	}
	for _, task := range tasks {
		if task.slice != "" {
			bySlice[task.slice] = append(bySlice[task.slice], task.id)
		}
	}
	return tasks, bySlice
}

// headingTaskID returns the first token after "### Task", which names the
// task when its field line carries no ID.
func headingTaskID(line string) string {
	fields := strings.Fields(strings.TrimPrefix(line, "### Task"))
	if len(fields) == 0 {
		return ""
	}
	return strings.TrimSuffix(fields[0], ":")
}

// briefFiles returns the paths of the Files item in the brief's Task section.
func briefFiles(body []byte) []string {
	var paths []string
	inTask := false
	var files filesBlock
	for _, line := range scopeLines(body) {
		if strings.HasPrefix(line, "## ") || line == "##" {
			inTask = line == "## Task"
		}
		found := files.step(line)
		if inTask {
			paths = append(paths, found...)
		}
	}
	return paths
}

// filesBlock follows one Files block through a document a line at a time.
// The block runs from its Files line to the next heading, or to a line after
// a blank that is not a list item; wrapped lines count at any indentation,
// because plans wrap long path lists without indenting them.
type filesBlock struct {
	active, afterBlank bool
}

// step reads the next line and returns the paths it contributes.
func (b *filesBlock) step(line string) []string {
	trimmed := strings.TrimLeft(line, " \t")
	switch {
	case anyHeading().MatchString(line):
		b.active = false
		return nil
	case strings.HasPrefix(trimmed[len(entryMarker().FindString(trimmed)):], "**Files:**"):
		b.active, b.afterBlank = true, false
		return linePaths(line)
	case !b.active:
		return nil
	case line == "":
		b.afterBlank = true
		return nil
	case b.afterBlank && !entryMarker().MatchString(trimmed):
		b.active = false
		return nil
	}
	b.afterBlank = false
	return linePaths(line)
}

// linePaths returns the normalized paths of every code span in line.
func linePaths(line string) []string {
	var paths []string
	for _, span := range backtickSpans(line) {
		if p, ok := normalizeScopePath(span); ok {
			paths = append(paths, p)
		}
	}
	return paths
}

// backtickSpans returns the contents of the code spans in line. A span opens
// with a run of backticks and closes at the next run of the same length, so a
// path quoted with double backticks may hold a single one; a run with no
// closing run is literal text.
func backtickSpans(line string) []string {
	var spans []string
	for i := 0; i < len(line); {
		if line[i] != '`' {
			i++
			continue
		}
		n := backtickRun(line, i)
		closed := false
		for k := i + n; k < len(line); {
			if line[k] != '`' {
				k++
				continue
			}
			m := backtickRun(line, k)
			if m == n {
				spans = append(spans, line[i+n:k])
				i, closed = k+m, true
				break
			}
			k += m
		}
		if !closed {
			i += n
		}
	}
	return spans
}

func backtickRun(line string, i int) int {
	n := 0
	for i+n < len(line) && line[i+n] == '`' {
		n++
	}
	return n
}

// normalizeScopePath strips a trailing line range and surrounding blanks from
// span, and reports false when nothing is left, as for a bare range. Globs,
// brace sets and directories stay as written for the matcher to interpret.
func normalizeScopePath(span string) (string, bool) {
	p := strings.TrimSpace(lineRange().ReplaceAllString(strings.TrimSpace(span), ""))
	return p, p != ""
}
