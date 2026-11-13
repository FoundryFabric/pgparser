package parser

import (
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// TestInitiallyValidIsPostgreSQL16s pins PostgreSQL 16's answers for 15
// table constraints of every kind, in CREATE TABLE and ALTER TABLE ADD,
// with and without NOT VALID, and the attributes their kinds refuse.
func TestInitiallyValidIsPostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/initially_valid_pg16.jsonl", 15)
}

// TestOnlyCheckAndForeignKeysAreInitiallyValid requires InitiallyValid to
// be set as PostgreSQL 16's grammar sets it: for CHECK and FOREIGN KEY
// constraints without NOT VALID, and never for UNIQUE, PRIMARY KEY and
// EXCLUDE, whose index makes them valid.
func TestOnlyCheckAndForeignKeysAreInitiallyValid(t *testing.T) {
	for sql, want := range map[string]bool{
		"alter table t add unique (a)":                             false,
		"alter table t add constraint c unique using index i":      false,
		"alter table t add primary key (a)":                        false,
		"alter table t add primary key using index i":              false,
		"alter table t add exclude (a with =)":                     false,
		"alter table t add check (a > 0)":                          true,
		"alter table t add check (a > 0) not valid":                false,
		"alter table t add foreign key (a) references s":           true,
		"alter table t add foreign key (a) references s not valid": false,
	} {
		list, err := Parse(sql)
		if err != nil {
			t.Fatal(err)
		}
		c := list.Items[0].(*nodes.AlterTableStmt).Cmds.Items[0].(*nodes.AlterTableCmd).Def.(*nodes.Constraint)
		if c.InitiallyValid != want {
			t.Errorf("%s: InitiallyValid %v, want %v", sql, c.InitiallyValid, want)
		}
	}
}
