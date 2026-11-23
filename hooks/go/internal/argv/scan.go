package argv

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Arity says which words an option takes as its value.
type Arity uint8

const (
	// ArityFlag takes no separate value; a `name=value` form still records one.
	ArityFlag Arity = iota
	// ArityValue takes the next word whatever it is, or the text after `=`.
	ArityValue
	// ArityJoined takes a value only in its `name=value` form.
	ArityJoined
	// ArityOptional may take the next word when it is one of the option's
	// Spellings.
	ArityOptional
)

// Option is one row of a grammar. Name includes its dashes, and each alias is
// its own row so that a consumer's table stays a closed, testable set.
type Option struct {
	Name  string
	Arity Arity
	// Spellings lists the next words an ArityOptional option may take.
	Spellings []string
}

// Trait changes how a Spec reads its words. Each consumer opts into exactly
// the readings its command needs, so that sharing the walk makes no consumer
// read a word more loosely than its own rule allows.
type Trait uint8

const (
	// FoldNames matches option names in any letter case and reports the
	// table's spelling. Cluster letters never fold, because `-s` and `-S`
	// are different options wherever letters cluster.
	FoldNames Trait = iota
	// Abbreviations resolves a prefix of exactly one `--` row. An exact row
	// wins over a longer one, and an ambiguous prefix is unknown, as git
	// reads its long options.
	Abbreviations
	// Clusters reads a single-dash word that is not a row letter by letter.
	// A value letter takes the rest of the word, or the next word when it
	// ends the word. Without Abbreviations, a `--` word that is not a row is
	// read the same way, as the shell builtins do.
	Clusters
	// AttachedShort reads `-Cdir` as the single-letter value option `-C`
	// with the value `dir`, and any other unmatched single-dash word of
	// several letters as unknown.
	AttachedShort
	// AnyLetter reads a cluster letter that is not a row as a flag, for
	// readers that must keep reading a cluster past a letter they do not
	// know rather than stop at it.
	AnyLetter
	// ReadNonLiteral reads a non-literal dash word by its text, for readers
	// that inspected text without testing Literal and must not become
	// looser by skipping those words.
	ReadNonLiteral
	traitCount
)

// Role is what a word is in the scanned command line.
type Role uint8

const (
	RoleOperand Role = iota
	RoleOption
	// RoleValue is a word that an option before it consumed.
	RoleValue
	// RoleEnd is the `--` that ends option reading.
	RoleEnd
	// RoleUnknown is a dash word the grammar cannot name; it is read as a
	// flag so that the walk continues, and the caller decides what it means.
	RoleUnknown
)

// Arg is one word of a scanned command line.
type Arg struct {
	Role Role
	At   int
	Word Word
	// Name is the matched row's name, or the owner's name for RoleValue.
	Name string
	// Exact reports that the name matched without folding or abbreviation.
	Exact bool
	// Letters holds the cluster bytes examined, in order: the value letter's
	// byte but never the value's, and for an unknown cluster the rejected
	// byte. A cluster's Name is its last letter's row, or "" when that letter
	// is no row.
	Letters string
	// Value is the option's value. An attached value copies the source word
	// with Text set to the value and Raw cleared, because no raw spelling of
	// the value alone exists.
	Value    Word
	HasValue bool
	Joined   bool
	// ValueAt is the index of a separate value word, or -1.
	ValueAt int
}

// ProblemKind names why a word could not be read.
type ProblemKind uint8

const (
	// UnknownOption is a dash word that names no row.
	UnknownOption ProblemKind = iota
	// MissingValue is a value option that ends the command line.
	MissingValue
	// NotLiteral is an option word or operand whose text depends on a
	// runtime expansion.
	NotLiteral
)

// String returns a fixed name that never includes the input, so a kind can
// appear in a message without rendering command content.
func (k ProblemKind) String() string {
	switch k {
	case UnknownOption:
		return "UnknownOption"
	case MissingValue:
		return "MissingValue"
	case NotLiteral:
		return "NotLiteral"
	}
	return "InvalidProblemKind"
}

// Problem locates the first word a Scan could not read. It deliberately has
// no Error or String method: the word may hold secrets or hostile text, so a
// consumer renders only its own message for the kind.
type Problem struct {
	Kind ProblemKind
	At   int
	Word Word
}

