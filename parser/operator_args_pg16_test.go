package parser

import "testing"

// TestOperatorArgumentsArePostgreSQL16s pins PostgreSQL 16's answers for 598
// statements naming an operator with its argument types in DROP OPERATOR,
// COMMENT ON, ALTER EXTENSION, ALTER OPERATOR and SECURITY LABEL: one type,
// two, NONE on either side, three, none, a trailing comma, followed by the
// rest of the statement or something PostgreSQL refuses. One type alone is
// refused with missing argument at its closing parenthesis.
func TestOperatorArgumentsArePostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/operator_args_pg16.jsonl", 598)
}
