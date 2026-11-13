package parser

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// pg16Answer is one statement of a testdata/*_pg16.jsonl file and what
// PostgreSQL 16.15 did with it: an error with its SQLSTATE, message and
// 0-based byte position (-1 where PostgreSQL gave none), or no code when it
// parsed. A class, such as pg17-ddl, marks a known difference from
// PostgreSQL 16: PostgreSQL 17 grammar that PostgreSQL 16 refuses. PG17
// names the PostgreSQL 17 grammar by which a PostgreSQL 17 parser answers
// the statement unlike PostgreSQL 16.
type pg16Answer struct {
	SQL      string `json:"sql"`
	Code     string `json:"code"`
	Message  string `json:"message"`
	Position int    `json:"position"`
	Class    string `json:"class"`
	PG17     string `json:"pg17"`
}

// agrees reports whether err is PostgreSQL 16's answer to c.
func (c pg16Answer) agrees(err error) bool {
	var pe *ParseError
	return c.Code == "" && err == nil || errors.As(err, &pe) && pe.Code == c.Code && pe.Message == c.Message && pe.Position == c.Position
}

// checkPG16Answers parses each of the want statements of path as
// PostgreSQL 16 and fails for each answer the parser does not give, and for
// each statement of a class whose answer it now gives. As PostgreSQL 17, a
// statement with PG17 must be answered otherwise and one without a class
// alike.
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
		_, err := parse16(c.SQL)
		if c.Class != "" {
			if c.agrees(err) {
				t.Errorf("Parse(%q) now gives PostgreSQL 16's answer: drop its class %q", c.SQL, c.Class)
			}
			continue
		}
		if _, err17 := Parse(c.SQL); c.agrees(err17) == (c.PG17 != "") {
			t.Errorf("as PostgreSQL 17, Parse(%q) = %v; PostgreSQL 16's answer is %s %s at %d, and pg17 is %q", c.SQL, err17, c.Code, c.Message, c.Position, c.PG17)
		}
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

// pg16Parser answers as PostgreSQL 16's parser, the oracle the
// testdata/*_pg16.jsonl files were recorded from.
var pg16Parser = &Parser{version: PostgreSQL16}

// parse16 parses sql as PostgreSQL 16 does.
func parse16(sql string) (*nodes.List, error) { return pg16Parser.Parse(sql) }
