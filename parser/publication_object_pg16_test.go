package parser

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"testing"
)

// publicationObjectCase is one statement of
// testdata/publication_object_pg16.jsonl and what PostgreSQL 16.15 did with
// it: a syntax error at a 0-based byte position, or none when it parsed.
type publicationObjectCase struct {
	SQL         string `json:"sql"`
	SyntaxError string `json:"syntaxError"`
	Position    int    `json:"position"`
}

// TestPublicationObjectsArePostgreSQL16s pins PostgreSQL 16's answers for 869
// object lists of CREATE PUBLICATION and ALTER PUBLICATION ADD, SET and DROP:
// TABLE, TABLES IN SCHEMA and CURRENT_SCHEMA objects, and objects that take
// the kind before them, as bare, qualified, ONLY and '*' names, with column
// lists and WHERE clauses, first and after each kind, and followed by WITH,
// a stray token or nothing. The first object names its kind; a table is not
// CURRENT_SCHEMA, and a schema is a bare name with neither a column list nor
// a WHERE clause.
func TestPublicationObjectsArePostgreSQL16s(t *testing.T) {
	f, err := os.Open("testdata/publication_object_pg16.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var cases []publicationObjectCase
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var c publicationObjectCase
		if err := json.Unmarshal(sc.Bytes(), &c); err != nil {
			t.Fatal(err)
		}
		cases = append(cases, c)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if len(cases) != 869 {
		t.Fatalf("%d cases, want 869", len(cases))
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
