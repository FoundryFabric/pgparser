package parser

import "testing"

// TestExplainCreateIsPostgreSQL16s pins PostgreSQL 16's answers for 156
// EXPLAIN statements, with and without ANALYZE, VERBOSE and an option list,
// over CREATE TABLE in its forms: with columns, LIKE, PARTITION OF and OF
// a type, which EXPLAIN does not take, and AS a query or EXECUTE, with and
// without its clauses, and CREATE MATERIALIZED VIEW, which it takes.
func TestExplainCreateIsPostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/explain_create_pg16.jsonl", 156)
}
