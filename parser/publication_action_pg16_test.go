package parser

import (
	"fmt"
	"strings"
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// TestPublicationActionIsPostgreSQL16s pins PostgreSQL 16's answers for 42
// statements of ALTER PUBLICATION ADD, DROP and SET and CREATE PUBLICATION,
// with tables and schemas after TABLE and TABLES IN SCHEMA, in mixed and
// malformed lists.
func TestPublicationActionIsPostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/publication_action_pg16.jsonl", 42)
}

// TestPublicationObjectsAreResolved requires ALTER PUBLICATION's action to
// be PostgreSQL 16's AlterPublicationAction, and each object after the first
// of its kind to take that kind, a schema its name, as libpg_query's tree has
// it.
func TestPublicationObjectsAreResolved(t *testing.T) {
	for _, c := range []struct {
		sql     string
		action  nodes.AlterPublicationAction
		objects string
	}{
		{"alter publication p add table t, u", nodes.AP_AddObjects, "0:t 0:u"},
		{"alter publication p drop tables in schema s, u", nodes.AP_DropObjects, "1:s 1:u"},
		{"alter publication p set tables in schema current_schema, s, current_schema", nodes.AP_SetObjects, "2: 1:s 2:"},
		{"alter publication p add table t, tables in schema s, u", nodes.AP_AddObjects, "0:t 1:s 1:u"},
		{"alter publication p set (publish = 'insert')", nodes.AP_AddObjects, ""},
	} {
		list, err := parse16(c.sql)
		if err != nil {
			t.Fatal(err)
		}
		stmt := list.Items[0].(*nodes.AlterPublicationStmt)
		var objects []string
		if stmt.Pubobjects != nil {
			for _, o := range stmt.Pubobjects.Items {
				spec := o.(*nodes.PublicationObjSpec)
				name := spec.Name
				if spec.Pubtable != nil {
					name = spec.Pubtable.Relation.Relname
				}
				objects = append(objects, fmt.Sprintf("%d:%s", spec.Pubobjtype, name))
			}
		}
		if got := strings.Join(objects, " "); stmt.Action != c.action || got != c.objects {
			t.Errorf("%s: action %d, objects %q; want %d, %q", c.sql, stmt.Action, got, c.action, c.objects)
		}
	}
}
