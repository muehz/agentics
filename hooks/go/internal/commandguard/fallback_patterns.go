package commandguard

import (
	"strings"

	"github.com/muehz/agentics/hooks/go/internal/asciifold"
)

func scanThreatPatternBank(texts []string, commandPresent, protectedDeletionComplete bool, report reportFunc) {
	folded := make([]string, len(texts))
	for i, text := range texts {
		folded[i] = asciifold.String(text)
	}
	rows := threatPatternBank()
	for _, pattern := range rows {
		if commandPresent && protectedDeletionComplete && pattern.superseded {
			continue
		}
		if commandPresent && pattern.fallbackOnly {
			continue
		}
		for i, text := range texts {
			if pattern.gate != "" && !strings.Contains(folded[i], pattern.gate) {
				continue
			}
			if match := pattern.find(text); match != "" {
				report(pattern.rule, match, pattern.suggestion)
				break
			}
		}
	}
}
