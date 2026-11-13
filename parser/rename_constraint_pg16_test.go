package parser

import (
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// TestRenameConstraintIsPostgreSQL16s pins PostgreSQL 16's answers for 10
// ALTER TABLE and ALTER DOMAIN ... RENAME statements, and malformed ones.
func TestRenameConstraintIsPostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/rename_constraint_pg16.jsonl", 10)
}

// TestRenameConstraintHasNoRelationType requires ALTER TABLE ... RENAME
// CONSTRAINT to leave RelationType unset, as PostgreSQL 16's grammar does,
// and RENAME COLUMN to keep OBJECT_TABLE.
func TestRenameConstraintHasNoRelationType(t *testing.T) {
	for sql, want := range map[string]nodes.ObjectType{
		"alter table t rename constraint c to d":           0,
		"alter table if exists t rename constraint c to d": 0,
		"alter table t rename column a to b":               nodes.OBJECT_TABLE,
	} {
		list, err := Parse(sql)
		if err != nil {
			t.Fatal(err)
		}
		if got := list.Items[0].(*nodes.RenameStmt).RelationType; got != want {
			t.Errorf("%s: RelationType %v, want %v", sql, got, want)
		}
	}
}
