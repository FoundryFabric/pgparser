package parser

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// boolShape renders the AND/OR/NOT, IS [NOT] NULL and IS [NOT] TRUE/FALSE/
// UNKNOWN nodes of a raw expression with their locations, one argument list
// per node; a column reference is its name and anything else is "_". So
// "a and b and c" parsed flat at byte 24 renders as "and@24(a,b,c)".
func boolShape(n nodes.Node) string {
	var sb strings.Builder
	var walk func(n nodes.Node)
	walk = func(n nodes.Node) {
		switch e := n.(type) {
		case *nodes.BoolExpr:
			fmt.Fprintf(&sb, "%s@%d(", [...]string{"and", "or", "not"}[e.Boolop], e.Location)
			for i, a := range e.Args.Items {
				if i > 0 {
					sb.WriteByte(',')
				}
				walk(a)
			}
			sb.WriteByte(')')
		case *nodes.NullTest:
			fmt.Fprintf(&sb, "%s@%d(", [...]string{"isnull", "notnull"}[e.Nulltesttype], e.Location)
			walk(e.Arg)
			sb.WriteByte(')')
		case *nodes.BooleanTest:
			fmt.Fprintf(&sb, "booltest%d@%d(", e.Booltesttype, e.Location)
			walk(e.Arg)
			sb.WriteByte(')')
		case *nodes.ColumnRef:
			sb.WriteString(e.Fields.Items[len(e.Fields.Items)-1].(*nodes.String).Str)
		default:
			sb.WriteByte('_')
		}
	}
	walk(n)
	return sb.String()
}

// selectOf is the parsed single SELECT.
func selectOf(t *testing.T, sql string) *nodes.SelectStmt {
	t.Helper()
	stmts, err := Parse(sql)
	if err != nil {
		t.Fatalf("Parse(%q): %v", sql, err)
	}
	n := stmts.Items[0]
	if raw, ok := n.(*nodes.RawStmt); ok {
		n = raw.Stmt
	}
	return n.(*nodes.SelectStmt)
}

// TestBoolExprShapeIsPostgreSQLs pins the raw tree of AND/OR/NOT chains and
// the location of every BoolExpr, NullTest and BooleanTest to PostgreSQL
// 16's. Each want was read from PostgreSQL 16.15's debug_print_parse for the
// same text over t(a, b, c, d bool, x int, j text, y xml), with the prefix
// before the SELECT subtracted (verified 2026-10-20); analysis keeps the
// raw BoolExpr nesting and locations. Locations are byte offsets, as in
// PostgreSQL: the 'é' case puts AND at 31, not at character 30.
//
// A chain extends its LEFT operand only when that is the same connective,
// so "(a and b) and c" is flat but "a and (b and c)" is not, and NOT is
// never merged. AND/OR are at their operator, NOT at its keyword (for
// "x NOT IN (...)" its NOT, for IS NOT DOCUMENT/NORMALIZED its IS, for IS
// NOT JSON its operand), IS [NOT] NULL and IS [NOT] TRUE/FALSE/UNKNOWN at
// IS, ISNULL and NOTNULL at themselves.
func TestBoolExprShapeIsPostgreSQLs(t *testing.T) {
	cases := []struct{ sql, want string }{
		{"select 1 from t where a and b and c and d", "and@24(a,b,c,d)"},
		{"select 1 from t where a or b or c", "or@24(a,b,c)"},
		{"select 1 from t where a and (b and c)", "and@24(a,and@31(b,c))"},
		{"select 1 from t where a or (b or c)", "or@24(a,or@30(b,c))"},
		{"select 1 from t where (a and b) and c", "and@25(a,b,c)"},
		{"select 1 from t where ((a and b)) and (c and d) and a", "and@26(a,b,and@41(c,d),a)"},
		{"select 1 from t where a or b and c or a", "or@24(a,and@29(b,c),a)"},
		{"select 1 from t where a or b or c and d and a or b", "or@24(a,b,and@34(c,d,a),b)"},
		{"select 1 from t where a and b or c and d or a and b", "or@30(and@24(a,b),and@35(c,d),and@46(a,b))"},
		{"select 1 from t where x = 1 and (a or b) and (c or d)", "and@28(_,or@35(a,b),or@48(c,d))"},
		{"select 1 from t where not a and not not b", "and@28(not@22(a),not@32(not@36(b)))"},
		{"select 1 from t where not (a and b) and c", "and@36(not@22(and@29(a,b)),c)"},
		{"select 1 from t where (not a) or b or c", "or@30(not@23(a),b,c)"},
		{"select 1 from t where a is null and a isnull or a is not null or a notnull or a is true",
			"or@45(and@32(isnull@24(a),isnull@38(a)),notnull@50(a),notnull@67(a),booltest0@80(a))"},
		{"select 1 from t where a is not false and a is unknown and a is not unknown and a is false and a is not true",
			"and@37(booltest3@24(a),booltest4@43(a),booltest5@60(a),booltest2@81(a),booltest1@96(a))"},
		{"select 1 from t where x not in (select 1) and a", "and@42(not@24(_),a)"},
		{"select 1 from t where j is not normalized or j is nfd normalized or j is not nfc normalized",
			"or@42(not@24(_),_,not@70(_))"},
		{"select 1 from t where j is not json or y is not document", "or@36(not@22(_),not@41(_))"},
		{"select 1 from t where   (x  is   null) and   a", "and@39(isnull@28(x),a)"},
		{"select 1 from t where 'é' = j and a and x isnull", "and@31(_,a,isnull@43(x))"},
	}
	for _, c := range cases {
		if got := boolShape(selectOf(t, c.sql).WhereClause); got != c.want {
			t.Errorf("%q\n got %s\nwant %s (PostgreSQL 16)", c.sql, got, c.want)
		}
	}
}

