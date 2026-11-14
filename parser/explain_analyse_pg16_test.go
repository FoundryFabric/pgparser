package parser

import (
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// TestExplainAnalyseIsPostgreSQL16s pins PostgreSQL 16's answers for 40
// statements spelling ANALYZE as ANALYSE, and the ANALYZE spellings beside
// them, in EXPLAIN with and without VERBOSE, ANALYZE, VACUUM and the option
// lists.
func TestExplainAnalyseIsPostgreSQL16s(t *testing.T) {
	checkPG16Answers(t, "testdata/explain_analyse_pg16.jsonl", 40)
}

// TestExplainAnalyseIsExplainAnalyze requires EXPLAIN ANALYSE to make the
// tree EXPLAIN ANALYZE makes.
func TestExplainAnalyseIsExplainAnalyze(t *testing.T) {
	for _, c := range []struct {
		sql  string
		want []string
	}{
		{"explain analyse select 1", []string{"analyze"}},
		{"explain analyse verbose select 1", []string{"analyze", "verbose"}},
	} {
		list, err := Parse(c.sql)
		if err != nil {
			t.Fatal(err)
		}
		opts := list.Items[0].(*nodes.ExplainStmt).Options
		if opts == nil || len(opts.Items) != len(c.want) {
			t.Fatalf("%s: options %+v, want %v", c.sql, opts, c.want)
		}
		for i, w := range c.want {
			if d := opts.Items[i].(*nodes.DefElem); d.Defname != w {
				t.Errorf("%s: option %d is %q, want %q", c.sql, i, d.Defname, w)
			}
		}
	}
}
