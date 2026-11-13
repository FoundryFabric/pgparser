package parser

import (
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// TestJSONArrayIsPostgreSQL16s pins PostgreSQL 16's answers for 42
// statements with JSON_ARRAY over values, a query and nothing, with and
// without a NULL clause, FORMAT JSON and an encoding, and RETURNING, and
// JSON_ARRAYAGG, JSON_OBJECT and JSON_OBJECTAGG beside them.
func TestJSONArrayIsPostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/json_array_pg16.jsonl", 42)
}

// TestJSONArrayLeavesOutNulls requires JSON_ARRAY's default to be ABSENT ON
// NULL whatever its arguments, and a query's FORMAT JSON to keep its
// encoding, as libpg_query's tree has it.
func TestJSONArrayLeavesOutNulls(t *testing.T) {
	for _, c := range []struct {
		sql    string
		absent bool
	}{
		{"select json_array()", true},
		{"select json_array(returning text)", true},
		{"select json_array(1, null)", true},
		{"select json_array(1, null null on null)", false},
		{"select json_array(select 1)", true},
	} {
		list, err := parse16(c.sql)
		if err != nil {
			t.Fatal(err)
		}
		var absent bool
		switch n := list.Items[0].(*nodes.SelectStmt).TargetList.Items[0].(*nodes.ResTarget).Val.(type) {
		case *nodes.JsonArrayConstructor:
			absent = n.AbsentOnNull
		case *nodes.JsonArrayQueryConstructor:
			absent = n.AbsentOnNull
		}
		if absent != c.absent {
			t.Errorf("%s: absent on null %v, want %v", c.sql, absent, c.absent)
		}
	}
	for sql, want := range map[string]nodes.JsonEncoding{
		"select json_array(select 1 format json)":                nodes.JS_ENC_DEFAULT,
		"select json_array(select 1 format json encoding utf8)":  nodes.JS_ENC_UTF8,
		"select json_array(select 1 format json encoding utf16)": nodes.JS_ENC_UTF16,
		"select json_array(select 1 format json encoding utf32)": nodes.JS_ENC_UTF32,
	} {
		list, err := parse16(sql)
		if err != nil {
			t.Fatal(err)
		}
		f := list.Items[0].(*nodes.SelectStmt).TargetList.Items[0].(*nodes.ResTarget).Val.(*nodes.JsonArrayQueryConstructor).Format
		if f == nil || f.FormatType != nodes.JS_FORMAT_JSON || f.Encoding != want {
			t.Errorf("%s: format %+v, want FORMAT JSON encoding %d", sql, f, want)
		}
	}
}
