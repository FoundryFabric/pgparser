package parser

import (
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// TestNormalizedIsPostgreSQL16s pins PostgreSQL 16's answers for 8 IS
// [NOT] [form] NORMALIZED expressions, and malformed ones.
func TestNormalizedIsPostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/normalized_pg16.jsonl", 8)
}

// TestNormalizedWithoutAFormHasOneArgument requires IS NORMALIZED without
// a form to call is_normalized on the string alone, as PostgreSQL 16 does,
// leaving the form to the function's default, and with one to pass it.
func TestNormalizedWithoutAFormHasOneArgument(t *testing.T) {
	for sql, want := range map[string]int{
		"select 'a' is normalized":     1,
		"select 'a' is nfd normalized": 2,
	} {
		list, err := Parse(sql)
		if err != nil {
			t.Fatal(err)
		}
		fc := list.Items[0].(*nodes.SelectStmt).TargetList.Items[0].(*nodes.ResTarget).Val.(*nodes.FuncCall)
		if len(fc.Args.Items) != want {
			t.Errorf("%s: %d arguments, want %d", sql, len(fc.Args.Items), want)
		}
	}
}
