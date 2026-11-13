package parser

import (
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// TestFuncFlagsArePostgreSQL16s pins PostgreSQL 16's answers for 14
// CREATE and ALTER FUNCTION and PROCEDURE statements with STRICT, CALLED
// and RETURNS NULL ON NULL INPUT, SECURITY, LEAKPROOF and WINDOW, and
// malformed ones.
func TestFuncFlagsArePostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/func_flags_pg16.jsonl", 14)
}

// TestFuncFlagsAreBooleans requires a function's flag options to have a
// Boolean argument, as PostgreSQL 16's grammar makes them.
func TestFuncFlagsAreBooleans(t *testing.T) {
	for sql, want := range map[string]bool{
		"create function f() returns int language sql strict as 'select 1'":                     true,
		"create function f() returns int language sql called on null input as 'select 1'":       false,
		"create function f() returns int language sql returns null on null input as 'select 1'": true,
		"create function f() returns int language sql security definer as 'select 1'":           true,
		"create function f() returns int language sql security invoker as 'select 1'":           false,
		"create function f() returns int language sql leakproof as 'select 1'":                  true,
		"create function f() returns int language sql not leakproof as 'select 1'":              false,
		"create function f() returns int language sql window as 'select 1'":                     true,
	} {
		list, err := Parse(sql)
		if err != nil {
			t.Fatal(err)
		}
		opts := list.Items[0].(*nodes.CreateFunctionStmt).Options.Items
		if b, ok := opts[1].(*nodes.DefElem).Arg.(*nodes.Boolean); !ok || b.Boolval != want {
			t.Errorf("%s: argument %#v, want Boolean %v", sql, opts[1].(*nodes.DefElem).Arg, want)
		}
	}
}
