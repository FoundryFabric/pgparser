package parser

import "testing"

// TestNotImplementedErrorsArePostgreSQL16s pins PostgreSQL 16's answers for
// 390 statements PostgreSQL 16's grammar reads and refuses with 0A000: the
// UNIQUE predicate in its forms and in expressions, CREATE ASSERTION with
// each constraint attribute and broken forms, and SET CATALOG in SET, ALTER
// ROLE, ALTER DATABASE and ALTER SYSTEM.
func TestNotImplementedErrorsArePostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/not_implemented_pg16.jsonl", 390)
}
