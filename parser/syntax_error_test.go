package parser

import (
	"errors"
	"testing"
)

// TestSyntaxErrorIsAtOrNearTheToken pins that a token the grammar refuses is
// named as PostgreSQL 16's scanner_yyerror names it: its source text as
// written, quotes included, or the end of the input. Messages and 0-based
// positions are PostgreSQL 16.15's answers.
func TestSyntaxErrorIsAtOrNearTheToken(t *testing.T) {
	for _, c := range []struct {
		sql, message string
		position     int
	}{
		{"select 'w' collate default", `syntax error at or near "default"`, 19},
		{"SELEC 1", `syntax error at or near "SELEC"`, 0},
		{"SELECT 1 1", `syntax error at or near "1"`, 9},
		{"SELECT * FRM t", `syntax error at or near "FRM"`, 9},
		{"SELECT 1 +", "syntax error at end of input", 10},
		{"select 1 + /* c */", "syntax error at end of input", 18},
		{"select 1 with 1", `syntax error at or near "with"`, 9}, // WITH reads a token ahead
		{"SELECT 'a' 'b'", `syntax error at or near "'b'"`, 11},
		{"SELECT E'a' E'b'", `syntax error at or near "E'b'"`, 12},
		{"select $$x$$ $$y$$", `syntax error at or near "$$y$$"`, 13},
		{`SELECT "Foo" "Bar" "Baz"`, `syntax error at or near ""Baz""`, 19},
		{"select 'a'\n'b' 'c'", `syntax error at or near "'c'"`, 15},
	} {
		_, err := Parse(c.sql)
		var pe *ParseError
		if !errors.As(err, &pe) || pe.Code != codeSyntaxError || pe.Message != c.message || pe.Position != c.position {
			t.Errorf("Parse(%q) = %#v, want 42601 %q at %d", c.sql, err, c.message, c.position)
		}
	}
}

// TestGrammarActionErrorIsKept pins that a grammar action's own message is
// not given the at-or-near suffix: PostgreSQL 16 reports these with a
// position only.
func TestGrammarActionErrorIsKept(t *testing.T) {
	_, err := Parse("create role r unencrypted password 'x'")
	var pe *ParseError
	if !errors.As(err, &pe) || pe.Message != "UNENCRYPTED PASSWORD is no longer supported" {
		t.Errorf("Parse(create role r unencrypted password 'x') = %#v, want UNENCRYPTED PASSWORD is no longer supported", err)
	}
}
