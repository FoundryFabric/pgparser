package parser

import "testing"

// TestUescapeIsPostgreSQL16s pins PostgreSQL 16's answers for 1,052
// statements with a U& string or identifier: followed by UESCAPE and every
// kind of token (each kind of string literal, valid and invalid escape
// characters, a number, a name, punctuation, a comment, the end of the
// input), by no UESCAPE, and by another U& literal, in a target list, as a
// label, a table name and an operand.
func TestUescapeIsPostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/uescape_pg16.jsonl", 1052)
}
