package parser

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// tokenName is the grammar's name for token tok, such as "ABORT_P".
func tokenName(tok int) string { return pgToknames[tok-pgPrivate+1] }

// TestLookupKeywordVersion16IsPostgreSQL16s checks LookupKeywordVersion(16, ·)
// against testdata/pg16_keywords.tsv, PostgreSQL 16.15's select word,
// catcode, barelabel from pg_get_keywords(), row for row, and that every
// other word in Keywords is no keyword there.
func TestLookupKeywordVersion16IsPostgreSQL16s(t *testing.T) {
	data, err := os.ReadFile("testdata/pg16_keywords.tsv")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if lines[0] != "word\tcatcode\tbarelabel" || len(lines) != 472 {
		t.Fatalf("header %q and %d rows, want word, catcode, barelabel and 471", lines[0], len(lines)-1)
	}
	pg16 := map[string]bool{}
	for _, line := range lines[1:] {
		cols := strings.Split(line, "\t")
		pg16[cols[0]] = true
		k := LookupKeywordVersion(PostgreSQL16, strings.ToUpper(cols[0]))
		if k == nil {
			t.Errorf("%s: no PostgreSQL 16 keyword", cols[0])
			continue
		}
		if got := []string{k.Name, string("UCTR"[k.Category]), map[bool]string{true: "t", false: "f"}[k.BareLabel]}; strings.Join(got, "\t") != line {
			t.Errorf("LookupKeywordVersion(16, %q) = %q, want %q", cols[0], got, cols)
		}
	}
	for _, k := range Keywords {
		if !pg16[k.Name] && LookupKeywordVersion(PostgreSQL16, k.Name) != nil {
			t.Errorf("LookupKeywordVersion(16, %q) is a keyword; PostgreSQL 16 has none", k.Name)
		}
	}
}

// TestLookupKeywordVersion17IsLookupKeyword checks that version 17's view is
// Keywords' own, and that an unsupported version has no keywords.
func TestLookupKeywordVersion17IsLookupKeyword(t *testing.T) {
	for i := range Keywords {
		if got := LookupKeywordVersion(PostgreSQL17, Keywords[i].Name); got != &Keywords[i] || LookupKeyword(Keywords[i].Name) != got {
			t.Errorf("LookupKeywordVersion(17, %q) = %v, want Keywords[%d]", Keywords[i].Name, got, i)
		}
	}
	if k := LookupKeywordVersion(15, "select"); k != nil {
		t.Errorf("LookupKeywordVersion(15, select) = %v, want nil", k)
	}
	if k := LookupKeywordVersion(PostgreSQL16, "json"); k == nil || k.Token != JSON_PG16 || k.Category != UnreservedKeyword {
		t.Errorf("LookupKeywordVersion(16, json) = %+v, want JSON_PG16, unreserved", k)
	}
}

// grammarList is the tokens gram.y lists as the alternatives of rule.
func grammarList(t *testing.T, gram, rule string) []string {
	t.Helper()
	m := regexp.MustCompile(`(?ms)^` + rule + `:\s*\n(.*?)^\s*;`).FindStringSubmatch(gram)
	if m == nil {
		t.Fatalf("gram.y has no rule %s", rule)
	}
	var toks []string
	for _, alt := range strings.Split(m[1], "|") {
		if f := strings.Fields(alt); len(f) > 0 {
			toks = append(toks, f[0])
		}
	}
	sort.Strings(toks)
	return toks
}

// TestGrammarKeywordListsAreTheKeywordTable checks gram.y's four category
// lists and bare_label_keyword against Keywords, as PostgreSQL's
// check_keywords.pl checks them against kwlist.h: each list holds the
// tokens of both versions' keywords in that category, or with that label.
func TestGrammarKeywordListsAreTheKeywordTable(t *testing.T) {
	data, err := os.ReadFile("gram.y")
	if err != nil {
		t.Fatal(err)
	}
	gram := string(data)
	lists := []string{"unreserved_keyword", "col_name_keyword", "type_func_name_keyword", "reserved_keyword"}
	want := map[string]map[string]bool{"bare_label_keyword": {}}
	for _, l := range lists {
		want[l] = map[string]bool{}
	}
	for _, k := range Keywords {
		want[lists[k.Category]][tokenName(k.Token)] = true
		if k.BareLabel {
			want["bare_label_keyword"][tokenName(k.Token)] = true
		}
		if k.Token16 != IDENT && k.Token16 != k.Token {
			want[lists[k.Category16]][tokenName(k.Token16)] = true
			if k.BareLabel16 {
				want["bare_label_keyword"][tokenName(k.Token16)] = true
			}
		}
	}
	for rule, toks := range want {
		var w []string
		for tok := range toks {
			w = append(w, tok)
		}
		sort.Strings(w)
		if got := grammarList(t, gram, rule); strings.Join(got, " ") != strings.Join(w, " ") {
			t.Errorf("gram.y's %s is\n%v\nwant\n%v", rule, got, w)
		}
	}
}

