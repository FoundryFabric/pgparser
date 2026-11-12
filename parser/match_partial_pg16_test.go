package parser

import "testing"

// TestMatchPartialIsPostgreSQL16s pins PostgreSQL 16's answers for 104
// foreign keys with MATCH PARTIAL, FULL, SIMPLE and broken match types: on
// a column and a table, in CREATE TABLE and ALTER TABLE, with columns, key
// actions and constraint attributes after it, and after an earlier error.
// MATCH PARTIAL is 0A000 at MATCH.
func TestMatchPartialIsPostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/match_partial_pg16.jsonl", 104)
}
