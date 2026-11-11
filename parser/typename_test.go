package parser

import (
	"reflect"
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// TestParseTypeNameTakesAnyNumberOfParts is PostgreSQL 16's GenericType:
// type_function_name attrs opt_type_modifiers, so a type name has any number
// of dotted parts, each a ColLabel after the first. Names holds every part in
// order; a typmod and array bounds still apply, and the TypeName is at its
// first part, GenericType's @1, also where no Typename wraps it (a
// sequence's AS). A name of three or more parts is refused only by name
// resolution, not by the grammar.
func TestParseTypeNameTakesAnyNumberOfParts(t *testing.T) {
	for _, c := range []struct {
		sql      string
		parts    []string
		typmods  int
		bounds   []int64
		location nodes.ParseLoc
	}{
		{"select null::a", []string{"a"}, 0, nil, 13},
		{"select null::a.b", []string{"a", "b"}, 0, nil, 13},
		{"select null::a.b.c", []string{"a", "b", "c"}, 0, nil, 13},
		{"select null::a.b.c.d", []string{"a", "b", "c", "d"}, 0, nil, 13},
		{"select null::otherdb.pg_catalog.int4", []string{"otherdb", "pg_catalog", "int4"}, 0, nil, 13},
		{"select null::a.b.from", []string{"a", "b", "from"}, 0, nil, 13},
		{`select null::a."B".c`, []string{"a", "B", "c"}, 0, nil, 13},
		{"select cast(null as a)", []string{"a"}, 0, nil, 20},
		{"select cast(null as a.b)", []string{"a", "b"}, 0, nil, 20},
		{"select cast(null as a.b.c)", []string{"a", "b", "c"}, 0, nil, 20},
		{"select cast(null as a.b.c.d)", []string{"a", "b", "c", "d"}, 0, nil, 20},
		{"select null::a.b.c(3)", []string{"a", "b", "c"}, 1, nil, 13},
		{"select cast(null as a.b.c.d(3, 4))", []string{"a", "b", "c", "d"}, 2, nil, 20},
		{"select null::a.b.c[]", []string{"a", "b", "c"}, 0, []int64{-1}, 13},
		{"select null::a.b.c.d(3)[2][]", []string{"a", "b", "c", "d"}, 1, []int64{2, -1}, 13},
		{"select null::a.b.c array", []string{"a", "b", "c"}, 0, []int64{-1}, 13},
		{"create table t (x a.b.c)", []string{"a", "b", "c"}, 0, nil, 18},
		{"create table t (x a.b.c.d(5)[])", []string{"a", "b", "c", "d"}, 1, []int64{-1}, 18},
		{"create sequence s as int8", []string{"int8"}, 0, nil, 21},
		{"create sequence s as pg_catalog.int8", []string{"pg_catalog", "int8"}, 0, nil, 21},
		{"create sequence s as a.b.c", []string{"a", "b", "c"}, 0, nil, 21},
	} {
		stmts, err := Parse(c.sql)
		if err != nil {
			t.Errorf("Parse(%q): %v", c.sql, err)
			continue
		}
		names := typeNames(stmts)
		if len(names) != 1 {
			t.Errorf("Parse(%q): %d TypeName nodes, want 1", c.sql, len(names))
			continue
		}
		tn := names[0]
		var parts []string
		for _, item := range tn.Names.Items {
			parts = append(parts, item.(*nodes.String).Str)
		}
		typmods := 0
		if tn.Typmods != nil {
			typmods = len(tn.Typmods.Items)
		}
		var bounds []int64
		if tn.ArrayBounds != nil {
			for _, item := range tn.ArrayBounds.Items {
				bounds = append(bounds, item.(*nodes.Integer).Ival)
			}
		}
		if !reflect.DeepEqual(parts, c.parts) || typmods != c.typmods || !reflect.DeepEqual(bounds, c.bounds) || tn.Location != c.location {
			t.Errorf("Parse(%q): TypeName %v, %d typmods, bounds %v, at %d; want %v, %d, %v, at %d",
				c.sql, parts, typmods, bounds, tn.Location, c.parts, c.typmods, c.bounds, c.location)
		}
	}
}

// typeNames is every TypeName reachable from root.
func typeNames(root nodes.Node) []*nodes.TypeName {
	var out []*nodes.TypeName
	var walk func(v reflect.Value)
	walk = func(v reflect.Value) {
		switch v.Kind() { //nolint:exhaustive // only the kinds that can carry a Node
		case reflect.Ptr, reflect.Interface:
			if v.IsNil() {
				return
			}
			if tn, ok := v.Interface().(*nodes.TypeName); ok && v.Kind() == reflect.Ptr {
				out = append(out, tn)
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
