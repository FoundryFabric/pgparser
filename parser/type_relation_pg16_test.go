package parser

import (
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// TestTypeRelationIsPostgreSQL16s pins PostgreSQL 16's answers for 15
// CREATE TYPE ... AS, ALTER TYPE and RENAME ATTRIBUTE statements, the
// ALTER TABLE forms beside them, and malformed ones.
func TestTypeRelationIsPostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/type_relation_pg16.jsonl", 15)
}

// TestTypeRelationIsNotInherited requires a composite type's RangeVar to
// have inh false and relpersistence permanent, as PostgreSQL 16's
// makeRangeVarFromAnyName makes it, and a table's to keep inh true.
func TestTypeRelationIsNotInherited(t *testing.T) {
	relation := func(n nodes.Node) *nodes.RangeVar {
		switch s := n.(type) {
		case *nodes.CompositeTypeStmt:
			return s.Typevar
		case *nodes.AlterTableStmt:
			return s.Relation
		}
		return n.(*nodes.RenameStmt).Relation
	}
	for sql, inh := range map[string]bool{
		"create type s.t as (a int)":           false,
		"alter type t drop attribute c":        false,
		"alter type t rename attribute a to b": false,
		"alter table t add column c int":       true,
		"alter table t rename column a to b":   true,
	} {
		list, err := Parse(sql)
		if err != nil {
			t.Fatal(err)
		}
		if rv := relation(list.Items[0]); rv.Inh != inh || rv.Relpersistence != 'p' {
			t.Errorf("%s: inh %v relpersistence %q, want %v 'p'", sql, rv.Inh, rv.Relpersistence, inh)
		}
	}
}
