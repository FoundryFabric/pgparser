package parser

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pgplex/pgparser/nodes"
)

func TestLexerKeywords(t *testing.T) {
	tests := []struct {
		input    string
		expected int
		str      string
	}{
		{"SELECT", SELECT, "select"},
		{"select", SELECT, "select"},
		{"FROM", FROM, "from"},
		{"WHERE", WHERE, "where"},
		{"AND", AND, "and"},
		{"OR", OR, "or"},
		{"CREATE", CREATE, "create"},
		{"TABLE", TABLE, "table"},
		{"INSERT", INSERT, "insert"},
		{"UPDATE", UPDATE, "update"},
		{"DELETE", DELETE_P, "delete"},
		{"NULL", NULL_P, "null"},
		{"TRUE", TRUE_P, "true"},
		{"FALSE", FALSE_P, "false"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			lexer := NewLexer(tt.input)
			tok := lexer.NextToken()
			if tok.Type != tt.expected {
				t.Errorf("expected token type %d, got %d", tt.expected, tok.Type)
			}
			if tok.Str != tt.str {
				t.Errorf("expected str %q, got %q", tt.str, tok.Str)
			}
		})
	}
}

func TestLexerIdentifiers(t *testing.T) {
	tests := []struct {
		input    string
		expected int
		str      string
	}{
		{"foo", lex_IDENT, "foo"},
		{"Foo", lex_IDENT, "foo"},
		{"FOO", lex_IDENT, "foo"},
		{"foo_bar", lex_IDENT, "foo_bar"},
		{"_foo", lex_IDENT, "_foo"},
		{"foo123", lex_IDENT, "foo123"},
		{"foo$bar", lex_IDENT, "foo$bar"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			lexer := NewLexer(tt.input)
			tok := lexer.NextToken()
			if tok.Type != tt.expected {
				t.Errorf("expected token type %d (IDENT), got %d", tt.expected, tok.Type)
			}
			if tok.Str != tt.str {
				t.Errorf("expected str %q, got %q", tt.str, tok.Str)
			}
		})
	}
}

func TestLexerDelimitedIdentifiers(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`"foo"`, "foo"},
		{`"Foo Bar"`, "Foo Bar"},
		{`"SELECT"`, "SELECT"},
		{`"foo""bar"`, `foo"bar`}, // escaped quote
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			lexer := NewLexer(tt.input)
			tok := lexer.NextToken()
			if tok.Type != lex_IDENT {
				t.Errorf("expected token type IDENT, got %d", tok.Type)
			}
			if tok.Str != tt.expected {
				t.Errorf("expected str %q, got %q", tt.expected, tok.Str)
			}
		})
	}
}

func TestLexerNumbers(t *testing.T) {
	tests := []struct {
		input    string
		tokType  int
		ival     int64
		str      string
	}{
		{"0", lex_ICONST, 0, "0"},
		{"42", lex_ICONST, 42, "42"},
		{"123456", lex_ICONST, 123456, "123456"},
		{"1_000_000", lex_ICONST, 1000000, "1000000"},
		{"0x10", lex_ICONST, 16, "0x10"},
		{"0xFF", lex_ICONST, 255, "0xFF"},
		{"0o17", lex_ICONST, 15, "0o17"},
		{"0b1010", lex_ICONST, 10, "0b1010"},
		{"3.14", lex_FCONST, 0, "3.14"},
		{".5", lex_FCONST, 0, ".5"},
		{"1e10", lex_FCONST, 0, "1e10"},
		{"1.5e-3", lex_FCONST, 0, "1.5e-3"},
		{"0X1F", lex_ICONST, 31, "0X1F"},
		{"9223372036854775807", lex_ICONST, 9223372036854775807, "9223372036854775807"},
		{"0x7FFF_FFFF_FFFF_FFFF", lex_ICONST, 9223372036854775807, "0x7FFFFFFFFFFFFFFF"},
		{"9223372036854775808", lex_FCONST, 0, "9223372036854775808"},
		{"0x8000000000000000", lex_FCONST, 0, "0x8000000000000000"},
		{"0b1" + strings.Repeat("0", 63), lex_FCONST, 0, "0b1" + strings.Repeat("0", 63)},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			lexer := NewLexer(tt.input)
			tok := lexer.NextToken()
			if tok.Type != tt.tokType {
				t.Errorf("expected token type %d, got %d", tt.tokType, tok.Type)
			}
			if tt.tokType == lex_ICONST && tok.Ival != tt.ival {
				t.Errorf("expected ival %d, got %d", tt.ival, tok.Ival)
			}
			if tt.tokType == lex_FCONST && tok.Str != tt.str {
				t.Errorf("expected str %q, got %q", tt.str, tok.Str)
			}
		})
	}
}

