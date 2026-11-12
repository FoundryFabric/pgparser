package parser

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"testing"
)

// cteColumnListCase is one statement of testdata/cte_column_list_pg16.jsonl
// and what PostgreSQL 16.15 did with it: a syntax error at a 0-based byte
// position, or none when it parsed.
type cteColumnListCase struct {
	SQL         string `json:"sql"`
	SyntaxError string `json:"syntaxError"`
	Position    int    `json:"position"`
}

// TestCTEColumnListsArePostgreSQL16s pins PostgreSQL 16's answers for 496
// common table expressions: column lists with and without parentheses,
// empty, with a trailing comma, a dotted name or a second list, after plain,
// quoted and keyword names, with and without RECURSIVE and MATERIALIZED, as
// a later CTE, in a subquery, a view, an INSERT and EXPLAIN, before SEARCH
// and CYCLE, and without AS or a statement after. A CTE's column names are
// in parentheses.
func TestCTEColumnListsArePostgreSQL16s(t *testing.T) {
	f, err := os.Open("testdata/cte_column_list_pg16.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var cases []cteColumnListCase
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var c cteColumnListCase
		if err := json.Unmarshal(sc.Bytes(), &c); err != nil {
			t.Fatal(err)
		}
		cases = append(cases, c)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if len(cases) != 496 {
		t.Fatalf("%d cases, want 496", len(cases))
	}
	for _, c := range cases {
		_, err := Parse(c.SQL)
		if c.SyntaxError == "" {
			if err != nil {
				t.Errorf("Parse(%q) = %v, want it parsed", c.SQL, err)
			}
			continue
		}
		var pe *ParseError
		if !errors.As(err, &pe) || pe.Code != codeSyntaxError || pe.Message != c.SyntaxError || pe.Position != c.Position {
			t.Errorf("Parse(%q) = %v, want 42601 %s at %d", c.SQL, err, c.SyntaxError, c.Position)
		}
	}
}
