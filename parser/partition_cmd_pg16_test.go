package parser

import "testing"

// TestPartitionCommandsArePostgreSQL16s pins PostgreSQL 16's answers for
// 1,268 statements with ATTACH PARTITION and DETACH PARTITION in each of
// their forms after ALTER TABLE (with IF EXISTS, ONLY and *), FOREIGN TABLE,
// VIEW, MATERIALIZED VIEW, SEQUENCE and INDEX, alone and before or after
// another command in a list. PostgreSQL 16 takes them alone after ALTER
// TABLE, and ATTACH PARTITION without a bound after ALTER INDEX.
func TestPartitionCommandsArePostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/partition_cmd_pg16.jsonl", 1268)
}