// Scan is the reading of one command line.
type Scan struct {
	Args []Arg
	// Problem is the first word that could not be read, or nil when every
	// word was read. A problem is distinct from "nothing found": a reader
	// that saw no match in an unreadable line has not shown the line is
	// harmless. The package documentation lists what each consumer does with
	// a problem.
	Problem *Problem
	// spec is the grammar that produced the reading, so Flag can resolve
	// names and compare values under the same case rule.
	spec Spec
}

// Spec is a grammar: a closed set of option rows and the traits that shape
// how they are read.
type Spec struct {
	options []Option
	byName  map[string]int
	// letters maps a letter byte to its single-letter row's index plus one,
	// so a cluster resolves each letter without building a name string.
	letters *[256]int32
	// folded lists the rows no earlier row folds onto; FoldNames reads only
	// these, so folded duplicates merge with the first one winning.
	folded []int
	traits uint8
}

// NewSpec builds a grammar from its rows. It panics on a row that no command
// line could match as written, and on two rows with one name and different
// arities, because the table would then be ambiguous. Rows with one name and
// one arity merge, and the first wins. With(FoldNames) applies the same rule
// to names that differ only in letter case.
func NewSpec(options ...Option) Spec {
	spec := Spec{byName: make(map[string]int, len(options)), letters: new([256]int32)}
	for _, option := range options {
		switch {
		case option.Name == "":
			panic("argv: option with an empty name")
		case !strings.HasPrefix(option.Name, "-"):
			panic("argv: option name without a leading dash: " + option.Name)
		case strings.Contains(option.Name, "="):
			panic("argv: option name containing '=': " + option.Name)
		}
		if i, ok := spec.byName[option.Name]; ok {
			if spec.options[i].Arity != option.Arity {
				panic("argv: option rows with one name and different arities: " + option.Name)
			}
			continue
		}
		spec.byName[option.Name] = len(spec.options)
		if len(option.Name) == 2 && option.Name[1] != '-' {
			spec.letters[option.Name[1]] = int32(len(spec.options) + 1)
		}
		spec.options = append(spec.options, option)
	}
	return spec
}

// With returns a copy of the grammar with the traits added. Adding FoldNames
// panics on two rows whose names differ only in case and whose arities
// differ, because a folded word could then mean either.
func (s Spec) With(traits ...Trait) Spec {
	for _, trait := range traits {
		if trait >= traitCount {
			panic("argv: undeclared trait")
		}
		s.traits |= 1 << trait
	}
	if s.has(FoldNames) && s.folded == nil {
		s.folded = foldedRows(s.options)
	}
	return s
}

func foldedRows(options []Option) []int {
	folded := make([]int, 0, len(options))
next:
	for i, option := range options {
		for _, j := range folded {
			if foldEqual(options[j].Name, option.Name) {
				if options[j].Arity != option.Arity {
					panic("argv: option rows that fold to one name with different arities: " + option.Name)
				}
				continue next
			}
		}
		folded = append(folded, i)
	}
	return folded
}

func (s Spec) has(trait Trait) bool {
	return s.traits&(1<<trait) != 0
}

func (s Spec) same(a, b string) bool {
	if s.has(FoldNames) {
		return foldEqual(a, b)
	}
	return a == b
}

// foldEqual reports whether strings.ToLower(a) == strings.ToLower(b). It folds
// as strings.ToLower does, which is stricter than strings.EqualFold: EqualFold
// also folds the long s (U+017F) onto s, so `--ſtack` would name `--stack`.
// Comparing rune by rune keeps that rule without allocating per word.
func foldEqual(a, b string) bool {
	rest, ok := foldTrimPrefix(a, b)
	return ok && rest == ""
}

// foldTrimPrefix reports whether strings.ToLower(s) starts with
// strings.ToLower(prefix), and returns the rest of s after it. Lowering maps
// each rune on its own and UTF-8 is prefix-free, so a byte prefix of the
// lowered strings is a rune prefix. DecodeRuneInString and range both read an
// invalid byte as U+FFFD, as strings.ToLower writes it.
func foldTrimPrefix(s, prefix string) (string, bool) {
	for _, p := range prefix {
		if s == "" {
			return "", false
		}
		r, n := utf8.DecodeRuneInString(s)
		if unicode.ToLower(r) != unicode.ToLower(p) {
			return "", false
		}
		s = s[n:]
	}
	return s, true
}

