package parser

import (
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// TestParamModeIsPostgreSQL16s pins PostgreSQL 16's answers for 15
// function, procedure and aggregate argument lists, with and without a
// mode, names and defaults, and malformed ones.
func TestParamModeIsPostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/param_mode_pg16.jsonl", 15)
}

// TestUnwrittenParamModeIsDefault requires an argument written without a
// mode to have FUNC_PARAM_DEFAULT, as PostgreSQL 16's func_arg gives it,
// and a written IN to keep FUNC_PARAM_IN.
func TestUnwrittenParamModeIsDefault(t *testing.T) {
	list, err := Parse("create function f(int, a int, in b int, c in int, out d int) language sql as ''")
	if err != nil {
		t.Fatal(err)
	}
	want := []nodes.FunctionParameterMode{nodes.FUNC_PARAM_DEFAULT, nodes.FUNC_PARAM_DEFAULT, nodes.FUNC_PARAM_IN, nodes.FUNC_PARAM_IN, nodes.FUNC_PARAM_OUT}
	for i, p := range list.Items[0].(*nodes.CreateFunctionStmt).Parameters.Items {
		if m := p.(*nodes.FunctionParameter).Mode; m != want[i] {
			t.Errorf("parameter %d: mode %q, want %q", i, m, want[i])
		}
	}
	list, err = Parse("create aggregate a(int) (sfunc = f, stype = int)")
	if err != nil {
		t.Fatal(err)
	}
	args := list.Items[0].(*nodes.DefineStmt).Args.Items[0].(*nodes.List)
	if m := args.Items[0].(*nodes.FunctionParameter).Mode; m != nodes.FUNC_PARAM_DEFAULT {
		t.Errorf("aggregate argument: mode %q, want %q", m, nodes.FUNC_PARAM_DEFAULT)
	}
}
