package parser

import (
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// TestAlterColumnUsingIsPostgreSQL16s pins PostgreSQL 16's answers for 10
// ALTER COLUMN ... TYPE statements, with and without SET DATA and USING,
// and malformed ones.
func TestAlterColumnUsingIsPostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/alter_column_using_pg16.jsonl", 10)
}

// TestAlterColumnKeepsUsing requires ALTER COLUMN ... TYPE ... USING to
// keep its expression in the ColumnDef's RawDefault, as PostgreSQL 16's
// grammar does, and leave it nil without USING.
func TestAlterColumnKeepsUsing(t *testing.T) {
	for sql, using := range map[string]bool{
		"alter table t alter column a type int using a::int":             true,
		"alter table t alter column a set data type text using upper(a)": true,
		"alter table t alter column a type int":                          false,
	} {
		list, err := Parse(sql)
		if err != nil {
			t.Fatal(err)
		}
		def := list.Items[0].(*nodes.AlterTableStmt).Cmds.Items[0].(*nodes.AlterTableCmd).Def.(*nodes.ColumnDef)
		if (def.RawDefault != nil) != using {
			t.Errorf("%s: RawDefault %v, want it set %v", sql, def.RawDefault, using)
		}
	}
}