// TestLongAndChainIsOneBoolExpr pins that a chain is built by extending one
// argument list, not by nesting: 10,000 terms arrive as one BoolExpr with
// 10,000 arguments in source order, and nothing on the way recurses per
// term.
func TestLongAndChainIsOneBoolExpr(t *testing.T) {
	const n = 10000
	for _, op := range []string{"and", "or"} {
		var sb strings.Builder
		sb.WriteString("select 1 from t where c0")
		for i := 1; i < n; i++ {
			fmt.Fprintf(&sb, " %s c%d", op, i)
		}
		sql := sb.String()
		be, ok := selectOf(t, sql).WhereClause.(*nodes.BoolExpr)
		if !ok {
			t.Fatalf("%s chain: WHERE is not a BoolExpr", op)
		}
		if got := be.Args.Len(); got != n {
			t.Fatalf("%s chain: %d arguments, want %d", op, got, n)
		}
		for i, a := range be.Args.Items {
			name := a.(*nodes.ColumnRef).Fields.Items[0].(*nodes.String).Str
			if name != fmt.Sprintf("c%d", i) {
				t.Fatalf("%s chain: argument %d is %s", op, i, name)
			}
		}
		if want := strings.Index(sql, " "+op+" ") + 1; int(be.Location) != want {
			t.Errorf("%s chain: Location %d, want the first %s at %d", op, be.Location, op, want)
		}
		if got := nodes.ExprLocation(be); got != strings.Index(sql, "c0") {
			t.Errorf("%s chain: ExprLocation %d, want %d", op, got, strings.Index(sql, "c0"))
		}
	}
}

