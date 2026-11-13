package parser

import (
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// TestReplicaIdentityIsPostgreSQL16s pins PostgreSQL 16's answers for 24
// statements of ALTER TABLE REPLICA IDENTITY and ALTER CONSTRAINT, with
// each identity and constraint attribute, and malformed ones.
func TestReplicaIdentityIsPostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/replica_identity_pg16.jsonl", 24)
}

// TestAlterTableCmdKeepsItsDefinition requires REPLICA IDENTITY's identity
// and ALTER CONSTRAINT's constraint to be the command's definition, as
// libpg_query's tree has them.
func TestAlterTableCmdKeepsItsDefinition(t *testing.T) {
	cmd := func(sql string) *nodes.AlterTableCmd {
		list, err := parse16(sql)
		if err != nil {
			t.Fatal(err)
		}
		return list.Items[0].(*nodes.AlterTableStmt).Cmds.Items[0].(*nodes.AlterTableCmd)
	}
	for sql, want := range map[string]nodes.ReplicaIdentityStmt{
		"alter table t replica identity default":       {IdentityType: nodes.REPLICA_IDENTITY_DEFAULT},
		"alter table t replica identity full":          {IdentityType: nodes.REPLICA_IDENTITY_FULL},
		"alter table t replica identity nothing":       {IdentityType: nodes.REPLICA_IDENTITY_NOTHING},
		"alter table t replica identity using index i": {IdentityType: nodes.REPLICA_IDENTITY_INDEX, Name: "i"},
	} {
		c := cmd(sql)
		if ri, ok := c.Def.(*nodes.ReplicaIdentityStmt); !ok || *ri != want || c.Name != "" {
			t.Errorf("%s: definition %#v, name %q; want %#v", sql, c.Def, c.Name, want)
		}
	}
	for sql, want := range map[string][2]bool{
		"alter table t alter constraint c":                    {false, false},
		"alter table t alter constraint c deferrable":         {true, false},
		"alter table t alter constraint c initially deferred": {true, true},
	} {
		c := cmd(sql)
		con, ok := c.Def.(*nodes.Constraint)
		if !ok || con.Contype != nodes.CONSTR_FOREIGN || con.Conname != "c" || con.Deferrable != want[0] || con.Initdeferred != want[1] || c.Name != "" {
			t.Errorf("%s: definition %#v, name %q; want foreign key c, deferrable %v, initially deferred %v", sql, c.Def, c.Name, want[0], want[1])
		}
	}
}