func TestLexerStrings(t *testing.T) {
	tests := []struct {
		input    string
		tokType  int
		expected string
	}{
		{"'hello'", lex_SCONST, "hello"},
		{"'hello world'", lex_SCONST, "hello world"},
		{"'it''s'", lex_SCONST, "it's"},      // escaped quote
		{"''", lex_SCONST, ""},               // empty string
		{"E'hello\\nworld'", lex_SCONST, "hello\nworld"}, // escape string
		{"E'tab\\there'", lex_SCONST, "tab\there"},
		{"B'101'", lex_BCONST, "b101"},       // bit string
		{"X'FF'", lex_XCONST, "xFF"},         // hex string
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			lexer := NewLexer(tt.input)
			tok := lexer.NextToken()
			if tok.Type != tt.tokType {
				t.Errorf("expected token type %d, got %d", tt.tokType, tok.Type)
			}
			if tok.Str != tt.expected {
				t.Errorf("expected str %q, got %q", tt.expected, tok.Str)
			}
		})
	}
}

func TestLexerDollarQuotes(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"$$hello$$", "hello"},
		{"$$hello world$$", "hello world"},
		{"$tag$hello$tag$", "hello"},
		{"$foo$some 'text' here$foo$", "some 'text' here"},
		{"$$line1\nline2$$", "line1\nline2"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			lexer := NewLexer(tt.input)
			tok := lexer.NextToken()
			if tok.Type != lex_SCONST {
				t.Errorf("expected token type lex_SCONST, got %d", tok.Type)
			}
			if tok.Str != tt.expected {
				t.Errorf("expected str %q, got %q", tt.expected, tok.Str)
			}
		})
	}
}

func TestLexerOperators(t *testing.T) {
	tests := []struct {
		input   string
		tokType int
		str     string
	}{
		{"::", lex_TYPECAST, "::"},
		{"..", lex_DOT_DOT, ".."},
		{":=", lex_COLON_EQUALS, ":="},
		{"=>", lex_EQUALS_GREATER, "=>"},
		{"<=", lex_LESS_EQUALS, "<="},
		{">=", lex_GREATER_EQUALS, ">="},
		{"<>", lex_NOT_EQUALS, "<>"},
		{"!=", lex_NOT_EQUALS, "!="},
		{"+", int('+'), "+"},
		{"-", int('-'), "-"},
		{"*", int('*'), "*"},
		{"/", int('/'), "/"},
		{"@", lex_Op, "@"},
		{"||", lex_Op, "||"},
		{"->", lex_Op, "->"},
		{"->>", lex_Op, "->>"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			lexer := NewLexer(tt.input)
			tok := lexer.NextToken()
			if tok.Type != tt.tokType {
				t.Errorf("expected token type %d, got %d", tt.tokType, tok.Type)
			}
			if tok.Str != tt.str {
				t.Errorf("expected str %q, got %q", tt.str, tok.Str)
			}
		})
	}
}

