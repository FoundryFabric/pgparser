package parser

import (
	"strings"
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// TestReturnsTableIsPostgreSQL16s pins PostgreSQL 16's answers for 39
// statements with RETURNS TABLE: one column and several, of plain,
// qualified, SQL-standard, typmod, array and %TYPE types, after IN, OUT,
// INOUT, VARIADIC and default arguments, and malformed column lists.
func TestReturnsTableIsPostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/returns_table_pg16.jsonl", 39)
}

// TestReturnsTableIsASet requires RETURNS TABLE's return type to be a set
// of its one column's type, else a set of record, at TABLE, as
// libpg_query's tree has it.
func TestReturnsTableIsASet(t *testing.T) {
	for _, c := range []struct {
		sql   string
		names []string
		mods  int
	}{
		{"create function f() returns table (a varchar(5)) language sql as ''", []string{"pg_catalog", "varchar"}, 1},
		{"create function f(x int) returns table (a s.t) language sql as ''", []string{"s", "t"}, 0},
		{"create function f() returns table (a int, b text) language sql as ''", []string{"pg_catalog", "record"}, 0},
	} {
		list, err := Parse(c.sql)
		if err != nil {
			t.Fatal(err)
		}
		tn := list.Items[0].(*nodes.CreateFunctionStmt).ReturnType
		var names []string
		for _, n := range tn.Names.Items {
			names = append(names, n.(*nodes.String).Str)
		}
		mods := 0
		if tn.Typmods != nil {
			mods = len(tn.Typmods.Items)
		}
		at := nodes.ParseLoc(strings.Index(c.sql, "table"))
		if !tn.Setof || tn.Location != at || strings.Join(names, ".") != strings.Join(c.names, ".") || mods != c.mods {
			t.Errorf("%s: return type %v setof %v at %d with %d typmods, want %v setof at %d with %d", c.sql, names, tn.Setof, tn.Location, mods, c.names, at, c.mods)
		}
	}
}
