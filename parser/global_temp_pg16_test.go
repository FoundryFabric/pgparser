package parser

import (
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// TestGlobalTempIsPostgreSQL16s pins PostgreSQL 16's answers for 34
// statements creating a table, a view or a sequence GLOBAL TEMP or GLOBAL
// TEMPORARY, the TEMP, LOCAL TEMP and UNLOGGED spellings beside them, and
// the GLOBAL spellings PostgreSQL 16 refuses.
func TestGlobalTempIsPostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/global_temp_pg16.jsonl", 34)
}

// TestGlobalTempIsTemp requires GLOBAL TEMP and GLOBAL TEMPORARY to make a
// temporary relation, as TEMP does.
func TestGlobalTempIsTemp(t *testing.T) {
	for _, sql := range []string{
		"create global temp table t (a int)",
		"create global temporary table t (a int)",
	} {
		list, err := Parse(sql)
		if err != nil {
			t.Fatal(err)
		}
		if p := list.Items[0].(*nodes.CreateStmt).Relation.Relpersistence; p != 't' {
			t.Errorf("%s: relpersistence %q, want 't'", sql, p)
		}
	}
}
