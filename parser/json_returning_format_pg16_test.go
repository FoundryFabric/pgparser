package parser

import (
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// TestJSONReturningFormatIsPostgreSQL16s pins PostgreSQL 16's answers for 40
// statements with RETURNING, with and without FORMAT JSON and an encoding,
// in JSON_ARRAY, JSON_OBJECT, JSON_ARRAYAGG and JSON_OBJECTAGG, and the
// FORMAT and ENCODING spellings PostgreSQL 16 refuses.
func TestJSONReturningFormatIsPostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/json_returning_format_pg16.jsonl", 40)
}

// TestJSONReturningKeepsItsFormat requires every RETURNING clause to carry
// its JsonReturning, with the FORMAT clause's type, encoding and location,
// or PostgreSQL 16's default format, unlocated, when it has none, as
// libpg_query's tree has it.
func TestJSONReturningKeepsItsFormat(t *testing.T) {
	for _, c := range []struct {
		sql      string
		format   nodes.JsonFormatType
		encoding nodes.JsonEncoding
		location nodes.ParseLoc
	}{
		{"select json_array(returning text)", nodes.JS_FORMAT_DEFAULT, nodes.JS_ENC_DEFAULT, -1},
		{"select json_array(returning bytea format json)", nodes.JS_FORMAT_JSON, nodes.JS_ENC_DEFAULT, 34},
		{"select json_array(1 returning bytea format json encoding utf8)", nodes.JS_FORMAT_JSON, nodes.JS_ENC_UTF8, 36},
		{"select json_array(select 1 format json returning bytea format json encoding utf16)", nodes.JS_FORMAT_JSON, nodes.JS_ENC_UTF16, 55},
		{"select json_object(returning text)", nodes.JS_FORMAT_DEFAULT, nodes.JS_ENC_DEFAULT, -1},
		{"select json_object('a': 1 returning bytea format json encoding utf32)", nodes.JS_FORMAT_JSON, nodes.JS_ENC_UTF32, 42},
		{"select json_objectagg(k: v returning bytea format json) from t", nodes.JS_FORMAT_JSON, nodes.JS_ENC_DEFAULT, 43},
		{"select json_arrayagg(v returning text) from t", nodes.JS_FORMAT_DEFAULT, nodes.JS_ENC_DEFAULT, -1},
		{"select json_serialize('1' returning bytea format json)", nodes.JS_FORMAT_JSON, nodes.JS_ENC_DEFAULT, 42},
		{"select json_value(jsonb '1', '$' returning text)", nodes.JS_FORMAT_DEFAULT, nodes.JS_ENC_DEFAULT, -1},
	} {
		list, err := Parse(c.sql)
		if err != nil {
			t.Fatal(err)
		}
		var out *nodes.JsonOutput
		switch n := list.Items[0].(*nodes.SelectStmt).TargetList.Items[0].(*nodes.ResTarget).Val.(type) {
		case *nodes.JsonArrayConstructor:
			out = n.Output
		case *nodes.JsonArrayQueryConstructor:
			out = n.Output
		case *nodes.JsonObjectConstructor:
			out = n.Output
		case *nodes.JsonObjectAgg:
			out = n.Constructor.Output
		case *nodes.JsonArrayAgg:
			out = n.Constructor.Output
		case *nodes.JsonSerializeExpr:
			out = n.Output
		case *nodes.JsonFuncExpr:
			out = n.Output
		}
		if out == nil || out.Returning == nil || out.Returning.Format == nil {
			t.Errorf("%s: output %+v, want a returning format", c.sql, out)
			continue
		}
		if f := out.Returning.Format; f.FormatType != c.format || f.Encoding != c.encoding || f.Location != c.location {
			t.Errorf("%s: format %+v, want type %d encoding %d at %d", c.sql, f, c.format, c.encoding, c.location)
		}
	}
}
