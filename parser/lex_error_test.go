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