// TestLexerOperatorRules pins scan.l's operator rule (see lexOperator). Each
// case matches PostgreSQL 16: the value of "select 1 <ops> 1", or the
// operator its 42883 error names.
func TestLexerOperatorRules(t *testing.T) {
	tests := []struct {
		input string
		want  []string // the tokens, comments dropped
	}{
		// trailing + and - trimmed when nothing qualifies
		{"+-", []string{"+", "-"}},                                              // 1 +- 1 = 0
		{"+-+-", []string{"+", "-", "+", "-"}},                                  // 1 +-+- 1 = 2
		{"<+", []string{"<", "+"}},                                              // 1 <+ 1 = f
		{"<=+", []string{"<=", "+"}},                                            // 1 <=+ 1 = t
		{"<>-", []string{"<>", "-"}},                                            // 1 <>- 1 = t
		{strings.Repeat("<", 63) + "+", []string{strings.Repeat("<", 63), "+"}}, // 42883 names the 63 <
		// a qualifying character anywhere keeps them
		{"@-", []string{"@-"}},
		{"~+", []string{"~+"}},
		{"!=-", []string{"!=-"}},
		{"%-", []string{"%-"}},
		{"^-", []string{"^-"}},
		{"?-", []string{"?-"}},
		{"@+-+-", []string{"@+-+-"}},
		{"+-@", []string{"+-@"}},
		// after a comment cut, the next token re-searches and re-trims
		{"@-/**/+-", []string{"@-", "+", "-"}},       // with @- defined: select 5 @-/**/+- 1 = 6
		{"+-/**/<<+", []string{"+", "-", "<<", "+"}}, // with prefix << defined: select 1 +-/**/<<+ 1 = 2
		// a comment begins inside the run
		{"*-- x\n", []string{"*"}},           // select 2 *-- x\n 3 = 6
		{"*/* x */", []string{"*"}},          // select 2 */* x */ 3 = 6
		{"+/*x*/+", []string{"+", "+"}},      // 1 +/*x*/+ 1 = 2
		{"*/*--*/", []string{"*"}},           // 1 */*--*/ 1 = 1: the earlier start wins
		{"*--/*\n", []string{"*"}},           // 1 *--/*\n 1 = 1
		{"+--~\n", []string{"+"}},            // 1 +--~\n 1 = 2: the ~ is in the comment
		{"-+/**/@", []string{"-", "+", "@"}}, // 1 -+/**/@ 1 = 0
		{"<-->", []string{"<"}},              // select 1 <--> 1: syntax error at end of input
		{"~~~~~~~~~~--" + strings.Repeat("~", 70) + "\n", []string{"~~~~~~~~~~"}},
		{strings.Repeat("+", 70) + "/*x*/~", append(strings.Split(strings.Repeat("+", 70), ""), "~")}, // = -1
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			lexer := NewLexer(tt.input)
			var got []string
			for tok := lexer.NextToken(); tok.Type != lex_EOF; tok = lexer.NextToken() {
				got = append(got, tok.Str)
				if len(tok.Str) == 1 && isSelf(tok.Str[0]) && tok.Type != int(tok.Str[0]) {
					t.Errorf("%q lexed as type %d, want the character token %d", tok.Str, tok.Type, tok.Str[0])
				}
			}
			if lexer.Err != nil {
				t.Fatalf("lexing %q: %v", tt.input, lexer.Err)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("lexing %q = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// opScanStepsFor lexes "select 1 " + n '+' + " 1", checks each '+' is its own
// token, and returns lexOperator's scan steps.
func opScanStepsFor(t *testing.T, n int) int {
	t.Helper()
	lexer := NewLexer("select 1 " + strings.Repeat("+", n) + " 1")
	plus := 0
	for tok := lexer.NextToken(); tok.Type != lex_EOF; tok = lexer.NextToken() {
		if tok.Type == '+' {
			plus++
		}
	}
	if lexer.Err != nil || plus != n {
		t.Fatalf("a run of %d '+' lexed as %d '+' tokens, error %v", n, plus, lexer.Err)
	}
	return lexer.opScanSteps
}

// TestLexerOperatorRunIsLinear guards against a super-linear operator run
// (4,000 '+' once took 12 seconds). The guard is a deadline: 1,000,000 '+'
// lex in ~20ms (~200ms under -race) and must finish within 2s; a goroutine
// makes a slow lexer fail rather than hang. The step count checks the caches:
// steps for 2,000 must stay within 4x those for 500, plus a constant.
func TestLexerOperatorRunIsLinear(t *testing.T) {
	const n = 500
	if small, large := opScanStepsFor(t, n), opScanStepsFor(t, 4*n); large > 4*small+64 {
		t.Fatalf("lexing a run of %d '+' examined %d positions, %.1fx the %d for %d: want linear growth (4x)",
			4*n, large, float64(large)/float64(small), small, n)
	}

	const run, budget = 1_000_000, 2 * time.Second
	type result struct {
		plus int
		err  error
	}
	done := make(chan result, 1)
	begin := time.Now()
	go func() {
		lexer := NewLexer("select 1 " + strings.Repeat("+", run) + " 1")
		plus := 0
		for tok := lexer.NextToken(); tok.Type != lex_EOF; tok = lexer.NextToken() {
			if tok.Type == '+' {
				plus++
			}
		}
		done <- result{plus, lexer.Err}
	}()
	timer := time.NewTimer(budget)
	defer timer.Stop()
	select {
	case r := <-done:
		if r.err != nil || r.plus != run {
			t.Fatalf("a run of %d '+' lexed as %d '+' tokens, error %v", run, r.plus, r.err)
		}
		t.Logf("a run of %d '+' lexed in %v", run, time.Since(begin))
	case <-timer.C:
		t.Fatalf("lexing a run of %d '+' took over %v", run, budget)
	}
}

// refToken is a token refLex expects: position, text (a literal's value)
// and type.
type refToken struct {
	loc  int
	text string
	typ  int
}

// refLex is a plain reference lexer for TestLexerOperatorMatchesReference's
// alphabet (whitespace, comments, '' strings, "" identifiers, "::", ":=", ':',
// identifiers, digits, operators), applying the operator rule per token with
// no caching. It returns the tokens and the expected error's 0-based position
// and message, or -1.
func refLex(s string) ([]refToken, int, string) {
	isLetter := func(c byte) bool { return c >= 'a' && c <= 'z' }
	isDigit := func(c byte) bool { return c >= '0' && c <= '9' }
	var toks []refToken
	for p := 0; p < len(s); {
		c := s[p]
		switch {
		case c == ' ' || c == '\n':
			p++
		case strings.HasPrefix(s[p:], "/*"):
			depth, q := 1, p+2
			for q < len(s) && depth > 0 {
				switch {
				case strings.HasPrefix(s[q:], "/*"):
					depth, q = depth+1, q+2
				case strings.HasPrefix(s[q:], "*/"):
					depth, q = depth-1, q+2
				default:
					q++
				}
			}
			if depth > 0 {
				return toks, p, `unterminated /* comment at or near "` + s[p:] + `"`
			}
			p = q
		case strings.HasPrefix(s[p:], "--"):
			for p < len(s) && s[p] != '\n' {
				p++
			}
		case c == '\'':
			var val strings.Builder
			q := p + 1
			for {
				if q >= len(s) {
					return toks, p, `unterminated quoted string at or near "` + s[p:] + `"`
				}
				if s[q] != '\'' {
					val.WriteByte(s[q])
					q++
					continue
				}
				if q+1 < len(s) && s[q+1] == '\'' {
					val.WriteByte('\'')
					q += 2
					continue
				}
				q++
				// whitespace holding a newline, then a quote, continues it
				r, newline := q, false
				for r < len(s) {
					if s[r] == ' ' || s[r] == '\n' {
						newline = newline || s[r] == '\n'
						r++
					} else if strings.HasPrefix(s[r:], "--") {
						for r < len(s) && s[r] != '\n' {
							r++
						}
					} else {
						break
					}
				}
				if !newline || r >= len(s) || s[r] != '\'' {
					break
				}
				q = r + 1
			}
			toks, p = append(toks, refToken{p, val.String(), lex_SCONST}), q
		case c == '"':
			var val strings.Builder
			q := p + 1
			for {
				if q >= len(s) {
					return toks, p, `unterminated quoted identifier at or near "` + s[p:] + `"`
				}
				if s[q] == '"' && q+1 < len(s) && s[q+1] == '"' {
					val.WriteByte('"')
					q += 2
				} else if s[q] == '"' {
					q++
					break
				} else {
					val.WriteByte(s[q])
					q++
				}
			}
			if val.Len() == 0 {
				return toks, p, `zero-length delimited identifier at or near "` + s[p:q] + `"`
			}
			ident := val.String()
			if len(ident) >= 64 { // ASCII: truncated to 63 bytes
				ident = ident[:63]
			}
			toks, p = append(toks, refToken{p, ident, lex_IDENT}), q
		case strings.HasPrefix(s[p:], "::"):
			toks, p = append(toks, refToken{p, "::", lex_TYPECAST}), p+2
		case strings.HasPrefix(s[p:], ":="):
			toks, p = append(toks, refToken{p, ":=", lex_COLON_EQUALS}), p+2
		case c == ':':
			toks, p = append(toks, refToken{p, ":", ':'}), p+1
		case isLetter(c):
			q := p
			for q < len(s) && (isLetter(s[q]) || isDigit(s[q])) {
				q++
			}
			toks, p = append(toks, refToken{p, s[p:q], lex_IDENT}), q
		case isDigit(c):
			q := p
			for q < len(s) && isDigit(s[q]) {
				q++
			}
			if q < len(s) && isLetter(s[q]) {
				for q < len(s) && (isLetter(s[q]) || isDigit(s[q])) {
					q++
				}
				return toks, p, `trailing junk after numeric literal at or near "` + s[p:q] + `"`
			}
			toks, p = append(toks, refToken{p, s[p:q], lex_ICONST}), q
		default: // an operator character
			q := p
			for q < len(s) && strings.IndexByte("~!@#^&|`?%+-*/<>=", s[q]) >= 0 {
				q++
			}
			op := s[p:q]
			if i := strings.Index(op, "/*"); i >= 0 {
				op = op[:i]
			}
			if i := strings.Index(op, "--"); i >= 0 {
				op = op[:i]
			}
			if !strings.ContainsAny(op, "~!@#^&|`?%") {
				for len(op) > 1 && (op[len(op)-1] == '+' || op[len(op)-1] == '-') {
					op = op[:len(op)-1]
				}
			}
			typ := lex_Op
			switch {
			case len(op) == 1 && strings.Contains("+-*/%^<>=", op):
				typ = int(op[0])
			case op == "<=":
				typ = lex_LESS_EQUALS
			case op == ">=":
				typ = lex_GREATER_EQUALS
			case op == "=>":
				typ = lex_EQUALS_GREATER
			case op == "<>" || op == "!=":
				typ = lex_NOT_EQUALS
			case len(op) >= 64:
				return toks, p, `operator too long at or near "` + op + `"`
			}
			toks, p = append(toks, refToken{p, op, typ}), p+len(op)
		}
	}
	return toks, -1, ""
}

// TestLexerOperatorMatchesReference compares the lexer with refLex on seeded
// random strings, often ending in an open quote or "/*": same tokens and same
// error.
func TestLexerOperatorMatchesReference(t *testing.T) {
	pieces := []string{
		"~", "!", "@", "#", "^", "&", "|", "`", "?", "%", "+", "-", "*", "/", "<", ">", "=",
		"/", "*", "-", "+", " ", "\n", "a", "c", "1", ":", "/**/", "/*+-*/", "--x\n", "<>", "::", "+-", "<<",
		"'", "'", "''", "\"", "\"",
	}
	rng := rand.New(rand.NewSource(20261020))
	count := 50_000
	if testing.Short() {
		count = 10_000
	}
	refused := 0
	for i := 0; i < count; i++ {
		var b strings.Builder
		for k := rng.Intn(30); k > 0; k-- {
			b.WriteString(pieces[rng.Intn(len(pieces))])
		}
		if rng.Intn(10) == 0 { // reach NAMEDATALEN
			b.WriteString(strings.Repeat(pieces[rng.Intn(17)], 60+rng.Intn(10)))
		}
		s := b.String()
		want, errAt, errMsg := refLex(s)

		lexer := NewLexer(s)
		var got []refToken
		for tok := lexer.NextToken(); tok.Type != lex_EOF; tok = lexer.NextToken() {
			got = append(got, refToken{tok.Loc, tok.Str, tok.Type})
		}
		var pe *ParseError
		switch {
		case errAt < 0 && lexer.Err != nil:
			t.Fatalf("lexing %q: %v, want no error", s, lexer.Err)
		case errAt >= 0 && (!errors.As(lexer.Err, &pe) || pe.Position != errAt || pe.Message != errMsg):
			t.Fatalf("lexing %q: error %v, want %q at %d", s, lexer.Err, errMsg, errAt)
		case !slices.Equal(got, want):
			t.Fatalf("lexing %q:\n got %v\nwant %v", s, got, want)
		}
		if strings.HasPrefix(errMsg, "operator too long") {
			refused++
		}
	}
	if refused == 0 {
		t.Errorf("no string reached an operator of %d bytes", namedatalen)
	}
}

// BenchmarkLexOperatorRun lexes a run of 100,000 '+', one token per '+'.
func BenchmarkLexOperatorRun(b *testing.B) {
	input := "select jsonb_" + strings.Repeat("+", 100_000)
	b.SetBytes(int64(len(input)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		lexer := NewLexer(input)
		for lexer.NextToken().Type != lex_EOF {
		}
	}
}

func TestLexerParameters(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"$1", 1},
		{"$2", 2},
		{"$10", 10},
		{"$123", 123},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			lexer := NewLexer(tt.input)
			tok := lexer.NextToken()
			if tok.Type != lex_PARAM {
				t.Errorf("expected token type lex_PARAM, got %d", tok.Type)
			}
			if tok.Ival != tt.expected {
				t.Errorf("expected ival %d, got %d", tt.expected, tok.Ival)
			}
		})
	}
}

func TestLexerComments(t *testing.T) {
	tests := []struct {
		input    string
		expected int
		str      string
	}{
		{"-- comment\nSELECT", SELECT, "select"},
		{"/* comment */ SELECT", SELECT, "select"},
		{"/* multi\nline */ SELECT", SELECT, "select"},
		{"/* nested /* comment */ */ SELECT", SELECT, "select"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			lexer := NewLexer(tt.input)
			tok := lexer.NextToken()
			if tok.Type != tt.expected {
				t.Errorf("expected token type %d, got %d", tt.expected, tok.Type)
			}
			if tok.Str != tt.str {
				t.Errorf("expected str %q, got %q", tt.str, tok.Str)
			}
		})
	}
}

