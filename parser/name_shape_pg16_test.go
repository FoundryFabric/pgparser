package parser

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// nameShapeCase is one statement of testdata/name_shape_pg16.jsonl and what
// PostgreSQL 16.15 did with it. A statement PostgreSQL refuses at parse time
// has its syntax error and 0-based byte position. Any other statement it
// parses, and pg16After is what analysis then said. Where that named the
// object (improper qualified name, cross-database reference, relation does
// not exist and the like), pg16Name holds the name, and the parse tree must
// carry it: a RangeVar, or a list of names, whose parts join to it or begin
// with it.
type nameShapeCase struct {
	SQL         string `json:"sql"`
	SyntaxError string `json:"syntaxError"`
	Position    int    `json:"position"`
	Divergence  string `json:"divergence"`
	PG16After   string `json:"pg16After"`
	PG16Name    string `json:"pg16Name"`
}

// TestNameShapesArePostgreSQL16s pins PostgreSQL 16's answers for 448
// statements whose names have one to seven parts, subscripts, slices and '*'
// in every place: column references, function calls, typed literals, casts,
// parenthesised, parameter and subquery indirection, INSERT and UPDATE
// targets, FROM, DML and utility relations, CREATE, ALTER, DROP, COMMENT,
// GRANT, operator, handler and collation names. PostgreSQL's grammar refuses
// a subscript or '*' in a function or relation name, a '*' before the end of
// an indirection and a relation name of more than three parts. A case with a
// divergence is a known disagreement: it is skipped while the parser still
// disagrees, and fails once it agrees so that the note is removed.
func TestNameShapesArePostgreSQL16s(t *testing.T) {
	f, err := os.Open("testdata/name_shape_pg16.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var cases []nameShapeCase
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var c nameShapeCase
		if err := json.Unmarshal(sc.Bytes(), &c); err != nil {
			t.Fatal(err)
		}
		cases = append(cases, c)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if len(cases) != 448 {
		t.Fatalf("%d cases, want 448", len(cases))
	}
	for _, c := range cases {
		t.Run(c.SQL, func(t *testing.T) {
			problem := nameShapeDisagreement(c)
			switch {
			case c.Divergence != "" && problem != "":
				t.Skip("known divergence from PostgreSQL 16: " + c.Divergence + "; " + problem)
			case c.Divergence != "":
				t.Errorf("Parse(%q) now agrees with PostgreSQL 16: drop its divergence note", c.SQL)
			case problem != "":
				t.Errorf("Parse(%q): %s", c.SQL, problem)
			}
		})
	}
}

// nameShapeDisagreement is how Parse(c.SQL) differs from PostgreSQL 16's
// answer in c, or "" when it agrees.
func nameShapeDisagreement(c nameShapeCase) string {
	stmts, err := parse16(c.SQL)
	got := "no error"
	if err != nil {
		got = err.Error()
	}
	if c.SyntaxError != "" {
		var pe *ParseError
		if !errors.As(err, &pe) || pe.Code != codeSyntaxError || pe.Message != c.SyntaxError || pe.Position != c.Position {
			if pe != nil {
				got += " at " + strconv.Itoa(pe.Position)
			}
			return "got " + got + ", want 42601 " + c.SyntaxError + " at " + strconv.Itoa(c.Position)
		}
		return ""
	}
	if err != nil {
		return "got " + got + ", want it parsed (PostgreSQL 16 then said " + c.PG16After + ")"
	}
	if c.PG16Name == "" {
		return ""
	}
	names := treeNames(stmts)
	for _, name := range names {
		if name == c.PG16Name || strings.HasPrefix(name, c.PG16Name+".") {
			return ""
		}
	}
	return "no name in the tree is PostgreSQL 16's " + c.PG16Name + ": " + strings.Join(names, ", ")
}

// treeNames is every RangeVar and every list of names (String or A_Star
// members) in the tree, each with its parts joined by dots.
func treeNames(root nodes.Node) []string {
	var res []string
	var walk func(v reflect.Value)
	walk = func(v reflect.Value) {
		switch v.Kind() { //nolint:exhaustive
		case reflect.Ptr, reflect.Interface:
			if v.IsNil() {
				return
			}
			switch n := v.Interface().(type) {
			case *nodes.RangeVar:
				var parts []string
				for _, p := range []string{n.Catalogname, n.Schemaname, n.Relname} {
					if p != "" {
						parts = append(parts, p)
					}
				}
				res = append(res, strings.Join(parts, "."))
				return
			case *nodes.List:
				var parts []string
				for _, item := range n.Items {
					switch x := item.(type) {
					case *nodes.String:
						parts = append(parts, x.Str)
					case *nodes.A_Star:
						parts = append(parts, "*")
					}
				}
				if len(parts) > 0 && len(parts) == len(n.Items) {
					res = append(res, strings.Join(parts, "."))
					return
				}
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
