package parser

import (
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// TestConstBitIsPostgreSQL16s pins PostgreSQL 16's answers for 123
// statements with a BIT or BIT VARYING constant, with and without a length,
// before strings, bit strings, numbers and nothing, and BIT in casts and
// column and parameter types.
func TestConstBitIsPostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/const_bit_pg16.jsonl", 123)
}

// TestConstBitHasNoLength requires a constant's BIT without a length to
// have no type modifiers, as libpg_query's tree has it, and a column's BIT
// to be BIT(1).
func TestConstBitHasNoLength(t *testing.T) {
	list, err := Parse("select bit '1'")
	if err != nil {
		t.Fatal(err)
	}
	tc := list.Items[0].(*nodes.SelectStmt).TargetList.Items[0].(*nodes.ResTarget).Val.(*nodes.TypeCast)
	if tc.TypeName.Typmods != nil {
		t.Errorf("select bit '1': typmods %v, want none", tc.TypeName.Typmods)
	}
	list, err = Parse("create table t (a bit)")
	if err != nil {
		t.Fatal(err)
	}
	cd := list.Items[0].(*nodes.CreateStmt).TableElts.Items[0].(*nodes.ColumnDef)
	if cd.TypeName.Typmods == nil || len(cd.TypeName.Typmods.Items) != 1 {
		t.Errorf("create table t (a bit): typmods %v, want one", cd.TypeName.Typmods)
	}
}