// TestExprNodeLocationsArePostgreSQLs pins the locations (byte offsets) of
// the other node kinds this fork locates to PostgreSQL 16.15's, read the
// same way as TestBoolExprShapeIsPostgreSQLs (2026-10-20) unless a row says
// otherwise: a sublink at its
// operator ("x IN", "x NOT IN", "x = ANY") or its first token ('(',
// EXISTS, ARRAY), CASE at CASE, ARRAY[...] at ARRAY with each nested
// [...] at its '[', ROW(...) at ROW and (a, b) at its '('.
func TestExprNodeLocationsArePostgreSQLs(t *testing.T) {
	cases := []struct {
		sql  string
		kind string
		want []nodes.ParseLoc
	}{
		{"select 1 from t where x in (select 1) and a", "SubLink", []nodes.ParseLoc{24}},
		{"select 1 from t where x not in (select 1)", "SubLink", []nodes.ParseLoc{24}},
		{"select 1 from t where x = any (select 1)", "SubLink", []nodes.ParseLoc{24}},
		{"select 1 from t where x < all (select 1) or exists (select 1)", "SubLink", []nodes.ParseLoc{24, 44}},
		{"select 1 from t where (select a) and b", "SubLink", []nodes.ParseLoc{22}},
		{"select (select array[1])[1], array(select 1), array[]::int[] from t", "SubLink", []nodes.ParseLoc{7, 29}},
		{"select case when a then 1 else 2 end, case x when 1 then 2 end from t", "CaseExpr", []nodes.ParseLoc{7, 38}},
		{"select array[1, 2], array[[1, 2], [3, 4]] from t", "A_ArrayExpr", []nodes.ParseLoc{7, 20, 26, 34}},
		{"select (select array[1])[1], array(select 1), array[]::int[] from t", "A_ArrayExpr", []nodes.ParseLoc{15, 46}},
		{"select 'é', array[x] from t", "A_ArrayExpr", []nodes.ParseLoc{13}},
		{"select row(1, 2), row(), (1, 2, 3) from t", "RowExpr", []nodes.ParseLoc{7, 18, 25}},
		// The second patch's kinds, read the same way: COALESCE, GREATEST,
		// LEAST, GROUPING, SQL value functions and named arguments at their
		// first token, COLLATE at COLLATE, IS [NOT] JSON at its operand.
		{"select coalesce(b, 1), greatest(b, 2), least(b, 3) from t", "CoalesceExpr", []nodes.ParseLoc{7}},
		{"select coalesce(b, 1), greatest(b, 2), least(b, 3) from t", "MinMaxExpr", []nodes.ParseLoc{23, 39}},
		{"select  current_date, current_timestamp(2), localtime, current_user, session_user, current_schema from t",
			"SQLValueFunction", []nodes.ParseLoc{8, 22, 44, 55, 69, 83}},
		{"select b from t group by b having grouping(b) = 0", "GroupingFunc", []nodes.ParseLoc{34}},
		{"select x collate \"C\", x   collate \"POSIX\" = x from t", "CollateClause", []nodes.ParseLoc{9, 26}},
		{"select x is json, x is not json object from t", "JsonIsPredicate", []nodes.ParseLoc{7, 18}},
		{"select make_interval(days => b), make_interval(hours := 2) from t", "NamedArgExpr", []nodes.ParseLoc{21, 47}},
		// DEFAULT at itself, as gram.y's @1 (analysis replaces it, so the
		// oracle cannot show it).
		{"insert into t values (1,  default)", "SetToDefault", []nodes.ParseLoc{26}},
		// TypeName at its first token: each is where PostgreSQL 16 reports
		// 'type "nosuch" does not exist' (position minus one).
		{"select nosuch 'x'", "TypeName", []nodes.ParseLoc{7}},
		{"select 'x'::nosuch", "TypeName", []nodes.ParseLoc{12}},
		{"select   cast('x' as  nosuch)", "TypeName", []nodes.ParseLoc{22}},
		{"select 1::nosuch[]", "TypeName", []nodes.ParseLoc{10}},
		{"select 1::nosuch array[3]", "TypeName", []nodes.ParseLoc{10}},
		{"select pg_catalog.nosuch 'x'", "TypeName", []nodes.ParseLoc{7}},
		{"select nosuch(1) 'x'", "TypeName", []nodes.ParseLoc{7}},
		{"select 1::public.nosuch(2)", "TypeName", []nodes.ParseLoc{10}},
		{"create table q (c nosuch)", "TypeName", []nodes.ParseLoc{18}},
		// Typed literals and %TYPE, read from PostgreSQL 16.15's raw tree
		// (debug_print_parse of a utility statement, which is logged before
		// analysis): the type name's first token, after SETOF for SETOF.
		{"select  interval '1 day'", "TypeName", []nodes.ParseLoc{8}},
		{"select  int '1', timestamp(3) 'x'", "TypeName", []nodes.ParseLoc{8, 17}},
		{"create function nosuch_f(p nosuch_t.c%TYPE, q int[], r setof text) returns nosuch_t.c%TYPE language sql as 'select 1'",
			"TypeName", []nodes.ParseLoc{27, 46, 61, 75}},
		{"create function nosuch_f(p int) returns setof  nosuch_t.c%TYPE language sql as 'select 1'", "TypeName", []nodes.ParseLoc{27, 47}},
	}
	for _, c := range cases {
		stmts, err := Parse(c.sql)
		if err != nil {
			t.Fatalf("Parse(%q): %v", c.sql, err)
		}
		if got := locationsOf(stmts, c.kind); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%q: %s at %v, PostgreSQL 16 says %v", c.sql, c.kind, got, c.want)
		}
	}
}
