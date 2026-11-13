package parser

import (
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// TestExternalSecurityIsPostgreSQL16s pins PostgreSQL 16's answers for 8
// CREATE and ALTER FUNCTION and PROCEDURE statements with EXTERNAL
// SECURITY DEFINER and INVOKER, and malformed ones.
func TestExternalSecurityIsPostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/external_security_pg16.jsonl", 8)
}

// TestExternalSecurityIsSecurity requires EXTERNAL SECURITY DEFINER and
// INVOKER to be the security option SECURITY DEFINER and INVOKER are, as
// PostgreSQL 16's grammar has them.
func TestExternalSecurityIsSecurity(t *testing.T) {
	for sql, want := range map[string]bool{
		"alter function f() external security definer": true,
		"alter function f() external security invoker": false,
	} {
		list, err := Parse(sql)
		if err != nil {
			t.Fatal(err)
		}
		d := list.Items[0].(*nodes.AlterFunctionStmt).Actions.Items[0].(*nodes.DefElem)
		if b, ok := d.Arg.(*nodes.Boolean); d.Defname != "security" || !ok || b.Boolval != want {
			t.Errorf("%s: %s %#v, want security %v", sql, d.Defname, d.Arg, want)
		}
	}
}
