package parser

import (
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// TestAlterAttributeIsPostgreSQL16s pins PostgreSQL 16's answers for 8
// ALTER TYPE ... ALTER ATTRIBUTE statements, with and without SET DATA,
// COLLATE and a drop behaviour, and malformed ones.
func TestAlterAttributeIsPostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/alter_attribute_pg16.jsonl", 8)
}

// TestAlterAttributeNamesTheCommand requires ALTER ATTRIBUTE ... TYPE to
// name the attribute in the AlterTableCmd only, its ColumnDef unnamed, as
// PostgreSQL 16's grammar has it.
func TestAlterAttributeNamesTheCommand(t *testing.T) {
	for _, sql := range []string{
		"alter type t alter attribute a type text",
		"alter type t alter attribute a set data type text",
	} {
		list, err := Parse(sql)
		if err != nil {
			t.Fatal(err)
		}
		cmd := list.Items[0].(*nodes.AlterTableStmt).Cmds.Items[0].(*nodes.AlterTableCmd)
		if def := cmd.Def.(*nodes.ColumnDef); cmd.Name != "a" || def.Colname != "" {
			t.Errorf("%s: command %q, column %q, want a and none", sql, cmd.Name, def.Colname)
		}
	}
}
