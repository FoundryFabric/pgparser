package nodes_test

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/pgplex/pgparser/nodes"
	"github.com/pgplex/pgparser/parser"
	"github.com/pgplex/pgparser/parser/pgregress"
)

// lastTarget is the last target of a single SELECT.
func lastTarget(t *testing.T, sql string) *nodes.ResTarget {
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
	return targets[len(targets)-1].(*nodes.ResTarget)
}

// TestExprLocationIsWhereTheExpressionStarts pins ExprLocation over parsed
// expressions of every kind it knows: each is the last target of "select
// 'é', <expr>", so the expression starts after a two-byte character and
// the answer is a byte offset (PostgreSQL's locations are byte offsets
// too; a client counting characters converts). starts is the text the
// expression starts with, found in the statement, or "" for -1.
//
// Every row but the typed literals and the XmlExpr ones was checked
// against PostgreSQL 16.15 (2026-10-20): "select point '(1,1)' union select
// <expr>" fails with its caret at exprLocation of <expr>. A typed literal is
// analysed into a constant that keeps the literal's own location, so that
// oracle shows the literal; the raw tree's rule gives the type name. The
// XmlExpr rows are -1 where PostgreSQL knows the start.
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
		// A left operand of a kind with its own rule, or now located: each
		// was the operator's offset, after the start.
		{"a[1] = 2", "a"},
		{"(a).f = 1", "a"},
		{"(x).a::int", "x"},
		{"x collate \"C\" = y", "x"},
		{"b::text collate \"C\"", "b"},
		{"coalesce(a, b) = 1", "coalesce"},
		{"coalesce(a, b) and c", "coalesce"},
		{"greatest(a, b) > 1", "greatest"},
		{"least(a, 2)", "least"},
		{"current_date = x", "current_date"},
		{"x is json and c", "x"},
		{"not x is not json", "not"},
		{"f(a => 1) + 1", "f"},
		// A typed literal starts at its type name, as PostgreSQL's raw
		// TypeCast rule (leftmost of cast, type name and argument) gives.
		{"date '2020-01-01'", "date"},
		{"int '1' + 1", "int"},
		{"interval(2) '1 day'", "interval"},
		{"pg_catalog.text 'x' || 'y'", "pg_catalog"},
		// An operand whose kind has no rule (XmlExpr) makes the start
		// unknown rather than the operator's offset.
		{"y is not document", ""},
		{"xmlconcat(y) is null", ""},
		{"xmlconcat(y) and c", ""},
	}
	for _, c := range cases {
		sql := "select 'é', " + c.expr
		want := -1
		if c.starts != "" {
			if want = strings.Index(sql, c.starts); want < 0 {
				t.Fatalf("%q does not contain %q", sql, c.starts)
			}
		}
		if got := nodes.ExprLocation(lastTarget(t, sql).Val); got != want {
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
		{"List", list(at(-1), &nodes.A_Const{Location: -1}, at(6)), 6},
		{"empty List", list(), -1},
		{"List of only unlocated members", list(&nodes.A_Const{Location: -1}, at(-1)), -1},
		// PostgreSQL's list rule: the first member that HAS a location, so a
		// leading member known to be unlocated is skipped.
		{"List skips an unlocated member", list(&nodes.A_Const{Location: -1}, at(6)), 6},
		{"nil List", (*nodes.List)(nil), -1},
		{"unlisted kind", &nodes.String{Str: "s"}, -1},
		// Nodes that are not expressions are unknown, never a panic, even
		// one with a Location field.
		{"String", &nodes.String{Str: "s"}, -1},
		{"Integer", &nodes.Integer{Ival: 3}, -1},
		{"SelectStmt", &nodes.SelectStmt{}, -1},
		{"RangeVar", &nodes.RangeVar{Location: 3}, -1},
		{"ResTarget", &nodes.ResTarget{Val: at(3), Location: 3}, -1},
		{"unknown leftmost leaf deep in a located spine", deepSpine(100, &nodes.XmlExpr{Location: 0}), -1},
		{"located leftmost leaf deep in a located spine", deepSpine(100, at(0)), 0},
		{"unlocated leftmost leaf deep in a located spine", deepSpine(100, at(-1)), 1},
		{"unlisted kind under an operator", &nodes.A_Expr{Lexpr: &nodes.XmlExpr{Location: 1}, Location: 5}, -1},
		{"unlisted kind first in AND", &nodes.BoolExpr{Args: list(&nodes.XmlExpr{}, at(7)), Location: 9}, -1},
		{"unlisted kind after a located arg", &nodes.BoolExpr{Args: list(at(2), &nodes.XmlExpr{}), Location: 9}, 2},
		{"unlisted kind first in a List", list(&nodes.XmlExpr{}, at(6)), -1},
		{"unlisted kind in a nested List", list(list(at(-1)), list(&nodes.XmlExpr{}), at(6)), -1},
		{"A_Indirection", &nodes.A_Indirection{Arg: at(4)}, 4},
		{"CollateClause", &nodes.CollateClause{Arg: at(4), Location: 6}, 4},
		{"CollateClause unknown arg", &nodes.CollateClause{Arg: at(-1), Location: 6}, 6},
		{"NamedArgExpr", &nodes.NamedArgExpr{Arg: at(9), Location: 2}, 2},
		{"CoalesceExpr", &nodes.CoalesceExpr{Args: list(at(1)), Location: 4}, 4},
		{"MinMaxExpr", &nodes.MinMaxExpr{Args: list(at(1)), Location: 4}, 4},
		{"GroupingFunc", &nodes.GroupingFunc{Args: list(at(1)), Location: 4}, 4},
		{"SQLValueFunction", &nodes.SQLValueFunction{Location: 4}, 4},
		{"SetToDefault", &nodes.SetToDefault{Location: 4}, 4},
		{"JsonIsPredicate", &nodes.JsonIsPredicate{Expr: at(1), Location: 4}, 4},
		// A typed nil is absent, as a nil pointer is in PostgreSQL.
		{"typed nil BoolExpr", (*nodes.BoolExpr)(nil), -1},
		{"typed nil A_Expr", (*nodes.A_Expr)(nil), -1},
		{"typed nil A_Const", (*nodes.A_Const)(nil), -1},
		{"typed nil operand", &nodes.A_Expr{Lexpr: (*nodes.ColumnRef)(nil), Location: 5}, 5},
		{"typed nil arg", &nodes.BoolExpr{Args: list((*nodes.FuncCall)(nil), at(3)), Location: 1}, 1},
		{"typed nil under indirection", &nodes.NullTest{Arg: &nodes.A_Indirection{Arg: (*nodes.TypeCast)(nil)}, Location: 8}, 8},
	}
	for _, c := range cases {
		if got := nodes.ExprLocation(c.n); got != c.want {
			t.Errorf("%s: ExprLocation = %d, want %d", c.name, got, c.want)
		}
	}
}