// lookup finds the row for a whole name, folding case under FoldNames. A name
// made only of dashes is never a row, so `-`, `--=x` and `---` stay unknown.
// Folding matches as strings.ToLower on both sides would, but compares in
// place rather than lowering the word, so a hostile line costs no allocation
// per word.
func (s Spec) lookup(name string) (option Option, exact, ok bool) {
	if strings.Trim(name, "-") == "" {
		return Option{}, false, false
	}
	if s.has(FoldNames) {
		for _, i := range s.folded {
			if foldEqual(s.options[i].Name, name) {
				return s.options[i], s.options[i].Name == name, true
			}
		}
		return Option{}, false, false
	}
	i, ok := s.byName[name]
	if !ok {
		return Option{}, false, false
	}
	return s.options[i], true, true
}

// resolve is lookup followed, under Abbreviations, by the prefix rule for a
// `--` name. The empty long name never abbreviates, since it is a prefix of
// every row.
func (s Spec) resolve(name string) (Option, bool, bool) {
	if option, exact, ok := s.lookup(name); ok || !s.has(Abbreviations) {
		return option, exact, ok
	}
	if !strings.HasPrefix(name, "--") || strings.Trim(name, "-") == "" {
		return Option{}, false, false
	}
	found := -1
	match := func(i int) bool {
		row := s.options[i].Name
		if !strings.HasPrefix(row, "--") || !s.prefixes(row, name) {
			return true
		}
		if found >= 0 {
			found = -2
			return false
		}
		found = i
		return true
	}
	if s.has(FoldNames) {
		for _, i := range s.folded {
			if !match(i) {
				break
			}
		}
	} else {
		for i := range s.options {
			if !match(i) {
				break
			}
		}
	}
	if found < 0 {
		return Option{}, false, false
	}
	return s.options[found], false, true
}

// prefixes reports whether name is a strict prefix of row under the grammar's
// case rule.
func (s Spec) prefixes(row, name string) bool {
	if s.has(FoldNames) {
		rest, ok := foldTrimPrefix(row, name)
		return ok && rest != ""
	}
	return len(row) > len(name) && row[:len(name)] == name
}

// letter returns the single-letter row for a cluster byte. Letters match
// exactly whatever the traits.
func (s Spec) letter(c byte) (Option, bool) {
	if s.letters == nil || s.letters[c] == 0 {
		return Option{}, false
	}
	return s.options[s.letters[c]-1], true
}

// spelled reports whether a word is one of an optional option's spellings.
func (s Spec) spelled(option Option, text string) bool {
	for _, spelling := range option.Spellings {
		if s.same(spelling, text) {
			return true
		}
	}
	return false
}

// walk carries one Scan so the per-word branches can share its state.
type walk struct {
	spec   Spec
	words  []Word
	result Scan
}

func (w *walk) record(kind ProblemKind, at int) {
	if w.result.Problem == nil {
		w.result.Problem = &Problem{Kind: kind, At: at, Word: w.words[at]}
	}
}

// push appends an arg and the value word it consumed, and returns the index
// of the last word used.
func (w *walk) push(arg Arg) int {
	w.result.Args = append(w.result.Args, arg)
	if arg.ValueAt < 0 {
		return arg.At
	}
	w.result.Args = append(w.result.Args, Arg{Role: RoleValue, At: arg.ValueAt, Word: w.words[arg.ValueAt], Name: arg.Name, ValueAt: -1})
	return arg.ValueAt
}

func (w *walk) unknown(arg Arg) int {
	arg.Role = RoleUnknown
	arg.Name, arg.Exact = "", false
	w.record(UnknownOption, arg.At)
	return w.push(arg)
}

func attach(arg *Arg, value string) {
	arg.Value = arg.Word
	arg.Value.Text = value
	arg.Value.Raw = ""
	arg.HasValue = true
	arg.Joined = true
}

// separate takes the next word as the value when the option's arity allows
// it, recording a value option that ends the line.
func (w *walk) separate(arg *Arg, option Option) {
	next := arg.At + 1
	switch {
	case option.Arity == ArityValue && next == len(w.words):
		w.record(MissingValue, arg.At)
	case option.Arity == ArityValue,
		option.Arity == ArityOptional && next < len(w.words) && w.spec.spelled(option, w.words[next].Text):
		arg.Value = w.words[next]
		arg.HasValue = true
		arg.ValueAt = next
	}
}

// option reads a word that resolved to a whole row.
func (w *walk) option(arg Arg, option Option, exact bool, value string, hasEquals bool) int {
	arg.Role = RoleOption
	arg.Name = option.Name
	arg.Exact = exact
	if hasEquals {
		attach(&arg, value)
	} else {
		w.separate(&arg, option)
	}
	return w.push(arg)
}