func TestLexerWhitespace(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"  SELECT", SELECT},
		{"\tSELECT", SELECT},
		{"\nSELECT", SELECT},
		{"\r\nSELECT", SELECT},
		{"  \t\n  SELECT", SELECT},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			lexer := NewLexer(tt.input)
			tok := lexer.NextToken()
			if tok.Type != tt.expected {
				t.Errorf("expected token type %d, got %d", tt.expected, tok.Type)
			}
		})
	}
}

func TestLexerSQLStatements(t *testing.T) {
	input := "SELECT * FROM users WHERE id = 1"
	expected := []struct {
		tokType int
		str     string
	}{
		{SELECT, "select"},
		{int('*'), "*"},
		{FROM, "from"},
		{lex_IDENT, "users"},
		{WHERE, "where"},
		{lex_IDENT, "id"},
		{int('='), "="},
		{lex_ICONST, "1"},
		{lex_EOF, ""},
	}

	lexer := NewLexer(input)
	for i, tt := range expected {
		tok := lexer.NextToken()
		if tok.Type != tt.tokType {
			t.Errorf("token %d: expected type %d, got %d", i, tt.tokType, tok.Type)
		}
		if tt.tokType != lex_EOF && tt.tokType != lex_ICONST && tok.Str != tt.str {
			t.Errorf("token %d: expected str %q, got %q", i, tt.str, tok.Str)
		}
	}
}

