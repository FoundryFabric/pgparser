package parser

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// typeNameSiteCase is one statement of testdata/typename_sites_pg16.jsonl:
// a typeNameCase, and for a DROP, COMMENT, SECURITY LABEL or ALTER EXTENSION
// statement PostgreSQL 16 parses, object is the shape of the object it names
// as PostgreSQL 16's grammar builds it ("List(TypeName)" for DROP TYPE a).
type typeNameSiteCase struct {
	typeNameCase
	Object string `json:"object"`
}

// TestTypeNameSitesArePostgreSQL16s pins PostgreSQL 16's answers for 1,018
// statements that name a type: DROP TYPE and DROP DOMAIN, COMMENT ON and
// SECURITY LABEL ON a type or domain, ALTER EXTENSION ADD and DROP of a type,
// domain, cast or transform, casts, transforms, operator classes and
// families, function, aggregate and operator argument types, and the sites
// where PostgreSQL takes a qualified name instead (ALTER TYPE, ALTER DOMAIN,
// GRANT ON TYPE and the like). Each site has plain, qualified, SQL-standard,
// typmod, array, SETOF and %TYPE type names, and malformed ones.
func TestTypeNameSitesArePostgreSQL16s(t *testing.T) {
	f, err := os.Open("testdata/typename_sites_pg16.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var cases []typeNameSiteCase
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var c typeNameSiteCase
		if err := json.Unmarshal(sc.Bytes(), &c); err != nil {
			t.Fatal(err)
		}
		cases = append(cases, c)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if len(cases) != 1018 {
		t.Fatalf("%d cases, want 1018", len(cases))
	}
	for _, c := range cases {
		t.Run(c.SQL, func(t *testing.T) {
			if problem := typeNameDisagreement(c.typeNameCase); problem != "" {
				t.Fatalf("Parse(%q): %s", c.SQL, problem)
			}
			if c.Object == "" {
				return
			}
			stmts, err := parse16(c.SQL)
			if err != nil {
				t.Fatal(err)
			}
			if got := objectShape(stmts.Items[0]); got != c.Object {
				t.Errorf("Parse(%q): object is %s, want %s", c.SQL, got, c.Object)
			}
		})
	}
}

// objectShape is the shape of the object a DROP, COMMENT, SECURITY LABEL or
// ALTER EXTENSION statement names: its node kinds, a list's in brackets.
func objectShape(stmt nodes.Node) string {
	var object nodes.Node
	switch s := stmt.(type) {
	case *nodes.DropStmt:
		object = s.Objects
	case *nodes.CommentStmt:
		object = s.Object
	case *nodes.SecLabelStmt:
		object = s.Object
	case *nodes.AlterExtensionContentsStmt:
		object = s.Object
	default:
		return fmt.Sprintf("%T", stmt)
	}
	var shape func(n nodes.Node) string
	shape = func(n nodes.Node) string {
		l, ok := n.(*nodes.List)
		if !ok {
			return strings.TrimPrefix(fmt.Sprintf("%T", n), "*nodes.")
		}
		parts := make([]string, len(l.Items))
		for i, item := range l.Items {
			parts[i] = shape(item)
		}
		return "List(" + strings.Join(parts, ",") + ")"
	}
	return shape(object)
}
