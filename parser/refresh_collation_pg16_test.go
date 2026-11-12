package parser

import "testing"

// TestRefreshCollationIsPostgreSQL16s pins PostgreSQL 16's answers for 147
// ALTER DATABASE statements with REFRESH COLLATION VERSION and its broken
// forms, on plain, quoted and keyword names, and the nearby ALTER COLLATION,
// ALTER TABLE and ALTER DATABASE forms.
func TestRefreshCollationIsPostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/refresh_collation_pg16.jsonl", 147)
}
