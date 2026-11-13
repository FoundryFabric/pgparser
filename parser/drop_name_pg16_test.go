package parser

import (
	"reflect"
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// TestDropNameIsPostgreSQL16s pins PostgreSQL 16's answers for 42 DROP,
// COMMENT, SECURITY LABEL and ALTER EXTENSION statements naming schemas,
// extensions, languages, publications, event triggers, foreign data
// wrappers, servers and access methods, and triggers, policies and rules on
// a relation, with qualified names PostgreSQL 16 refuses.
func TestDropNameIsPostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/drop_name_pg16.jsonl", 42)
}

// TestDropNamesAreShapedAsPostgreSQL16s requires DROP's objects to be
// PostgreSQL 16's: a String for each object named by a bare name, and one
// name list ending in the object's name for a trigger, policy or rule on a
// relation.
func TestDropNamesAreShapedAsPostgreSQL16s(t *testing.T) {
	strs := func(n nodes.Node) any {
		if s, ok := n.(*nodes.String); ok {
			return s.Str
		}
		var out []string
		for _, s := range n.(*nodes.List).Items {
			out = append(out, s.(*nodes.String).Str)
		}
		return out
	}
	for sql, want := range map[string][]any{
		"drop schema s, t":               {"s", "t"},
		"drop procedural language l":     {"l"},
		"drop access method a":           {"a"},
		"drop event trigger if exists e": {"e"},
		"drop trigger t on s.r":          {[]string{"s", "r", "t"}},
		"drop policy if exists p on r":   {[]string{"r", "p"}},
		"drop rule r on t":               {[]string{"t", "r"}},
		"drop table t, s.u":              {[]string{"t"}, []string{"s", "u"}},
	} {
		list, err := Parse(sql)
		if err != nil {
			t.Fatal(err)
		}
		var got []any
		for _, o := range list.Items[0].(*nodes.DropStmt).Objects.Items {
			got = append(got, strs(o))
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: objects %v, want %v", sql, got, want)
		}
	}
}
