package flow

import (
	"fmt"
	"path/filepath"
)

// path prints the absolute directory that holds a flow folder's files, so a
// caller can read or write an artifact in whichever store holds the flow. It
// only reports: a folder no store holds yet prints where flow log would create
// it, and nothing is created.
func (p *program) path(args []string) int {
	if code := p.checkStorage(); code != 0 {
		return code
	}
	folder := ""
	for i := 0; i < len(args); i++ {
		if args[i] != "--folder" || i+1 >= len(args) {
			fmt.Fprint(p.stderr, usage)
			return 1
		}
		i++
		folder = args[i]
	}
	if folder == "" {
		var code int
		if folder, code = p.resolveInto(); code != 0 {
			return code
		}
	}
	loc, err := p.locateFlow(folder)
	if err != nil {
		return p.locateFailed(err)
	}
	dir, err := filepath.Abs(loc.stored.dir)
	if err != nil {
		return p.fail(fmt.Sprintf("cannot make %s absolute: %v", loc.stored.dir, err))
	}
	fmt.Fprintln(p.stdout, dir)
	return 0
}
