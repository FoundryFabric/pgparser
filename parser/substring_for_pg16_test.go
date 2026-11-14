package parser

import (
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// TestSubstringForCastsTheLength requires SUBSTRING(x FOR n) to be
// pg_catalog.substring(x, 1, n::pg_catalog.int4), as PostgreSQL 16's
// substr_list builds it: the start an unlocated Integer 1, and the length
// in an unlocated cast to int4. FROM ... FOR keeps the length bare.
func TestSubstringForCastsTheLength(t *testing.T) {
	args := func(sql string) []nodes.Node {
		t.Helper()
		list, err := Parse(sql)
		if err != nil {
			t.Fatal(err)
		}
		return list.Items[0].(*nodes.SelectStmt).TargetList.Items[0].(*nodes.ResTarget).Val.(*nodes.FuncCall).Args.Items
	}
	a := args("select substring(d for 30)")
	if len(a) != 3 {
		t.Fatalf("%d arguments, want 3", len(a))
	}
	if c, ok := a[1].(*nodes.A_Const); !ok || c.Location != -1 || c.Val.(*nodes.Integer).Ival != 1 {
		t.Errorf("start %#v, want the unlocated Integer 1", a[1])
	}
	tc, ok := a[2].(*nodes.TypeCast)
	if !ok {
		t.Fatalf("length %T, want a TypeCast", a[2])
	}
	names := tc.TypeName.Names.Items
	if tc.Location != -1 || tc.TypeName.Location != -1 || len(names) != 2 ||
		names[0].(*nodes.String).Str != "pg_catalog" || names[1].(*nodes.String).Str != "int4" {
		t.Errorf("length cast %#v to %v, want an unlocated cast to pg_catalog.int4", tc, names)
	}
	if c, ok := tc.Arg.(*nodes.A_Const); !ok || c.Location != 23 || c.Val.(*nodes.Integer).Ival != 30 {
		t.Errorf("cast argument %#v, want 30 at 23", tc.Arg)
	}
	if _, ok := args("select substring(d from 1 for 30)")[2].(*nodes.A_Const); !ok {
		t.Error("FROM ... FOR: the length is cast, want it bare")
	}
}
