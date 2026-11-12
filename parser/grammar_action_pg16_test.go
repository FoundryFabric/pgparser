package parser

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"testing"
)

// grammarActionCase is one statement of testdata/grammar_action_pg16.jsonl
// and what PostgreSQL 16.15 did with it: an error with its SQLSTATE, message
// and 0-based byte position (-1 where PostgreSQL gave none), or no code when
// it parsed.
type grammarActionCase struct {
	SQL      string `json:"sql"`
	Code     string `json:"code"`
	Message  string `json:"message"`
	Position int    `json:"position"`
}

// TestGrammarActionsArePostgreSQL16s pins PostgreSQL 16's answers for 1,131
// statements its grammar accepts but a grammar action may refuse: COPY with
// PROGRAM and STDIN/STDOUT or COPY TO with WHERE, a partitioning strategy,
// an index column number in ALTER COLUMN n SET STATISTICS, CREATE SCHEMA IF
// NOT EXISTS with schema elements, repeated trigger events, XMLTABLE column
// options, a JSON encoding and window frame bounds, with the forms around
// each that PostgreSQL accepts.
func TestGrammarActionsArePostgreSQL16s(t *testing.T) {
	f, err := os.Open("testdata/grammar_action_pg16.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var cases []grammarActionCase
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var c grammarActionCase
		if err := json.Unmarshal(sc.Bytes(), &c); err != nil {
			t.Fatal(err)
		}
		cases = append(cases, c)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if len(cases) != 1131 {
		t.Fatalf("%d cases, want 1131", len(cases))
	}
	for _, c := range cases {
		_, err := Parse(c.SQL)
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
