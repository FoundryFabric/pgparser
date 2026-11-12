package parser

import (
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// TestParseJSONTableNestedPath pins how a JSON_TABLE column that starts with
// NESTED is read: NESTED [PATH] followed by a path string is a nested column
// path, and NESTED followed by a type is a column named nested.
func TestParseJSONTableNestedPath(t *testing.T) {
	for _, c := range []struct {
		sql  string
		typ  nodes.JsonTableColumnType
		name string
	}{
		{"select * from json_table('{}', '$' columns (nested path '$' columns (a int)))", nodes.JTC_NESTED, ""},
		{"select * from json_table('{}', '$' columns (nested '$' columns (a int)))", nodes.JTC_NESTED, ""},
		{"select * from json_table('{}', '$' columns (nested int))", nodes.JTC_REGULAR, "nested"},
		{"select * from json_table('{}', '$' columns (nested text path '$'))", nodes.JTC_REGULAR, "nested"},
	} {
		stmts, err := Parse(c.sql)
		if err != nil {
			t.Fatalf("Parse(%q): %v", c.sql, err)
		}
		jt := stmts.Items[0].(*nodes.SelectStmt).FromClause.Items[0].(*nodes.JsonTable)
		col := jt.Columns.Items[0].(*nodes.JsonTableColumn)
		if col.Coltype != c.typ || col.Name != c.name {
			t.Errorf("Parse(%q): column {%v, %q}, want {%v, %q}", c.sql, col.Coltype, col.Name, c.typ, c.name)
		}
	}
	if _, err := Parse("select * from json_table('{}', '$' columns (nested path))"); err == nil {
		t.Error("Parse of a column named nested of type path: no error, want the syntax error NESTED PATH without a path gives")
	}
}
