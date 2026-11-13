package parser

import (
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// TestSortByIsPostgreSQL16s pins PostgreSQL 16's answers for 12 ORDER BY
// clauses of a query, a window, an aggregate, WITHIN GROUP and the JSON
// aggregates, with and without USING, and malformed ones.
func TestSortByIsPostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/sortby_location_pg16.jsonl", 12)
}

// TestSortByWithoutUsingIsUnlocated requires a SortBy without USING to
// have location -1, as PostgreSQL 16's sortby leaves it: its location is
// its operator's.
func TestSortByWithoutUsingIsUnlocated(t *testing.T) {
	list, err := Parse("select * from t order by a desc nulls first, b")
	if err != nil {
		t.Fatal(err)
	}
	for i, s := range list.Items[0].(*nodes.SelectStmt).SortClause.Items {
		if loc := s.(*nodes.SortBy).Location; loc != -1 {
			t.Errorf("sort key %d: location %d, want -1", i, loc)
		}
	}
}
