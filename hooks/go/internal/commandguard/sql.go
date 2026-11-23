package commandguard

import (
	"strings"
	"unicode"

	"github.com/muehz/agentics/hooks/go/internal/argv"
	"github.com/muehz/agentics/hooks/go/internal/shellwords"
	"github.com/muehz/agentics/hooks/go/internal/toolname"
)

type sqlTokenKind uint8

const (
	sqlWord sqlTokenKind = iota
	sqlIdentifier
	sqlPunctuation
)

type sqlToken struct {
	text  string
	kind  sqlTokenKind
	depth int
}

type sqlDialect string

const (
	sqlUnknown  sqlDialect = ""
	sqlPostgres sqlDialect = "psql"
	sqlMySQL    sqlDialect = "mysql"
	sqlMariaDB  sqlDialect = "mariadb"
	sqlServer   sqlDialect = "sqlcmd"
	sqlSQLite   sqlDialect = "sqlite3"
)

type sqlCandidate struct {
	text    string
	dialect sqlDialect
}

func checkDestructiveSQL(fields normalizedFields, invocations []shellwords.Invocation, report reportFunc) {
	for _, candidate := range sqlCandidates(fields, invocations) {
		dialects := []sqlDialect{candidate.dialect}
		if candidate.dialect == sqlUnknown {
			// A structured SQL tool does not identify its database dialect. No
			// one dialect's string/comment syntax can waive another's mutation.
			dialects = []sqlDialect{sqlPostgres, sqlMySQL, sqlMariaDB, sqlServer, sqlSQLite}
		}
		for _, dialect := range dialects {
			for _, statement := range tokenizeSQLDialect(candidate.text, dialect) {
				if !sqlStatementIsDestructive(statement) {
					continue
				}
				suggestion := "Add a specific WHERE clause to 'DELETE FROM' instead of deleting every row."
				if len(statement) > 0 && strings.EqualFold(statement[0].text, "truncate") {
					suggestion = "Use 'DELETE FROM ... WHERE' with a specific condition instead of truncating the table."
				}
				report(ruleDestructiveSQL, strings.TrimSpace(candidate.text), suggestion)
				return
			}
		}
	}
}

func sqlCandidates(fields normalizedFields, invocations []shellwords.Invocation) []sqlCandidate {
	var out []sqlCandidate
	if fields.commandPresent {
		for _, inv := range invocations {
			if len(inv.Words) == 0 {
				continue
			}
			dialect := sqlDialect(strings.ToLower(shellwords.ExecutableName(inv.Words[0].Text)))
			switch dialect {
			case sqlPostgres, sqlMySQL, sqlMariaDB, sqlServer, sqlSQLite:
			default:
				dialect = sqlUnknown
			}
			for _, text := range sqlArguments(inv) {
				out = append(out, sqlCandidate{text, dialect})
			}
		}
		// Bash promotes its command and deliberately ignores sibling prose. Other
		// execution-shaped tools retain exact structured SQL fields as well as
		// any literal SQL client operands.
		if !toolname.IsShell(fields.toolName) {
			if fields.command != "" {
				out = append(out, sqlCandidate{text: fields.command})
			}
			for key, child := range fields.input {
				if key != "command" {
					collectSQLFields(map[string]any{key: child}, &out)
				}
			}
		}
		return out
	}
	collectSQLFields(fields.input, &out)
	return out
}

func collectSQLFields(value any, out *[]sqlCandidate) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			switch strings.ToLower(key) {
			case "query", "sql", "statement", "command":
				if text, ok := child.(string); ok {
					*out = append(*out, sqlCandidate{text: text})
				}
			case "value", "arguments", "input", "params":
				collectSQLFields(child, out)
			}
		}
	case []any:
		for _, child := range typed {
			collectSQLFields(child, out)
		}
	}
}

// sqlLetterRows returns a single-letter row of the given arity for each byte of
// letters, so a client's table keeps the letter sets its manual lists.
func sqlLetterRows(arity argv.Arity, letters string) []argv.Option {
	rows := make([]argv.Option, 0, len(letters))
	for i := 0; i < len(letters); i++ {
		rows = append(rows, argv.Option{Name: "-" + letters[i:i+1], Arity: arity})
	}
	return rows
}

// sqlNamedRows returns one row of the given arity for each name.
func sqlNamedRows(arity argv.Arity, names ...string) []argv.Option {
	rows := make([]argv.Option, 0, len(names))
	for _, name := range names {
		rows = append(rows, argv.Option{Name: name, Arity: arity})
	}
	return rows
}

