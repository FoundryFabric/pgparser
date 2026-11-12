package parser

import "testing"

// TestAlterRoleIsPostgreSQL16s pins PostgreSQL 16's answers for 1,053 ALTER
// ROLE, USER and GROUP statements: each kind of role, with no options, WITH
// alone, options with and without WITH, SET, RESET and RENAME, and broken
// tails. ALTER ROLE and ALTER USER take an empty option list.
func TestAlterRoleIsPostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/alter_role_pg16.jsonl", 1053)
}
