package parser

import "testing"

// TestColumnOptionsArePostgreSQL16s pins PostgreSQL 16's answers for 302
// statements with a parenthesised option list after ALTER [COLUMN] ... SET
// and RESET and after a relation's SET and RESET: plain and qualified names,
// with and without values, and broken qualified names, after ALTER TABLE,
// INDEX, VIEW, MATERIALIZED VIEW and FOREIGN TABLE. A column's options are
// reloptions, as a relation's are.
func TestColumnOptionsArePostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/column_options_pg16.jsonl", 302)
}
