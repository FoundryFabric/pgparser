package parser

import (
	"errors"
	"strings"
	"testing"
)

// lexErrorCases are statements PostgreSQL 16's scanner (scan.l) or its U&
// decoder (parser.c's str_udeescape) refuses, each with the SQLSTATE, 1-based
// position, message and hint PostgreSQL 16.4 answered (verified 2026-10-12).
// The fork's Parse dropped every one of these lexer errors: a malformed
// literal parsed as a SELECT with no target, and 1_, 1__0 and 0x1__0 as
// numbers.
var lexErrorCases = []struct {
	sql, code string
	position  int
	message   string
	hint      string
}{
	// malformed numeric literals
	{"select 0x", "42601", 8, `invalid hexadecimal integer at or near "0x"`, ""},
	{"select 0x_", "42601", 8, `invalid hexadecimal integer at or near "0x_"`, ""},
	{"select 0b", "42601", 8, `invalid binary integer at or near "0b"`, ""},
	{"select 0o", "42601", 8, `invalid octal integer at or near "0o"`, ""},
	{"select 0o8", "42601", 8, `trailing junk after numeric literal at or near "0o8"`, ""},
	{"select 0b2", "42601", 8, `trailing junk after numeric literal at or near "0b2"`, ""},
	{"select 0x1g", "42601", 8, `trailing junk after numeric literal at or near "0x1g"`, ""},
	{"select 1e", "42601", 8, `trailing junk after numeric literal at or near "1e"`, ""},
	{"select 123abc", "42601", 8, `trailing junk after numeric literal at or near "123abc"`, ""},
	{"select 1_", "42601", 8, `trailing junk after numeric literal at or near "1_"`, ""},
	{"select 1__0", "42601", 8, `trailing junk after numeric literal at or near "1__0"`, ""},
	{"select 0x1__0", "42601", 8, `trailing junk after numeric literal at or near "0x1__0"`, ""},
	{"select 1_000_", "42601", 8, `trailing junk after numeric literal at or near "1_000_"`, ""},
	{"select 1._5", "42601", 8, `trailing junk after numeric literal at or near "1._5"`, ""},
	{"select 1.5abc", "42601", 8, `trailing junk after numeric literal at or near "1.5abc"`, ""},
	{"select 1.5e+", "42601", 8, `trailing junk after numeric literal at or near "1.5e+"`, ""},
	{"select 1e5x", "42601", 8, `trailing junk after numeric literal at or near "1e5x"`, ""},
	{"select .5e", "42601", 8, `trailing junk after numeric literal at or near ".5e"`, ""},
	{"select $1abc", "42601", 8, `trailing junk after parameter at or near "$1abc"`, ""},
	{"select not 0x", "42601", 12, `invalid hexadecimal integer at or near "0x"`, ""},
	// unterminated strings, identifiers and comments
	{"select 'abc", "42601", 8, `unterminated quoted string at or near "'abc"`, ""},
	{"select E'abc", "42601", 8, `unterminated quoted string at or near "E'abc"`, ""},
	{"select $$abc", "42601", 8, `unterminated dollar-quoted string at or near "$$abc"`, ""},
	{`select "abc`, "42601", 8, `unterminated quoted identifier at or near ""abc"`, ""},
	{"select 1 /* x", "42601", 10, `unterminated /* comment at or near "/* x"`, ""},
	{"select 1 /*", "42601", 10, `unterminated /* comment at or near "/*"`, ""},
	{"select 1 +/*", "42601", 11, `unterminated /* comment at or near "/*"`, ""},
	// a literal whose opening is the last byte of the input (16.15, 2026-10-20)
	{"select '", "42601", 8, `unterminated quoted string at or near "'"`, ""},
	{`select "`, "42601", 8, `unterminated quoted identifier at or near """`, ""},
	{"select $$", "42601", 8, `unterminated dollar-quoted string at or near "$$"`, ""},
	{"select $tag$", "42601", 8, `unterminated dollar-quoted string at or near "$tag$"`, ""},
	{"select $_$", "42601", 8, `unterminated dollar-quoted string at or near "$_$"`, ""},
	{"select $tag$ x", "42601", 8, `unterminated dollar-quoted string at or near "$tag$ x"`, ""},
	{"select $a$ x $b$", "42601", 8, `unterminated dollar-quoted string at or near "$a$ x $b$"`, ""},
	{"select E'", "42601", 8, `unterminated quoted string at or near "E'"`, ""},
	{"select e'", "42601", 8, `unterminated quoted string at or near "e'"`, ""},
	{"select B'", "42601", 8, `unterminated bit string literal at or near "B'"`, ""},
	{"select b'", "42601", 8, `unterminated bit string literal at or near "b'"`, ""},
	{"select X'", "42601", 8, `unterminated hexadecimal string literal at or near "X'"`, ""},
	{"select x'", "42601", 8, `unterminated hexadecimal string literal at or near "x'"`, ""},
	{"select N'", "42601", 9, `unterminated quoted string at or near "'"`, ""},
	{"select n'", "42601", 9, `unterminated quoted string at or near "'"`, ""},
	{"select U&'", "42601", 8, `unterminated quoted string at or near "U&'"`, ""},
	{"select u&'", "42601", 8, `unterminated quoted string at or near "u&'"`, ""},
	{`select U&"`, "42601", 8, `unterminated quoted identifier at or near "U&""`, ""},
	{`select u&"`, "42601", 8, `unterminated quoted identifier at or near "u&""`, ""},
	{"select 1, '", "42601", 11, `unterminated quoted string at or near "'"`, ""},
	{"select 'a' '", "42601", 12, `unterminated quoted string at or near "'"`, ""},
	{"select 'a'\n'", "42601", 8, "unterminated quoted string at or near \"'a'\n'\"", ""}, // a continuation
	{"select B'01", "42601", 8, `unterminated bit string literal at or near "B'01"`, ""},
	{"select X'0f", "42601", 8, `unterminated hexadecimal string literal at or near "X'0f"`, ""},
	{`select ""`, "42601", 8, `zero-length delimited identifier at or near """"`, ""},
	// E'' Unicode escapes (scan.l's xeunicode, xeunicodefail, xeu, addunicode)
	{`select E'\u12'`, "22025", 10, "invalid Unicode escape", `Unicode escapes must be \uXXXX or \UXXXXXXXX.`},
	{`select E'\u'`, "22025", 10, "invalid Unicode escape", `Unicode escapes must be \uXXXX or \UXXXXXXXX.`},
	{`select E'\U0001'`, "22025", 10, "invalid Unicode escape", `Unicode escapes must be \uXXXX or \UXXXXXXXX.`},
	{`select E'\uD800\u12'`, "22025", 16, "invalid Unicode escape", `Unicode escapes must be \uXXXX or \UXXXXXXXX.`},
	{`select E'\u0000'`, "42601", 10, `invalid Unicode escape value at or near "\u0000"`, ""},
	{`select E'\U00110000'`, "42601", 10, `invalid Unicode escape value at or near "\U00110000"`, ""},
	{`select E'\uD800'`, "42601", 16, `invalid Unicode surrogate pair at or near "'"`, ""},
	{`select E'\uD800x'`, "42601", 16, `invalid Unicode surrogate pair at or near "x"`, ""},
	{`select E'\uD800\u0041'`, "42601", 16, `invalid Unicode surrogate pair at or near "\u0041"`, ""},
	{`select E'\uDC00'`, "42601", 10, `invalid Unicode surrogate pair at or near "\uDC00"`, ""},
	// U&'' escapes (parser.c's str_udeescape and check_unicode_value)
	{`select U&'\zz'`, "42601", 11, "invalid Unicode escape", `Unicode escapes must be \XXXX or \+XXXXXX.`},
	{`select U&'a\'`, "42601", 12, "invalid Unicode escape", `Unicode escapes must be \XXXX or \+XXXXXX.`},
	{`select U&'''\zz'`, "42601", 12, "invalid Unicode escape", `Unicode escapes must be \XXXX or \+XXXXXX.`},
	{`select ''''||U&'\zz'`, "42601", 17, "invalid Unicode escape", `Unicode escapes must be \XXXX or \+XXXXXX.`},
	{`select U&'\0000'`, "42601", 11, "invalid Unicode escape value", ""},
	{`select U&'\+110000'`, "42601", 11, "invalid Unicode escape value", ""},
	{`select U&'\D800'`, "42601", 16, "invalid Unicode surrogate pair", ""},
	{`select U&'\D800\0041'`, "42601", 16, "invalid Unicode surrogate pair", ""},
	{`select U&'\D800\\'`, "42601", 16, "invalid Unicode surrogate pair", ""},
	{`select U&'\DC00'`, "42601", 11, "invalid Unicode surrogate pair", ""},
	{`select U&"\zz"`, "42601", 11, "invalid Unicode escape", `Unicode escapes must be \XXXX or \+XXXXXX.`},
	// operators of 64 bytes or more, after the comment cut and +/- trim
	{"select 1 " + strings.Repeat("~", 64) + " 1", "42601", 10, `operator too long at or near "` + strings.Repeat("~", 64) + `"`, ""},
	{"select 1, 2 " + strings.Repeat("~", 64), "42601", 13, `operator too long at or near "` + strings.Repeat("~", 64) + `"`, ""},
	{"select 1 " + strings.Repeat("~", 62) + "++ 1", "42601", 10, `operator too long at or near "` + strings.Repeat("~", 62) + `++"`, ""},
	{"select 1 " + strings.Repeat("<", 64) + "++ 1", "42601", 10, `operator too long at or near "` + strings.Repeat("<", 64) + `"`, ""},
	{"select 1 " + strings.Repeat("+", 70) + "~ 1", "42601", 10, `operator too long at or near "` + strings.Repeat("+", 70) + `~"`, ""},
}

