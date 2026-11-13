package parser

import (
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// TestBooleanConstIsPostgreSQL16s pins PostgreSQL 16's answers for 37
// statements with a BOOLEAN 'string' constant, the spellings of it
// PostgreSQL 16 refuses, and the other typed constants and BOOLEAN types
// beside them.
func TestBooleanConstIsPostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/boolean_const_pg16.jsonl", 37)
}

// TestBooleanConstIsACast requires BOOLEAN 'string' to be the string cast
// to bool, as PostgreSQL 16's AexprConst makes it.
func TestBooleanConstIsACast(t *testing.T) {
	list, err := Parse("select boolean 'true'")
	if err != nil {
		t.Fatal(err)
	}
	tc, ok := list.Items[0].(*nodes.SelectStmt).TargetList.Items[0].(*nodes.ResTarget).Val.(*nodes.TypeCast)
	if !ok {
		t.Fatalf("value is not a TypeCast")
	}
	names := tc.TypeName.Names.Items
	if s := names[len(names)-1].(*nodes.String).Str; s != "bool" {
		t.Errorf("type %q, want bool", s)
	}
	if a, ok := tc.Arg.(*nodes.A_Const); !ok || a.Val.(*nodes.String).Str != "true" {
		t.Errorf("argument %+v, want the string true", tc.Arg)
	}
}