// deepSpine nests leaf as the first operand of depth located nodes,
// cycling through every kind that walks into its first operand, each
// located at byte 1 or later: A_Expr, BoolExpr, NullTest, BooleanTest,
// SubLink, CollateClause, TypeCast, NamedArgExpr, A_Indirection, FuncCall.
func deepSpine(depth int, leaf nodes.Node) nodes.Node {
	n := leaf
	for i := 0; i < depth; i++ {
		loc := nodes.ParseLoc(1 + i)
		switch i % 10 {
		case 0:
			n = &nodes.A_Expr{Lexpr: n, Location: loc}
		case 1:
			n = &nodes.BoolExpr{Boolop: nodes.AND_EXPR, Args: &nodes.List{Items: []nodes.Node{n, &nodes.ColumnRef{Location: 900}}}, Location: loc}
		case 2:
			n = &nodes.NullTest{Arg: n, Location: loc}
		case 3:
			n = &nodes.BooleanTest{Arg: n, Location: loc}
		case 4:
			n = &nodes.SubLink{Testexpr: n, Location: loc}
		case 5:
			n = &nodes.CollateClause{Arg: n, Location: loc}
		case 6:
			n = &nodes.TypeCast{Arg: n, TypeName: &nodes.TypeName{Location: -1}, Location: loc}
		case 7:
			n = &nodes.NamedArgExpr{Arg: n, Location: loc}
		case 8:
			n = &nodes.A_Indirection{Arg: n}
		case 9:
			n = &nodes.FuncCall{Args: &nodes.List{Items: []nodes.Node{n}}, Location: loc + 1000}
		}
	}
	return n
}