// TestParseReportsLexerErrors pins each scanner error to PostgreSQL 16's
// answer: Parse returns it as a *ParseError, never a statement.
func TestParseReportsLexerErrors(t *testing.T) {
	for _, c := range lexErrorCases {
		stmts, err := Parse(c.sql)
		var pe *ParseError
		if !errors.As(err, &pe) {
			t.Errorf("Parse(%q) = %v, %v; want a *ParseError", c.sql, stmts, err)
			continue
		}
		if pe.Code != c.code || pe.Position+1 != c.position || pe.Message != c.message || pe.Hint != c.hint {
			t.Errorf("Parse(%q) = %s at %d %q hint %q; PostgreSQL 16 says %s at %d %q hint %q",
				c.sql, pe.Code, pe.Position+1, pe.Message, pe.Hint, c.code, c.position, c.message, c.hint)
		}
	}
}

// TestParseAcceptsWellFormedLiterals pins the literals scan.l accepts next to
// the refused ones: an underscore between digits, after a radix prefix, and a
// surrogate pair written as two escapes.
func TestParseAcceptsWellFormedLiterals(t *testing.T) {
	for _, sql := range []string{
		"select 1_000, 0x_1, 0b_1_0, 0o1_7, 1_000.000_1, 1e5, 1.5e-3, .5, 1., 0x7FFF_FFFF",
		`select E'\uD83D\uDE00', E'\U0001F600', U&'\D83D\DE00', U&'\+01F600', U&'!0041' UESCAPE '!', E'\x41\101\n'`,
	} {
		if _, err := Parse(sql); err != nil {
			t.Errorf("Parse(%q) = %v, want no error", sql, err)
		}
	}
}