// cluster reads a word letter by letter from its second byte. The walk is one
// pass over the bytes and Letters is a substring of the word, so a long
// hostile cluster allocates nothing per letter.
func (w *walk) cluster(arg Arg) int {
	text := arg.Word.Text
	for j := 1; j < len(text); j++ {
		option, ok := w.spec.letter(text[j])
		if !ok {
			if w.spec.has(AnyLetter) {
				arg.Name, arg.Exact = "", false
				continue
			}
			arg.Letters = text[1 : j+1]
			return w.unknown(arg)
		}
		arg.Name, arg.Exact = option.Name, true
		if option.Arity == ArityFlag {
			continue
		}
		arg.Role = RoleOption
		arg.Letters = text[1 : j+1]
		if rest := text[j+1:]; rest != "" {
			attach(&arg, rest)
		} else {
			w.separate(&arg, option)
		}
		return w.push(arg)
	}
	arg.Role = RoleOption
	arg.Letters = text[1:]
	return w.push(arg)
}

// attached reads a single-dash word that is no row under AttachedShort alone:
// only a single-letter value option may carry its value in the same word.
func (w *walk) attached(arg Arg) int {
	text := arg.Word.Text
	if option, ok := w.spec.letter(text[1]); ok && option.Arity == ArityValue && len(text) > 2 {
		arg.Role = RoleOption
		arg.Name, arg.Exact = option.Name, true
		attach(&arg, text[2:])
		return w.push(arg)
	}
	return w.unknown(arg)
}

// Scan reads every word in order. It decides no posture: an unknown or
// unreadable word is recorded and the walk continues, so every caller sees
// the whole line and applies its own rule to the problem.
func (s Spec) Scan(words []Word) Scan {
	scan, _ := s.scan(words, nil, len(words))
	return scan
}

// ScanStop reads words in order as Scan does, but stops before the first word
// at which an argument would start while options are still read and for which
// stop reports true. It returns the reading of the words before that word and
// the word's index, or len(words) when no word stops the walk. A word that an
// option consumes as its value never stops it, and neither does a word after
// the `--` that ends option reading. The reading of the words before the stop
// is what Scan reports for them, problem included, so a caller can handle a
// form the grammar cannot express and resume after it without reading any
// word twice.
func (s Spec) ScanStop(words []Word, stop func(Word) bool) (Scan, int) {
	// The result grows with the words read rather than with the words given,
	// so a caller that stops often pays only for what each walk read.
	return s.scan(words, stop, 0)
}

func (s Spec) scan(words []Word, stop func(Word) bool, capacity int) (Scan, int) {
	w := walk{spec: s, words: words}
	w.result.spec = s
	w.result.Args = make([]Arg, 0, capacity)
	ended := false
	for i := 0; i < len(words); i++ {
		word := words[i]
		if !ended && stop != nil && stop(word) {
			return w.result, i
		}
		arg := Arg{At: i, Word: word, ValueAt: -1}
		if ended || !strings.HasPrefix(word.Text, "-") {
			arg.Role = RoleOperand
			if !word.Literal {
				w.record(NotLiteral, i)
			}
			w.push(arg)
			continue
		}
		if !word.Literal {
			w.record(NotLiteral, i)
			if !s.has(ReadNonLiteral) {
				arg.Role = RoleUnknown
				w.push(arg)
				continue
			}
		}
		text := word.Text
		if text == "--" {
			arg.Role = RoleEnd
			ended = true
			w.push(arg)
			continue
		}
		// Under the short traits a single-dash word is never split at `=`:
		// `-d=@x` gives curl's -d the value `=@x`, as curl itself does.
		if len(text) >= 2 && text[1] != '-' && (s.has(Clusters) || s.has(AttachedShort)) {
			switch option, exact, ok := s.lookup(text); {
			case ok:
				i = w.option(arg, option, exact, "", false)
			case s.has(Clusters):
				i = w.cluster(arg)
			default:
				i = w.attached(arg)
			}
			continue
		}
		name, value, hasEquals := strings.Cut(text, "=")
		option, exact, ok := s.resolve(name)
		switch {
		case ok:
			i = w.option(arg, option, exact, value, hasEquals)
		case s.has(Clusters) && !s.has(Abbreviations) && strings.HasPrefix(text, "--"):
			i = w.cluster(arg)
		default:
			i = w.unknown(arg)
		}
	}
	return w.result, len(words)
}