// TestExprLocationTypedNil pins that a typed nil of every kind with a rule
// is absent, never dereferenced, both alone and as an operand.
func TestExprLocationTypedNil(t *testing.T) {
	for _, n := range []nodes.Node{
		(*nodes.A_Const)(nil), (*nodes.ColumnRef)(nil), (*nodes.ParamRef)(nil),
		(*nodes.A_ArrayExpr)(nil), (*nodes.CaseExpr)(nil), (*nodes.RowExpr)(nil),
		(*nodes.CoalesceExpr)(nil), (*nodes.MinMaxExpr)(nil), (*nodes.GroupingFunc)(nil),
		(*nodes.SQLValueFunction)(nil), (*nodes.SetToDefault)(nil), (*nodes.JsonIsPredicate)(nil),
		(*nodes.A_Expr)(nil), (*nodes.NullTest)(nil), (*nodes.BooleanTest)(nil),
		(*nodes.SubLink)(nil), (*nodes.CollateClause)(nil), (*nodes.NamedArgExpr)(nil),
		(*nodes.A_Indirection)(nil), (*nodes.TypeCast)(nil), (*nodes.BoolExpr)(nil),
		(*nodes.FuncCall)(nil), (*nodes.List)(nil),
	} {
		if got := nodes.ExprLocation(n); got != -1 {
			t.Errorf("ExprLocation(%T(nil)) = %d, want -1", n, got)
		}
		if got := nodes.ExprLocation(&nodes.A_Expr{Lexpr: n, Location: 5}); got != 5 {
			t.Errorf("ExprLocation(A_Expr{Lexpr: %T(nil)}) = %d, want 5", n, got)
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
	expr := lastTarget(t, sql).Val
	if got, want := nodes.ExprLocation(expr), strings.Index(sql, "c0"); got != want {
		t.Errorf("%d-term + chain: ExprLocation %d, want %d", n, got, want)
	}
	if allocs := testing.AllocsPerRun(10, func() { nodes.ExprLocation(expr) }); allocs != 0 {
		t.Errorf("ExprLocation allocates %.0f times per call", allocs)
	}
}

// TestExprLocationDeepChains pins that 10,000 nested NOTs and a 10,000-deep
// left spine of alternating AND/OR (each level parenthesised, so nothing
// flattens) parse and answer without exhausting the stack.
func TestExprLocationDeepChains(t *testing.T) {
	const n = 10000
	nots := "select 'é', " + strings.Repeat("not ", n) + "a"
	if got, want := nodes.ExprLocation(lastTarget(t, nots).Val), strings.Index(nots, "not"); got != want {
		t.Errorf("%d NOTs: ExprLocation %d, want %d", n, got, want)
	}
	var sb strings.Builder
	sb.WriteString("select 'é', ")
	sb.WriteString(strings.Repeat("(", n))
	sb.WriteString("a0")
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&sb, " %s a%d)", [2]string{"and", "or"}[i%2], i)
	}
	spine := sb.String()
	expr := lastTarget(t, spine).Val
	depth := 0
	for be, ok := expr.(*nodes.BoolExpr); ok; be, ok = be.Args.Items[0].(*nodes.BoolExpr) {
		if be.Args.Len() != 2 {
			t.Fatalf("level %d has %d arguments; alternating connectives must not merge", depth, be.Args.Len())
		}
		depth++
	}
	if depth != n {
		t.Errorf("left spine is %d deep, want %d", depth, n)
	}
	if got, want := nodes.ExprLocation(expr), strings.Index(spine, "a0"); got != want {
		t.Errorf("%d-deep spine: ExprLocation %d, want %d", n, got, want)
	}
}