// TestParseLexerErrorPrecedesSyntaxError: the scanner's error ends the token
// stream, so the grammar's error at the end of that stream is not reported.
func TestParseLexerErrorPrecedesSyntaxError(t *testing.T) {
	_, err := Parse("select (0x")
	var pe *ParseError
	if !errors.As(err, &pe) || pe.Message != `invalid hexadecimal integer at or near "0x"` {
		t.Errorf("Parse(select (0x) = %v, want the scanner's error", err)
	}
}

// TestParseErrorPositionIsTheRefusedToken pins a grammar error's Position to
// the first byte of the token refused, as PostgreSQL reports it, and to the
// input's length at end of input.
func TestParseErrorPositionIsTheRefusedToken(t *testing.T) {
	for _, c := range []struct {
		sql      string
		position int // 1-based, as PostgreSQL 16 answered
	}{
		{"SELEC 1", 1},
		{"  SELEC 1", 3},
		{"SELECT 1 1", 10},
		{"SELECT * FRM t", 10},
		{"select 1 +", 11},
		{"select (1", 10},
		{"select (0x", 9},
		{"select 1..", 9},
		{"select 1 with 1", 10}, // WITH reads a token ahead; the error is still at WITH
		// Malformed AND/OR/NOT/IS chains, in WHERE so no keyword can be read
		// as a bare column label (PostgreSQL 16 accepts "select a and" as
		// "a AS and"). Positions from PostgreSQL 16.15, 2026-10-20; origin/ff
		// answered the same before AND/OR chains were flattened.
		{"select 1 from t where a and", 28},
		{"select 1 from t where and a", 23},
		{"select 1 from t where a and and b", 29},
		{"select 1 from t where a or", 27},
		{"select 1 from t where not", 26},
		{"select 1 from t where a is", 27},
		{"select 1 from t where a is not", 31},
		{"select 1 from t where is null", 26},
		{"select 1 from t where a is not null null", 37},
		{"select 1 from t where a and or b", 29},
		{"select 1 from t where a or not", 31},
		{"select 1 from t where (a and b", 31},
		{"select 1 from t where a is null is", 35},
		{"select 1 from t where a and b or", 33},
		{"select 1 from t where not and a", 27},
		{"select 1 from t where a and not", 32},
		{"select +", 9},
		// A signed FETCH FIRST or SQL OFFSET count is a number, as in
		// PostgreSQL 16, which refuses these at the same positions.
		{"select 1 fetch first +x rows only", 23},
		{"select 1 fetch first -x rows only", 23},
		{"select 1 fetch first +(1) rows only", 23},
		{"select 1 fetch first + 'a' rows only", 24},
		{"select 1 offset +x rows", 20},
		{"select 1 offset - (2) rows", 23},
		{"insert into t (", 16},
		{"insert into t (a,) values (1)", 18},
		// A subscript follows only a column, a parameter or a parenthesised
		// expression, as in PostgreSQL 16, which refuses these at the "[".
		{"SELECT 1[1]", 9},
		{"SELECT now()[1]", 13},
		{"SELECT 'a'[1]", 11},
		{"SELECT ARRAY[1,2][1]", 18},
	} {
		_, err := Parse(c.sql)
		var pe *ParseError
		if !errors.As(err, &pe) || pe.Code != "42601" || pe.Position+1 != c.position {
			t.Errorf("Parse(%q) = %#v, want 42601 at %d", c.sql, err, c.position)
		}
	}
}

