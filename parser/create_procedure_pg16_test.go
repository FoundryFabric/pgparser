package parser

import (
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// TestCreateProcedureIsPostgreSQL16s pins PostgreSQL 16's answers for 12
// CREATE PROCEDURE and CREATE FUNCTION statements, with and without OR
// REPLACE, arguments, options and a body, and malformed ones.
func TestCreateProcedureIsPostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/create_procedure_pg16.jsonl", 12)
}

// TestCreateProcedureIsMarked requires CREATE PROCEDURE to be marked by
// IsProcedure, with only its own options, as libpg_query's tree has it.
func TestCreateProcedureIsMarked(t *testing.T) {
	for sql, want := range map[string]bool{
		"create procedure p() language sql as 'select 1'":                      true,
		"create or replace procedure p() language sql as 'select 1'":           true,
		"create function f() returns int language sql as 'select 1'":           false,
		"create or replace function f() language sql as 'select 1'":            false,
		"create function f() returns table (a int) language sql as 'select 1'": false,
	} {
		list, err := Parse(sql)
		if err != nil {
			t.Fatal(err)
		}
		n := list.Items[0].(*nodes.CreateFunctionStmt)
		if n.IsProcedure != want || len(n.Options.Items) != 2 {
			t.Errorf("%s: procedure %v with %d options, want %v with 2", sql, n.IsProcedure, len(n.Options.Items), want)
		}
	}
}