func TestLexerCreateTable(t *testing.T) {
	input := `CREATE TABLE users (
		id INTEGER PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		email TEXT
	)`

	lexer := NewLexer(input)
	tokens := []Token{}
	for {
		tok := lexer.NextToken()
		tokens = append(tokens, tok)
		if tok.Type == lex_EOF {
			break
		}
	}

	// Just verify we got through without errors
	if lexer.Err != nil {
		t.Errorf("unexpected error: %v", lexer.Err)
	}

	// Verify some key tokens
	if tokens[0].Type != CREATE {
		t.Errorf("expected CREATE, got %d", tokens[0].Type)
	}
	if tokens[1].Type != TABLE {
		t.Errorf("expected TABLE, got %d", tokens[1].Type)
	}
}

func TestLexerLocation(t *testing.T) {
	input := "SELECT foo"
	lexer := NewLexer(input)

	tok := lexer.NextToken()
	if tok.Loc != 0 {
		t.Errorf("expected SELECT location 0, got %d", tok.Loc)
	}

	tok = lexer.NextToken()
	if tok.Loc != 7 {
		t.Errorf("expected foo location 7, got %d", tok.Loc)
	}
}

// TestLexerEveryStateAnswersEndOfInput puts the lexer in each of its states
// at the end of the input. The initial state returns EOF and the quote-stop
// state its completed literal; every other state is inside a literal or
// comment, which the end leaves unterminated, so it must report an error of
// its own. A state added without a case in NextToken fails here with the
// "unknown lexer state" error.
func TestLexerEveryStateAnswersEndOfInput(t *testing.T) {
	for state := LexerState(0); state < numLexerStates; state++ {
		lexer := NewLexer("x")
		lexer.pos, lexer.state = 1, state
		tok := lexer.NextToken()
		var pe *ParseError
		switch {
		case state == stateInitial:
			if tok.Type != lex_EOF || lexer.Err != nil {
				t.Errorf("state %d at the end: %+v, %v; want EOF", state, tok, lexer.Err)
			}
		case state == stateXQS:
			if tok.Type != lex_SCONST || lexer.Err != nil {
				t.Errorf("state %d at the end: %+v, %v; want the completed literal", state, tok, lexer.Err)
			}
		case !errors.As(lexer.Err, &pe) || pe.Code == "XX000" || !strings.HasPrefix(pe.Message, "unterminated "):
			t.Errorf("state %d at the end: %+v, %v; want an unterminated-literal error", state, tok, lexer.Err)
		}
		if next := lexer.NextToken(); next.Type != lex_EOF {
			t.Errorf("state %d: after the end, %+v; want EOF", state, next)
		}
	}
}