// TestParseRefusesLimitComma: PostgreSQL 16's grammar refuses LIMIT #,# with
// 42601 and a HINT, at the LIMIT keyword (verified against 16.4: position
// 17 for the first statement), rather than reading it as OFFSET and LIMIT.
func TestParseRefusesLimitComma(t *testing.T) {
	for _, c := range []struct {
		sql      string
		position int // 1-based, as PostgreSQL 16 answered
	}{
		{"SELECT a FROM t LIMIT 1, 2", 17},
		{"select a from t where a > 1   limit $1, $2 for update", 31},
		{"(select 1 limit 1, 2) union select 2", 11},
	} {
		_, err := Parse(c.sql)
		var pe *ParseError
		if !errors.As(err, &pe) || pe.Code != "42601" || pe.Message != "LIMIT #,# syntax is not supported" ||
			pe.Hint != "Use separate LIMIT and OFFSET clauses." || pe.Position+1 != c.position {
			t.Errorf("Parse(%q) = %#v, want 42601 LIMIT #,# at %d", c.sql, err, c.position)
		}
	}
	if _, err := Parse("SELECT a FROM t LIMIT 2 OFFSET 1"); err != nil {
		t.Errorf("LIMIT and OFFSET as separate clauses: %v", err)
	}
}

// stackDepthShapes generate statements nested n deep, each a shape whose
// parse stack PostgreSQL 16 fills at its own n: it holds a state per
// grammar symbol pending, so a NOT costs one and -( two.
var stackDepthShapes = map[string]func(n int) string{
	"not":     func(n int) string { return "select " + strings.Repeat("not ", n) + "true" },
	"neg":     func(n int) string { return "select " + strings.Repeat("-(", n) + "1" + strings.Repeat(")", n) },
	"negbare": func(n int) string { return "select " + strings.Repeat("- ", n) + "1" },
	"and": func(n int) string {
		return "select 1 where " + strings.Repeat("a and (", n) + "a" + strings.Repeat(")", n)
	},
	"paren":      func(n int) string { return "select " + strings.Repeat("(1 + ", n) + "1" + strings.Repeat(")", n) },
	"parenonly":  func(n int) string { return "select " + strings.Repeat("(", n) + "1" + strings.Repeat(")", n) },
	"subselect":  func(n int) string { return "select " + strings.Repeat("(select ", n) + "1" + strings.Repeat(")", n) },
	"array":      func(n int) string { return "select " + strings.Repeat("array[", n) + "1" + strings.Repeat("]", n) },
	"arraymix":   func(n int) string { return "select " + strings.Repeat("array[(", n) + "1" + strings.Repeat(")]", n) },
	"paren1":     func(n int) string { return "select " + strings.Repeat("(", n) + "1)" },
	"paren1b":    func(n int) string { return "select " + strings.Repeat("(", n) + "1)[1]" },
	"paren1x":    func(n int) string { return "select " + strings.Repeat("(", n) + "1) x" },
	"parenopen":  func(n int) string { return "select " + strings.Repeat("(", n) + "1" },
	"parenopen2": func(n int) string { return "select " + strings.Repeat("(", n) + "1 + 2" },
	"notparen":   func(n int) string { return "select " + strings.Repeat("not ", n) + "(true)" },
	"notstr":     func(n int) string { return "select " + strings.Repeat("not ", n) + "'x'\n'y'  " },
	"notestr":    func(n int) string { return "select " + strings.Repeat("not ", n) + "E'\\x41' -- c" },
	"notfunc":    func(n int) string { return "select " + strings.Repeat("not ", n) + "f()" },
	"notident":   func(n int) string { return "select " + strings.Repeat("not ", n) + "\"Ab\"" },
}

