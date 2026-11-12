package parser

import "testing"

// TestRoleOptionsArePostgreSQL16s pins PostgreSQL 16's answers for 2,553
// statements with a role option in CREATE ROLE, USER and GROUP and ALTER ROLE
// and USER: known options in any case, unknown words, quoted words and
// keywords, followed by nothing, another option, a statement or something
// PostgreSQL refuses there, and UNENCRYPTED PASSWORD. An unknown option is
// refused when it is read, before the token after it.
func TestRoleOptionsArePostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/role_option_pg16.jsonl", 2553)
}
