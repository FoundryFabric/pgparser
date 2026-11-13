package parser

import (
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// TestIsJSONIsPostgreSQL16s pins PostgreSQL 16's answers for 8 IS [NOT]
// JSON predicates of each item type, with and without unique keys, and
// malformed ones.
func TestIsJSONIsPostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/is_json_pg16.jsonl", 8)
}

// TestIsJSONHasTheDefaultFormat requires IS [NOT] JSON to carry the
// default, unlocated JsonFormat, as PostgreSQL 16's grammar gives it.
func TestIsJSONHasTheDefaultFormat(t *testing.T) {
	for _, sql := range []string{"select '1' is json", "select '1' is not json"} {
		list, err := Parse(sql)
		if err != nil {
			t.Fatal(err)
		}
		n := list.Items[0].(*nodes.SelectStmt).TargetList.Items[0].(*nodes.ResTarget).Val
		if b, ok := n.(*nodes.BoolExpr); ok {
			n = b.Args.Items[0]
		}
		if f := n.(*nodes.JsonIsPredicate).Format; f == nil || f.FormatType != nodes.JS_FORMAT_DEFAULT || f.Location != -1 {
			t.Errorf("%s: format %+v, want the default at -1", sql, f)
		}
	}
}
