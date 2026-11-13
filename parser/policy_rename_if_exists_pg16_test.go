package parser

import (
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// TestPolicyRenameIfExistsIsPostgreSQL16s pins PostgreSQL 16's answers for
// 20 ALTER POLICY statements renaming with and without IF EXISTS, a policy
// named if, the IF EXISTS forms PostgreSQL 16 refuses, and the trigger and
// rule renames beside them.
func TestPolicyRenameIfExistsIsPostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/policy_rename_if_exists_pg16.jsonl", 20)
}

// TestPolicyRenameIfExistsIsMissingOk requires ALTER POLICY IF EXISTS ...
// RENAME to rename the policy with missing_ok set, and a policy named if to
// be renamed without it.
func TestPolicyRenameIfExistsIsMissingOk(t *testing.T) {
	for _, c := range []struct {
		sql       string
		name      string
		missingOk bool
	}{
		{"alter policy if exists p on t rename to q", "p", true},
		{"alter policy if on t rename to q", "if", false},
	} {
		list, err := Parse(c.sql)
		if err != nil {
			t.Fatal(err)
		}
		r := list.Items[0].(*nodes.RenameStmt)
		if r.RenameType != nodes.OBJECT_POLICY || r.Subname != c.name || r.Newname != "q" || r.Relation.Relname != "t" || r.MissingOk != c.missingOk {
			t.Errorf("%s: %+v", c.sql, r)
		}
	}
}
