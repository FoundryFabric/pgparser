package parser

import (
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// TestTrimArgsArePostgreSQL16s pins PostgreSQL 16's answers for 10 TRIM
// calls with BOTH, LEADING and TRAILING, with and without the characters
// and FROM, and malformed ones.
func TestTrimArgsArePostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/trim_args_pg16.jsonl", 10)
}

// TestTrimCharactersComeLast requires TRIM(x FROM y)'s arguments to be y
// then x, as PostgreSQL 16's trim_list appends the characters to trim to
// the string they are trimmed from (btrim(string, characters)).
func TestTrimCharactersComeLast(t *testing.T) {
	list, err := Parse("select trim(both 'x' from 'xay')")
	if err != nil {
		t.Fatal(err)
	}
	args := list.Items[0].(*nodes.SelectStmt).TargetList.Items[0].(*nodes.ResTarget).Val.(*nodes.FuncCall).Args.Items
	var got []string
	for _, a := range args {
		got = append(got, a.(*nodes.A_Const).Val.(*nodes.String).Str)
	}
	if len(got) != 2 || got[0] != "xay" || got[1] != "x" {
		t.Errorf("arguments %q, want xay then x", got)
	}
}
