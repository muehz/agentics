package commandguard

import (
	"strings"
	"testing"
	"time"

	"github.com/muehz/agentics/hooks/go/internal/shellwords"
)

// tokenizeSQL tokenizes with the default dialect the tests pin.
func tokenizeSQL(input string) [][]sqlToken {
	return tokenizeSQLDialect(input, sqlPostgres)
}

func TestSQLStatementPolicyIsLexicalAndStatementScoped(t *testing.T) {
	tests := []struct {
		name string
		text string
		want bool
	}{
		{"schema qualified delete", `DELETE FROM "app"."users"`, true},
		{"truncate with only", `TRUNCATE ONLY public.users`, true},
		{"scoped delete", `DELETE FROM app.users WHERE id = 1`, false},
		{"where in a later statement", `DELETE FROM users; SELECT 'where'`, true},
		{"where in a comment", `DELETE FROM users /* WHERE id = 1 */`, true},
		{"where in a line comment", "DELETE FROM users -- WHERE id = 1\n", true},
		{"where in a nested comment", `/* outer /* WHERE */ still */ DELETE FROM users`, true},
		{"where in a dollar quote", `DELETE FROM users RETURNING $tag$ WHERE $tag$`, true},
		{"where inside parentheses", `DELETE FROM users AND (WHERE id = 1)`, true},
		{"where after parentheses", `DELETE FROM users WHERE (id IN (SELECT id FROM t))`, false},
		{"quoted where is data", `DELETE FROM users "WHERE"`, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			statements := tokenizeSQL(test.text)
			if len(statements) == 0 {
				t.Fatal("tokenizer returned no statements")
			}
			got := false
			for _, statement := range statements {
				got = got || sqlStatementIsDestructive(statement)
			}
			if got != test.want {
				t.Fatalf("destructive = %v, want %v; statements = %#v", got, test.want, statements)
			}
		})
	}
}

func TestSQLCandidatesRespectClientOptionsAndLiteralProvenance(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    string
	}{
		{"psql data option before command", `psql --dbname "DELETE FROM ignored" -c "DELETE FROM users"`, "DELETE FROM users"},
		{"psql attached short command", `psql -c"DELETE FROM users"`, "DELETE FROM users"},
		{"psql HTML flag", `psql -H -c "DELETE FROM users"`, "DELETE FROM users"},
		{"psql short option cluster", `psql -XAc "DELETE FROM users"`, "DELETE FROM users"},
		{"psql cluster data operand", `psql -Ah -c "DELETE FROM ignored"`, ""},
		{"psql password prompt", `psql --password -c "DELETE FROM users"`, "DELETE FROM users"},
		{"psql attached host", `psql -hlocalhost -c "DELETE FROM users"`, "DELETE FROM users"},
		{"psql command is one operand", `psql -cSELECT database WHERE`, "SELECT"},
		{"psql username data", `psql -U -c "DELETE FROM ignored"`, ""},
		{"psql comment", `psql db #comment -c "DELETE FROM ignored"`, ""},
		{"comment at a value boundary", `psql --file #comment -c "DELETE FROM ignored"`, ""},
		{"mysql data option before execute", `mysql --host "DELETE FROM ignored" -e "DELETE FROM users"`, "DELETE FROM users"},
		{"mysql password prompt", `mysql -p -e "DELETE FROM users"`, "DELETE FROM users"},
		{"mysql short option cluster", `mysql -Ne "DELETE FROM users"`, "DELETE FROM users"},
		{"mysql attached host", `mysql -hlocalhost -e "DELETE FROM users"`, "DELETE FROM users"},
		{"sqlcmd data option before query", `sqlcmd -d "DELETE FROM ignored" -Q "DELETE FROM users"`, "DELETE FROM users"},
		{"sqlcmd integrated authentication", `sqlcmd -E -Q "DELETE FROM users"`, "DELETE FROM users"},
		{"sqlite init value is not SQL", `sqlite3 -init "DELETE FROM ignored" db "DELETE FROM users"`, "DELETE FROM users"},
		{"sqlite cmd option", `sqlite3 -cmd "DELETE FROM users" db`, "DELETE FROM users"},
		{"dynamic command operand is not literal", `psql -c "$SQL"`, ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			invocations := shellwords.Parse(test.command, shellwords.POSIX).Invocations
			candidates := sqlCandidates(normalizedFields{toolName: "UnknownExecutor", commandPresent: true}, invocations)
			var texts []string
			for _, candidate := range candidates {
				texts = append(texts, candidate.text)
			}
			if got := strings.Join(texts, "\x00"); got != test.want {
				t.Fatalf("candidates = %q, want %q; invocations = %#v", got, test.want, invocations)
			}
		})
	}
}

