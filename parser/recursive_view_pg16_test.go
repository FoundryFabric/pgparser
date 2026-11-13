package parser

import (
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// TestRecursiveViewIsPostgreSQL16s pins PostgreSQL 16's answers for 8
// CREATE [OR REPLACE] RECURSIVE VIEW statements, and malformed ones.
func TestRecursiveViewIsPostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/recursive_view_pg16.jsonl", 8)
}

// TestRecursiveViewSelectsItsColumns requires a recursive view's query to
// be PostgreSQL 16's makeRecursiveViewSelect: WITH RECURSIVE over the view's
// columns, selecting each by name, unlocated.
func TestRecursiveViewSelectsItsColumns(t *testing.T) {
	list, err := Parse("create recursive view v (a, b) as values (1, 2)")
	if err != nil {
		t.Fatal(err)
	}
	sel := list.Items[0].(*nodes.ViewStmt).Query.(*nodes.SelectStmt)
	var cols []string
	for _, rt := range sel.TargetList.Items {
		cols = append(cols, rt.(*nodes.ResTarget).Val.(*nodes.ColumnRef).Fields.Items[0].(*nodes.String).Str)
	}
	cte := sel.WithClause.Ctes.Items[0].(*nodes.CommonTableExpr)
	if len(cols) != 2 || cols[0] != "a" || cols[1] != "b" || sel.WithClause.Location != -1 || cte.Location != -1 || cte.Ctename != "v" {
		t.Errorf("columns %v, WITH at %d, CTE %s at %d; want a and b, unlocated, v", cols, sel.WithClause.Location, cte.Ctename, cte.Location)
	}
}
