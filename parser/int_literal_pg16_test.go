package parser

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"slices"
	"sort"
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// TestIntLiteralIsPostgreSQL16s pins PostgreSQL 16's answers for 249
// statements holding integer literals at and past int32's bounds, in each
// radix, with underscores and minus signs, wherever the grammar takes a
// number: an Iconst or SignedIconst (FETCH counts, typmods of
// time and character types, array bounds, statistics targets, CONNECTION
// LIMIT of a role, operator class strategy numbers, MODULUS) refuses one
// past int32, while a NumericOnly or an expression takes it as a Float.
func TestIntLiteralIsPostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/int_literal_pg16.jsonl", 249)
}

// TestIntLiteralTreesAreLibpgQuerys pins the numeric constants of the
// statements of testdata/int_literal_pg16.jsonl that parse to libpg_query's,
// PostgreSQL 16's parser built as a library: an integer literal whose value
// fits int32 is an Integer, and any other is a Float holding the literal's
// text, radix prefix and underscores kept, with a minus sign before it when
// negated; a decimal keeps its underscores too.
func TestIntLiteralTreesAreLibpgQuerys(t *testing.T) {
	f, err := os.Open("testdata/int_literal_pg16.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<16), 1<<20)
	n := 0
	for sc.Scan() {
		var c struct {
			SQL     string   `json:"sql"`
			Numbers []string `json:"numbers"`
		}
		if err := json.Unmarshal(sc.Bytes(), &c); err != nil {
			t.Fatal(err)
		}
		if c.Numbers == nil {
			continue
		}
		n++
		tree, err := parse16(c.SQL)
		if err != nil {
			t.Errorf("Parse(%q): %v", c.SQL, err)
			continue
		}
		if got := numbersOf(tree); !slices.Equal(got, c.Numbers) {
			t.Errorf("%q: numbers %q, libpg_query's are %q", c.SQL, got, c.Numbers)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if n != 225 {
		t.Fatalf("%d statements with numbers, want 225", n)
	}
}

// numbersOf lists the Integer and Float values of a tree, sorted, in the
// form the test data records libpg_query's: "A_Const.Integer 5@7" for a
// constant at its location, "DefElem.Float 1e10" for a value held by
// another node.
func numbersOf(root nodes.Node) []string {
	out := []string{}
	value := func(n nodes.Node) string {
		switch v := n.(type) {
		case *nodes.Integer:
			return fmt.Sprintf("Integer %d", v.Ival)
		case *nodes.Float:
			return "Float " + v.Fval
		}
		return ""
	}
	var walk func(v reflect.Value, owner string)
	walk = func(v reflect.Value, owner string) {
		switch v.Kind() { //nolint:exhaustive // only the kinds that can carry a Node
		case reflect.Ptr, reflect.Interface:
			if v.IsNil() {
				return
			}
			switch n := v.Interface().(type) {
			case *nodes.A_Const:
				if s := value(n.Val); s != "" {
					out = append(out, fmt.Sprintf("A_Const.%s@%d", s, n.Location))
				}
				return
			case *nodes.Integer, *nodes.Float:
				out = append(out, owner+"."+value(n.(nodes.Node)))
				return
			}
			walk(v.Elem(), owner)
		case reflect.Struct:
			if name := v.Type().Name(); name != "List" {
				owner = name
			}
			for i := 0; i < v.NumField(); i++ {
				walk(v.Field(i), owner)
			}
		case reflect.Slice:
			for i := 0; i < v.Len(); i++ {
				walk(v.Index(i), owner)
			}
		}
	}
	walk(reflect.ValueOf(root), "")
	sort.Strings(out)
	return out
}
