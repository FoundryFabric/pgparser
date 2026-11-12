package parser

import "testing"

// TestReindexIsPostgreSQL16s pins PostgreSQL 16's answers for 886 REINDEX
// statements: SYSTEM, DATABASE, SCHEMA, TABLE and INDEX with and without an
// option list and CONCURRENTLY, followed by no name, a plain, quoted,
// qualified or keyword name, two names or a statement. REINDEX SYSTEM and
// REINDEX DATABASE take no name.
func TestReindexIsPostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/reindex_pg16.jsonl", 886)
}
