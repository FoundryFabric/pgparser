package parser

import (
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// TestViewCheckOptionIsPostgreSQL16s pins PostgreSQL 16's answers for 35
// statements with and without each check option, on plain, temporary,
// replaced and recursive views.
func TestViewCheckOptionIsPostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/view_check_option_pg16.jsonl", 35)
}

// TestViewCheckOptionIsCascadedByDefault requires WITH CHECK OPTION to be
// CASCADED, as libpg_query's tree has it.
func TestViewCheckOptionIsCascadedByDefault(t *testing.T) {
	for _, c := range []struct {
		sql  string
		want int
	}{
		{"create view v as select 1", VIEW_CHECK_OPTION_NONE},
		{"create view v as select 1 with check option", VIEW_CHECK_OPTION_CASCADED},
		{"create view v as select 1 with cascaded check option", VIEW_CHECK_OPTION_CASCADED},
		{"create or replace view v as select 1 with local check option", VIEW_CHECK_OPTION_LOCAL},
		{"create recursive view v (a) as select 1", VIEW_CHECK_OPTION_NONE},
	} {
		list, err := Parse(c.sql)
		if err != nil {
			t.Fatal(err)
		}
		if got := list.Items[0].(*nodes.ViewStmt).WithCheckOption; got != c.want {
			t.Errorf("%s: check option %d, want %d", c.sql, got, c.want)
		}
	}
}