func TestLexerErrors(t *testing.T) {
	tests := []struct {
		input string
		err   string
	}{
		{"'unterminated", `unterminated quoted string at or near "'unterminated"`},
		{`"unterminated`, `unterminated quoted identifier at or near ""unterminated"`},
		{"/* unterminated", `unterminated /* comment at or near "/* unterminated"`},
		{"$$unterminated", `unterminated dollar-quoted string at or near "$$unterminated"`},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			lexer := NewLexer(tt.input)
			for {
				tok := lexer.NextToken()
				if tok.Type == lex_EOF {
					break
				}
			}
			if lexer.Err == nil {
				t.Errorf("expected error containing %q, got nil", tt.err)
			} else if lexer.Err.Error() != tt.err {
				t.Errorf("expected error %q, got %q", tt.err, lexer.Err.Error())
			}
		})
	}
}

// TestIntegerLiteral pins the reading of an integer literal's text that the
// scanner and doNegate share: each radix, either case of prefix, up to
// int64's greatest value and, negated, its least; anything else (too
// large, a decimal, an exponent, a sign already present, a bare prefix) is
// not an integer.
func TestIntegerLiteral(t *testing.T) {
	const least = -9223372036854775808
	for _, c := range []struct {
		text     string
		negative bool
		want     int64
		ok       bool
	}{
		{"0", false, 0, true},
		{"0", true, 0, true},
		{"42", true, -42, true},
		{"0x1f", false, 31, true},
		{"0XFF", true, -255, true},
		{"0o17", false, 15, true},
		{"0O17", true, -15, true},
		{"0b101", false, 5, true},
		{"0B101", true, -5, true},
		{"9223372036854775807", false, 9223372036854775807, true},
		{"9223372036854775808", false, 0, false},
		{"9223372036854775808", true, least, true},
		{"0x8000000000000000", true, least, true},
		{"0o1000000000000000000000", true, least, true},
		{"0b1" + strings.Repeat("0", 63), true, least, true},
		{"9223372036854775809", true, 0, false},
		{"0x8000000000000001", true, 0, false},
		{"1.5", true, 0, false},
		{"1e3", true, 0, false},
		{".5", false, 0, false},
		{"-1", true, 0, false},
		{"+1", false, 0, false},
		{"0x", false, 0, false},
		{"0x-1", false, 0, false},
		{"0b2", false, 0, false},
		{"", false, 0, false},
	} {
		got, ok := integerLiteral(c.text, c.negative)
		if got != c.want || ok != c.ok {
			t.Errorf("integerLiteral(%q, %v) = %d, %v; want %d, %v", c.text, c.negative, got, ok, c.want, c.ok)
		}
	}
}