func sqlJoinRows(groups ...[]argv.Option) []argv.Option {
	var rows []argv.Option
	for _, group := range groups {
		rows = append(rows, group...)
	}
	return rows
}

// The SQL client tables list only what decides which word is SQL: the options
// that take a value, and the flag letters that may stand before a value letter
// in one cluster. A letter outside a table ends its cluster unread, which is
// how an optional-value letter such as mysql's -p ends one. The readers below
// ignore the scan's problem: a word the grammar cannot read is skipped and the
// search for a SQL operand goes on past it, so an unreadable word is never
// taken to mean that no SQL follows.
var (
	psqlOptions = sqlJoinRows(
		sqlNamedRows(argv.ArityValue, "--command", "--dbname", "--file", "--field-separator", "--host", "--log-file", "--output", "--port", "--pset", "--record-separator", "--table-attr", "--variable", "--set", "--username"),
		sqlLetterRows(argv.ArityValue, "c"+"dfFhLopPRTUv"),
		sqlLetterRows(argv.ArityFlag, "aAbeEHlnqsStVwWxXz01?"),
	)
	mysqlOptions = sqlJoinRows(
		sqlNamedRows(argv.ArityValue, "--execute", "--init-command", "--init-command-add", "--host", "--port", "--user", "--database", "--socket", "--protocol", "--ssl-ca", "--ssl-cert", "--ssl-key", "--defaults-file", "--defaults-extra-file"),
		sqlLetterRows(argv.ArityValue, "e"+"hPuSD"),
		sqlLetterRows(argv.ArityFlag, "ABbcCEfGHiIjLnNoqrstTvVUwWX?"),
	)
	// sqlcmd's /S, /D and /U, its -v, -N and -r, and sqlite's two-value options
	// are not rows: no row can express a word with no dash, a run of words
	// chosen by their text, a value that is optional by its first byte, or two
	// values. The readers stop the scan at each of them, read the form, and
	// resume the scan after it.
	sqlcmdOptions = sqlJoinRows(
		sqlLetterRows(argv.ArityValue, "Qq"+"ahHSdiostlUPwmKfFJVcyYzZ"),
		sqlLetterRows(argv.ArityFlag, "Nrv"),
	)
	sqliteOptions = sqlNamedRows(argv.ArityValue,
		"-cmd", "-init", "-separator", "-newline", "-nullvalue", "-vfs", "-nonce", "-escape", "-maxsize", "-screenwidth", "-alg", "-hexkey", "-key", "-textkey",
		"-lookaside", "-pagecache",
	)

	psqlSpec   = argv.NewSpec(psqlOptions...).With(argv.Clusters)
	mysqlSpec  = argv.NewSpec(mysqlOptions...).With(argv.Clusters)
	sqlcmdSpec = argv.NewSpec(sqlcmdOptions...).With(argv.AttachedShort)
	sqliteSpec = argv.NewSpec(sqliteOptions...)
)

// sqliteTwoValueOptions are the sqlite options that take two words when
// written without an equals sign.
var sqliteTwoValueOptions = map[string]bool{"-lookaside": true, "-pagecache": true}

// sqlArguments reads the SQL operands of a client invocation from the
// shared option walk.
func sqlArguments(inv shellwords.Invocation) []string {
	if !inv.Inspectable || len(inv.Words) < 2 || !inv.Words[0].Literal {
		return nil
	}
	name := strings.ToLower(shellwords.ExecutableName(inv.Words[0].Text))
	words := inv.Words[1:]
	for i, word := range words {
		if isPosixCommentWord(word) {
			words = words[:i]
			break
		}
	}
	client, ok := sqlClients[name]
	if !ok {
		return nil
	}
	return client(inv, words)
}

// sqlClient reads the SQL operands of one client's invocation. words are the invocation's words after the command,
// cut at a comment.
type sqlClient func(inv shellwords.Invocation, words []shellwords.Word) []string

// sqlClients holds the reader for every command the guard reads for SQL. truncate and delete have no client grammar:
// the whole line is the SQL text. Each name must be Classified in argv.
var sqlClients = map[string]sqlClient{
	"truncate": wholeLineSQL,
	"delete":   wholeLineSQL,
	"psql":     wordsSQL(psqlScanArguments),
	"mysql":    wordsSQL(mysqlScanArguments),
	"mariadb":  wordsSQL(mysqlScanArguments),
	"sqlcmd":   wordsSQL(sqlcmdScanArguments),
	"sqlite3":  wordsSQL(sqliteScanArguments),
}

