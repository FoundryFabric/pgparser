package parser

import (
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// TestColumnOptionsOrderIsPostgreSQL16s pins PostgreSQL 16's answers for
// 20 column definitions with STORAGE, COMPRESSION and OPTIONS before and
// after the constraints and each other, DEFAULT for either, and a domain's,
// which PostgreSQL 16 refuses.
func TestColumnOptionsOrderIsPostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/column_options_order_pg16.jsonl", 20)
}

// TestColumnOptionsAreTheColumns requires a column's STORAGE, COMPRESSION
// and OPTIONS to be its ColumnDef's, as PostgreSQL 16's columnDef has them.
func TestColumnOptionsAreTheColumns(t *testing.T) {
	list, err := Parse("create foreign table t (a text storage default compression pglz options (x 'y') not null) server s")
	if err != nil {
		t.Fatal(err)
	}
	c := list.Items[0].(*nodes.CreateForeignTableStmt).Base.TableElts.Items[0].(*nodes.ColumnDef)
	if c.StorageName != "default" || c.Compression != "pglz" || len(c.Fdwoptions.Elems()) != 1 || len(c.Constraints.Elems()) != 1 {
		t.Errorf("storage %q, compression %q, %d options, %d constraints; want default, pglz, 1 and 1", c.StorageName, c.Compression, len(c.Fdwoptions.Elems()), len(c.Constraints.Elems()))
	}
}
