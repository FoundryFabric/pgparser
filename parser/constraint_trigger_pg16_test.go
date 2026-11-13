package parser

import (
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// TestConstraintTriggerIsPostgreSQL16s pins PostgreSQL 16's answers for 25
// CREATE CONSTRAINT TRIGGER and CREATE TRIGGER statements with each
// deferrability attribute, alone, together and contradicting each other,
// with and without OR REPLACE.
func TestConstraintTriggerIsPostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/constraint_trigger_pg16.jsonl", 25)
}

// TestInitiallyDeferredTriggerIsDeferrable requires a constraint trigger
// INITIALLY DEFERRED to be DEFERRABLE, as libpg_query's tree has it.
func TestInitiallyDeferredTriggerIsDeferrable(t *testing.T) {
	for _, c := range []struct {
		attrs                    string
		deferrable, initdeferred bool
	}{
		{"", false, false},
		{"deferrable", true, false},
		{"initially deferred", true, true},
		{"not deferrable initially immediate", false, false},
	} {
		sql := "create constraint trigger t after insert on r " + c.attrs + " for each row execute function f()"
		list, err := Parse(sql)
		if err != nil {
			t.Fatal(err)
		}
		n := list.Items[0].(*nodes.CreateTrigStmt)
		if n.Deferrable != c.deferrable || n.Initdeferred != c.initdeferred {
			t.Errorf("%s: deferrable %v, initially deferred %v; want %v, %v", sql, n.Deferrable, n.Initdeferred, c.deferrable, c.initdeferred)
		}
	}
}
