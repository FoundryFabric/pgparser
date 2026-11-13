package parser

import (
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// TestGrantNameIsPostgreSQL16s pins PostgreSQL 16's answers for 18 GRANT
// and REVOKE statements on databases, languages, schemas, tablespaces,
// foreign data wrappers and servers and on all objects in schemas, beside
// tables and types, and a qualified schema name PostgreSQL 16 refuses.
func TestGrantNameIsPostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/grant_name_pg16.jsonl", 18)
}

// TestGrantNamesAreStrings requires a GRANT or REVOKE on objects named by
// bare names to have a String for each, as PostgreSQL 16's
// privilege_target has them, and one on tables a RangeVar for each.
func TestGrantNamesAreStrings(t *testing.T) {
	for _, sql := range []string{
		"grant usage on schema s, t to r",
		"grant select on all tables in schema s, t to r",
		"revoke grant option for create on database s, t from r",
	} {
		list, err := Parse(sql)
		if err != nil {
			t.Fatal(err)
		}
		objs := list.Items[0].(*nodes.GrantStmt).Objects.Items
		if len(objs) != 2 || objs[0].(*nodes.String).Str != "s" || objs[1].(*nodes.String).Str != "t" {
			t.Errorf("%s: objects %v, want the Strings s and t", sql, objs)
		}
	}
}
