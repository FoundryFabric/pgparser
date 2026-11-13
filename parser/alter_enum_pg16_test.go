package parser

import (
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// TestAlterEnumIsPostgreSQL16s pins PostgreSQL 16's answers for 12 ALTER
// TYPE ... ADD VALUE and RENAME VALUE statements, with and without BEFORE
// and AFTER, and malformed ones.
func TestAlterEnumIsPostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/alter_enum_pg16.jsonl", 12)
}

// TestAddValueIsAfterByDefault requires ADD VALUE without BEFORE or AFTER
// to set NewvalIsAfter, as PostgreSQL 16's grammar does, and BEFORE to
// clear it.
func TestAddValueIsAfterByDefault(t *testing.T) {
	for sql, want := range map[string]bool{
		"alter type e add value 'x'":            true,
		"alter type e add value 'x' after 'y'":  true,
		"alter type e add value 'x' before 'y'": false,
		"alter type e rename value 'x' to 'y'":  false,
	} {
		list, err := Parse(sql)
		if err != nil {
			t.Fatal(err)
		}
		if got := list.Items[0].(*nodes.AlterEnumStmt).NewvalIsAfter; got != want {
			t.Errorf("%s: NewvalIsAfter %v, want %v", sql, got, want)
		}
	}
}
