package parser

import (
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// TestBoolConstIsPostgreSQL16s pins PostgreSQL 16's answers for 21
// statements with a CYCLE clause, with and without TO and DEFAULT, and
// XMLPARSE with and without a whitespace option, and malformed ones.
func TestBoolConstIsPostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/bool_const_pg16.jsonl", 21)
}

// TestBoolConstIsBoolean requires the constants the grammar makes for a
// CYCLE clause's marks and XMLPARSE's whitespace option to be booleans, as
// libpg_query's tree has them.
func TestBoolConstIsBoolean(t *testing.T) {
	boolval := func(n nodes.Node) any {
		if b, ok := n.(*nodes.A_Const).Val.(*nodes.Boolean); ok {
			return b.Boolval
		}
		return n.(*nodes.A_Const).Val
	}
	list, err := Parse("with recursive t (a) as (select 1 union all select a + 1 from t) cycle a set c using p select * from t")
	if err != nil {
		t.Fatal(err)
	}
	cc := list.Items[0].(*nodes.SelectStmt).WithClause.Ctes.Items[0].(*nodes.CommonTableExpr).CycleClause.(*nodes.CTECycleClause)
	if v, d := boolval(cc.CycleMarkValue), boolval(cc.CycleMarkDefault); v != true || d != false {
		t.Errorf("cycle marks %v and %v, want true and false", v, d)
	}
	for sql, want := range map[string]bool{
		"select xmlparse(content '<a/>')":                     false,
		"select xmlparse(content '<a/>' preserve whitespace)": true,
		"select xmlparse(document '<a/>' strip whitespace)":   false,
	} {
		list, err := Parse(sql)
		if err != nil {
			t.Fatal(err)
		}
		x := list.Items[0].(*nodes.SelectStmt).TargetList.Items[0].(*nodes.ResTarget).Val.(*nodes.XmlExpr)
		if got := boolval(x.Args.Items[1]); got != want {
			t.Errorf("%s: preserve whitespace %v, want %v", sql, got, want)
		}
	}
}