func TestSQLCandidatesKeepStructuredFieldsAlongsideUnknownCommands(t *testing.T) {
	fields := normalizedFields{
		toolName:       "UnknownExecutor",
		commandPresent: true,
		input: map[string]any{
			"command": "psql -c 'DELETE FROM users'",
			"value":   map[string]any{"query": "TRUNCATE TABLE audit"},
		},
	}
	fields.command = fields.input["command"].(string)
	candidates := sqlCandidates(fields, shellwords.Parse(fields.command, shellwords.POSIX).Invocations)
	if len(candidates) != 3 || candidates[0].text != "DELETE FROM users" || candidates[1].text != fields.command || candidates[2].text != "TRUNCATE TABLE audit" {
		t.Fatalf("unknown executor candidates = %#v", candidates)
	}

	fields.toolName = "Bash"
	candidates = sqlCandidates(fields, shellwords.Parse(fields.command, shellwords.POSIX).Invocations)
	if len(candidates) != 1 || candidates[0].text != "DELETE FROM users" {
		t.Fatalf("Bash candidates = %#v, want only promoted command SQL", candidates)
	}
}

func TestSQLClientArityReviewControls(t *testing.T) {
	for _, flag := range []string{"-F", "-J", "-V", "-c", "-y", "-Y", "-z", "-Z"} {
		command := "sqlcmd " + flag + ` -Q "DELETE FROM ignored"`
		inv := shellwords.Parse(command, shellwords.POSIX).Invocations[0]
		if got := sqlArguments(inv); len(got) != 0 {
			t.Errorf("%s: operand became SQL: %#v", flag, got)
		}
	}
	for _, flag := range []string{"A", "B", "b", "c", "C", "E", "f", "G", "H", "i", "j", "L", "n", "N", "o", "q", "r", "s", "t", "T", "U", "v", "w", "W", "X"} {
		command := "mysql -" + flag + `e "DELETE FROM users"`
		inv := shellwords.Parse(command, shellwords.POSIX).Invocations[0]
		if got := sqlArguments(inv); len(got) != 1 || got[0] != "DELETE FROM users" {
			t.Errorf("%s: query hidden: %#v", flag, got)
		}
	}
	for _, command := range []string{`sqlcmd -N -Q "DELETE FROM users"`, `sqlcmd -r -Q "DELETE FROM users"`, `sqlcmd -N false -Q "DELETE FROM users"`, `sqlcmd -v "X=-Q" "Y=data" -Q "DELETE FROM users"`} {
		inv := shellwords.Parse(command, shellwords.POSIX).Invocations[0]
		if got := sqlArguments(inv); len(got) != 1 || got[0] != "DELETE FROM users" {
			t.Errorf("query hidden: %#v", got)
		}
	}
}

func TestMySQLDoubleQuotedEscapesRemainStringData(t *testing.T) {
	for _, statement := range tokenizeSQLDialect(`SELECT "quoted\"; DELETE FROM users"`, sqlMySQL) {
		if sqlStatementIsDestructive(statement) {
			t.Fatal("SQL inside escaped double-quoted string became executable")
		}
	}
	if statements := tokenizeSQLDialect(`DELETE FROM "users"`, sqlMySQL); len(statements) != 1 || !sqlStatementIsDestructive(statements[0]) {
		t.Fatal("possible double-quoted table name lost its identifier token")
	}
}

func TestSQLUnknownDialectCannotHideMutation(t *testing.T) {
	invocations := shellwords.Parse("DELETE FROM users #comment WHERE id=1", shellwords.POSIX).Invocations
	for _, candidate := range sqlCandidates(normalizedFields{toolName: "UnknownExecutor", commandPresent: true}, invocations) {
		if candidate.dialect != sqlUnknown {
			t.Errorf("direct SQL falsely tagged with client dialect %q", candidate.dialect)
		}
	}
	for _, query := range []string{`SELECT '\'; DELETE FROM users`, `DELETE FROM #temp`, `/*! DELETE FROM users */`, `WITH x AS (DELETE FROM users RETURNING *) SELECT * FROM x WHERE id=1`} {
		found := false
		checkDestructiveSQL(normalizedFields{input: map[string]any{"query": query}}, nil, func(rule, string, string) { found = true })
		if !found {
			t.Errorf("unknown-dialect mutation hidden: %q", query)
		}
	}
}