// Command returns the words from the first operand on, operands after `--`
// included, and the first problem before that operand. With no operand it
// returns no words and any problem.
func (s Scan) Command() ([]Word, *Problem) {
	for i, arg := range s.Args {
		if arg.Role != RoleOperand {
			continue
		}
		words := make([]Word, 0, len(s.Args)-i)
		for _, rest := range s.Args[i:] {
			words = append(words, rest.Word)
		}
		if s.Problem != nil && s.Problem.At < arg.At {
			return words, s.Problem
		}
		return words, nil
	}
	return nil, s.Problem
}

// End returns the index of the `--` that ended option reading, or -1. Each
// consumer keeps its own rule for what that word means.
func (s Scan) End() int {
	for _, arg := range s.Args {
		if arg.Role == RoleEnd {
			return arg.At
		}
	}
	return -1
}

// Operands returns the operand words in order, without `--`.
func (s Scan) Operands() []Word {
	var operands []Word
	for _, arg := range s.Args {
		if arg.Role == RoleOperand {
			operands = append(operands, arg.Word)
		}
	}
	return operands
}

// Values returns the value of each occurrence of the named option that has
// one, in order.
func (s Scan) Values(name string) []Word {
	var values []Word
	for _, arg := range s.Args {
		if arg.Role == RoleOption && arg.Name == name && arg.HasValue {
			values = append(values, arg.Value)
		}
	}
	return values
}

// FlagState is what a command line says about one boolean option.
type FlagState uint8

const (
	// FlagAbsent means neither the option nor its negative occurs.
	FlagAbsent FlagState = iota
	FlagTrue
	FlagFalse
	// FlagUnknown means an occurrence could not be read as true or false.
	FlagUnknown
	// FlagConflict means occurrences read both true and false. A later
	// occurrence does not override an earlier one, because the tools that
	// consume these lines do not agree on which one wins.
	FlagConflict
)

// Flag reads a boolean option and its negative by their resolved names. A
// bare name reads true and a bare negative false. A value of `false` reads
// false, and `true` or one of the row's spellings reads true; any other value
// reads unknown, compared under the grammar's case rule. An ArityOptional
// occurrence that leaves a next word untaken reads unknown, since that word
// may have been meant as its value. Unknown wins over conflict, conflict over
// true or false. An empty negative matches nothing, and a name with no row
// reads unknown, so a misspelled table entry can never read as absent.
func (s Scan) Flag(name, negative string) FlagState {
	row, _, ok := s.spec.lookup(name)
	if !ok {
		return FlagUnknown
	}
	name = row.Name
	if negative != "" {
		if negativeRow, _, ok := s.spec.lookup(negative); ok {
			negative = negativeRow.Name
		} else {
			negative = ""
		}
	}
	nameLetter, nameIsLetter := letterOf(name)
	negativeLetter, negativeIsLetter := letterOf(negative)
	seenTrue, seenFalse, unknown := false, false, false
	for _, arg := range s.Args {
		if arg.Role != RoleOption {
			continue
		}
		// Every cluster letter before the last is a flag; the last is
		// read through Name below.
		for k := 0; k+1 < len(arg.Letters); k++ {
			switch c := arg.Letters[k]; {
			case nameIsLetter && c == nameLetter:
				seenTrue = true
			case negativeIsLetter && c == negativeLetter:
				seenFalse = true
			}
		}
		switch {
		case arg.Name == name && arg.HasValue:
			switch value := arg.Value.Text; {
			case s.spec.same(value, "false"):
				seenFalse = true
			case s.spec.same(value, "true"), s.spec.spelled(row, value):
				seenTrue = true
			default:
				unknown = true
			}
		case arg.Name == name && row.Arity == ArityOptional && arg.At+1 < len(s.Args):
			unknown = true
		case arg.Name == name:
			seenTrue = true
		case negative != "" && arg.Name == negative && arg.HasValue:
			unknown = true
		case negative != "" && arg.Name == negative:
			seenFalse = true
		}
	}
	switch {
	case unknown:
		return FlagUnknown
	case seenTrue && seenFalse:
		return FlagConflict
	case seenTrue:
		return FlagTrue
	case seenFalse:
		return FlagFalse
	}
	return FlagAbsent
}

// letterOf returns the letter of a single-letter option name, which is how
// that option appears inside a cluster.
func letterOf(name string) (byte, bool) {
	if len(name) == 2 && name[0] == '-' && name[1] != '-' {
		return name[1], true
	}
	return 0, false
}
