package parser

import "testing"

// TestRoleSpecKeywordsArePostgreSQL16s pins PostgreSQL 16's answers for 1,120
// statements with each of PostgreSQL 16's type_func_name keywords
// (AUTHORIZATION, BINARY, COLLATION, ... VERBOSE), a reserved keyword, a
// quoted keyword and a name at 40 places that take a role: GRANT and REVOKE,
// OWNER TO, CREATE SCHEMA AUTHORIZATION, ALTER GROUP, DROP ROLE and OWNED,
// REASSIGN OWNED, user mappings, default privileges, policies, ALTER ROLE,
// CREATE ROLE's lists, SET ROLE and a RoleId. A role is a NonReservedWord.
func TestRoleSpecKeywordsArePostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/role_spec_keywords_pg16.jsonl", 1120)
}
