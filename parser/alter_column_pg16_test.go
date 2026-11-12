package parser

import "testing"

// TestAlterColumnIsPostgreSQL16s pins PostgreSQL 16's answers for 657 ALTER
// [COLUMN] commands: every action ALTER COLUMN takes, and broken ones, with
// and without COLUMN, on a plain, quoted, keyword and numbered column, after
// ALTER TABLE, FOREIGN TABLE, MATERIALIZED VIEW, INDEX and VIEW, alone and in
// a list. COLUMN is optional in each of them.
func TestAlterColumnIsPostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/alter_column_pg16.jsonl", 657)
}
