package argv

import "strings"

// GitState says how far ReadGit could read a git command line.
type GitState uint8

const (
	// GitUnread means the grammar could not tell where the subcommand starts:
	// an unknown option, a `--`, a flag given a value, a value option without
	// its value, a non-literal verb, or no verb at all. It is the zero value,
	// so a Git that was never read cannot pass for a readable line. It is
	// distinct from a readable line that does nothing a consumer looks for.
	// The package documentation lists what each consumer does with it.
	GitUnread GitState = iota
	// GitVerb means every global option was read and a literal verb follows.
	GitVerb
	// GitPrints means a global option makes git print help, a version, or a
	// path and exit, so no following word runs as a subcommand. It is distinct
	// from a readable line. The package documentation lists what each consumer
	// does with it.
	GitPrints
)

// GitGlobal is one global option read before the verb.
type GitGlobal struct {
	// Name is the row's spelling: `-C` for `-Cdir`, `--git-dir` for
	// `--git-dir=x`.
	Name string
	// Option is the source word.
	Option Word
	At     int
	// Value is the option's value. An attached value copies the source word
	// with Text set to the value and Raw cleared.
	Value    Word
	HasValue bool
	// ValueAt is the index of a separate value word, or -1.
	ValueAt int
}

// Git is the reading of the words after `git`. Indices count from the first
// of those words.
type Git struct {
	State GitState
	// Globals holds every global option read before the walk stopped, in
	// every state. A value option that ends the line is kept with HasValue
	// false, because a consumer may treat even that word as evidence.
	Globals []GitGlobal
	// Verb is the subcommand word and VerbAt its index; VerbAt is -1 and Verb
	// the zero Word unless the state is GitVerb.
	Verb   Word
	VerbAt int
	// Args are the words after the verb, and empty unless the state is
	// GitVerb.
	Args []Word
	// Rest holds the words from the first word the grammar could not read.
	// A literal option word that stopped the walk is left out, since it was
	// read and refused; a non-literal word or verb is kept, since its text is
	// not known. Rest is empty in the GitVerb state, and RestAt is then
	// len(args).
	Rest   []Word
	RestAt int
	// words is the input, kept for ConfigSpecs, which reads past options the
	// strict grammar refuses.
	words []Word
}

// gitGlobalRows are git's global options, the closed set the strict walk reads
// before the verb. The value options take their value in the next word,
// attached to a single-letter name, or after `=`.
// `--exec-path=` sets a path; the bare `--exec-path` prints one and is in
// gitPrintingOptions.
var gitGlobalRows = []Option{
	{Name: "-C", Arity: ArityValue},
	{Name: "-c", Arity: ArityValue},
	{Name: "--git-dir", Arity: ArityValue},
	{Name: "--work-tree", Arity: ArityValue},
	{Name: "--namespace", Arity: ArityValue},
	{Name: "--config-env", Arity: ArityValue},
	{Name: "--exec-path", Arity: ArityJoined},
	{Name: "--literal-pathspecs", Arity: ArityFlag},
	{Name: "--glob-pathspecs", Arity: ArityFlag},
	{Name: "--noglob-pathspecs", Arity: ArityFlag},
	{Name: "--icase-pathspecs", Arity: ArityFlag},
	{Name: "--no-optional-locks", Arity: ArityFlag},
	{Name: "--bare", Arity: ArityFlag},
	{Name: "--no-pager", Arity: ArityFlag},
	{Name: "-P", Arity: ArityFlag},
	{Name: "--paginate", Arity: ArityFlag},
	{Name: "-p", Arity: ArityFlag},
	{Name: "--no-replace-objects", Arity: ArityFlag},
	{Name: "--no-lazy-fetch", Arity: ArityFlag},
	{Name: "--no-advice", Arity: ArityFlag},
}

// gitPrintingOptions make git print and exit. They are matched by whole text
// before the table, so `--exec-path` without `=` prints rather than sets.
var gitPrintingOptions = []string{
	"--exec-path", "--html-path", "--man-path", "--info-path", "--help", "-h", "--version", "-v",
}

