package nodes_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/pgplex/pgparser/nodes"
	"github.com/pgplex/pgparser/parser"
)

// targetExpr is the expression of the last target of a single SELECT.
func targetExpr(t *testing.T, sql string) nodes.Node {
	t.Helper()
	stmts, err := parser.Parse(sql)
	if err != nil {
		t.Fatalf("Parse(%q): %v", sql, err)
	}
	n := stmts.Items[0]
	if raw, ok := n.(*nodes.RawStmt); ok {
		n = raw.Stmt
	}
	targets := n.(*nodes.SelectStmt).TargetList.Items
	return targets[len(targets)-1].(*nodes.ResTarget).Val
}

// TestExprLocationIsWhereTheExpressionStarts pins ExprLocation over parsed
// expressions of every kind it knows: each is the last target of "select
// 'é', <expr>", so the expression starts after a two-byte character and
// the answer is a byte offset (PostgreSQL's locations are byte offsets
// too; a client counting characters converts). want is the text the
// expression starts with, found in the statement.
func TestExprLocationIsWhereTheExpressionStarts(t *testing.T) {
	cases := []struct{ expr, starts string }{
		{"42", "42"},
		{"'s'", "'s'"},
		{"-42", "-42"},
		{"t.x", "t.x"},
		{"$1", "$1"},
		{"x + y * z", "x"},
		{"1 + 2 + 3 + 4", "1"},
		{"- x", "-"},
		{"x is distinct from y", "x"},
		{"x and y and z", "x"},
		{"x or y and z", "x"},
		{"not x", "not"},
		{"(not x) and y", "not"},
		{"x is null", "x"},
		{"x isnull", "x"},
		{"(x) is not null", "x"},
		{"x notnull", "x"},
		{"x is not true", "x"},
		{"f(x)", "f"},
		// timezone(zone, x) at AT: PostgreSQL's rule looks at the name and
		// the first argument, the zone, so AT wins over the earlier x.
		{"x at time zone 'utc'", "at"},
		{"x::int", "x"},
		{"cast(x as int)", "cast"},
		{"array[x, y]", "array"},
		{"array[[x], [y]]", "array"},
		{"array(select 1)", "array"},
		{"(select 1)", "(select"},
		{"exists (select 1)", "exists"},
		{"x in (select 1)", "x"},
		{"x not in (select 1)", "x"},
		{"x = any (select 1)", "x"},
		{"case when x then 1 end", "case"},
		{"row(x, y)", "row"},
		{"(x, y, z)", "(x"},
		{"x is true and y is null or not z", "x"},
	}
	for _, c := range cases {
		sql := "select 'é', " + c.expr
		want := strings.Index(sql, c.starts)
		if want < 0 {
			t.Fatalf("%q does not contain %q", sql, c.starts)
		}
		if got := nodes.ExprLocation(targetExpr(t, sql)); got != want {
			t.Errorf("ExprLocation(%s) = %d, want %d (%q)", c.expr, got, want, c.starts)
		}
	}
}

// TestExprLocationRules pins the rules on hand-built nodes, where each
// location can be placed independently of the grammar: an own location
// that precedes the operand wins, an unknown one (-1) defers to the
// operand, a list is at its first member with a location, a TypeCast also
// considers its TypeName, and an unlisted kind is unknown.
func TestExprLocationRules(t *testing.T) {
	at := func(l nodes.ParseLoc) nodes.Node { return &nodes.ColumnRef{Location: l} }
	list := func(items ...nodes.Node) *nodes.List { return &nodes.List{Items: items} }
	cases := []struct {
		name string
		n    nodes.Node
		want int
	}{
		{"nil", nil, -1},
		{"A_Const", &nodes.A_Const{Location: 3}, 3},
		{"ParamRef unknown", &nodes.ParamRef{Location: -1}, -1},
		{"A_Expr operand first", &nodes.A_Expr{Lexpr: at(2), Location: 5}, 2},
		{"A_Expr prefix", &nodes.A_Expr{Rexpr: at(9), Location: 5}, 5},
		{"A_Expr operand unknown", &nodes.A_Expr{Lexpr: at(-1), Location: 5}, 5},
		{"A_Expr both unknown", &nodes.A_Expr{Lexpr: at(-1), Location: -1}, -1},
		{"BoolExpr NOT before operand", &nodes.BoolExpr{Boolop: nodes.NOT_EXPR, Args: list(at(4)), Location: 0}, 0},
		{"BoolExpr first located arg", &nodes.BoolExpr{Args: list(at(-1), at(7), at(1)), Location: 9}, 7},
		{"BoolExpr nil args", &nodes.BoolExpr{Location: 9}, 9},
		{"NullTest", &nodes.NullTest{Arg: at(1), Location: 3}, 1},
		{"NullTest unknown arg", &nodes.NullTest{Arg: at(-1), Location: 3}, 3},
		{"BooleanTest", &nodes.BooleanTest{Arg: at(1), Location: 3}, 1},
		{"FuncCall name first", &nodes.FuncCall{Args: list(at(5)), Location: 0}, 0},
		{"FuncCall arg first", &nodes.FuncCall{Args: list(at(0)), Location: 2}, 0},
		{"TypeCast typename first", &nodes.TypeCast{Arg: at(10), TypeName: &nodes.TypeName{Location: 3}, Location: -1}, 3},
		{"TypeCast own first", &nodes.TypeCast{Arg: at(10), TypeName: &nodes.TypeName{Location: 3}, Location: 1}, 1},
		{"TypeCast no typename", &nodes.TypeCast{Arg: at(10), Location: -1}, 10},
		{"SubLink testexpr first", &nodes.SubLink{Testexpr: at(1), Location: 3}, 1},
		{"SubLink no testexpr", &nodes.SubLink{Location: 3}, 3},
		{"A_ArrayExpr own only", &nodes.A_ArrayExpr{Elements: list(at(1)), Location: 4}, 4},
		{"CaseExpr own only", &nodes.CaseExpr{Arg: at(1), Location: 4}, 4},
		{"RowExpr own only", &nodes.RowExpr{Args: list(at(1)), Location: 4}, 4},
		{"List", list(at(-1), &nodes.String{Str: "s"}, at(6)), 6},
		{"empty List", list(), -1},
		{"nil List", (*nodes.List)(nil), -1},
		{"unlisted kind", &nodes.String{Str: "s"}, -1},
		{"unlisted kind under an operator", &nodes.A_Expr{Lexpr: &nodes.String{Str: "s"}, Location: 5}, 5},
	}
	for _, c := range cases {
		if got := nodes.ExprLocation(c.n); got != c.want {
			t.Errorf("%s: ExprLocation = %d, want %d", c.name, got, c.want)
		}
	}
}

// TestExprLocationLongChains pins that a 10,000-term left-deep operator
// chain is answered by walking its left spine, without allocating.
func TestExprLocationLongChains(t *testing.T) {
	const n = 10000
	var sb strings.Builder
	sb.WriteString("select 'é', c0")
	for i := 1; i < n; i++ {
		fmt.Fprintf(&sb, " + c%d", i)
	}
	sql := sb.String()
	expr := targetExpr(t, sql)
	if got, want := nodes.ExprLocation(expr), strings.Index(sql, "c0"); got != want {
		t.Errorf("%d-term + chain: ExprLocation %d, want %d", n, got, want)
	}
	if allocs := testing.AllocsPerRun(10, func() { nodes.ExprLocation(expr) }); allocs != 0 {
		t.Errorf("ExprLocation allocates %.0f times per call", allocs)
	}
}
