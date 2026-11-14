package parser

import (
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// TestTimeZoneIntervalIsPostgreSQL16s pins PostgreSQL 16's answers for 38
// statements setting the time zone to an interval, with and without HOUR,
// HOUR TO MINUTE and a precision, the interval fields PostgreSQL 16 refuses
// there, and the other time zone values beside them.
func TestTimeZoneIntervalIsPostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/time_zone_interval_pg16.jsonl", 38)
}

// TestTimeZoneIntervalIsACast requires SET TIME ZONE INTERVAL to set the
// time zone to the string cast to interval, with the fields or precision as
// the type's modifiers, as PostgreSQL 16's zone_value makes it.
func TestTimeZoneIntervalIsACast(t *testing.T) {
	for _, c := range []struct {
		sql     string
		typmods int
	}{
		{"set time zone interval '1'", 0},
		{"set time zone interval '+1:00' hour to minute", 1},
		{"set time zone interval(3) '1'", 2},
	} {
		list, err := Parse(c.sql)
		if err != nil {
			t.Fatal(err)
		}
		s := list.Items[0].(*nodes.VariableSetStmt)
		if s.Name != "timezone" || s.Kind != nodes.VAR_SET_VALUE || s.Args == nil || len(s.Args.Items) != 1 {
			t.Fatalf("%s: %+v, want one timezone value", c.sql, s)
		}
		tc, ok := s.Args.Items[0].(*nodes.TypeCast)
		if !ok {
			t.Fatalf("%s: value %T, want a TypeCast", c.sql, s.Args.Items[0])
		}
		n := 0
		if tc.TypeName.Typmods != nil {
			n = len(tc.TypeName.Typmods.Items)
		}
		if n != c.typmods {
			t.Errorf("%s: %d type modifiers, want %d", c.sql, n, c.typmods)
		}
	}
}
