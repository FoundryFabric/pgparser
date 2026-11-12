package parser

import "testing"

// TestPolicyOptionsArePostgreSQL16s pins PostgreSQL 16's answers for 99
// CREATE POLICY statements with AS and PERMISSIVE, RESTRICTIVE in any case or
// quoted, an unknown word or a keyword, followed by each of the policy's
// clauses or something PostgreSQL refuses. An unknown word is unrecognized
// row security option, at the word.
func TestPolicyOptionsArePostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/policy_option_pg16.jsonl", 99)
}
