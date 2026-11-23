package commandguard

import (
	"strings"

	"github.com/muehz/agentics/hooks/go/internal/shellwords"
)

func checkNuGetPublication(inv shellwords.Invocation, executable string, report reportFunc) {
	words := inv.Words
	denyNonliteral := func() {
		report(ruleNonLiteralNuGet, joinInvocation(words), "Cannot inspect a non-literal NuGet operation; use literal command names so package publication can be checked.")
	}
	i := 1
	if executable == "dotnet" {
		for i < len(words) && words[i].Literal && (words[i].Text == "--diagnostics" || words[i].Text == "-d") {
			i++
		}
		if i < len(words) && !words[i].Literal {
			denyNonliteral()
			return
		}
		if i >= len(words) || !strings.EqualFold(words[i].Text, "nuget") {
			return
		}
		i++
	}
	for i < len(words) && words[i].Literal && (strings.EqualFold(words[i].Text, "-ForceEnglishOutput") || words[i].Text == "--force-english-output" || strings.EqualFold(words[i].Text, "-utf8")) {
		i++
	}
	if i < len(words) && !words[i].Literal {
		denyNonliteral()
		return
	}
	if i >= len(words) {
		return
	}
	verb := strings.ToLower(words[i].Text)
	// NuGet.exe resolves unique command prefixes; dotnet uses exact subcommands.
	if verb != "push" && !(executable == "nuget" && (verb == "pu" || verb == "pus")) {
		return
	}
	args := words[i+1:]
	// A standalone help invocation has no operand that could consume the flag.
	// Push has no dry-run; source configuration can point at any feed.
	if inv.Complete && len(args) == 1 && args[0].Literal {
		help := strings.ToLower(args[0].Text)
		if executable == "dotnet" && (help == "--help" || help == "-h" || help == "-?") {
			return
		}
		if executable == "nuget" && (help == "-help" || help == "-?") {
			return
		}
	}
	report(ruleNuGetPublish, joinInvocation(words), nugetPublishSuggestion)
}
