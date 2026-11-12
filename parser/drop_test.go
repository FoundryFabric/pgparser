package parser

import (
	"reflect"
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// TestParseDropFunctionIsObjectWithArgs pins the shape a DROP FUNCTION,
// PROCEDURE or ROUTINE name without arguments takes, qualified or not: an
// ObjectWithArgs with no argument list, as PostgreSQL 16's
// function_with_argtypes builds it.
func TestParseDropFunctionIsObjectWithArgs(t *testing.T) {
	for _, c := range []struct {
		sql   string
		parts []string
	}{
		{"drop function f", []string{"f"}},
		{"drop function if exists f cascade", []string{"f"}},
		{"drop procedure p restrict", []string{"p"}},
		{"drop routine r", []string{"r"}},
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