// corpusStatements is every statement of the repository's SQL corpora
// (PostgreSQL's regression SQL under parser/pgregress and the grammar
// fixtures under tests/grammar), psql variables replaced as TestPGRegress
// replaces them.
func corpusStatements(t testing.TB) []string {
	t.Helper()
	var files []string
	for _, glob := range []string{"../parser/pgregress/testdata/sql/*.sql", "../tests/grammar/sql/*.sql"} {
		m, err := filepath.Glob(glob)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, m...)
	}
	if len(files) == 0 {
		t.Fatal("no corpus files found")
	}
	sort.Strings(files)
	var out []string
	for _, f := range files {
		content, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, st := range pgregress.ExtractStatements(filepath.Base(f), content) {
			sql := st.SQL
			if st.HasPsqlVar {
				sql, _ = pgregress.ReplacePsqlVariables(sql)
			}
			out = append(out, sql)
		}
	}
	return out
}

// invariantCounts is how many nodes checkInvariants asserted on.
type invariantCounts struct {
	connectives, nots, nullTests, located int
}

// checkInvariants asserts, over every node of a parsed statement:
//   - an AND/OR BoolExpr is at the text AND/OR, and its first argument is
//     never the same connective (a left chain is always flat);
//   - a NOT is at NOT, at the IS of IS NOT DOCUMENT/NORMALIZED, or at its
//     IS NOT JSON operand;
//   - a NullTest is at IS, ISNULL or NOTNULL;
//   - ExprLocation of every node is -1 or a byte of the statement that is
//     not whitespace, no later than the node's own location, and no later
//     than ExprLocation of the first operand it walks into.
func checkInvariants(t testing.TB, sql string, root nodes.Node, c *invariantCounts) {
	t.Helper()
	word := func(loc nodes.ParseLoc) string {
		i := int(loc)
		if i < 0 || i >= len(sql) {
			return ""
		}
		j := i
		for j < len(sql) && (sql[j] == '_' || sql[j] >= 'a' && sql[j] <= 'z' || sql[j] >= 'A' && sql[j] <= 'Z' || sql[j] >= '0' && sql[j] <= '9') {
			j++
		}
		return strings.ToLower(sql[i:j])
	}
	fail := func(format string, args ...any) {
		t.Helper()
		t.Errorf("%q: "+format, append([]any{sql}, args...)...)
	}
	var walk func(v reflect.Value)
	walk = func(v reflect.Value) {
		switch v.Kind() { //nolint:exhaustive // only the kinds that can carry a Node
		case reflect.Ptr, reflect.Interface:
			if v.IsNil() {
				return
			}
			if n, ok := v.Interface().(nodes.Node); ok && v.Kind() == reflect.Ptr {
				checkNode(sql, n, word, fail, c)
			}
			walk(v.Elem())
		case reflect.Struct:
			for i := 0; i < v.NumField(); i++ {
				walk(v.Field(i))
			}
		case reflect.Slice:
			for i := 0; i < v.Len(); i++ {
				walk(v.Index(i))
			}
		}
	}
	walk(reflect.ValueOf(root))
}

func checkNode(sql string, n nodes.Node, word func(nodes.ParseLoc) string, fail func(string, ...any), c *invariantCounts) {
	switch e := n.(type) {
	case *nodes.BoolExpr:
		switch e.Boolop {
		case nodes.AND_EXPR, nodes.OR_EXPR:
			c.connectives++
			want := [...]string{"and", "or"}[e.Boolop]
			if w := word(e.Location); w != want {
				fail("%s BoolExpr at %d reads %q", want, e.Location, w)
			}
			if first, ok := e.Args.Items[0].(*nodes.BoolExpr); ok && first.Boolop == e.Boolop {
				fail("%s BoolExpr at %d has a %s as its first argument: the chain is not flat", want, e.Location, want)
			}
		case nodes.NOT_EXPR:
			c.nots++
			w := word(e.Location)
			jp, isJSON := e.Args.Items[0].(*nodes.JsonIsPredicate)
			if w != "not" && w != "is" && !(isJSON && jp.Location == e.Location) {
				fail("NOT BoolExpr at %d reads %q", e.Location, w)
			}
		}
	case *nodes.NullTest:
		c.nullTests++
		if w := word(e.Location); w != "is" && w != "isnull" && w != "notnull" {
			fail("NullTest at %d reads %q", e.Location, w)
		}
	}
	loc := nodes.ExprLocation(n)
	if loc < 0 {
		return
	}
	c.located++
	if loc >= len(sql) {
		fail("ExprLocation(%T) = %d, past the end", n, loc)
		return
	}
	if strings.ContainsRune(" \t\n\r\f\v", rune(sql[loc])) {
		fail("ExprLocation(%T) = %d is whitespace", n, loc)
	}
	own, first := ownAndFirst(n)
	if own >= 0 && loc > own {
		fail("ExprLocation(%T) = %d is after its own location %d", n, loc, own)
	}
	if first != nil {
		if fl := nodes.ExprLocation(first); fl >= 0 && loc > fl {
			fail("ExprLocation(%T) = %d is after its first operand's %d", n, loc, fl)
		}
	}
}

