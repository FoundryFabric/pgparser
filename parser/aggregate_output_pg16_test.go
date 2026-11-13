package parser

import "testing"

// TestAggregateOutputIsPostgreSQL16s pins PostgreSQL 16's answers for 13
// aggregate argument lists: OUT and INOUT ones, which PostgreSQL 16 refuses
// as 0A000 aggregates cannot have output arguments at the argument, in
// CREATE, DROP, ALTER and COMMENT ON AGGREGATE and among direct and ordered
// arguments, and IN, VARIADIC and unmarked ones, which it takes.
func TestAggregateOutputIsPostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/aggregate_output_pg16.jsonl", 13)
}
