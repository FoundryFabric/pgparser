package parser

import "testing"

// TestGrantOnParameterIsPostgreSQL16s pins PostgreSQL 16's answers for 500
// GRANT, REVOKE and REVOKE GRANT OPTION FOR statements on PARAMETER: SET,
// ALTER SYSTEM, ALL and other privileges, with column lists, on plain,
// dotted, quoted and keyword names and lists of them, with each clause the
// statements take, and broken names; and a table named parameter.
func TestGrantOnParameterIsPostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/grant_parameter_pg16.jsonl", 500)
}
