package parser

import (
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// TestDomainCollateIsPostgreSQL16s pins PostgreSQL 16's answers for 16
// CREATE DOMAIN, column and WITH OPTIONS or partition column definitions
// with COLLATE beside constraints, one COLLATE too many, which PostgreSQL
// 16 refuses at the second, and a domain's COMPRESSION and STORAGE, which
// it refuses and the parser does not yet (class column-qualifier-order).
func TestDomainCollateIsPostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/domain_collate_pg16.jsonl", 16)
}

// TestDomainCollateIsItsOwn requires CREATE DOMAIN's COLLATE to be its
// CollClause, at the COLLATE, and not one of its constraints, as
// PostgreSQL 16's SplitColQualList splits them.
func TestDomainCollateIsItsOwn(t *testing.T) {
	for sql, want := range map[string][2]int{
		`create domain d as text collate "C"`:                           {24, 0},
		`create domain d text collate "C" not null check (value <> '')`: {21, 2},
	} {
		list, err := Parse(sql)
		if err != nil {
			t.Fatal(err)
		}
		d := list.Items[0].(*nodes.CreateDomainStmt)
		if d.CollClause == nil || int(d.CollClause.Location) != want[0] || len(d.Constraints.Elems()) != want[1] {
			t.Errorf("%s: collate %+v, %d constraints, want collate at %d and %d", sql, d.CollClause, len(d.Constraints.Elems()), want[0], want[1])
		}
	}
}
