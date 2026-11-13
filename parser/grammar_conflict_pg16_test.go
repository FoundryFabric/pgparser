package parser

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// grammarConflictCase is one statement of a testdata/grammar_conflict_*_pg16.jsonl
// file and what PostgreSQL 16.15 did with it: an error with its SQLSTATE,
// message and 0-based byte position (-1 where PostgreSQL gave none), or no
// code when it parsed.
type grammarConflictCase struct {
	SQL      string `json:"sql"`
	Code     string `json:"code"`
	Message  string `json:"message"`
	Position int    `json:"position"`
}

// TestGrammarConflictsArePostgreSQL16s pins PostgreSQL 16's answers for the
// statements that reach a state where gram.y had a conflict, one file for
// each conflict's cause, with the forms around it: what the parser decides
// there must be what PostgreSQL 16 decides.
func TestGrammarConflictsArePostgreSQL16s(t *testing.T) {
	paths, err := filepath.Glob("testdata/grammar_conflict_*_pg16.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("no testdata/grammar_conflict_*_pg16.jsonl files")
	}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			for _, c := range readGrammarConflictCases(t, path) {
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
		})
	}
}

func readGrammarConflictCases(t *testing.T, path string) []grammarConflictCase {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var cases []grammarConflictCase
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var c grammarConflictCase
		if err := json.Unmarshal(sc.Bytes(), &c); err != nil {
			t.Fatal(err)
		}
		cases = append(cases, c)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if len(cases) == 0 {
		t.Fatalf("%s has no cases", path)
	}
	return cases
}
