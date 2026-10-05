package parser

import (
	"reflect"
	"strings"
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// targetName parses `SELECT 1 AS <label>` and returns the column name the
// parse tree carries, with any notices raised.
func targetName(t *testing.T, label string) (string, []Notice) {
	t.Helper()
	var notices []Notice
	list, err := Parse("SELECT 1 AS "+label, WithNoticeHandler(func(n Notice) { notices = append(notices, n) }))
	if err != nil {
		t.Fatalf("Parse(%q): %v", label, err)
	}
	return list.Items[0].(*nodes.SelectStmt).TargetList.Items[0].(*nodes.ResTarget).Name, notices
}

// TestParseDowncasesASCIIOnly pins PostgreSQL 16's downcase_identifier in a
// multibyte encoding (UTF-8): an unquoted identifier has only ASCII 'A'..'Z'
// lowercased; every other byte is kept, so the length never changes. A
// PostgreSQL 16 server (UTF8) names the columns of
// `SELECT 1 AS Éa, 2 AS İx, 3 AS ABC` "Éa", "İx" and "abc".
func TestParseDowncasesASCIIOnly(t *testing.T) {
	tests := []struct {
		label string
		want  string
	}{
		{"Éa", "Éa"},
		{"İx", "İx"},
		{"ABC", "abc"},
		{"MiXeD", "mixed"},
		{"ÉABC", "Éabc"},
		{`"ABC"`, "ABC"},
		{"\u212Aey", "\u212Aey"}, // KELVIN SIGN is not 'K': an identifier, not the keyword KEY
	}
	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			got, notices := targetName(t, tt.label)
			if got != tt.want {
				t.Errorf("name = %q (%d bytes), want %q (%d bytes)", got, len(got), tt.want, len(tt.want))
			}
			if notices != nil {
				t.Errorf("notices = %+v, want none", notices)
			}
		})
	}
}

// TestParseNonASCIIIsNeverAKeyword pins PostgreSQL's ScanKeywordLookup, which
// folds ASCII case only: "İn" is an identifier named "İn", not the keyword IN
// (which, as a label, would be named "in").
func TestParseNonASCIIIsNeverAKeyword(t *testing.T) {
	if got, _ := targetName(t, "İn"); got != "İn" {
		t.Errorf("name = %q, want %q", got, "İn")
	}
	if kw := LookupKeyword("\u212Aey"); kw != nil {
		t.Errorf("LookupKeyword(KELVIN SIGN + \"ey\") = %+v, want nil", kw)
	}
	if kw := LookupKeyword("KeY"); kw == nil || kw.Name != "key" {
		t.Errorf("LookupKeyword(\"KeY\") = %+v, want key", kw)
	}
}

// TestParseTruncatesDowncasedForm pins that truncation is computed on the
// PostgreSQL-downcased identifier: a 70-byte unquoted identifier keeps its
// leading 2-byte "İ" and is clipped to 63 bytes, where lowercasing "İ" to a
// 1-byte "i" would have kept a different 63-byte prefix.
func TestParseTruncatesDowncasedForm(t *testing.T) {
	label := "İ" + strings.Repeat("Ab", 34)
	if len(label) != 70 {
		t.Fatalf("label is %d bytes, want 70", len(label))
	}
	downcased := "İ" + strings.Repeat("ab", 34)
	want := downcased[:63]

	got, notices := targetName(t, label)
	if got != want {
		t.Errorf("name = %q (%d bytes), want %q (%d bytes)", got, len(got), want, len(want))
	}
	if wantN := []Notice{truncNotice(downcased, want)}; !reflect.DeepEqual(notices, wantN) {
		t.Errorf("notices = %+v, want %+v", notices, wantN)
	}
}

// TestDowncaseKeepsInputWithoutUpper pins that downcase returns its input
// without allocating when there is no ASCII uppercase letter.
func TestDowncaseKeepsInputWithoutUpper(t *testing.T) {
	in := "éa_lower_İ"
	if allocs := testing.AllocsPerRun(100, func() { _ = downcase(in) }); allocs != 0 {
		t.Errorf("downcase allocated %v times, want 0", allocs)
	}
	if got := downcase(in); got != in {
		t.Errorf("downcase(%q) = %q", in, got)
	}
}
