package parser

import (
	"reflect"
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// TestParseQualifiedDropFunctionIsObjectWithArgs pins the shape a qualified
// DROP FUNCTION, PROCEDURE or ROUTINE name takes since any_name became
// PostgreSQL 16's `ColId attrs`: an ObjectWithArgs with no argument list, as
// PostgreSQL 16's function_with_argtypes builds it. An unqualified name is still
// the fork's any_name list (PostgreSQL 16 builds an ObjectWithArgs there too).
func TestParseQualifiedDropFunctionIsObjectWithArgs(t *testing.T) {
	for _, c := range []struct {
		sql   string
		parts []string
	}{
		{"drop function s.f", []string{"s", "f"}},
		{"drop function if exists s.f cascade", []string{"s", "f"}},
		{"drop procedure s.p", []string{"s", "p"}},
		{"drop routine if exists a.b.c", []string{"a", "b", "c"}},
	} {
		obj := dropObject(t, c.sql)
		owa, ok := obj.(*nodes.ObjectWithArgs)
		if !ok {
			t.Errorf("Parse(%q): object is %T, want *nodes.ObjectWithArgs", c.sql, obj)
			continue
		}
		if got := stringParts(owa.Objname); !reflect.DeepEqual(got, c.parts) || !owa.ArgsUnspecified || owa.Objargs != nil {
			t.Errorf("Parse(%q): ObjectWithArgs{%v, args %v, unspecified %v}, want {%v, nil, true}",
				c.sql, got, owa.Objargs, owa.ArgsUnspecified, c.parts)
		}
	}
	if obj, ok := dropObject(t, "drop function f").(*nodes.List); !ok || !reflect.DeepEqual(stringParts(obj), []string{"f"}) {
		t.Errorf("Parse(drop function f): object is %#v, want the list [f]", obj)
	}
}

// dropObject is the one object a one-statement DROP names.
func dropObject(t *testing.T, sql string) nodes.Node {
	t.Helper()
	stmts, err := Parse(sql)
	if err != nil {
		t.Fatalf("Parse(%q): %v", sql, err)
	}
	drop, ok := stmts.Items[0].(*nodes.DropStmt)
	if !ok || len(drop.Objects.Items) != 1 {
		t.Fatalf("Parse(%q): %#v, want a DropStmt of one object", sql, stmts.Items[0])
	}
	return drop.Objects.Items[0]
}

func stringParts(l *nodes.List) []string {
	var out []string
	for _, item := range l.Items {
		out = append(out, item.(*nodes.String).Str)
	}
	return out
}
