package parser

import (
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// TestObjectTypeNameIsPostgreSQL16s pins PostgreSQL 16's answers for 68
// COMMENT, SECURITY LABEL and ALTER EXTENSION ADD and DROP statements on an
// event trigger, an extension, a foreign-data wrapper and a [PROCEDURAL]
// language, by a bare name and a qualified one, and on the other objects
// named by a bare name beside them.
func TestObjectTypeNameIsPostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/object_type_name_pg16.jsonl", 68)
}

// TestObjectTypeNameNamesTheObject requires the objects PostgreSQL 16's
// object_type_name names to be a bare name of their type.
func TestObjectTypeNameNamesTheObject(t *testing.T) {
	for _, c := range []struct {
		sql string
		typ nodes.ObjectType
	}{
		{"comment on event trigger x is 'c'", nodes.OBJECT_EVENT_TRIGGER},
		{"comment on extension x is 'c'", nodes.OBJECT_EXTENSION},
		{"comment on foreign data wrapper x is 'c'", nodes.OBJECT_FDW},
		{"comment on language x is 'c'", nodes.OBJECT_LANGUAGE},
		{"comment on procedural language x is 'c'", nodes.OBJECT_LANGUAGE},
		{"security label on extension x is 'c'", nodes.OBJECT_EXTENSION},
		{"alter extension e add procedural language x", nodes.OBJECT_LANGUAGE},
	} {
		list, err := Parse(c.sql)
		if err != nil {
			t.Fatal(err)
		}
		var typ nodes.ObjectType
		var obj nodes.Node
		switch s := list.Items[0].(type) {
		case *nodes.CommentStmt:
			typ, obj = s.Objtype, s.Object
		case *nodes.SecLabelStmt:
			typ, obj = s.Objtype, s.Object
		case *nodes.AlterExtensionContentsStmt:
			typ, obj = s.Objtype, s.Object
		}
		if s, ok := obj.(*nodes.String); typ != c.typ || !ok || s.Str != "x" {
			t.Errorf("%s: type %d object %+v, want type %d and the name x", c.sql, typ, obj, c.typ)
		}
	}
}
