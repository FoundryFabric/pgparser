package parser

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"testing"
)

// selectClauseCase is one statement of testdata/select_clause_pg16.jsonl and
// what PostgreSQL 16.15 did with it: an error with its SQLSTATE, message and
// 0-based byte position (-1 where PostgreSQL gave none), or no code when it
// parsed.
type selectClauseCase struct {
	SQL      string `json:"sql"`
	Code     string `json:"code"`
	Message  string `json:"message"`
	Position int    `json:"position"`
}

// TestSelectClausesArePostgreSQL16s pins PostgreSQL 16's answers for 1,632
// statements whose SELECT clauses or WITHIN GROUP call its grammar accepts
// but an action may refuse: FETCH ... WITH TIES with and without ORDER BY and
// with each locking clause, a parenthesized select followed by a second
// ORDER BY, OFFSET, LIMIT or WITH, a limit after WITH TIES, and a WITHIN
// GROUP call whose arguments have ORDER BY, DISTINCT or VARIADIC, in the
// contexts a select can take, with the forms around each that PostgreSQL
// accepts.
func TestSelectClausesArePostgreSQL16s(t *testing.T) {
	f, err := os.Open("testdata/select_clause_pg16.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var cases []selectClauseCase
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var c selectClauseCase
		if err := json.Unmarshal(sc.Bytes(), &c); err != nil {
			t.Fatal(err)
		}
		cases = append(cases, c)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if len(cases) != 1632 {
		t.Fatalf("%d cases, want 1632", len(cases))
	}
	for _, c := range cases {
		_, err := parse16(c.SQL)
		if c.Code == "" {
			if err != nil {
				t.Errorf("Parse(%q) = %v, want it parsed", c.SQL, err)
			}
			continue
		}
		var pe *ParseError
		if !errors.As(err, &pe) || pe.Code != c.Code || pe.Message != c.Message || pe.Position != c.Position {
			t.Errorf("Parse(%q) = %v, want %s %s at %d", c.SQL, err, c.Code, c.Message, c.Position)
		}
	}
}