// lexStackChildEnv names the variable that makes TestLexerStackIsBounded run
// its inputs instead of starting a child to run them.
const lexStackChildEnv = "PGPARSER_LEX_STACK_CHILD"

// lexStackParentEnv carries the parent's process ID: a process runs as the
// child only when it was started by that parent, so a lexStackChildEnv that
// leaked into a test run's environment fails rather than passing in-process.
const lexStackParentEnv = "PGPARSER_LEX_STACK_PARENT"

// lexStackMarker is what the child prints once every input has lexed and
// parsed, so the parent cannot pass on a child that ran no test at all.
const lexStackMarker = "lexer stack bounded: all inputs lexed"

// TestLexerStackIsBounded lexes and parses 5,000,000 string continuations,
// 5,000,000 comments and a mix of both with other tokens on an 8 MB stack.
// A state that reaches the next token by calling NextToken, rather than
// returning to its loop, recurses once per comment or continuation and
// overflows that stack. The inputs run in a child process because a stack
// overflow is fatal: it fails this test rather than killing the suite.
func TestLexerStackIsBounded(t *testing.T) {
	if _, set := os.LookupEnv(lexStackChildEnv); set {
		if os.Getenv(lexStackChildEnv) != "1" || os.Getenv(lexStackParentEnv) != strconv.Itoa(os.Getppid()) {
			t.Fatalf("%s is set but this process was not started by TestLexerStackIsBounded; unset it", lexStackChildEnv)
		}
		lexStackChild()
		return
	}
	if testing.Short() {
		t.Skip("starts a child process lexing 60 MB")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestLexerStackIsBounded$", "-test.count=1", "-test.v")
	cmd.Env = append(os.Environ(), lexStackChildEnv+"=1", lexStackParentEnv+"="+strconv.Itoa(os.Getpid()))
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("child did not finish in time: %v", ctx.Err())
	}
	if err != nil || !strings.Contains(string(out), lexStackMarker) {
		if len(out) > 2000 {
			out = out[:2000]
		}
		t.Fatalf("child failed: %v\n%s", err, out)
	}
}

// lexStackChild runs the inputs of TestLexerStackIsBounded on an 8 MB stack
// and prints lexStackMarker when each lexes to the expected tokens and parses.
func lexStackChild() {
	debug.SetMaxStack(8 << 20)
	const n = 5_000_000
	fail := func(format string, args ...any) {
		fmt.Fprintf(os.Stderr, format+"\n", args...)
		os.Exit(1)
	}
	lexAll := func(input string) []Token {
		lexer := NewLexer(input)
		var toks []Token
		for tok := lexer.NextToken(); tok.Type != lex_EOF; tok = lexer.NextToken() {
			toks = append(toks, tok)
		}
		if lexer.Err != nil {
			fail("lexing: %v", lexer.Err)
		}
		return toks
	}

	continuations := "select 'a'" + strings.Repeat("\n'a'", n-1)
	if toks := lexAll(continuations); len(toks) != 2 || toks[1].Type != lex_SCONST || toks[1].Str != strings.Repeat("a", n) || toks[1].Loc != 7 {
		fail("continuations: %d tokens", len(toks))
	}
	comments := "select " + strings.Repeat("/**/", n) + " 1"
	if toks := lexAll(comments); len(toks) != 2 || toks[1].Type != lex_ICONST || toks[1].Loc != 7+4*n+1 {
		fail("comments: %d tokens", len(toks))
	}
	// A mix: each unit is a continued string, a comment, a nested comment, a
	// line comment and a '+', so states alternate as well as repeat.
	const unit = "'a'\n'b' /**/ /* /**/ */ -- c\n+ "
	mixed := "select " + strings.Repeat(unit, n/len(unit)) + "'z'"
	toks := lexAll(mixed)
	if want := 1 + 2*(n/len(unit)) + 1; len(toks) != want {
		fail("mix: %d tokens, want %d", len(toks), want)
	}
	for i, tok := range toks[1 : len(toks)-1] {
		if want := []int{lex_SCONST, '+'}[i%2]; tok.Type != want {
			fail("mix: token %d is %d, want %d", i+1, tok.Type, want)
		}
	}
	for _, input := range []string{continuations, comments, mixed} {
		if _, err := Parse(input); err != nil {
			fail("parsing: %v", err)
		}
	}
	fmt.Println(lexStackMarker)
}

