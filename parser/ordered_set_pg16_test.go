package parser

import (
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// TestOrderedSetArgsArePostgreSQL16s pins PostgreSQL 16's answers for 21
// ordered-set aggregate argument lists: a VARIADIC last direct argument
// with one VARIADIC ordered argument of the same type, which PostgreSQL 16
// takes, and of another type (names, typmods, array bounds, SETOF), more
// than one or not VARIADIC, which it refuses at the ordered argument's
// type, in CREATE, ALTER and DROP AGGREGATE.
func TestOrderedSetArgsArePostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/ordered_set_pg16.jsonl", 21)
}

// TestOrderedSetVariadicIsMerged requires an ordered-set aggregate's
// VARIADIC ordered argument to be dropped after a VARIADIC direct argument
// of its type, as PostgreSQL 16's makeOrderedSetArgs drops it, and kept
// otherwise.
func TestOrderedSetVariadicIsMerged(t *testing.T) {
	for sql, want := range map[string][2]int{
		`create aggregate a(variadic "any" order by variadic "any") (sfunc = f, stype = internal)`:      {1, 1},
		`create aggregate a(int, variadic int[] order by variadic int[]) (sfunc = f, stype = internal)`: {2, 2},
		`create aggregate a(int order by variadic "any") (sfunc = f, stype = internal)`:                 {2, 1},
	} {
		list, err := Parse(sql)
		if err != nil {
			t.Fatal(err)
		}
		args := list.Items[0].(*nodes.DefineStmt).Args
		got := [2]int{len(args.Items[0].(*nodes.List).Items), int(args.Items[1].(*nodes.Integer).Ival)}
		if got != want {
			t.Errorf("%s: %d arguments, %d direct, want %d and %d", sql, got[0], got[1], want[0], want[1])
		}
	}
}
