package parser

import (
	"reflect"
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// TestOverlapsIsPostgreSQL16s pins PostgreSQL 16's answers for 10 OVERLAPS
// expressions: pairs, explicit and implicit rows, and rows of another
// length, which PostgreSQL 16 refuses at the row.
func TestOverlapsIsPostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/overlaps_pg16.jsonl", 10)
}

// TestOverlapsIsPgCatalogs requires OVERLAPS to be a call of
// pg_catalog.overlaps on the four values, as PostgreSQL 16's
// SystemFuncName("overlaps") names it.
func TestOverlapsIsPgCatalogs(t *testing.T) {
	list, err := Parse("select (1, 2) overlaps row(3, 4)")
	if err != nil {
		t.Fatal(err)
	}
	fc := list.Items[0].(*nodes.SelectStmt).TargetList.Items[0].(*nodes.ResTarget).Val.(*nodes.FuncCall)
	var name []string
	for _, s := range fc.Funcname.Items {
		name = append(name, s.(*nodes.String).Str)
	}
	if !reflect.DeepEqual(name, []string{"pg_catalog", "overlaps"}) || len(fc.Args.Items) != 4 {
		t.Errorf("%v of %d arguments, want pg_catalog.overlaps of 4", name, len(fc.Args.Items))
	}
}
