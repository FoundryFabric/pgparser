package parser

import (
	"errors"
	"math/rand"
	"slices"
	"strings"
	"testing"
	"time"
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

// TestLexerOperatorRules pins scan.l's operator rule: an operator is the
// maximal run of operator characters, cut where "/*" or "--" begins a comment
// inside it, and with every trailing + and - trimmed unless it holds one of
// ~ ! @ # ^ & | ` ? %. The rest of the run lexes as the following tokens. Each
// case was checked against PostgreSQL 16.15 (2026-10-20) as the comment says:
// the value of "select 1 <ops> 1", or the operator its 42883 error names.
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
		// a later token of the run, after a comment cut, needs a fresh search
		// for a qualifying character, and a fresh trim
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

// opScanStepsFor lexes "select 1 " followed by n '+' and " 1", failing t
// unless every '+' is its own token, and returns the positions lexOperator's
// scans examined.
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

// TestLexerOperatorRunIsLinear guards the lexer against an operator run that
// takes super-linear time: one short statement could tie up a CPU, as a run
// of 4,000 '+' took 12 seconds when each token rescanned the rest of the run.
//
// The deadline is the guard: a run of 1,000,000 '+' lexes in about 20ms (about
// 200ms under -race), and must finish within 2 seconds wherever the slowness
// would come from. It lexes in a goroutine so that a slow lexer fails the test
// at the deadline rather than hanging it; the goroutine is left running then.
//
// The step count is a cheap check of lexOperator's caches in particular: the
// positions its scans examine for a run of 2,000 must stay within 4 times
// those for 500, plus a constant.
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

// refToken is a token as refLex finds it: where it starts, its text, and the
// token type the lexer must give it.
type refToken struct {
	loc  int
	text string
	typ  int
}

// refLex lexes s from scratch, as scan.l does, for the alphabet
// TestLexerOperatorMatchesReference draws from: whitespace, nested /* */ and
// -- comments, "::" and ":=", ':', identifiers of letters and digits, digit
// runs, and operators. An operator is found by the plain per-token rule: the
// run of operator characters, cut at the first "/*" or "--" in it, then with
// every trailing + and - trimmed unless the operator holds ~ ! @ # ^ & | ` ?
// or %. It returns the tokens, and the error the lexer must stop with: its
// 0-based position and message, or -1.
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

// TestLexerOperatorMatchesReference lexes seeded random strings of operator
// characters, comment pieces, whitespace, letters, digits and colons with the
// lexer and with refLex, and wants the same tokens (position, text and type)
// and the same error.
func TestLexerOperatorMatchesReference(t *testing.T) {
	pieces := []string{
		"~", "!", "@", "#", "^", "&", "|", "`", "?", "%", "+", "-", "*", "/", "<", ">", "=",
		"/", "*", "-", "+", " ", "\n", "a", "b", "1", ":", "/**/", "/*+-*/", "--x\n", "<>", "::", "+-", "<<",
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
			got = append(got, refToken{tok.Loc, s[tok.Loc:min(lexer.pos, len(s))], tok.Type})
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
