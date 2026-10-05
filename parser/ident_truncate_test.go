package parser

import (
	"reflect"
	"strings"
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// truncNotice is the Notice PostgreSQL 16's truncate_identifier raises.
func truncNotice(original, truncated string) Notice {
	return Notice{
		Code:    "42622",
		Message: `identifier "` + original + `" will be truncated to "` + truncated + `"`,
	}
}

// TestLexerTruncatesIdentifiers pins PostgreSQL 16's truncate_identifier: an
// identifier of NAMEDATALEN (64) bytes or more, quoted or not, is clipped to
// at most 63 bytes on a UTF-8 character boundary (pg_mbcliplen), and each
// clip is reported to the lexer's notice handler. Unquoted identifiers are
// downcased first.
func TestLexerTruncatesIdentifiers(t *testing.T) {
	a := func(n int) string { return strings.Repeat("a", n) }
	e := func(n int) string { return strings.Repeat("é", n) } // 2 bytes each

	tests := []struct {
		name      string
		input     string
		want      string
		truncated bool   // a notice must be reported
		original  string // identifier quoted in the notice, when truncated
	}{
		{"unquoted ascii 63 untouched", a(63), a(63), false, ""},
		{"unquoted ascii 64", a(64), a(63), true, a(64)},
		{"unquoted ascii 70", a(70), a(63), true, a(70)},
		{"quoted ascii 63 untouched", `"` + a(63) + `"`, a(63), false, ""},
		{"quoted ascii 64", `"` + a(64) + `"`, a(63), true, a(64)},
		{"quoted ascii 70", `"` + a(70) + `"`, a(63), true, a(70)},
		{"unquoted 40 e-acute", e(40), e(31), true, e(40)},
		{"quoted 40 e-acute", `"` + e(40) + `"`, e(31), true, e(40)},
		{"unquoted 3-byte rune straddles 63", a(62) + "€b", a(62), true, a(62) + "€b"},
		{"quoted 3-byte rune straddles 63", `"` + a(61) + "€" + `"`, a(61), true, a(61) + "€"},
		{"unquoted 4-byte rune straddles 63", a(60) + "😀", a(60), true, a(60) + "😀"},
		{"quoted 4-byte rune straddles 63", `"` + a(62) + "😀" + `"`, a(62), true, a(62) + "😀"},
		{"unquoted downcased then truncated", strings.Repeat("A", 70), a(63), true, a(70)},
		{"quoted keeps case", `"` + strings.Repeat("A", 70) + `"`, strings.Repeat("A", 63), true, strings.Repeat("A", 70)},
		{"quoted doubled quote is decoded", `"` + a(68) + `""x"`, a(63), true, a(68) + `"x`},
		{"unicode-escaped quoted ascii 70", `U&"` + a(70) + `"`, a(63), true, a(70)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []Notice
			lexer := NewLexer(tt.input)
			lexer.noticeHandler = func(n Notice) { got = append(got, n) }
			tok := lexer.NextToken()
			if tok.Type != lex_IDENT {
				t.Fatalf("expected token type IDENT, got %d (err %v)", tok.Type, lexer.Err)
			}
			if tok.Str != tt.want {
				t.Errorf("str = %q (%d bytes), want %q (%d bytes)", tok.Str, len(tok.Str), tt.want, len(tt.want))
			}
			var want []Notice
			if tt.truncated {
				want = []Notice{truncNotice(tt.original, tt.want)}
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("notices = %+v, want %+v", got, want)
			}
		})
	}
}

// TestLexerTruncatesWithoutNoticeHandler pins that truncation does not depend
// on a handler being set: a nil notice handler still clips, and does not panic.
func TestLexerTruncatesWithoutNoticeHandler(t *testing.T) {
	lexer := NewLexer(`"` + strings.Repeat("é", 40) + `"`)
	tok := lexer.NextToken()
	if want := strings.Repeat("é", 31); tok.Str != want {
		t.Errorf("str = %q, want %q", tok.Str, want)
	}
}