// ownAndFirst is a node's own location (-1 if it has none) and the first
// operand ExprLocation walks into (nil if none).
func ownAndFirst(n nodes.Node) (int, nodes.Node) {
	first := func(l *nodes.List) nodes.Node {
		if l == nil || len(l.Items) == 0 {
			return nil
		}
		return l.Items[0]
	}
	switch e := n.(type) {
	case *nodes.A_Expr:
		return int(e.Location), e.Lexpr
	case *nodes.BoolExpr:
		return int(e.Location), first(e.Args)
	case *nodes.NullTest:
		return int(e.Location), e.Arg
	case *nodes.BooleanTest:
		return int(e.Location), e.Arg
	case *nodes.SubLink:
		return int(e.Location), e.Testexpr
	case *nodes.CollateClause:
		return int(e.Location), e.Arg
	case *nodes.NamedArgExpr:
		return int(e.Location), e.Arg
	case *nodes.TypeCast:
		return int(e.Location), e.Arg
	case *nodes.A_Indirection:
		return -1, e.Arg
	case *nodes.FuncCall:
		return int(e.Location), first(e.Args)
	case *nodes.A_Const:
		return int(e.Location), nil
	case *nodes.ColumnRef:
		return int(e.Location), nil
	}
	return -1, nil
}

// TestInvariantsOverCorpus parses every statement of the repository's SQL
// corpora and checks checkInvariants over each tree. Only statements the
// parser rejects are skipped.
func TestInvariantsOverCorpus(t *testing.T) {
	var c invariantCounts
	checked, rejected := 0, 0
	for _, sql := range corpusStatements(t) {
		stmts, err := parser.Parse(sql)
		if err != nil {
			rejected++
			continue
		}
		checked++
		checkInvariants(t, sql, stmts, &c)
	}
	t.Logf("%d statements checked (%d rejected by the parser, skipped): %d AND/OR, %d NOT, %d NullTest asserted; %d nodes with a known ExprLocation",
		checked, rejected, c.connectives, c.nots, c.nullTests, c.located)
	if checked < 1000 || c.connectives < 100 || c.nullTests < 100 {
		t.Errorf("corpus too small to mean anything: %d statements, %d connectives, %d NullTests", checked, c.connectives, c.nullTests)
	}
}

// FuzzExprLocation parses arbitrary text, seeded from the SQL corpora, and
// checks that nothing panics and checkInvariants holds for whatever parses.
func FuzzExprLocation(f *testing.F) {
	for _, sql := range corpusStatements(f) {
		f.Add(sql)
	}
	f.Fuzz(func(t *testing.T, sql string) {
		// The lexer's operator scan is super-linear (about cubic) in the
		// operator's length, on origin/ff too: 4,000 '+' take ~12s. That
		// defect is outside ExprLocation, so inputs that reach it are
		// skipped rather than reported as hangs.
		if longestOperatorRun(sql) >= 100 {
			return
		}
		stmts, err := parser.Parse(sql)
		if err != nil {
			return
		}
		var c invariantCounts
		checkInvariants(t, sql, stmts, &c)
	})
}

// longestOperatorRun is the length of the longest run of operator
// characters in sql.
func longestOperatorRun(sql string) int {
	longest, run := 0, 0
	for i := 0; i < len(sql); i++ {
		if strings.IndexByte("+-*/<>=~!@#%^&|`?", sql[i]) >= 0 {
			run++
			if run > longest {
				longest = run
			}
		} else {
			run = 0
		}
	}
	return longest
}