// gitSpec reads attached short values but never clusters, so `-Pp` stays
// unknown as git's own parser has it. Option words are read by their text
// whether literal or not, because skipping a word from an expansion would
// read less of the line than its text shows.
var gitSpec = NewSpec(gitGlobalRows...).With(AttachedShort, ReadNonLiteral)

func gitPrinting(text string) bool {
	for _, name := range gitPrintingOptions {
		if text == name {
			return true
		}
	}
	return false
}

// ReadGit reads the words after `git`: its global options, then the verb.
// Option values need not be literal.
func ReadGit(args []Word) Git {
	g := Git{State: GitUnread, VerbAt: -1, RestAt: len(args), words: args}
	scan := gitSpec.Scan(args)
	for _, arg := range scan.Args {
		if arg.Role == RoleValue {
			continue
		}
		word := arg.Word
		switch {
		case arg.Role == RoleOperand && word.Literal:
			g.State = GitVerb
			g.Verb, g.VerbAt = word, arg.At
			if arg.At+1 < len(args) {
				g.Args = args[arg.At+1:]
			}
			return g
		case arg.Role == RoleOperand:
			g.stop(GitUnread, arg.At, false)
			return g
		case gitPrinting(word.Text):
			g.stop(GitPrints, arg.At, word.Literal)
			return g
		case arg.Role != RoleOption, arg.HasValue && gitFlagRow(arg.Name):
			g.stop(GitUnread, arg.At, word.Literal)
			return g
		}
		// A value option without its value is kept as a global; Scan reads
		// one only as the last word, so the line then ends unread.
		g.Globals = append(g.Globals, GitGlobal{
			Name: arg.Name, Option: word, At: arg.At,
			Value: arg.Value, HasValue: arg.HasValue, ValueAt: arg.ValueAt,
		})
	}
	return g
}

func gitFlagRow(name string) bool {
	option, _, ok := gitSpec.lookup(name)
	return ok && option.Arity == ArityFlag
}

// stop ends the walk at the word at index at. A literal word was read and
// refused, so Rest starts after it.
func (g *Git) stop(state GitState, at int, skip bool) {
	g.State = state
	g.RestAt = at
	if skip {
		g.RestAt++
	}
	if g.RestAt < len(g.words) {
		g.Rest = g.words[g.RestAt:]
	}
}

// Subcommand returns the verb in lower case, or "" unless the state is
// GitVerb.
func (g Git) Subcommand() string {
	if g.State != GitVerb {
		return ""
	}
	return strings.ToLower(g.Verb.Text)
}

// ConfigSpecs returns the configuration each `-c` and `--config-env` sets,
// in a lenient walk that stops at the first word that is not an option.
// It skips `-C`, `--git-dir`, `--work-tree`, and `--namespace` with their
// values and any other dash word, and stops at the first word that is
// neither. missing reports a `-c` or `--config-env` without its word.
//
// It is its own walk because it reads past options the strict grammar
// refuses, which makes it evidence that denies and never proof that allows:
// a line it finds nothing in may still set configuration git reads.
// Each spec keeps the Literal of the word it came from; an attached spec
// copies its source word with Text set to the spec and Raw cleared.
func (g Git) ConfigSpecs() (specs []Word, missing bool) {
	words := g.words
	for i := 0; i < len(words); i++ {
		text := words[i].Text
		switch {
		case text == "-c" || text == "--config-env":
			if i+1 >= len(words) {
				return specs, true
			}
			i++
			specs = append(specs, words[i])
		case strings.HasPrefix(text, "--config-env="):
			specs = append(specs, attachedSpec(words[i], text[len("--config-env="):]))
		case strings.HasPrefix(text, "-c") && len(text) > 2:
			specs = append(specs, attachedSpec(words[i], text[2:]))
		case text == "-C" || text == "--git-dir" || text == "--work-tree" || text == "--namespace":
			i++
		case !strings.HasPrefix(text, "-"):
			return specs, false
		}
	}
	return specs, false
}

func attachedSpec(source Word, spec string) Word {
	source.Text = spec
	source.Raw = ""
	return source
}
