package parser

import (
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// TestRangeFunctionIsPostgreSQL16s pins PostgreSQL 16's answers for 10
// functions in FROM, alone and in ROWS FROM, with ordinality, aliases,
// column definitions and LATERAL, and malformed ones.
func TestRangeFunctionIsPostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/range_function_pg16.jsonl", 10)
}

// TestRangeFunctionItemsArePairs requires each of a RangeFunction's
// functions to be the pair PostgreSQL 16 makes, the call and its column
// definition list, nil when there is none, for a lone function and in ROWS
// FROM.
func TestRangeFunctionItemsArePairs(t *testing.T) {
	for sql, want := range map[string][]bool{
		"select * from f()": {false},
		"select * from rows from (f(), g() as (a int)) as x": {false, true},
	} {
		list, err := Parse(sql)
		if err != nil {
			t.Fatal(err)
		}
		rf := list.Items[0].(*nodes.SelectStmt).FromClause.Items[0].(*nodes.RangeFunction)
		for i, item := range rf.Functions.Items {
			pair := item.(*nodes.List).Items
			if _, ok := pair[0].(*nodes.FuncCall); !ok || len(pair) != 2 || (pair[1] != nil) != want[i] {
				t.Errorf("%s: function %d is %#v, want a call and a definition list %v", sql, i, pair, want[i])
			}
		}
	}
}
