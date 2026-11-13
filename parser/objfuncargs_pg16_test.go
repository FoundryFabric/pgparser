package parser

import (
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// TestObjFuncArgsIsPostgreSQL16s pins PostgreSQL 16's answers for 22
// statements naming a function, procedure, routine or aggregate with its
// arguments: DROP, ALTER, GRANT, COMMENT, SECURITY LABEL, CREATE CAST and
// TRANSFORM and operator families, with OUT, INOUT and VARIADIC arguments.
func TestObjFuncArgsIsPostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/objfuncargs_pg16.jsonl", 22)
}

// TestObjectWithArgsKeepsItsArguments requires an ObjectWithArgs to keep
// the arguments as written in Objfuncargs and the types identifying the
// function in Objargs, OUT arguments left out, as PostgreSQL 16's
// function_with_argtypes and aggregate_with_argtypes have them.
func TestObjectWithArgsKeepsItsArguments(t *testing.T) {
	for sql, want := range map[string][2]int{
		"drop function f(a int, out b int, inout c text)": {2, 3},
		"drop procedure p(out int, int, int)":             {2, 3},
		"drop function f()":                               {0, 0},
		"drop aggregate a(*)":                             {0, 0},
		"drop aggregate a(int order by text)":             {2, 2},
	} {
		list, err := Parse(sql)
		if err != nil {
			t.Fatal(err)
		}
		o := list.Items[0].(*nodes.DropStmt).Objects.Items[0].(*nodes.ObjectWithArgs)
		if got := [2]int{len(o.Objargs.Elems()), len(o.Objfuncargs.Elems())}; got != want {
			t.Errorf("%s: %d argument types and %d arguments, want %d and %d", sql, got[0], got[1], want[0], want[1])
		}
		if want[1] > 0 && o.Objfuncargs.Items[0].(*nodes.FunctionParameter).ArgType == nil {
			t.Errorf("%s: an argument without its type", sql)
		}
	}
}
