package parser

import (
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// TestTableLikeIsPostgreSQL16s pins PostgreSQL 16's answers for 21
// statements with LIKE and its INCLUDING and EXCLUDING options, ALL among
// them, and malformed ones.
func TestTableLikeIsPostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/table_like_pg16.jsonl", 21)
}

// TestTableLikeAllIsPostgreSQL16s requires INCLUDING ALL to be PostgreSQL
// 16's CREATE_TABLE_LIKE_ALL, PG_INT32_MAX, as libpg_query's tree has it.
func TestTableLikeAllIsPostgreSQL16s(t *testing.T) {
	for sql, want := range map[string]uint32{
		"create table t (like s including all)":                    0x7FFFFFFF,
		"create table t (like s including all excluding comments)": 0x7FFFFFFE,
		"create table t (like s excluding all including defaults)": 8,
	} {
		list, err := parse16(sql)
		if err != nil {
			t.Fatal(err)
		}
		if got := list.Items[0].(*nodes.CreateStmt).TableElts.Items[0].(*nodes.TableLikeClause).Options; got != want {
			t.Errorf("%s: options %#x, want %#x", sql, got, want)
		}
	}
}
