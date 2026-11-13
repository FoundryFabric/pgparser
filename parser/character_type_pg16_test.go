package parser

import (
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// TestCharacterTypeIsPostgreSQL16s pins PostgreSQL 16's answers for 14
// statements with every spelling of CHARACTER, VARCHAR and NCHAR, with and
// without a length and VARYING, as a column, a cast, a constant's type and
// an argument, and malformed ones.
func TestCharacterTypeIsPostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/character_type_pg16.jsonl", 14)
}

// TestCharacterTypmods requires a character type's length to be an A_Const
// typmod, as PostgreSQL 16's makeIntConst makes it, a CHAR without one to
// be CHAR(1) except as a constant's type, and VARCHAR without one to have
// no typmod.
func TestCharacterTypmods(t *testing.T) {
	typmod := func(tn *nodes.TypeName) any {
		if tn.Typmods == nil {
			return nil
		}
		return tn.Typmods.Items[0].(*nodes.A_Const).Val.(*nodes.Integer).Ival
	}
	for sql, want := range map[string]any{
		"select 'x'::char":       int64(1),
		"select 'x'::nchar(5)":   int64(5),
		"select 'x'::varchar":    nil,
		"select 'x'::varchar(3)": int64(3),
	} {
		list, err := Parse(sql)
		if err != nil {
			t.Fatal(err)
		}
		tc := list.Items[0].(*nodes.SelectStmt).TargetList.Items[0].(*nodes.ResTarget).Val.(*nodes.TypeCast)
		if got := typmod(tc.TypeName); got != want {
			t.Errorf("%s: typmod %v, want %v", sql, got, want)
		}
	}
	for sql, want := range map[string]any{
		"select char 'x'":    nil,
		"select char(2) 'x'": int64(2),
	} {
		list, err := Parse(sql)
		if err != nil {
			t.Fatal(err)
		}
		tc := list.Items[0].(*nodes.SelectStmt).TargetList.Items[0].(*nodes.ResTarget).Val.(*nodes.TypeCast)
		if got := typmod(tc.TypeName); got != want {
			t.Errorf("%s: typmod %v, want %v", sql, got, want)
		}
	}
}
