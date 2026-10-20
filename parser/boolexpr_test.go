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
		// Must NOT flatten: the right operand, a different connective, or a
		// NOT on the left. Read from PostgreSQL 16.15's RAW tree ("create
		// view v as <stmt>" over a missing relation, whose debug_print_parse
		// is logged before analysis), 2026-10-20; every row above was
		// re-checked the same way.
		{"select 1 from t where a and b or c", "or@30(and@24(a,b),c)"},
		{"select 1 from t where a or b and c", "or@24(a,and@29(b,c))"},
		{"select 1 from t where (a or b) and c", "and@31(or@25(a,b),c)"},
		{"select 1 from t where not not a", "not@22(not@26(a))"},
		{"select 1 from t where not a and b", "and@28(not@22(a),b)"},
		{"select 1 from t where (not a) and (not b) and c", "and@30(not@23(a),not@35(b),c)"},
		{"select 1 from t where a and (b or c) and d", "and@24(a,or@31(b,c),d)"},
		{"select 1 from t where (a and b) or (c and d)", "or@32(and@25(a,b),and@38(c,d))"},
		{"select 1 from t where not (not a and b)", "not@22(and@33(not@27(a),b))"},
	}
	for _, c := range cases {
		if got := boolShape(selectOf(t, c.sql).WhereClause); got != c.want {
			t.Errorf("%q\n got %s\nwant %s (PostgreSQL 16)", c.sql, got, c.want)
		}
	}
}

