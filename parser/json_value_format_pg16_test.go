package parser

import (
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// TestJSONValueFormatIsPostgreSQL16s pins PostgreSQL 16's answers for 14
// SQL/JSON constructor and aggregate arguments, query forms and RETURNING
// clauses with and without FORMAT JSON and ENCODING, and malformed ones.
func TestJSONValueFormatIsPostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/json_value_format_pg16.jsonl", 14)
}

// TestJSONValueKeepsItsFormat requires a JSON value expression to keep its
// FORMAT clause, and one without to have the default, unlocated format, as
// PostgreSQL 16's json_format_clause_opt makes it.
func TestJSONValueKeepsItsFormat(t *testing.T) {
	list, err := Parse("select json_array('1' format json encoding utf8, 2)")
	if err != nil {
		t.Fatal(err)
	}
	exprs := list.Items[0].(*nodes.SelectStmt).TargetList.Items[0].(*nodes.ResTarget).Val.(*nodes.JsonArrayConstructor).Exprs.Items
	f, g := exprs[0].(*nodes.JsonValueExpr).Format, exprs[1].(*nodes.JsonValueExpr).Format
	if f.FormatType != nodes.JS_FORMAT_JSON || f.Encoding != nodes.JS_ENC_UTF8 || f.Location != 22 {
		t.Errorf("FORMAT JSON ENCODING UTF8: %+v", f)
	}
	if g.FormatType != nodes.JS_FORMAT_DEFAULT || g.Encoding != nodes.JS_ENC_DEFAULT || g.Location != -1 {
		t.Errorf("no FORMAT: %+v, want the default at -1", g)
	}
	list, err = Parse("select json_array(select 1)")
	if err != nil {
		t.Fatal(err)
	}
	if q := list.Items[0].(*nodes.SelectStmt).TargetList.Items[0].(*nodes.ResTarget).Val.(*nodes.JsonArrayQueryConstructor); q.Format == nil || q.Format.Location != -1 {
		t.Errorf("query form without FORMAT: %+v, want the default at -1", q.Format)
	}
}
