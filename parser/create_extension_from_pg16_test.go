package parser

import "testing"

// TestCreateExtensionFromIsPostgreSQL16s pins PostgreSQL 16's answers for
// 23 CREATE EXTENSION statements with FROM, which PostgreSQL 16 refuses as
// no longer supported, and with its other options, and ALTER EXTENSION
// UPDATE beside them.
func TestCreateExtensionFromIsPostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/create_extension_from_pg16.jsonl", 23)
}
