package parser

import (
	"errors"
	"reflect"
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// locationCases are statements PostgreSQL 16.4 refuses at a node's location:
// the 1-based position it reported (verified 2026-10-12) is that node's
// Location plus one, the node being the index-th of its kind in the tree
// (struct fields in declaration order).
var locationCases = []struct {
	sql      string
	kind     string
	index    int
	position int
}{
	// RangeVar: qualified_name's @1
	{"select * from nosuch", "RangeVar", 0, 15},
	{"select * from  s.nosuch", "RangeVar", 0, 16},
	{"select x from nosuch3 as n", "RangeVar", 0, 15},
	{"select * from only nosuch4", "RangeVar", 0, 20},
	// ColumnRef: columnref's @1, and target_el '*'
	{"select nosuch", "ColumnRef", 0, 8},
	{"select 1, t.nosuch from (select 1) t", "ColumnRef", 0, 11},
	{"select   x.y.z from (select 1) x", "ColumnRef", 0, 10},
	// ParamRef: PARAM's @1
	{"select $2", "ParamRef", 0, 8},
	{"select 1 + $3", "ParamRef", 0, 12},
	// A_Const: AexprConst's @1; a typed literal's string at @2; a negated
	// number at its minus sign (doNegate)
	{"select 'x'::int", "A_Const", 0, 8},
	{"select 1, 'abc'::int", "A_Const", 1, 11},
	{"select not 1", "A_Const", 0, 12},
	{"select not -1", "A_Const", 0, 12},
	{"select not - 1", "A_Const", 0, 12},
	{"select date 'x'", "A_Const", 0, 13},
	{"select interval 'x'", "A_Const", 0, 17},
	// A_Expr: the operator's @2, a prefix operator's @1, NULLIF's @1
	{"select 1 + 'a'::text", "A_Expr", 0, 10},
	{"select 1 @@@ 2", "A_Expr", 0, 10},
	{"select 'a'::text like 1", "A_Expr", 0, 18},
	{"select true like 'a' escape 'b'", "A_Expr", 0, 13},
	{"select 1 in ('a'::text)", "A_Expr", 0, 10},
	{"select 1 not in ('a'::text)", "A_Expr", 0, 10},
	{"select 1 between 'a'::text and 2", "A_Expr", 0, 10},
	{"select 1 is distinct from 'a'::text", "A_Expr", 0, 10},
	{"select nullif(1, 'a'::text)", "A_Expr", 0, 8},
	{"select - 'a'::text", "A_Expr", 0, 8},
	// FuncCall: func_name's @1, a SQL-syntax function's keyword, AT's @2
	{"select nosuchfn(1)", "FuncCall", 0, 8},
	{"select pg_catalog.nosuchfn(1, 2)", "FuncCall", 0, 8},
	{"select count(*) + nosuchfn()", "FuncCall", 1, 19},
	{"select extract(nosuch from 1)", "FuncCall", 0, 8},
	{"select substring(1 from 1)", "FuncCall", 0, 8},
	{"select 1 at time zone 'x'", "FuncCall", 0, 10},
	// TypeCast: TYPECAST's @2, CAST's @1
	{"select 1::date", "TypeCast", 0, 9},
	{"select true::date", "TypeCast", 0, 12},
	{"select cast(1 as date)", "TypeCast", 0, 8},
}

// TestLocationsArePostgreSQLs pins each node kind's Location to the position
// PostgreSQL 16 reports for an error at that node.
func TestLocationsArePostgreSQLs(t *testing.T) {
	for _, c := range locationCases {
		stmts, err := Parse(c.sql)
		if err != nil {
			t.Errorf("Parse(%q): %v", c.sql, err)
			continue
		}
		found := locationsOf(stmts, c.kind)
		if c.index >= len(found) {
			t.Errorf("%q: %d %s nodes, want at least %d", c.sql, len(found), c.kind, c.index+1)
			continue
		}
		if got := int(found[c.index]) + 1; got != c.position {
			t.Errorf("%q: %s[%d] is at %d, PostgreSQL 16 says %d", c.sql, c.kind, c.index, got, c.position)
		}
	}
}

// TestParseCollateTakesAQualifiedName is PostgreSQL 16's any_name for a
// COLLATE: ColId attrs, so a part after a dot is a ColLabel and takes a
// reserved keyword, and a bare reserved word stays a syntax error.
func TestParseCollateTakesAQualifiedName(t *testing.T) {
	for _, c := range []struct {
		sql      string
		parts    []string
		location nodes.ParseLoc
	}{
		{"select 'w' collate pg_catalog.default", []string{"pg_catalog", "default"}, 11},
		{`select 'w' collate "default"`, []string{"default"}, 11},
		{`select 'w' collate "C"`, []string{"C"}, 11},
		{"select 'w' collate a.b.c.d", []string{"a", "b", "c", "d"}, 11},
		{`select x collate pg_catalog."POSIX" from t`, []string{"pg_catalog", "POSIX"}, 9},
	} {
		stmts, err := Parse(c.sql)
		if err != nil {
			t.Fatalf("Parse(%q): %v", c.sql, err)
		}
		clauses := collateClauses(stmts)
		if len(clauses) != 1 {
			t.Fatalf("Parse(%q): %d CollateClause nodes, want 1", c.sql, len(clauses))
		}
		var got []string
		for _, item := range clauses[0].Collname.Items {
			got = append(got, item.(*nodes.String).Str)
		}
		if !reflect.DeepEqual(got, c.parts) || clauses[0].Location != c.location {
			t.Errorf("Parse(%q): COLLATE names %v at %d, want %v at %d",
				c.sql, got, clauses[0].Location, c.parts, c.location)
		}
	}
	_, err := Parse("select 'w' collate default")
	var pe *ParseError
	if !errors.As(err, &pe) || pe.Code != "42601" || pe.Message != `syntax error at or near "default"` || pe.Position != 19 {
		t.Errorf("Parse(select 'w' collate default) = %#v, want 42601 syntax error at or near \"default\" at 19", err)
	}
}

// collateClauses is every CollateClause reachable from root.
func collateClauses(root nodes.Node) []*nodes.CollateClause {
	var out []*nodes.CollateClause
	var walk func(v reflect.Value)
	walk = func(v reflect.Value) {
		switch v.Kind() { //nolint:exhaustive // only the kinds that can carry a Node
		case reflect.Ptr, reflect.Interface:
			if v.IsNil() {
				return
			}
			if c, ok := v.Interface().(*nodes.CollateClause); ok && v.Kind() == reflect.Ptr {
				out = append(out, c)
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
	return out
}

// locationsOf is the Location of every node of the named kind reachable
// from root, walking struct fields in declaration order.
func locationsOf(root nodes.Node, kind string) []nodes.ParseLoc {
	var out []nodes.ParseLoc
	var walk func(v reflect.Value)
	walk = func(v reflect.Value) {
		switch v.Kind() { //nolint:exhaustive // only the kinds that can carry a Node
		case reflect.Ptr, reflect.Interface:
			if v.IsNil() {
				return
			}
			if v.Kind() == reflect.Ptr && v.Elem().Kind() == reflect.Struct && v.Elem().Type().Name() == kind {
				out = append(out, nodes.ParseLoc(v.Elem().FieldByName("Location").Int()))
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
	return out
}
