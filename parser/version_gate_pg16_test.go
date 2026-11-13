package parser

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"testing"
)

// TestVersionGatesArePostgreSQL16s pins PostgreSQL 16.15's answers for 27
// statements that use PostgreSQL 17 grammar reached through words PostgreSQL
// 16 also has: MERGE's WHEN NOT MATCHED BY SOURCE or TARGET and RETURNING,
// SET ACCESS METHOD DEFAULT, SET EXPRESSION, AT LOCAL, ALTER DOMAIN ADD NOT
// NULL, an operator option without a value, CLUSTER with options and no
// table, COPY's FORCE NULL and FORCE NOT NULL with *, and ALTER STATISTICS
// SET STATISTICS DEFAULT. As PostgreSQL 16 each is refused at the first
// token PostgreSQL 16's grammar cannot take; as PostgreSQL 17 each parses,
// but for a MERGE whose BY has no SOURCE or TARGET, which PostgreSQL 17
// refuses at the token after BY.
func TestVersionGatesArePostgreSQL16s(t *testing.T) {
	const path = "testdata/version_gate_pg16.jsonl"
	checkPG16Answers(t, path, 27)
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	refused17 := map[string]string{
		"merge into t using s on t.a = s.a when not matched by then delete": `syntax error at or near "then"`,
	}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var c pg16Answer
		if err := json.Unmarshal(sc.Bytes(), &c); err != nil {
			t.Fatal(err)
		}
		_, err := Parse(c.SQL)
		var pe *ParseError
		switch want, ok := refused17[c.SQL]; {
		case ok && (!errors.As(err, &pe) || pe.Message != want):
			t.Errorf("as PostgreSQL 17, Parse(%q) = %v, want %s", c.SQL, err, want)
		case !ok && err != nil:
			t.Errorf("as PostgreSQL 17, Parse(%q) = %v, want it parsed", c.SQL, err)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
}
