package parser

import "testing"

// TestWithOidsIsPostgreSQL16s pins PostgreSQL 16's answers for 376 statements
// with WITH OIDS, WITHOUT OIDS and WITH options after CREATE TABLE in its
// forms (column list, empty, PARTITION OF, OF type, AS, LIKE, INHERITS) and
// CREATE MATERIALIZED VIEW, and SET WITH OIDS, SET WITHOUT OIDS and SET WITH
// (...) in ALTER TABLE, INDEX, VIEW, MATERIALIZED VIEW, FOREIGN TABLE and
// SEQUENCE, alone and in a command list. PostgreSQL 16 has only WITHOUT OIDS.
func TestWithOidsIsPostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/with_oids_pg16.jsonl", 376)
}
