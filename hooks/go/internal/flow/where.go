package flow

import "fmt"

func (p *program) where(args []string) int {
	if code := p.checkStorage(); code != 0 {
		return code
	}
	found := p.lookupFlow()
	switch found.outcome {
	case lookupFound:
		fmt.Fprintln(p.stdout, found.printed)
		return 0
	case lookupSmallFix:
		fmt.Fprintln(p.stderr, "flow: small fix in progress on "+found.branch+"; no work folder")
		return 5
	case lookupAmbiguous:
		return 4
	}
	fmt.Fprintln(p.stderr, "flow: no flow found")
	return 3
}