// TestParseStackDepthIsPostgreSQLs parses each shape of stackDepthShapes at
// the two deepest n PostgreSQL 16.15 parses and the two shallowest it
// refuses with 42601 "memory exhausted" (2026-10-20), the parse stack being
// full at Bison's YYMAXDEPTH of 10,000 states. A refused one must be refused
// with PostgreSQL's message and 1-based position; an accepted one must not
// be refused for its depth (some are syntax errors, at end of input, as in
// PostgreSQL).
func TestParseStackDepthIsPostgreSQLs(t *testing.T) {
	for _, c := range []struct {
		shape   string
		n       int
		message string // "" where PostgreSQL parses it
		pos     int
	}{
		{"not", 9994, "", 0},
		{"not", 9995, "", 0},
		{"not", 9996, "memory exhausted at or near \"true\"", 39992},
		{"not", 9997, "memory exhausted at or near \"not\"", 39992},
		{"neg", 4995, "", 0},
		{"neg", 4996, "", 0},
		{"neg", 4997, "memory exhausted at or near \")\"", 10003},
		{"neg", 4998, "memory exhausted at or near \"1\"", 10004},
		{"negbare", 9994, "", 0},
		{"negbare", 9995, "", 0},
		{"negbare", 9996, "memory exhausted at or near \"1\"", 20000},
		{"negbare", 9997, "memory exhausted at or near \"-\"", 20000},
		{"and", 3328, "", 0},
		{"and", 3329, "", 0},
		{"and", 3330, "memory exhausted at or near \")\"", 23327},
		{"and", 3331, "memory exhausted at or near \"(\"", 23332},
		{"paren", 3330, "", 0},
		{"paren", 3331, "", 0},
		{"paren", 3332, "memory exhausted at or near \"1\"", 16668},
		{"paren", 3333, "memory exhausted at or near \"(\"", 16668},
		{"parenonly", 9992, "", 0},
		{"parenonly", 9993, "", 0},
		{"parenonly", 9994, "memory exhausted at or near \")\"", 10003},
		{"parenonly", 9995, "memory exhausted at or near \")\"", 10004},
		{"subselect", 3328, "", 0},
		{"subselect", 3329, "", 0},
		{"subselect", 3330, "memory exhausted at or near \")\"", 26649},
		{"subselect", 3331, "memory exhausted at or near \")\"", 26657},
		{"array", 4996, "", 0},
		{"array", 4997, "", 0},
		{"array", 4998, "memory exhausted at or near \"1\"", 29996},
		{"array", 4999, "memory exhausted at or near \"array\"", 29996},
		{"arraymix", 3330, "", 0},
		{"arraymix", 3331, "", 0},
		{"arraymix", 3332, "memory exhausted at or near \"1\"", 23332},
		{"arraymix", 3333, "memory exhausted at or near \"array\"", 23332},
		{"paren1", 9992, "", 0},
		{"paren1", 9993, "", 0},
		{"paren1", 9994, "memory exhausted at or near \")\"", 10003},
		{"paren1", 9995, "memory exhausted at or near \")\"", 10004},
		{"paren1b", 9989, "", 0},
		{"paren1b", 9990, "", 0},
		{"paren1b", 9991, "memory exhausted at or near \"]\"", 10003},
		{"paren1b", 9992, "memory exhausted at or near \"1\"", 10003},
		{"paren1x", 9992, "", 0},
		{"paren1x", 9993, "", 0},
		{"paren1x", 9994, "memory exhausted at or near \")\"", 10003},
		{"paren1x", 9995, "memory exhausted at or near \")\"", 10004},
		{"parenopen", 9994, "", 0},
		{"parenopen", 9995, "", 0},
		{"parenopen", 9996, "memory exhausted at or near \"1\"", 10004},
		{"parenopen", 9997, "memory exhausted at or near \"(\"", 10004},
		{"parenopen2", 9992, "", 0},
		{"parenopen2", 9993, "", 0},
		{"parenopen2", 9994, "memory exhausted at or near \"2\"", 10006},
		{"parenopen2", 9995, "memory exhausted at or near \"+\"", 10005},
		{"notparen", 9991, "", 0},
		{"notparen", 9992, "", 0},
		{"notparen", 9993, "memory exhausted at or near \")\"", 39985},
		{"notparen", 9994, "memory exhausted at or near \")\"", 39989},
		{"notstr", 9994, "", 0},
		{"notstr", 9995, "", 0},
		{"notstr", 9996, "memory exhausted at or near \"'x'\n'y'\"", 39992},
		{"notstr", 9997, "memory exhausted at or near \"not\"", 39992},
		{"notestr", 9994, "", 0},
		{"notestr", 9995, "", 0},
		{"notestr", 9996, "memory exhausted at or near \"E'\\x41'\"", 39992},
		{"notestr", 9997, "memory exhausted at or near \"not\"", 39992},
		{"notfunc", 9991, "", 0},
		{"notfunc", 9992, "", 0},
		{"notfunc", 9993, "memory exhausted at end of input", 39983},
		{"notfunc", 9994, "memory exhausted at or near \")\"", 39986},
		{"notident", 9994, "", 0},
		{"notident", 9995, "", 0},
		{"notident", 9996, "memory exhausted at or near \"\"Ab\"\"", 39992},
		{"notident", 9997, "memory exhausted at or near \"not\"", 39992},
	} {
		sql := stackDepthShapes[c.shape](c.n)
		_, err := Parse(sql)
		var pe *ParseError
		refused := errors.As(err, &pe) && strings.HasPrefix(pe.Message, "memory exhausted")
		switch {
		case c.message == "" && refused:
			t.Errorf("%s %d: %v at %d; PostgreSQL parses it", c.shape, c.n, err, pe.Position+1)
		case c.message != "" && !refused:
			t.Errorf("%s %d: %v; want %q", c.shape, c.n, err, c.message)
		case c.message != "" && (pe.Code != codeSyntaxError || pe.Message != c.message || pe.Position+1 != c.pos || pe.Hint != ""):
			t.Errorf("%s %d: %s %q at %d; want 42601 %q at %d", c.shape, c.n, pe.Code, pe.Message, pe.Position+1, c.message, c.pos)
		}
	}
}

// TestParseStackDepthIsBounded parses a million nested NOTs: the parse is
// refused at the 9,997th, as PostgreSQL 16 refuses it, and its stack never
// holds more than 10,000 states.
func TestParseStackDepthIsBounded(t *testing.T) {
	sql := stackDepthShapes["not"](1_000_000)
	_, err := Parse(sql)
	var pe *ParseError
	if want := len("select ") + 4*9996; !errors.As(err, &pe) || pe.Message != `memory exhausted at or near "not"` || pe.Position != want {
		t.Fatalf("%v; want memory exhausted at %d", err, want)
	}
}
