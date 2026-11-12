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

// roleSpecCase is one statement of testdata/role_spec_pg16.jsonl and what
// PostgreSQL 16.15 did with it: an error with its SQLSTATE, message and
// 0-based byte position, or no code when it parsed. Roletypes are then the
// RoleSpec kinds of its tree in the order a walk of its fields meets them,
// as libpg_query, PostgreSQL 16's parser built as a library, gave them.
type roleSpecCase struct {
	SQL       string   `json:"sql"`
	Code      string   `json:"code"`
	Message   string   `json:"message"`
	Position  int      `json:"position"`
	Roletypes []string `json:"roletypes"`
}

// roleSpecTypeNames are libpg_query's names for nodes.RoleSpecType values.
var roleSpecTypeNames = [...]string{
	nodes.ROLESPEC_CSTRING:      "ROLESPEC_CSTRING",
	nodes.ROLESPEC_CURRENT_ROLE: "ROLESPEC_CURRENT_ROLE",
	nodes.ROLESPEC_CURRENT_USER: "ROLESPEC_CURRENT_USER",
	nodes.ROLESPEC_SESSION_USER: "ROLESPEC_SESSION_USER",
	nodes.ROLESPEC_PUBLIC:       "ROLESPEC_PUBLIC",
}

// roleSpecTypes appends the kinds of the RoleSpecs under v, in field order.
func roleSpecTypes(v reflect.Value, out []string) []string {
	switch v.Kind() {
	case reflect.Interface, reflect.Pointer:
		if v.IsNil() {
			return out
		}
		if rs, ok := v.Interface().(*nodes.RoleSpec); ok {
			return append(out, roleSpecTypeNames[rs.Roletype])
		}
		return roleSpecTypes(v.Elem(), out)
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).IsExported() {
				out = roleSpecTypes(v.Field(i), out)
			}
		}
	case reflect.Slice:
		for i := 0; i < v.Len(); i++ {
			out = roleSpecTypes(v.Index(i), out)
		}
	}
	return out
}

// TestRoleSpecsArePostgreSQL16s pins PostgreSQL 16's answers for 1,555
// statements with a role in each place its grammar takes one (GRANT and
// REVOKE, GRANTED BY, ALTER DEFAULT PRIVILEGES, OWNER TO, CREATE SCHEMA
// AUTHORIZATION, user mappings, policies, DROP and REASSIGN OWNED, CREATE,
// ALTER, RENAME and DROP ROLE, SET ROLE), each written as public, "public",
// PUBLIC, none, "none", current_user and the like. Where PostgreSQL parses
// it, the tree's RoleSpecs must have libpg_query's kinds: public in any case,
// quoted or not, is PUBLIC.
func TestRoleSpecsArePostgreSQL16s(t *testing.T) {
	f, err := os.Open("testdata/role_spec_pg16.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var cases []roleSpecCase
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var c roleSpecCase
		if err := json.Unmarshal(sc.Bytes(), &c); err != nil {
			t.Fatal(err)
		}
		cases = append(cases, c)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if len(cases) != 1555 {
		t.Fatalf("%d cases, want 1555", len(cases))
	}
	for _, c := range cases {
		tree, err := Parse(c.SQL)
		if c.Code == "" {
			if err != nil {
				t.Errorf("Parse(%q) = %v, want it parsed", c.SQL, err)
			} else if got := roleSpecTypes(reflect.ValueOf(tree), nil); !slices.Equal(got, c.Roletypes) {
				t.Errorf("Parse(%q) has RoleSpecs %v, want %v", c.SQL, got, c.Roletypes)
			}
			continue
		}
		var pe *ParseError
		if !errors.As(err, &pe) || pe.Code != c.Code || pe.Message != c.Message || pe.Position != c.Position {
			t.Errorf("Parse(%q) = %v, want %s %s at %d", c.SQL, err, c.Code, c.Message, c.Position)
		}
	}
}
