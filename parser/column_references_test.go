package parser

import (
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// TestParseColumnReferencesAttributesAreTheirOwn pins the shape of a column
// REFERENCES followed by DEFERRABLE, NOT DEFERRABLE or INITIALLY: each
// attribute is a constraint of its own in the column's list, after the
// foreign key, as PostgreSQL 16's ColQualList builds it, and a constraint
// may follow them.
func TestParseColumnReferencesAttributesAreTheirOwn(t *testing.T) {
	for _, c := range []struct {
		sql   string
		types []nodes.ConstrType
	}{
		{"create table t (a int references x)", []nodes.ConstrType{nodes.CONSTR_FOREIGN}},
		{"create table t (a int references x deferrable initially deferred)",
			[]nodes.ConstrType{nodes.CONSTR_FOREIGN, nodes.CONSTR_ATTR_DEFERRABLE, nodes.CONSTR_ATTR_DEFERRED}},
		{"create table t (a int references x on delete cascade not deferrable not null)",
			[]nodes.ConstrType{nodes.CONSTR_FOREIGN, nodes.CONSTR_ATTR_NOT_DEFERRABLE, nodes.CONSTR_NOTNULL}},
		{"create table t (a int references x initially immediate references y)",
			[]nodes.ConstrType{nodes.CONSTR_FOREIGN, nodes.CONSTR_ATTR_IMMEDIATE, nodes.CONSTR_FOREIGN}},
	} {
		stmts, err := Parse(c.sql)
		if err != nil {
			t.Fatalf("Parse(%q): %v", c.sql, err)
		}
		col := stmts.Items[0].(*nodes.CreateStmt).TableElts.Items[0].(*nodes.ColumnDef)
		var got []nodes.ConstrType
		for _, n := range col.Constraints.Elems() {
			got = append(got, n.(*nodes.Constraint).Contype)
		}
		if len(got) != len(c.types) {
			t.Errorf("Parse(%q): constraints %v, want %v", c.sql, got, c.types)
			continue
		}
		for i := range got {
			if got[i] != c.types[i] {
				t.Errorf("Parse(%q): constraints %v, want %v", c.sql, got, c.types)
				break
			}
		}
		fk := col.Constraints.Items[0].(*nodes.Constraint)
		if fk.Deferrable || fk.Initdeferred {
			t.Errorf("Parse(%q): the foreign key is marked deferrable %v, initially deferred %v; want neither", c.sql, fk.Deferrable, fk.Initdeferred)
		}
	}
}
