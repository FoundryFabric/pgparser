package parser

import "testing"

// TestRuleCoverageIsPostgreSQL16s pins PostgreSQL 16's answers for 314
// statements, at least one for each grammar rule whose action no other test
// reduced, parsed as PostgreSQL 16. 9 are PostgreSQL 17 grammar, which a
// PostgreSQL 17 parser answers otherwise, marked with their pg17 class.
func TestRuleCoverageIsPostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/rule_coverage_pg16.jsonl", 314)
}