// TestBoolExprArgListsAreNotShared pins that extending a chain never
// reaches a BoolExpr it does not own: in "(a and b), (c and d) and e" the
// first target keeps exactly (a, b) after the second is built and extended,
// and the two argument lists do not share storage.
func TestBoolExprArgListsAreNotShared(t *testing.T) {
	sel := selectOf(t, "select (a and b), (c and d) and e, (f or g) or h, i and j")
	targets := sel.TargetList.Items
	want := []string{"and@10(a,b)", "and@21(c,d,e)", "or@38(f,g,h)", "and@52(i,j)"} // PostgreSQL 16.15's raw tree
	var lists [][]nodes.Node
	for i, rt := range targets {
		val := rt.(*nodes.ResTarget).Val
		if got := boolShape(val); got != want[i] {
			t.Errorf("target %d is %s, want %s", i, got, want[i])
		}
		lists = append(lists, val.(*nodes.BoolExpr).Args.Items)
	}
	for i := range lists {
		for j := range lists {
			if i != j && &lists[i][0] == &lists[j][0] {
				t.Errorf("targets %d and %d share an argument array", i, j)
			}
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
		// ResTarget at its column name (INSERT's column list, UPDATE's SET,
		// a multi-column SET's members) or at its expression's first token
		// (a target or RETURNING item, XMLATTRIBUTES and XMLNAMESPACES), DEFAULT
		// for a default namespace; read from the raw trees of CREATE RULE
		// ... DO INSTEAD <statement> and CREATE TEMP VIEW, with the prefix
		// subtracted. Byte offsets: é is two bytes.
		{`insert into t (a, b.c, d[1], "Qé", é) values (1,2,3,4,5)`, "ResTarget", []nodes.ParseLoc{15, 18, 23, 29, 36}},
		{"insert into t (/*c*/ a,\n  -- x\n  b) values (1,2)", "ResTarget", []nodes.ParseLoc{21, 33}},
		{`update t set a = 1, b.c = 2, (d, "é"[2]) = (3, 4)`, "ResTarget", []nodes.ParseLoc{13, 20, 30, 33}},
		{"insert into t default values returning  a, b as c, d e, *", "ResTarget", []nodes.ParseLoc{40, 43, 51, 56}},
		{"select xmlelement(name e, xmlattributes(a as b, c)) from t", "ResTarget", []nodes.ParseLoc{7, 40, 48}},
		{"select x.* from nosuch_q, xmltable(xmlnamespaces('u' as a, default 'w'), '/r' passing y columns c int) x",
			"ResTarget", []nodes.ParseLoc{7, 49, 59}},
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

// exprShape renders a raw expression with its locations: an operator is
// "op@loc(left,right)" with "<>" for an absent left operand, a constant is
// "int:v@loc", "float:text@loc" or "str:text@loc", AND/OR/NOT is
// "and@loc(args)", a cast is "cast@loc(arg)", a list is "[members]", a
// column reference is
// "name@loc" and anything else is "_".
func exprShape(n nodes.Node) string {
	var sb strings.Builder
	var walk func(n nodes.Node)
	walk = func(n nodes.Node) {
		switch e := n.(type) {
		case nil:
			sb.WriteString("<>")
		case *nodes.A_Expr:
			fmt.Fprintf(&sb, "%s@%d(", e.Name.Items[0].(*nodes.String).Str, e.Location)
			walk(e.Lexpr)
			sb.WriteByte(',')
			walk(e.Rexpr)
			sb.WriteByte(')')
		case *nodes.A_Const:
			switch v := e.Val.(type) {
			case *nodes.Integer:
				fmt.Fprintf(&sb, "int:%d", v.Ival)
			case *nodes.Float:
				fmt.Fprintf(&sb, "float:%s", v.Fval)
			case *nodes.String:
				fmt.Fprintf(&sb, "str:%s", v.Str)
			default:
				sb.WriteByte('_')
			}
			fmt.Fprintf(&sb, "@%d", e.Location)
		case *nodes.BoolExpr:
			fmt.Fprintf(&sb, "%s@%d(", [...]string{"and", "or", "not"}[e.Boolop], e.Location)
			for i, a := range e.Args.Items {
				if i > 0 {
					sb.WriteByte(',')
				}
				walk(a)
			}
			sb.WriteByte(')')
		case *nodes.TypeCast:
			fmt.Fprintf(&sb, "cast@%d(", e.Location)
			walk(e.Arg)
			sb.WriteByte(')')
		case *nodes.List:
			sb.WriteByte('[')
			for i, a := range e.Items {
				if i > 0 {
					sb.WriteByte(',')
				}
				walk(a)
			}
			sb.WriteByte(']')
		case *nodes.ColumnRef:
			fmt.Fprintf(&sb, "%s@%d", e.Fields.Items[len(e.Fields.Items)-1].(*nodes.String).Str, e.Location)
		default:
			sb.WriteByte('_')
		}
	}
	walk(n)
	return sb.String()
}

// targetShapes is exprShape of each target of the single SELECT sql.
func targetShapes(t *testing.T, sql string) []string {
	t.Helper()
	var out []string
	for _, rt := range selectOf(t, sql).TargetList.Items {
		out = append(out, exprShape(rt.(*nodes.ResTarget).Val))
	}
	return out
}

// TestUnaryPlusIsKept pins that unary plus is an operator in the raw tree,
// as PostgreSQL 16 builds it: an A_Expr named "+" with no left operand, at
// the plus sign, over constants too (+1 is not folded; only minus folds a
// constant). It was dropped, so +'abc' and +true parsed as 'abc' and true,
// which PostgreSQL refuses ("operator does not exist: + boolean"). Each
// want was read from PostgreSQL 16.15's debug_print_parse of CREATE TEMP
// VIEW v AS <the same SELECT> FROM nosuch_q, whose raw tree is logged
// before the missing table fails it, with the 22-byte prefix subtracted
// (verified 2026-10-20).
func TestUnaryPlusIsKept(t *testing.T) {
	cases := []struct {
		sql  string
		want []string
	}{
		{"select +x, + +x, -+x, +(a and b), +1, a + +b, +a*b, +a::int", []string{
			"+@7(<>,x@8)",
			"+@11(<>,+@13(<>,x@14))",
			"-@17(<>,+@18(<>,x@19))",
			"+@22(<>,and@26(a@24,b@30))",
			"+@34(<>,int:1@35)",
			"+@40(a@38,+@42(<>,b@43))",
			"*@48(+@46(<>,a@47),b@49)",
			"+@52(<>,cast@54(a@53))",
		}},
		{"select + -1, -+1, - - 1", []string{"+@7(<>,int:-1@9)", "-@13(<>,+@14(<>,int:1@15))", "int:1@18"}},
		{"select +'abc'", []string{"+@7(<>,str:abc@8)"}},
		// BETWEEN's lower bound is a b_expr, whose unary plus is kept too.
		{"select x between +a and b, x between - +1 and 2", []string{
			"BETWEEN@9(x@7,[+@17(<>,a@18),b@24])",
			"BETWEEN@29(x@27,[-@37(<>,+@39(<>,int:1@40)),int:2@46])",
		}},
	}
	for _, c := range cases {
		if got := targetShapes(t, c.sql); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%q\n got %v\nwant %v (PostgreSQL 16)", c.sql, got, c.want)
		}
	}
	for _, sql := range []string{"select +", "select (+)", "select + from t"} {
		if _, err := Parse(sql); err == nil {
			t.Errorf("Parse(%q) accepted a plus with no operand", sql)
		}
	}
}

// TestSignedFetchCountIsAConstant pins a signed FETCH FIRST or SQL OFFSET
// count to PostgreSQL 16's rule: the sign takes a number, not an
// expression; plus is kept as an operator and minus folds into the
// constant. Read as for TestUnaryPlusIsKept, from PostgreSQL 16.15's raw
// tree of the same SELECT (2026-10-20). The refusals of +x, -x, +(1) and
// + 'a' are pinned in TestParseErrorPositionIsTheRefusedToken.
func TestSignedFetchCountIsAConstant(t *testing.T) {
	cases := []struct{ sql, offset, count string }{
		{"select 1 from nosuch_q fetch first +2 rows only", "<>", "+@35(<>,int:2@36)"},
		{"select 1 from nosuch_q fetch first +2.5 rows only", "<>", "+@35(<>,float:2.5@36)"},
		{"select 1 from nosuch_q order by 1 offset +3 rows fetch next +1 row with ties", "+@41(<>,int:3@42)", "+@60(<>,int:1@61)"},
		{"select 1 from nosuch_q offset -3 rows fetch first -2.5 rows only", "int:-3@30", "float:-2.5@50"},
		{"select 1 from nosuch_q fetch first x rows only", "<>", "x@35"},
	}
	for _, c := range cases {
		sel := selectOf(t, c.sql)
		if got := exprShape(sel.LimitOffset); got != c.offset {
			t.Errorf("%q: offset %s, want %s (PostgreSQL 16)", c.sql, got, c.offset)
		}
		if got := exprShape(sel.LimitCount); got != c.count {
			t.Errorf("%q: count %s, want %s (PostgreSQL 16)", c.sql, got, c.count)
		}
	}
}

// stmtOf is the first statement of a parse, unwrapped from its RawStmt.
func stmtOf(stmts *nodes.List) nodes.Node {
	n := stmts.Items[0]
	if raw, ok := n.(*nodes.RawStmt); ok {
		n = raw.Stmt
	}
	return n
}

// TestNumericOnlyFoldsPlus pins the places PostgreSQL 16's grammar folds a
// sign into a number rather than building an operator: NumericOnly
// (sequence options) and SignedIconst (SET STATISTICS). A leading plus
// leaves the number, and NumericOnly's minus over a Float keeps a Float,
// int64's least value included (the Integer fold is doNegate's, in
// expressions only); PostgreSQL 16.15's raw tree has 5, 1.5, the Float
// -9223372036854775808 and 5.
func TestNumericOnlyFoldsPlus(t *testing.T) {
	stmts, err := Parse("create sequence s minvalue +5 increment -9223372036854775808 maxvalue +1.5")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, o := range stmtOf(stmts).(*nodes.CreateSeqStmt).Options.Items {
		switch v := o.(*nodes.DefElem).Arg.(type) {
		case *nodes.Integer:
			got = append(got, fmt.Sprintf("int:%d", v.Ival))
		case *nodes.Float:
			got = append(got, "float:"+v.Fval)
		default:
			got = append(got, fmt.Sprintf("%T", v))
		}
	}
	if want := []string{"int:5", "float:-9223372036854775808", "float:1.5"}; !reflect.DeepEqual(got, want) {
		t.Errorf("sequence options %v, want %v", got, want)
	}
	stmts, err = Parse("alter table t alter column a set statistics +5")
	if err != nil {
		t.Fatal(err)
	}
	def := stmtOf(stmts).(*nodes.AlterTableStmt).Cmds.Items[0].(*nodes.AlterTableCmd).Def
	// The fork wraps the number in an unlocated A_Const (PostgreSQL 17's
	// set_statistics_value shape; 16 has the bare Integer); either way the
	// plus is folded, not an operator.
	if ac, ok := def.(*nodes.A_Const); ok {
		def = ac.Val
	}
	if i, ok := def.(*nodes.Integer); !ok || i.Ival != 5 {
		t.Errorf("set statistics +5: %#v, want Integer 5", def)
	}
}

// TestNegatedLiteralShape pins minus over a constant: a number is folded
// into a constant at the minus sign, as PostgreSQL 16 does, parenthesised
// or not, and a string is not (it is an operator, though PostgreSQL still
// moves the string to the minus sign). Where the negation of an integer
// literal fits int64 the constant is an Integer: int64's least value in
// every radix, which the scanner reads as a Float because its magnitude
// does not fit. That is this fork's shape, not PostgreSQL's: PostgreSQL's
// Integer is int32-sized, its raw tree keeps every one of these as a Float
// with the literal's text, and analysis (make_const) reads the int8. One
// past the least value, a positive magnitude, a decimal or an exponent
// stays a Float; negating the least Integer again gives back a Float, as
// int64 cannot hold its magnitude. Locations and the Float texts were read
// as for TestUnaryPlusIsKept; the int rows are Floats with the same text
// in PostgreSQL.
func TestNegatedLiteralShape(t *testing.T) {
	cases := []struct {
		sql  string
		want []string
	}{
		{"select -9223372036854775808, -0x8000000000000000, -0o1000000000000000000000, " +
			"-0b1000000000000000000000000000000000000000000000000000000000000000, -9_223_372_036_854_775_808", []string{
			"int:-9223372036854775808@7",
			"int:-9223372036854775808@29",
			"int:-9223372036854775808@50",
			"int:-9223372036854775808@77",
			"int:-9223372036854775808@146",
		}},
		{"select -9223372036854775809, -0x8000000000000001, 9223372036854775808, - 9223372036854775808, " +
			"- -1, -(9223372036854775808), -(1), -1.5, -1e3, -'1', - -9223372036854775808", []string{
			"float:-9223372036854775809@7",
			"float:-0x8000000000000001@29",
			"float:9223372036854775808@50",
			"int:-9223372036854775808@71",
			"int:1@94",
			"int:-9223372036854775808@100",
			"int:-1@124",
			"float:-1.5@130",
			"float:-1e3@136",
			"-@142(<>,str:1@142)",
			"float:9223372036854775808@148",
		}},
		// Upper-case radix prefixes, and the greatest int64 either way.
		{"select -0X8000000000000000, -0O1000000000000000000000, -0B1" + strings.Repeat("0", 63) +
			", -9223372036854775807, 9223372036854775807", []string{
			"int:-9223372036854775808@7",
			"int:-9223372036854775808@28",
			"int:-9223372036854775808@55",
			"int:-9223372036854775807@124",
			"int:9223372036854775807@146",
		}},
		// -- starts a comment, so this is the constant 1 on the next line;
		// with a space it is a double negation.
		{"select --1\n 1, - -1", []string{"int:1@12", "int:1@15"}},
		// Upper case one past the least stays a Float; leading zeros do not
		// change the value, so they fold too (PostgreSQL: Floats with the
		// literal's text).
		{"select -0X8000000000000001, -00009223372036854775808, -0x0008000000000000000", []string{
			"float:-0X8000000000000001@7",
			"int:-9223372036854775808@28",
			"int:-9223372036854775808@54",
		}},
		// A double negation that does not fit stays a Float with the
		// decimal text: PostgreSQL keeps the inner literal's own text
		// (0x8000000000000000).
		{"select - -0x8000000000000000", []string{"float:9223372036854775808@7"}},
	}
	for _, c := range cases {
		if got := targetShapes(t, c.sql); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%q\n got %v\nwant %v", c.sql, got, c.want)
		}
	}
}
