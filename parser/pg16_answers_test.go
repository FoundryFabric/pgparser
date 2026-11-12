package parser

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"testing"
)

// pg16Answer is one statement of a testdata/*_pg16.jsonl file and what
// PostgreSQL 16.15 did with it: an error with its SQLSTATE, message and
// 0-based byte position (-1 where PostgreSQL gave none), or no code when it
// parsed.
type pg16Answer struct {
	SQL      string `json:"sql"`
	Code     string `json:"code"`
	Message  string `json:"message"`
	Position int    `json:"position"`
}

// checkPG16Answers parses each of the want statements of path and fails for
// each answer the parser does not give.
func checkPG16Answers(t *testing.T, path string, want int) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var cases []pg16Answer
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var c pg16Answer
		if err := json.Unmarshal(sc.Bytes(), &c); err != nil {
			t.Fatal(err)
		}
		cases = append(cases, c)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if len(cases) != want {
		t.Fatalf("%s: %d cases, want %d", path, len(cases), want)
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