// TestLexerNeverReturnsAgain lexes the PostgreSQL regression corpus and
// requires that lex_AGAIN, which a state returns to NextToken's loop, never
// reaches the caller.
func TestLexerNeverReturnsAgain(t *testing.T) {
	for _, input := range lexCorpus(t) {
		lexer := NewLexer(input)
		for tok := lexer.NextToken(); tok.Type != lex_EOF; tok = lexer.NextToken() {
			if tok.Type == lex_AGAIN {
				t.Fatalf("NextToken returned lex_AGAIN at %d", tok.Loc)
			}
		}
	}
}

// FuzzLexerNeverReturnsAgain requires that lex_AGAIN never leaves NextToken
// (the parser would read it, being at most 0, as the end of the input and
// accept a truncated statement), and that Parse does not panic. The seeds
// reach every state.
func FuzzLexerNeverReturnsAgain(f *testing.F) {
	for _, seed := range []string{
		"select 1", "/* a /* b */ c */ x", "-- c\n1", "'a'\n'b'", "E'\\x41'\n'b'",
		"B'1'\n'0'", "X'f'", "U&'d\\0061t' UESCAPE '!'", "U&\"a\"\n'x'", "\"a\"\"b\"",
		"$a$x$a$", "$1 + $2", "1.5e3 .5 0x1F 1_000", "a <= b != c :: int", "'unterminated",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		lexer := NewLexer(input)
		for i := 0; ; i++ {
			tok := lexer.NextToken()
			if tok.Type == lex_AGAIN {
				t.Fatalf("NextToken returned lex_AGAIN at %d", tok.Loc)
			}
			if tok.Type == lex_EOF {
				break
			}
			if i > len(input)+1 {
				t.Fatalf("more tokens than bytes")
			}
		}
		_, _ = Parse(input)
	})
}

// lexCorpus reads every statement file of the PostgreSQL regression corpus.
func lexCorpus(tb testing.TB) []string {
	files, err := filepath.Glob("pgregress/testdata/sql/*.sql")
	if err != nil || len(files) == 0 {
		tb.Fatalf("no corpus: %v", err)
	}
	inputs := make([]string, 0, len(files))
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			tb.Fatal(err)
		}
		inputs = append(inputs, string(data))
	}
	return inputs
}

// BenchmarkLexCorpus lexes every statement file of the PostgreSQL regression
// corpus.
func BenchmarkLexCorpus(b *testing.B) {
	inputs := lexCorpus(b)
	var size int64
	for _, input := range inputs {
		size += int64(len(input))
	}
	b.SetBytes(size)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		for _, input := range inputs {
			lexer := NewLexer(input)
			for lexer.NextToken().Type != lex_EOF {
			}
		}
	}
}

// BenchmarkLexContinuations lexes 10,000 string continuations and 10,000
// comments.
func BenchmarkLexContinuations(b *testing.B) {
	input := "select 'a'" + strings.Repeat("\n'a'", 10_000) + strings.Repeat("/**/", 10_000)
	b.SetBytes(int64(len(input)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		lexer := NewLexer(input)
		for lexer.NextToken().Type != lex_EOF {
		}
	}
}

// TestLexerUnicodeIdentifierIsNotContinued pins that a U&"..." identifier
// followed by a newline and a quoted string is an identifier and a string, as
// PostgreSQL 16 lexes it (2026-10-20: `select U&"a"` newline `'x' as b` is
// the typed literal a 'x', refused in analysis with 42704 type "a" does not
// exist). Only a string continues across a newline. A UESCAPE on the next
// line still applies.
func TestLexerUnicodeIdentifierIsNotContinued(t *testing.T) {
	for _, c := range []struct {
		input string
		want  []Token
	}{
		{"U&\"a\"\n'x'", []Token{{Type: lex_IDENT, Str: "a", Loc: 0}, {Type: lex_SCONST, Str: "x", Loc: 6}}},
		{"U&\"a\" -- c\n 'x'", []Token{{Type: lex_IDENT, Str: "a", Loc: 0}, {Type: lex_SCONST, Str: "x", Loc: 12}}},
		{"U&\"!0061\"\nuescape '!'", []Token{{Type: lex_IDENT, Str: "a", Loc: 0}}},
		{"U&'a'\n'b'", []Token{{Type: lex_USCONST, Str: "ab", Loc: 0}}},
	} {
		lexer := NewLexer(c.input)
		var got []Token
		for tok := lexer.NextToken(); tok.Type != lex_EOF; tok = lexer.NextToken() {
			got = append(got, tok)
		}
		if lexer.Err != nil || !slices.Equal(got, c.want) {
			t.Errorf("lexing %q: %v, %v; want %v", c.input, got, lexer.Err, c.want)
		}
	}
	tree, err := Parse("select U&\"a\"\n'x' as b")
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}
	val := tree.Items[0].(*nodes.SelectStmt).TargetList.Items[0].(*nodes.ResTarget).Val
	if tc, ok := val.(*nodes.TypeCast); !ok || tc.TypeName == nil {
		t.Errorf("select U&\"a\" 'x': %T, want the typed literal a 'x'", val)
	}
}
