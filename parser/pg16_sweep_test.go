package parser_test

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/pgplex/pgparser/parser"
	"github.com/pgplex/pgparser/parser/pgregress"
)

// sweepCase is one statement of testdata/pg16_sweep.jsonl and PostgreSQL
// 16.15's parser's verdict on it. The statement is a regression statement
// (File and Stmt, its index in pgregress.ExtractStatements, with Sum the
// fnv-1a hash of its text, which the test checks so that a change to the
// extraction is seen), that statement with Edit applied (at, delete, insert:
// a mutation, whose statement has a row of its own), or SQL itself (a hand-written probe or a grammar
// fixture). Code, Msg and Pos are PostgreSQL's error, Pos a 0-based byte
// offset or -1 where PostgreSQL gave none; no Code means PostgreSQL's parser
// accepted it. Class names the cause of a known disagreement, as
// testdata/pg16_sweep_triage.md lists them. PG17 names the PostgreSQL 17
// grammar, such as pg17-sqljson, by which a PostgreSQL 17 parser answers
// the statement unlike PostgreSQL 16.
type sweepCase struct {
	SQL   string `json:"sql"`
	File  string `json:"file"`
	Stmt  int    `json:"stmt"`
	Sum   string `json:"sum"`
	Edit  []any  `json:"edit"`
	Code  string `json:"code"`
	Msg   string `json:"msg"`
	Pos   *int   `json:"pos"`
	Class string `json:"class"`
	PG17  string `json:"pg17"`
}

// TestSweepIsPostgreSQL16s pins PostgreSQL 16's parser's verdicts on the
// regression statements, on mutations of them (a token deleted, duplicated
// or swapped with the next; '.', '[1]', '[1:2]', '.*', '*', '(', ')' or ','
// inserted; a dotted part, a type modifier or '[]' added; a word replaced by
// a reserved keyword) and on hand-written probes, as tools/pg16sweep
// recorded them by sending each statement after "select 1/0;" to
// PostgreSQL 16.15. A PostgreSQL 16 parser must accept what PostgreSQL accepts and
// refuse the rest with PostgreSQL's SQLSTATE, message and position. A case
// with a class is a known disagreement: the class is skipped while the
// parser still disagrees on every case in it, and a case that now agrees
// fails so that its class tag is removed.
func TestSweepIsPostgreSQL16s(t *testing.T) {
	cases := readSweep(t)
	if len(cases) != 58864 {
		t.Fatalf("%d cases, want 58864", len(cases))
	}
	p, err := parser.New(parser.PostgreSQL16)
	if err != nil {
		t.Fatal(err)
	}
	stmts := map[string][]pgregress.ExtractedStmt{}
	type outcome struct{ agree, disagree []string }
	byClass := map[string]*outcome{}
	for i, c := range cases {
		sql, err := sweepSQL(c, stmts)
		if err != nil {
			t.Fatalf("case %d: %v", i+1, err)
		}
		o := byClass[c.Class]
		if o == nil {
			o = &outcome{}
			byClass[c.Class] = o
		}
		label := fmt.Sprintf("case %d (%s:%d) %q", i+1, c.File, c.Stmt, sql)
		if problem := sweepDisagreement(p, c, sql); problem != "" {
			o.disagree = append(o.disagree, label+": "+problem)
		} else {
			o.agree = append(o.agree, label)
		}
	}
	classes := make([]string, 0, len(byClass))
	for c := range byClass {
		classes = append(classes, c)
	}
	sort.Strings(classes)
	for _, class := range classes {
		o := byClass[class]
		name := class
		if name == "" {
			name = "agreeing"
		}
		t.Run(name, func(t *testing.T) {
			if class == "" {
				for i, d := range o.disagree {
					if i == 20 {
						t.Fatalf("and %d more", len(o.disagree)-20)
					}
					t.Error(d)
				}
				return
			}
			for i, a := range o.agree {
				if i == 20 {
					t.Fatalf("and %d more", len(o.agree)-20)
				}
				t.Errorf("%s now agrees with PostgreSQL 16: drop its class %q", a, class)
			}
			if len(o.agree) == 0 {
				t.Skipf("known divergence from PostgreSQL 16: %d statements", len(o.disagree))
			}
		})
	}
}