func wholeLineSQL(inv shellwords.Invocation, _ []shellwords.Word) []string {
	return []string{inv.Raw}
}

// wordsSQL adapts a reader that needs only the words after the command.
func wordsSQL(read func([]shellwords.Word) []string) sqlClient {
	return func(_ shellwords.Invocation, words []shellwords.Word) []string {
		return read(words)
	}
}

func psqlScanArguments(words []shellwords.Word) []string {
	return sqlOptionValues(psqlSpec, words, "-c", "--command")
}

func mysqlScanArguments(words []shellwords.Word) []string {
	return sqlOptionValues(mysqlSpec, words, "-e", "--execute", "--init-command", "--init-command-add")
}

// sqlOptionValues returns the literal value of each occurrence of the named
// SQL options before a `--`. A value that comes from an expansion is not SQL
// the guard can read, so it is left out, as is an option with no value.
func sqlOptionValues(spec argv.Spec, words []shellwords.Word, names ...string) []string {
	scan := spec.Scan(words)
	end := scan.End()
	var out []string
	for _, arg := range scan.Args {
		if end >= 0 && arg.At >= end {
			break
		}
		if arg.Role != argv.RoleOption || !arg.HasValue || !arg.Value.Literal {
			continue
		}
		for _, name := range names {
			if arg.Name == name {
				out = append(out, arg.Value.Text)
				break
			}
		}
	}
	return out
}

// sqlcmdScanArguments walks the line once: the scan stops at each form the
// grammar cannot express, the form takes its words, and the scan resumes after
// them, so no word is read twice however many such forms the line holds.
func sqlcmdScanArguments(words []shellwords.Word) []string {
	var out []string
	for len(words) > 0 {
		scan, stop := sqlcmdSpec.ScanStop(words, sqlcmdSpecialWord)
		end := scan.End()
		for _, arg := range scan.Args {
			if end >= 0 && arg.At >= end {
				break
			}
			if arg.Role == argv.RoleOption && arg.HasValue && arg.Value.Literal && (arg.Name == "-Q" || arg.Name == "-q") {
				out = append(out, arg.Value.Text)
			}
		}
		if stop == len(words) {
			break
		}
		words = words[sqlcmdSpecialForm(words, stop):]
	}
	return out
}

// sqlcmdSpecialWord reports a word that, where an argument starts, begins one
// of the sqlcmd forms the grammar cannot express. sqlcmdSpec reads such a word
// as an operand or as exactly that option, so the word alone decides.
func sqlcmdSpecialWord(word shellwords.Word) bool {
	if !word.Literal {
		return false
	}
	switch text := word.Text; {
	case len(text) == 2 && text[0] == '/' && strings.ContainsRune("SDU", rune(text[1])):
		return true
	case text == "-N", text == "-r", text == "-v":
		return true
	}
	return false
}

// sqlcmdSpecialForm returns where the words that the form starting at at
// leaves unread begin.
func sqlcmdSpecialForm(words []shellwords.Word, at int) int {
	rest := at + 1
	switch words[at].Text {
	case "-N", "-r":
		// The Go and ODBC clients disagree on whether these take a value, so
		// a following word that is not an option is read as theirs and a
		// following option stays visible.
		if rest < len(words) && !strings.HasPrefix(words[rest].Text, "-") {
			rest++
		}
		return rest
	case "-v":
		// The variable option takes every following word that assigns one.
		for rest < len(words) && strings.Contains(words[rest].Text, "=") && !strings.HasPrefix(words[rest].Text, "-") {
			rest++
		}
		return rest
	}
	// The slash forms of the server, database and user options take the
	// next word whatever it is.
	return min(rest+1, len(words))
}

// sqliteSpecialWord reports a word that, where an argument starts, may begin
// a form the grammar cannot express: a two-value option written without an
// equals sign, -cmd (whose value the guard may be unable to read), and a `--`
// from an expansion. sqliteSpec reads a literal such word as exactly that
// option.
func sqliteSpecialWord(word shellwords.Word) bool {
	if !word.Literal {
		return word.Text == "--"
	}
	return word.Text == "-cmd" || sqliteTwoValueOptions[word.Text]
}

