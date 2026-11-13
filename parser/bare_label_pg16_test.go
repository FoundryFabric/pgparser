package parser

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"slices"
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// bareLabelCase is one statement of testdata/bare_label_pg16.jsonl and what
// PostgreSQL 16.15 did with it: a syntax error at a 0-based byte position,
// or, when it parsed, the column labels its targets carry, in source order.
type bareLabelCase struct {
	SQL         string   `json:"sql"`
	SyntaxError string   `json:"syntaxError"`
	Position    int      `json:"position"`
	Labels      []string `json:"labels"`
}

// TestBareLabelsArePostgreSQL16s pins PostgreSQL 16's answers for 5,437
// statements: every keyword as a column label without AS after a constant,
// a column, a function call and a cast, in a list, a subquery, with FROM and
// in RETURNING, and with AS; a keyword followed by an identifier and an
// identifier followed by a keyword; and identifier labels and near misses.
// PostgreSQL 16 takes 432 of its 471 keywords as a bare label, and the 20
// keywords it does not have are identifiers to it.
func TestBareLabelsArePostgreSQL16s(t *testing.T) {
	f, err := os.Open("testdata/bare_label_pg16.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var cases []bareLabelCase
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var c bareLabelCase
		if err := json.Unmarshal(sc.Bytes(), &c); err != nil {
			t.Fatal(err)
		}
		cases = append(cases, c)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if len(cases) != 5437 {
		t.Fatalf("%d cases, want 5437", len(cases))
	}
	for _, c := range cases {
		stmts, err := parse16(c.SQL)
		if c.SyntaxError != "" {
			var pe *ParseError
			if !errors.As(err, &pe) || pe.Code != codeSyntaxError || pe.Message != c.SyntaxError || pe.Position != c.Position {
				t.Errorf("Parse(%q) = %v, want 42601 %s at %d", c.SQL, err, c.SyntaxError, c.Position)
			}
			continue
		}
		if err != nil {
			t.Errorf("Parse(%q): %v, want it parsed", c.SQL, err)
			continue
		}
		if got := targetLabels(stmts); !slices.Equal(got, c.Labels) {
			t.Errorf("Parse(%q) labels %q, want %q", c.SQL, got, c.Labels)
		}
	}
}

// TestBareLabelIsTheKeywordTable checks that a keyword is a column label
// without AS exactly when its version's keyword table marks it BareLabel,
// and that a word that is no keyword in a version is a label there.
func TestBareLabelIsTheKeywordTable(t *testing.T) {
	for _, v := range []Version{PostgreSQL16, PostgreSQL17} {
		p := &Parser{version: v}
		for _, kw := range Keywords {
			sql := "select 1 " + kw.Name + ", 2"
			stmts, err := p.Parse(sql)
			labelled := err == nil && slices.Equal(targetLabels(stmts), []string{kw.Name})
			k := LookupKeywordVersion(v, kw.Name)
			if want := k == nil || k.BareLabel; labelled != want {
				t.Errorf("%d: Parse(%q): labelled %v, err %v; want %v", v, sql, labelled, err, want)
			}
		}
	}
}

// targetLabels is the Name of every named ResTarget in the tree, in order.
func targetLabels(root nodes.Node) []string {
	var res []string
	var walk func(v reflect.Value)
	walk = func(v reflect.Value) {
		switch v.Kind() { //nolint:exhaustive
		case reflect.Ptr, reflect.Interface:
			if v.IsNil() {
				return
			}
			if rt, ok := v.Interface().(*nodes.ResTarget); ok && v.Kind() == reflect.Ptr && rt.Name != "" {
				res = append(res, rt.Name)
			}
			walk(v.Elem())
		case reflect.Struct:
			for i := 0; i < v.NumField(); i++ {
				walk(v.Field(i))
			}
		case reflect.Slice:
			for i := 0; i < v.Len(); i++ {
				walk(v.Index(i))
			}
		}
	}
	walk(reflect.ValueOf(root))
	return res
}