// TestFinishReportsAFailedParse checks that a parse that fails without an
// error from the lexer or the grammar is still an error.
func TestFinishReportsAFailedParse(t *testing.T) {
	_, err := newParserLexer("select", PostgreSQL16).finish(1)
	var pe *ParseError
	if !errors.As(err, &pe) || pe.Code != codeSyntaxError || pe.Message != "parse error (ret=1)" {
		t.Errorf("finish(1) = %v, want 42601 parse error (ret=1)", err)
	}
}

func TestNewRefusesAnUnsupportedVersion(t *testing.T) {
	for _, v := range []Version{0, 15, 18} {
		p, err := New(v)
		if p != nil || err == nil || err.Error() != "unsupported PostgreSQL version "+map[Version]string{0: "0", 15: "15", 18: "18"}[v] {
			t.Errorf("New(%d) = %v, %v, want an unsupported version error", v, p, err)
		}
	}
}

// TestParserAnswersAsItsVersion parses statements whose answer differs
// between the versions with a Parser for each, and with Parse, which is 17.
func TestParserAnswersAsItsVersion(t *testing.T) {
	p16, err := New(PostgreSQL16)
	if err != nil {
		t.Fatal(err)
	}
	p17, err := New(PostgreSQL17)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		sql        string
		err16      string
		pos16      int
		node16     string
		node17     string
		parsedAt17 bool
	}{
		{sql: "select json_value(j, '$') from t", node16: "*nodes.FuncCall", node17: "*nodes.JsonFuncExpr", parsedAt17: true},
		{sql: "select x::json", node16: "*nodes.TypeCast", node17: "*nodes.TypeCast", parsedAt17: true},
		{sql: "select json(3)", node16: "*nodes.FuncCall", node17: "*nodes.JsonParseExpr", parsedAt17: true},
		{sql: "select x is json object", node16: "*nodes.JsonIsPredicate", node17: "*nodes.JsonIsPredicate", parsedAt17: true},
		{sql: "select json_array(returning text format json)", node16: "*nodes.JsonArrayConstructor", node17: "*nodes.JsonArrayConstructor", parsedAt17: true},
		{sql: "select json_value(j, '$' returning text)", err16: `syntax error at or near "returning"`, pos16: 25, node17: "*nodes.JsonFuncExpr", parsedAt17: true},
		{sql: "select 1 path", node16: "*nodes.A_Const", node17: "*nodes.A_Const", parsedAt17: true},
	} {
		list, err := p16.Parse(c.sql)
		if c.err16 != "" {
			var pe *ParseError
			if !errors.As(err, &pe) || pe.Code != codeSyntaxError || pe.Message != c.err16 || pe.Position != c.pos16 {
				t.Errorf("16: Parse(%q) = %v, want 42601 %s at %d", c.sql, err, c.err16, c.pos16)
			}
		} else if err != nil || typeOfTarget(list) != c.node16 {
			t.Errorf("16: Parse(%q) = %s, %v, want %s", c.sql, typeOfTarget(list), err, c.node16)
		}
		for _, parse := range []func(string) (*nodes.List, error){p17.Parse, func(s string) (*nodes.List, error) { return Parse(s) }} {
			list, err := parse(c.sql)
			if err != nil || typeOfTarget(list) != c.node17 {
				t.Errorf("17: Parse(%q) = %s, %v, want %s", c.sql, typeOfTarget(list), err, c.node17)
			}
		}
	}
}

// typeOfTarget is the Go type of the first target of list's select.
func typeOfTarget(list *nodes.List) string {
	if list == nil {
		return "nil"
	}
	return fmt.Sprintf("%T", list.Items[0].(*nodes.SelectStmt).TargetList.Items[0].(*nodes.ResTarget).Val)
}

// TestParserKeepsItsNoticeHandler checks that a Parser made with
// WithNoticeHandler sends each parse's notices to it.
func TestParserKeepsItsNoticeHandler(t *testing.T) {
	var got []Notice
	p, err := New(PostgreSQL16, WithNoticeHandler(func(n Notice) { got = append(got, n) }))
	if err != nil {
		t.Fatal(err)
	}
	long := strings.Repeat("a", 70)
	for i := 0; i < 2; i++ {
		if _, err := p.Parse("select " + long); err != nil {
			t.Fatal(err)
		}
	}
	if len(got) != 2 || got[0].Code != "42622" {
		t.Errorf("notices %v, want 42622 twice", got)
	}
}

// TestParserIsSafeForConcurrentUse parses with one Parser of each version
// from several goroutines at once; go test -race checks it.
func TestParserIsSafeForConcurrentUse(t *testing.T) {
	var wg sync.WaitGroup
	for _, v := range []Version{PostgreSQL16, PostgreSQL17} {
		p, err := New(v)
		if err != nil {
			t.Fatal(err)
		}
		for g := 0; g < 8; g++ {
			wg.Add(1)
			go func(p *Parser, v Version) {
				defer wg.Done()
				for i := 0; i < 50; i++ {
					list, err := p.Parse("select json_scalar(1)")
					if err != nil || (typeOfTarget(list) == "*nodes.FuncCall") != (v == PostgreSQL16) {
						t.Errorf("%d: %s, %v", v, typeOfTarget(list), err)
						return
					}
				}
			}(p, v)
		}
	}
	wg.Wait()
}