func TestSQLClientOptionFormsCoverSupportedOperands(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    string
	}{
		{"psql long command", `psql --command "DELETE FROM users"`, "DELETE FROM users"},
		{"psql joined command", `psql --command="DELETE FROM users"`, "DELETE FROM users"},
		{"psql option terminator", `psql -- --command "DELETE FROM users"`, ""},
		{"psql valued option equals", `psql --dbname=demo --command "DELETE FROM users"`, "DELETE FROM users"},
		{"psql command without operand", `psql -c`, ""},
		{"mysql long execute", `mysql --execute "DELETE FROM users"`, "DELETE FROM users"},
		{"mysql joined execute", `mysql --execute="DELETE FROM users"`, "DELETE FROM users"},
		{"mysql init command add", `mysql --init-command-add "DELETE FROM users"`, "DELETE FROM users"},
		{"mysql valued option equals", `mysql --host=demo --execute "DELETE FROM users"`, "DELETE FROM users"},
		{"mysql option terminator", `mysql -- --execute "DELETE FROM users"`, ""},
		{"mysql execute without operand", `mysql -e`, ""},
		{"mariadb execute", `mariadb --execute "DELETE FROM users"`, "DELETE FROM users"},
		{"sqlcmd lower query", `sqlcmd -q "DELETE FROM users"`, "DELETE FROM users"},
		{"sqlcmd attached upper query", `sqlcmd -Q"DELETE FROM users"`, "DELETE FROM users"},
		{"sqlcmd query without operand", `sqlcmd -Q`, ""},
		{"sqlcmd option terminator", `sqlcmd -- -Q "DELETE FROM users"`, ""},
		{"sqlcmd slash valued option", `sqlcmd /S server -Q "DELETE FROM users"`, "DELETE FROM users"},
		{"sqlite joined command", `sqlite3 -cmd="DELETE FROM users" db`, "DELETE FROM users"},
		{"sqlite option terminator", `sqlite3 -- db "DELETE FROM users"`, "DELETE FROM users"},
		{"sqlite two valued option operands", `sqlite3 -lookaside one two db "DELETE FROM users"`, "DELETE FROM users"},
		{"sqlite equal valued option", `sqlite3 -lookaside=one db "DELETE FROM users"`, "DELETE FROM users"},
		{"sqlite database only", `sqlite3 db`, ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			invocations := shellwords.Parse(test.command, shellwords.POSIX).Invocations
			candidates := sqlCandidates(normalizedFields{toolName: "UnknownExecutor", commandPresent: true}, invocations)
			var texts []string
			for _, candidate := range candidates {
				texts = append(texts, candidate.text)
			}
			if got := strings.Join(texts, "\x00"); got != test.want {
				t.Fatalf("candidates = %q, want %q; invocations = %#v", got, test.want, invocations)
			}
		})
	}
}

func TestSQLStructuredFieldsTraverseArraysAndReportSuggestions(t *testing.T) {
	var reports []threat
	checkDestructiveSQL(normalizedFields{input: map[string]any{
		"params": []any{
			map[string]any{"query": "SELECT 1"},
			map[string]any{"statement": "TRUNCATE TABLE audit"},
		},
	}}, nil, func(r rule, match, suggestion string) {
		reports = append(reports, threat{rule: r, match: match, suggestion: suggestion})
	})
	if len(reports) != 1 || reports[0].rule.category != "database_destruction" || reports[0].rule.severity != "critical" || reports[0].match != "TRUNCATE TABLE audit" || !strings.Contains(reports[0].suggestion, "truncating") {
		t.Fatalf("structured SQL reports = %#v", reports)
	}

	reports = nil
	checkDestructiveSQL(normalizedFields{input: map[string]any{"query": "DELETE FROM audit WHERE id = 1"}}, nil, func(r rule, match, suggestion string) {
		reports = append(reports, threat{rule: r, match: match, suggestion: suggestion})
	})
	if len(reports) != 0 {
		t.Fatalf("scoped DELETE reported as destructive: %#v", reports)
	}
}

