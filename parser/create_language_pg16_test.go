package parser

import (
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// TestCreateLanguageIsPostgreSQL16s pins PostgreSQL 16's answers for 395
// CREATE LANGUAGE statements, with and without OR REPLACE, TRUSTED,
// PROCEDURAL and a handler, with plain, quoted, keyword, qualified and
// missing names, and broken forms.
func TestCreateLanguageIsPostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/create_language_pg16.jsonl", 395)
}

// TestCreateLanguageWithoutHandlerIsAnExtension requires CREATE LANGUAGE
// without a handler to be CREATE EXTENSION, OR REPLACE its IF NOT EXISTS
// and TRUSTED dropped, as libpg_query's tree has it.
func TestCreateLanguageWithoutHandlerIsAnExtension(t *testing.T) {
	list, err := parse16("create or replace trusted procedural language l")
	if err != nil {
		t.Fatal(err)
	}
	got, ok := list.Items[0].(*nodes.CreateExtensionStmt)
	if !ok || got.Extname != "l" || !got.IfNotExists || got.Options != nil {
		t.Errorf("got %#v, want CreateExtensionStmt{Extname: l, IfNotExists: true}", list.Items[0])
	}
}
