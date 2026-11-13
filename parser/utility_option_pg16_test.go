package parser

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"testing"
)

// utilityOptionCase is one statement of testdata/utility_option_pg16.jsonl
// and what PostgreSQL 16.15 did with it: a syntax error at a 0-based byte
// position, or none when it parsed.
type utilityOptionCase struct {
	SQL         string `json:"sql"`
	SyntaxError string `json:"syntaxError"`
	Position    int    `json:"position"`
}

// TestUtilityOptionsArePostgreSQL16s pins PostgreSQL 16's answers for 1,984
// parenthesised option lists of EXPLAIN, VACUUM, ANALYZE, CLUSTER, REINDEX
// and COPY: option names that are identifiers and keywords of each kind,
// with no argument, a boolean, string, number, word, keyword, '*' or a
// parenthesised list as the argument, and malformed lists. COPY takes any
// keyword as a name and '*' or a list as an argument; the others take
// neither, nor DEFAULT as a name or an argument.
func TestUtilityOptionsArePostgreSQL16s(t *testing.T) {
	f, err := os.Open("testdata/utility_option_pg16.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var cases []utilityOptionCase
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var c utilityOptionCase
		if err := json.Unmarshal(sc.Bytes(), &c); err != nil {
			t.Fatal(err)
		}
		cases = append(cases, c)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if len(cases) != 1984 {
		t.Fatalf("%d cases, want 1984", len(cases))
	}
	for _, c := range cases {
		_, err := parse16(c.SQL)
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
