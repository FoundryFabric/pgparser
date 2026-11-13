package parser

import (
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// TestViewRenameIfExistsIsPostgreSQL16s pins PostgreSQL 16's answers for 29
// statements renaming a view's or a materialized view's column with IF
// EXISTS, with and without COLUMN, the forms PostgreSQL 16 refuses, and
// the other renames beside them.
func TestViewRenameIfExistsIsPostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/view_rename_if_exists_pg16.jsonl", 29)
}

// TestViewRenameIfExistsRenamesTheColumn requires ALTER VIEW and ALTER
// MATERIALIZED VIEW IF EXISTS ... RENAME [COLUMN] to rename the column with
// missing_ok set, as PostgreSQL 16's RenameStmt does.
func TestViewRenameIfExistsRenamesTheColumn(t *testing.T) {
	for _, c := range []struct {
		sql string
		rel nodes.ObjectType
	}{
		{"alter view if exists v rename column a to b", nodes.OBJECT_VIEW},
		{"alter view if exists v rename a to b", nodes.OBJECT_VIEW},
		{"alter materialized view if exists v rename column a to b", nodes.OBJECT_MATVIEW},
		{"alter materialized view if exists v rename a to b", nodes.OBJECT_MATVIEW},
	} {
		list, err := Parse(c.sql)
		if err != nil {
			t.Fatal(err)
		}
		r := list.Items[0].(*nodes.RenameStmt)
		if r.RenameType != nodes.OBJECT_COLUMN || r.RelationType != c.rel || !r.MissingOk || r.Relation.Relname != "v" || r.Subname != "a" || r.Newname != "b" {
			t.Errorf("%s: %+v", c.sql, r)
		}
	}
}
