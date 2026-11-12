package main

import (
	"os"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	if err := loadForkEnums("../../nodes"); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

func bothTrees(t *testing.T, sql string) (any, any) {
	t.Helper()
	lq, ltree := parseLibpg(0, sql)
	if !lq.OK {
		t.Fatalf("%s: libpg_query refuses: %s", sql, describe(lq))
	}
	ftree, fk := parseTree(0, sql)
	if !fk.OK {
		t.Fatalf("%s: the fork refuses: %s", sql, describe(fk))
	}
	return treesOf(ftree, ltree)
}

func TestParseLibpg(t *testing.T) {
	if r, _ := parseLibpg(1, "select 1"); !r.OK || r.Pos != -1 {
		t.Errorf("select 1: %+v", r)
	}
	// The cursor position counts characters; the result's is in bytes.
	sql := "select 'éé日本' + ;"
	r, tree := parseLibpg(2, sql)
	if r.OK || tree != nil {
		t.Fatalf("%s: accepted", sql)
	}
	if r.Msg != `syntax error at or near ";"` || r.Pos != strings.Index(sql, ";") || r.Code != "" {
		t.Errorf("%s: %+v", sql, r)
	}
	if fk := parseOne(2, sql); fk.Pos != r.Pos || fk.Msg != r.Msg {
		t.Errorf("%s: fork %s, libpg_query %s", sql, describe(fk), describe(r))
	}
}

func TestOracleKind(t *testing.T) {
	ok := result{OK: true, Pos: -1}
	bad := result{Code: "42601", Msg: "syntax error", Pos: 3}
	for _, c := range []struct {
		pg, lq result
		want   string
	}{
		{ok, ok, "both accept"},
		{ok, bad, classOraclesDisagree},
		{bad, ok, classOraclesDisagree},
		{bad, result{Msg: "syntax error", Pos: 3}, "both refuse alike"},
		{bad, result{Msg: "syntax error", Pos: 4}, "both refuse, different position"},
		{bad, result{Msg: "other", Pos: 3}, "both refuse, different message"},
	} {
		if got := oracleKind(c.pg, c.lq); got != c.want {
			t.Errorf("oracleKind(%+v, %+v) = %q, want %q", c.pg, c.lq, got, c.want)
		}
	}
}

// TestTreesAlike covers each normalisation rule with a statement whose
// trees agree once it is applied.
func TestTreesAlike(t *testing.T) {
	for _, sql := range []string{
		"select a.b, t.* from s.t where x::numeric(10,2)[] = 1 and y = 'z' limit 3",
		"select 1.5, true, b'101', null",
		"create index i on t (a)",
		"create or replace function f() returns int return 1",
		"savepoint x",
		"alter type e add value 'v' after 'u'",
		"select * from a join b using (i) as j",
		"merge into t using s on t.a = s.a when matched then delete",
		"deallocate all",
		"select * from t for update",
		"alter table t drop column c",
	} {
		fv, lv := bothTrees(t, sql)
		for _, m := range compareTrees(fv, lv) {
			t.Errorf("%s: %s: fork %s, libpg_query %s", sql, m.class(), shortValue(m.fork), shortValue(m.libpg))
		}
	}
}

// TestTreesDiffer checks that each mismatch kind is seen, on statements
// the fork's tree differs on and on a tree changed by hand.
func TestTreesDiffer(t *testing.T) {
	for _, c := range []struct{ sql, class string }{
		{"drop server s", "node-type DropStmt.objects"},
		{"grant all on t to public", "value GrantStmt.grantees/RoleSpec.roletype"},
		{"select * from f()", "list-length RangeFunction.functions"},
		{"select 1 from t order by a", "overplaced SelectStmt.sortclause/SortBy.location"},
		{"create table t (a int)", "unlocated CreateStmt.tableelts/ColumnDef.location"},
	} {
		fv, lv := bothTrees(t, c.sql)
		var got []string
		for _, m := range compareTrees(fv, lv) {
			got = append(got, m.class())
		}
		if !contains(got, c.class) {
			t.Errorf("%s: %v, want %q among them", c.sql, got, c.class)
		}
	}
	// A char enum compares by member: only the locations differ here.
	fv, lv := bothTrees(t, "create table p (a int) partition by list (a)")
	for _, m := range compareTrees(fv, lv) {
		if m.kind != treeUnlocated {
			t.Errorf("partition by list: %s", m.class())
		}
	}
	sql := "select a.b from s.t"
	for _, c := range []struct {
		change func(stmt *tnode)
		class  string
	}{
		{func(s *tnode) { colRef(s).fields["fields"].([]any)[1].(*tnode).fields["sval"] = "c" }, "name ColumnRef.fields/String.sval"},
		{func(s *tnode) { f := colRef(s); f.fields["fields"] = f.fields["fields"].([]any)[:1] }, "list-length ColumnRef.fields"},
		{func(s *tnode) { colRef(s).fields["location"] = int64(8) }, "location ResTarget.val/ColumnRef.location"},
		{func(s *tnode) { colRef(s).fields["fields"].([]any)[0].(*tnode).typ = "A_Star" }, "node-type ColumnRef.fields"},
		{func(s *tnode) { s.fields["whereclause"] = &tnode{typ: "A_Const"} }, "presence SelectStmt.whereclause"},
		{func(s *tnode) { s.fields["all"] = true }, "value SelectStmt.all"},
	} {
		fv, lv := bothTrees(t, sql)
		if ms := compareTrees(fv, lv); len(ms) != 0 {
			t.Fatalf("%s: %v", sql, ms)
		}
		c.change(fv.([]any)[0].(*tnode))
		ms := compareTrees(fv, lv)
		if len(ms) != 1 || ms[0].class() != c.class {
			t.Errorf("%s changed: %v, want one %q", sql, ms, c.class)
		}
	}
}

func colRef(stmt *tnode) *tnode {
	return stmt.fields["targetlist"].([]any)[0].(*tnode).fields["val"].(*tnode)
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}