func TestSQLLexicalSafetyAcrossDialects(t *testing.T) {
	tests := []struct {
		name       string
		command    string
		wantReport bool
	}{
		{"mysql escaped string", `mysql -e "SELECT 'quoted\\'; DELETE FROM users'"`, false},
		{"mysql hash comment", "mysql -e 'DELETE FROM users # WHERE id=1\n'", true},
		{"mysql executable comment", `mysql -e "/*!40101 DELETE FROM users */"`, true},
		{"mariadb executable comment", `mariadb -e "/*M!123 DELETE FROM users */"`, true},
		{"mysql go terminator", `mysql -e "SELECT 1\\G DELETE FROM users"`, true},
		{"sqlcmd bracket identifier", `sqlcmd -Q "DELETE FROM [users]"`, true},
		{"postgres escaped string", `psql -c "SELECT E'quoted\\'; DELETE FROM users'"`, false},
		{"postgres unterminated dollar quote", `psql -c "SELECT $tag$ DELETE FROM users"`, false},
		{"postgres invalid dollar marker", `psql -c 'SELECT $1; DELETE FROM users'`, true},
		{"nested comment before delete", `psql -c "/* outer /* inert */ still */ DELETE FROM users"`, true},
		{"unterminated block comment", `psql -c "/* inert DELETE FROM users"`, false},
		{"parenthesized semicolon", `psql -c "SELECT (DELETE FROM users; SELECT 1)"`, false},
		{"doubled quoted identifier", "sqlcmd -Q \"DELETE FROM [user]]s]\"", true},
		{"unterminated quoted identifier", `sqlcmd -Q "DELETE FROM [users"`, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			invocations := shellwords.Parse(test.command, shellwords.POSIX).Invocations
			reported := false
			checkDestructiveSQL(normalizedFields{toolName: "UnknownExecutor", commandPresent: true}, invocations, func(rule, string, string) {
				reported = true
			})
			if reported != test.wantReport {
				t.Fatalf("reported = %v, want %v; invocations = %#v", reported, test.wantReport, invocations)
			}
		})
	}
}

func TestSQLMalformedAndEmptyInvocationsHaveNoCandidates(t *testing.T) {
	if got := sqlCandidates(normalizedFields{commandPresent: true}, []shellwords.Invocation{{}}); len(got) != 0 {
		t.Fatalf("empty invocation candidates = %#v", got)
	}
	for _, invocation := range []shellwords.Invocation{
		{Words: []shellwords.Word{{Text: "psql", Literal: true}}, Inspectable: true},
		{Words: []shellwords.Word{{Text: "psql", Literal: true}, {Text: "-c", Literal: true}}, Inspectable: true},
	} {
		if got := sqlArguments(invocation); len(got) != 0 {
			t.Fatalf("malformed SQL invocation candidates = %#v", got)
		}
	}
}

// TestSQLReadersAreLinear: a client line packed with the forms no grammar row
// expresses must still be read in one pass. A reader that rescans the rest of
// the line at each such form costs time quadratic in their number, and a line
// of about 100 KB would then outlast the hook timeout, after which a host may
// run the tool without the deny.
func TestSQLReadersAreLinear(t *testing.T) {
	const bound = time.Second
	for _, c := range []struct {
		name, command, repeat, tail string
		count                       int
	}{
		{"sqlcmd -N", "sqlcmd", " -N", " -Q 'DELETE FROM t'", 30000},
		{"sqlite3 -lookaside", "sqlite3", " -lookaside 1 2", " db 'DELETE FROM t'", 20000},
	} {
		t.Run(c.name, func(t *testing.T) {
			small := c.command + c.repeat + c.tail
			hostile := c.command + strings.Repeat(c.repeat, c.count) + c.tail
			want := sqlArguments(shellwords.Parse(small, shellwords.POSIX).Invocations[0])
			if len(want) != 1 || want[0] != "DELETE FROM t" {
				t.Fatalf("small line %q reads %q, want the DELETE", small, want)
			}
			start := time.Now()
			got := sqlArguments(shellwords.Parse(hostile, shellwords.POSIX).Invocations[0])
			if elapsed := time.Since(start); elapsed > bound {
				t.Fatalf("reading %d repeated forms took %v, want under %v", c.count, elapsed, bound)
			}
			if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
				t.Fatalf("hostile line reads %q, want %q as the small line does", got, want)
			}
			start = time.Now()
			assertGhOutcome(t, "Bash", hostile, "deny", "database_destruction")
			if elapsed := time.Since(start); elapsed > bound {
				t.Fatalf("the guard took %v on %d repeated forms, want under %v", elapsed, c.count, bound)
			}
		})
	}
}