// sqliteScanArguments walks the line once, as sqlcmdScanArguments does.
func sqliteScanArguments(words []shellwords.Word) []string {
	var out []string
	positionals := make([]shellwords.Word, 0, len(words))
	for len(words) > 0 {
		scan, stop := sqliteSpec.ScanStop(words, sqliteSpecialWord)
		for _, arg := range scan.Args {
			switch arg.Role {
			case argv.RoleOperand:
				positionals = append(positionals, arg.Word)
			case argv.RoleUnknown:
				// A word from an expansion may be anything, so it holds a
				// positional's place; a literal option the grammar does not
				// know is skipped.
				if !arg.Word.Literal {
					positionals = append(positionals, arg.Word)
				}
			case argv.RoleOption:
				if arg.Name == "-cmd" && arg.HasValue && arg.Value.Literal {
					out = append(out, arg.Value.Text)
				}
			}
		}
		if stop == len(words) {
			break
		}
		word, rest := words[stop], stop+1
		switch {
		case !word.Literal:
			// A `--` from an expansion may end the options, so every word
			// after it holds a positional's place.
			positionals = append(positionals, words[rest:]...)
			rest = len(words)
		case word.Text == "-cmd" && rest < len(words) && words[rest].Literal:
			out = append(out, words[rest].Text)
			rest++
		case word.Text == "-cmd":
			// A value from an expansion is not SQL the guard can read and is
			// not consumed, so it is read again as an ordinary word.
		case rest < len(words):
			// A two-value option takes the next two words whatever they are.
			rest = min(rest+2, len(words))
		}
		words = words[rest:]
	}
	// sqlite3's first positional is the database filename. SQL starts at the
	// second positional; dynamic words remain outside this literal scanner.
	if len(positionals) > 1 {
		for _, word := range positionals[1:] {
			if word.Literal {
				out = append(out, word.Text)
			}
		}
	}
	return out
}

func tokenizeSQLDialect(input string, dialect sqlDialect) [][]sqlToken {
	mysql := dialect == sqlMySQL || dialect == sqlMariaDB
	var statements [][]sqlToken
	var current []sqlToken
	depth := 0
	flush := func() {
		if len(current) > 0 {
			statements = append(statements, current)
			current = nil
		}
	}
	for i := 0; i < len(input); {
		if unicode.IsSpace(rune(input[i])) {
			i++
			continue
		}
		if mysql && input[i] == '\\' && i+1 < len(input) && (input[i+1] == 'g' || input[i+1] == 'G') {
			flush()
			depth = 0
			i += 2
			continue
		}
		dashComment := input[i] == '-' && i+1 < len(input) && input[i+1] == '-'
		if mysql && dashComment {
			dashComment = i+2 == len(input) || input[i+2] <= ' '
		}
		if dashComment || mysql && input[i] == '#' {
			for i < len(input) && input[i] != '\n' && input[i] != '\r' {
				i++
			}
			continue
		}
		if input[i] == '/' && i+1 < len(input) && input[i+1] == '*' {
			if mysql && strings.HasPrefix(input[i:], "/*!") || dialect == sqlMariaDB && strings.HasPrefix(input[i:], "/*M!") {
				if input[i+2] == 'M' {
					i++
				}
				i += 3
				for i < len(input) && input[i] >= '0' && input[i] <= '9' {
					i++
				}
				continue
			}
			i = skipSQLBlockComment(input, i, dialect == sqlPostgres || dialect == sqlServer)
			continue
		}
		if mysql && input[i] == '"' {
			start := i
			i = skipSQLQuoted(input, i, '"', '\\')
			// Retain a possible quoted identifier without exposing any keyword
			// inside the span, whether the server treats it as data or a name.
			current = append(current, sqlToken{text: input[start:i], kind: sqlIdentifier, depth: depth})
			continue
		}
		if input[i] == '\'' {
			var escape byte
			if mysql {
				escape = '\\'
			}
			i = skipSQLQuoted(input, i, input[i], escape)
			continue
		}
		if dialect == sqlPostgres && (input[i] == 'E' || input[i] == 'e') && i+1 < len(input) && input[i+1] == '\'' {
			i = skipSQLQuoted(input, i+1, '\'', '\\')
			continue
		}
		if dialect == sqlPostgres && input[i] == '$' {
			if delimiter, end := sqlDollarDelimiter(input, i); end > i {
				if close := strings.Index(input[end:], delimiter); close >= 0 {
					i = end + close + len(delimiter)
				} else {
					i = len(input)
				}
				continue
			}
		}
		if input[i] == '"' || input[i] == '`' || input[i] == '[' {
			close := input[i]
			if input[i] == '[' {
				close = ']'
			}
			var identifier string
			i, identifier = readSQLIdentifier(input, i, close)
			current = append(current, sqlToken{text: identifier, kind: sqlIdentifier, depth: depth})
			continue
		}
		if isSQLWordByte(input[i]) {
			start := i
			for i < len(input) && isSQLWordByte(input[i]) && !(mysql && input[i] == '#') {
				i++
			}
			current = append(current, sqlToken{text: input[start:i], kind: sqlWord, depth: depth})
			continue
		}
		switch input[i] {
		case ';':
			if depth == 0 {
				flush()
			} else {
				current = append(current, sqlToken{text: ";", kind: sqlPunctuation, depth: depth})
			}
		case '(':
			current = append(current, sqlToken{text: "(", kind: sqlPunctuation, depth: depth})
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
			current = append(current, sqlToken{text: ")", kind: sqlPunctuation, depth: depth})
		case '.':
			current = append(current, sqlToken{text: ".", kind: sqlPunctuation, depth: depth})
		}
		i++
	}
	flush()
	return statements
}

