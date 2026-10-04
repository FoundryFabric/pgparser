package parser

import (
	"reflect"
	"strings"
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// TestLexerTruncatesIdentifiers pins PostgreSQL 16's truncate_identifier: an
// identifier of NAMEDATALEN (64) bytes or more, quoted or not, is clipped to
// at most 63 bytes on a UTF-8 character boundary (pg_mbcliplen), and each
// clip is reported. Unquoted identifiers are downcased first.
func TestLexerTruncatesIdentifiers(t *testing.T) {
	a := func(n int) string { return strings.Repeat("a", n) }
	e := func(n int) string { return strings.Repeat("é", n) } // 2 bytes each

	tests := []struct {
		name      string
		input     string
		want      string
		truncated bool   // a truncation must be reported
		original  string // reported Original, when truncated
	}{
		{"unquoted ascii 63 untouched", a(63), a(63), false, ""},
		{"unquoted ascii 64", a(64), a(63), true, a(64)},
		{"unquoted ascii 70", a(70), a(63), true, a(70)},
		{"quoted ascii 63 untouched", `"` + a(63) + `"`, a(63), false, ""},
		{"quoted ascii 70", `"` + a(70) + `"`, a(63), true, a(70)},
		{"unquoted 40 e-acute", e(40), e(31), true, e(40)},
		{"quoted 40 e-acute", `"` + e(40) + `"`, e(31), true, e(40)},
		{"unquoted 3-byte rune straddles 63", a(62) + "€b", a(62), true, a(62) + "€b"},
		{"quoted 3-byte rune straddles 63", `"` + a(61) + "€" + `"`, a(61), true, a(61) + "€"},
		{"unquoted 4-byte rune straddles 63", a(60) + "😀", a(60), true, a(60) + "😀"},
		{"quoted 4-byte rune straddles 63", `"` + a(62) + "😀" + `"`, a(62), true, a(62) + "😀"},
		{"unquoted downcased then truncated", strings.Repeat("A", 70), a(63), true, a(70)},
		{"quoted keeps case", `"` + strings.Repeat("A", 70) + `"`, strings.Repeat("A", 63), true, strings.Repeat("A", 70)},
		{"unicode-escaped quoted ascii 70", `U&"` + a(70) + `"`, a(63), true, a(70)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lexer := NewLexer(tt.input)
			tok := lexer.NextToken()
			if tok.Type != lex_IDENT {
				t.Fatalf("expected token type IDENT, got %d (err %v)", tok.Type, lexer.Err)
			}
			if tok.Str != tt.want {
				t.Errorf("str = %q (%d bytes), want %q (%d bytes)", tok.Str, len(tok.Str), tt.want, len(tt.want))
			}
			var want []IdentTruncation
			if tt.truncated {
				want = []IdentTruncation{{Original: tt.original, Truncated: tt.want, Loc: 0}}
			}
			if !reflect.DeepEqual(lexer.Truncations, want) {
				t.Errorf("Truncations = %+v, want %+v", lexer.Truncations, want)
			}
		})
	}
}

// TestParseWithTruncationsReportsInSourceOrder pins that ParseWithTruncations
// returns the lexer's truncations, in source order with token offsets, and
// that the parse tree carries the truncated names.
func TestParseWithTruncationsReportsInSourceOrder(t *testing.T) {
	col := strings.Repeat("C", 70)
	tbl := strings.Repeat("é", 40)
	sql := "SELECT x AS " + col + ` FROM "` + tbl + `"`

	list, truncations, err := ParseWithTruncations(sql)
	if err != nil {
		t.Fatalf("ParseWithTruncations: %v", err)
	}
	want := []IdentTruncation{
		{Original: strings.Repeat("c", 70), Truncated: strings.Repeat("c", 63), Loc: len("SELECT x AS ")},
		{Original: tbl, Truncated: strings.Repeat("é", 31), Loc: len("SELECT x AS " + col + " FROM ")},
	}
	if !reflect.DeepEqual(truncations, want) {
		t.Errorf("truncations = %+v, want %+v", truncations, want)
	}
	if list == nil || len(list.Items) != 1 {
		t.Fatalf("expected 1 statement, got %#v", list)
	}
	sel, ok := list.Items[0].(*nodes.SelectStmt)
	if !ok {
		t.Fatalf("expected *nodes.SelectStmt, got %T", list.Items[0])
	}
	if got := sel.TargetList.Items[0].(*nodes.ResTarget).Name; got != want[0].Truncated {
		t.Errorf("target name = %q, want %q", got, want[0].Truncated)
	}
	if got := sel.FromClause.Items[0].(*nodes.RangeVar).Relname; got != want[1].Truncated {
		t.Errorf("relname = %q, want %q", got, want[1].Truncated)
	}
}