// TestSweepAsPostgreSQL17 parses the sweep as PostgreSQL 17, the version
// Parse answers as: a case with PG17, or with a pg17 class, is answered
// unlike PostgreSQL 16, by PostgreSQL 17's grammar, and every other case
// without a class as PostgreSQL 16 answers it.
func TestSweepAsPostgreSQL17(t *testing.T) {
	cases := readSweep(t)
	p, err := parser.New(parser.PostgreSQL17)
	if err != nil {
		t.Fatal(err)
	}
	stmts := map[string][]pgregress.ExtractedStmt{}
	byPG17 := map[string]int{}
	failures := 0
	for i, c := range cases {
		pg17 := c.PG17
		if strings.HasPrefix(c.Class, "pg17-") {
			pg17 = c.Class
		} else if c.Class != "" {
			continue
		}
		sql, err := sweepSQL(c, stmts)
		if err != nil {
			t.Fatalf("case %d: %v", i+1, err)
		}
		problem := sweepDisagreement(p, c, sql)
		switch {
		case pg17 != "" && problem == "":
			t.Errorf("case %d (%s:%d) %q: as PostgreSQL 17 answers as PostgreSQL 16, want PostgreSQL 17's %s", i+1, c.File, c.Stmt, sql, pg17)
			failures++
		case pg17 == "" && problem != "":
			t.Errorf("case %d (%s:%d) %q: as PostgreSQL 17: %s", i+1, c.File, c.Stmt, sql, problem)
			failures++
		}
		if failures == 20 {
			t.Fatal("and more")
		}
		byPG17[pg17]++
	}
	if byPG17["pg17-sqljson"] != 714 || byPG17["pg17-json-names"] != 75 || byPG17["pg17-merge"] != 58 || byPG17["pg17-ddl"] != 42 {
		t.Errorf("PostgreSQL 17 cases %v, want 714 pg17-sqljson, 75 pg17-json-names, 58 pg17-merge and 42 pg17-ddl", byPG17)
	}
}

func readSweep(t *testing.T) []sweepCase {
	t.Helper()
	f, err := os.Open("testdata/pg16_sweep.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var cases []sweepCase
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var c sweepCase
		if err := json.Unmarshal(sc.Bytes(), &c); err != nil {
			t.Fatal(err)
		}
		cases = append(cases, c)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return cases
}

// sweepSQL is the statement c stands for. Regression files are extracted
// once, into stmts.
func sweepSQL(c sweepCase, stmts map[string][]pgregress.ExtractedStmt) (string, error) {
	if c.File == "" {
		return c.SQL, nil
	}
	list, ok := stmts[c.File]
	if !ok {
		content, err := os.ReadFile(filepath.Join("pgregress", "testdata", "sql", c.File))
		if err != nil {
			return "", err
		}
		list = pgregress.ExtractStatements(c.File, content)
		stmts[c.File] = list
	}
	if c.Stmt >= len(list) {
		return "", fmt.Errorf("%s has %d statements, want statement %d", c.File, len(list), c.Stmt)
	}
	sql := list[c.Stmt].SQL
	h := fnv.New32a()
	h.Write([]byte(sql))
	if sum := fmt.Sprintf("%08x", h.Sum32()); c.Edit == nil && sum != c.Sum {
		return "", fmt.Errorf("%s statement %d hashes to %s, want %s: the extraction changed", c.File, c.Stmt, sum, c.Sum)
	}
	if c.Edit == nil {
		return sql, nil
	}
	if len(c.Edit) != 3 {
		return "", fmt.Errorf("%s statement %d: bad edit %v", c.File, c.Stmt, c.Edit)
	}
	at, okAt := c.Edit[0].(float64)
	del, okDel := c.Edit[1].(float64)
	ins, okIns := c.Edit[2].(string)
	if !okAt || !okDel || !okIns || int(at)+int(del) > len(sql) {
		return "", fmt.Errorf("%s statement %d: bad edit %v", c.File, c.Stmt, c.Edit)
	}
	return sql[:int(at)] + ins + sql[int(at)+int(del):], nil
}

// sweepDisagreement is how p.Parse(sql) differs from PostgreSQL 16's
// verdict in c, or "" when it agrees.
func sweepDisagreement(p *parser.Parser, c sweepCase, sql string) (problem string) {
	defer func() {
		if r := recover(); r != nil {
			problem = fmt.Sprint("Parse panicked: ", r)
		}
	}()
	_, err := p.Parse(sql)
	if c.Code == "" {
		if err != nil {
			return "got " + err.Error() + ", want it parsed"
		}
		return ""
	}
	want := fmt.Sprintf("%s %s", c.Code, c.Msg)
	if c.Pos != nil && *c.Pos >= 0 {
		want += fmt.Sprintf(" at %d", *c.Pos)
	}
	var pe *parser.ParseError
	if !errors.As(err, &pe) {
		if err == nil {
			return "got no error, want " + want
		}
		return "got " + err.Error() + ", want " + want
	}
	if pe.Code != c.Code || pe.Message != c.Msg || c.Pos != nil && *c.Pos >= 0 && pe.Position != *c.Pos {
		return fmt.Sprintf("got %s %s at %d, want %s", pe.Code, pe.Message, pe.Position, want)
	}
	return ""
}