func skipSQLBlockComment(input string, start int, nested bool) int {
	depth := 1
	for i := start + 2; i < len(input); {
		switch {
		case nested && i+1 < len(input) && input[i] == '/' && input[i+1] == '*':
			depth++
			i += 2
		case i+1 < len(input) && input[i] == '*' && input[i+1] == '/':
			depth--
			i += 2
			if depth == 0 {
				return i
			}
		default:
			i++
		}
	}
	return len(input)
}

func skipSQLQuoted(input string, start int, quote, escape byte) int {
	for i := start + 1; i < len(input); i++ {
		if escape != 0 && input[i] == escape && i+1 < len(input) {
			i++
			continue
		}
		if input[i] != quote {
			continue
		}
		if i+1 < len(input) && input[i+1] == quote {
			i++
			continue
		}
		return i + 1
	}
	return len(input)
}

func sqlDollarDelimiter(input string, start int) (string, int) {
	end := strings.IndexByte(input[start+1:], '$')
	if end < 0 {
		return "", start
	}
	end += start + 1
	tag := input[start+1 : end]
	if tag != "" {
		if !isSQLIdentStart(tag[0]) {
			return "", start
		}
		for i := 1; i < len(tag); i++ {
			if !isSQLWordByte(tag[i]) {
				return "", start
			}
		}
	}
	return input[start : end+1], end + 1
}

func readSQLIdentifier(input string, start int, close byte) (int, string) {
	i := start + 1
	var identifier strings.Builder
	for i < len(input) {
		if input[i] == close {
			if i+1 < len(input) && input[i+1] == close {
				identifier.WriteByte(close)
				i += 2
				continue
			}
			return i + 1, identifier.String()
		}
		identifier.WriteByte(input[i])
		i++
	}
	return i, identifier.String()
}

func isSQLIdentStart(char byte) bool {
	return char == '_' || char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z'
}

func isSQLWordByte(char byte) bool {
	return char == '_' || char == '$' || char == '#' || char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9'
}

func sqlStatementIsDestructive(tokens []sqlToken) bool {
	if len(tokens) == 0 || tokens[0].kind != sqlWord {
		return false
	}
	switch {
	case strings.EqualFold(tokens[0].text, "with"):
		for i, token := range tokens {
			if token.kind == sqlWord && strings.EqualFold(token.text, "delete") && sqlDeleteIsDestructive(tokens[i:]) {
				return true
			}
		}
		return false
	case strings.EqualFold(tokens[0].text, "truncate"):
		i := 1
		for i < len(tokens) && tokens[i].kind == sqlWord && (strings.EqualFold(tokens[i].text, "table") || strings.EqualFold(tokens[i].text, "only")) {
			i++
		}
		return i < len(tokens) && (tokens[i].kind == sqlWord || tokens[i].kind == sqlIdentifier)
	case strings.EqualFold(tokens[0].text, "delete"):
		return sqlDeleteIsDestructive(tokens)
	}
	return false
}

func sqlDeleteIsDestructive(tokens []sqlToken) bool {
	if len(tokens) < 3 || tokens[1].kind != sqlWord || !strings.EqualFold(tokens[1].text, "from") || (tokens[2].kind != sqlWord && tokens[2].kind != sqlIdentifier) {
		return false
	}
	depth := tokens[0].depth
	if tokens[1].depth != depth || tokens[2].depth != depth {
		return false
	}
	for _, token := range tokens[3:] {
		if token.depth < depth || token.depth == depth && token.text == ";" {
			break
		}
		if token.kind == sqlWord && token.depth == depth && strings.EqualFold(token.text, "where") {
			return false
		}
	}
	return true
}