// TestParseNoticesInSourceOrder pins that WithNoticeHandler receives one
// notice per truncated identifier, in source order, with no position, and
// that the parse tree carries the truncated names.
func TestParseNoticesInSourceOrder(t *testing.T) {
	col := strings.Repeat("C", 70)
	tbl := strings.Repeat("é", 40)
	sch := strings.Repeat("s", 64)
	sql := "SELECT x AS " + col + ` FROM ` + sch + `."` + tbl + `"`

	var got []Notice
	list, err := Parse(sql, WithNoticeHandler(func(n Notice) { got = append(got, n) }))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := []Notice{
		truncNotice(strings.Repeat("c", 70), strings.Repeat("c", 63)),
		truncNotice(sch, strings.Repeat("s", 63)),
		truncNotice(tbl, strings.Repeat("é", 31)),
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("notices = %+v\nwant %+v", got, want)
	}
	if list == nil || len(list.Items) != 1 {
		t.Fatalf("expected 1 statement, got %#v", list)
	}
	sel, ok := list.Items[0].(*nodes.SelectStmt)
	if !ok {
		t.Fatalf("expected *nodes.SelectStmt, got %T", list.Items[0])
	}
	if got := sel.TargetList.Items[0].(*nodes.ResTarget).Name; got != strings.Repeat("c", 63) {
		t.Errorf("target name = %q", got)
	}
	rv := sel.FromClause.Items[0].(*nodes.RangeVar)
	if rv.Schemaname != strings.Repeat("s", 63) {
		t.Errorf("schemaname = %q", rv.Schemaname)
	}
	if rv.Relname != strings.Repeat("é", 31) {
		t.Errorf("relname = %q", rv.Relname)
	}
}

// TestParseNoticesBeforeError pins that notices raised before a syntax error
// are still delivered, and the error is still returned.
func TestParseNoticesBeforeError(t *testing.T) {
	ident := strings.Repeat("a", 64)
	var got []Notice
	_, err := Parse("SELECT "+ident+" FROM", WithNoticeHandler(func(n Notice) { got = append(got, n) }))
	if err == nil {
		t.Fatal("Parse: expected a syntax error")
	}
	want := []Notice{truncNotice(ident, ident[:63])}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("notices = %+v, want %+v", got, want)
	}
}

// TestParseNoticeBoundary pins NAMEDATALEN: a 63-byte identifier raises no
// notice and a 64-byte one raises exactly one.
func TestParseNoticeBoundary(t *testing.T) {
	for _, n := range []int{63, 64} {
		ident := strings.Repeat("z", n)
		var got []Notice
		if _, err := Parse("SELECT 1 AS "+ident, WithNoticeHandler(func(n Notice) { got = append(got, n) })); err != nil {
			t.Fatalf("Parse(%d bytes): %v", n, err)
		}
		var want []Notice
		if n == 64 {
			want = []Notice{truncNotice(ident, ident[:63])}
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%d bytes: notices = %+v, want %+v", n, got, want)
		}
	}
}

// TestParseMultibyteNotice pins that the notice and the tree both cut a
// multibyte identifier on a character boundary: 62 ASCII bytes then a 3-byte
// rune keeps 62 bytes, never 63 with half a rune.
func TestParseMultibyteNotice(t *testing.T) {
	ident := strings.Repeat("a", 62) + "€€"
	var got []Notice
	list, err := Parse(`SELECT 1 AS "`+ident+`"`, WithNoticeHandler(func(n Notice) { got = append(got, n) }))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := []Notice{truncNotice(ident, ident[:62])}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("notices = %+v, want %+v", got, want)
	}
	name := list.Items[0].(*nodes.SelectStmt).TargetList.Items[0].(*nodes.ResTarget).Name
	if name != ident[:62] {
		t.Errorf("name = %q (%d bytes), want %d bytes", name, len(name), 62)
	}
}

// TestParseWithoutNoticeHandler pins that the handler only observes: Parse
// with no options does not panic and yields the same tree as Parse with one.
func TestParseWithoutNoticeHandler(t *testing.T) {
	sql := `SELECT ` + strings.Repeat("Q", 80) + ` FROM "` + strings.Repeat("é", 40) + `"`
	plain, err := Parse(sql)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	var count int
	observed, err := Parse(sql, WithNoticeHandler(func(Notice) { count++ }))
	if err != nil {
		t.Fatalf("Parse with handler: %v", err)
	}
	if count != 2 {
		t.Errorf("handler called %d times, want 2", count)
	}
	if !reflect.DeepEqual(plain, observed) {
		t.Errorf("trees differ:\nplain    %#v\nobserved %#v", plain, observed)
	}
}
