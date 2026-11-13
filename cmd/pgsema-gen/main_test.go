package main

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite parser/keywords.go")

// kwlistOf renders the PostgreSQL 17 columns of a generated keywords.go as
// kwlist.h's PG_KEYWORD lines, the generator's input.
func kwlistOf(t *testing.T, src []byte) string {
	t.Helper()
	row := regexp.MustCompile(`(?m)^\t\{"([a-z_]+)", (\w+), (\w+)Keyword, (true|false)[,}]`)
	cats := map[string]string{"Unreserved": "UNRESERVED_KEYWORD", "ColName": "COL_NAME_KEYWORD", "TypeFuncName": "TYPE_FUNC_NAME_KEYWORD", "Reserved": "RESERVED_KEYWORD"}
	var b strings.Builder
	for _, m := range row.FindAllSubmatch(src, -1) {
		label := "AS_LABEL"
		if string(m[4]) == "true" {
			label = "BARE_LABEL"
		}
		b.WriteString(`PG_KEYWORD("` + string(m[1]) + `", ` + string(m[2]) + ", " + cats[string(m[3])] + ", " + label + ")\n")
	}
	return b.String()
}

// TestKeywordsGoIsGenerated checks that parser/keywords.go is what the
// generator makes of its PostgreSQL 17 rows and testdata/pg16_keywords.tsv:
// its PostgreSQL 16 columns come from the oracle's pg_get_keywords().
func TestKeywordsGoIsGenerated(t *testing.T) {
	path := filepath.Join("..", "..", "parser", "keywords.go")
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	kwlist := filepath.Join(dir, "kwlist.h")
	if err := os.WriteFile(kwlist, []byte(kwlistOf(t, want)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := generateKeywords(kwlist, filepath.Join("..", "..", "parser", "testdata", "pg16_keywords.tsv"), dir); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "keywords.go"))
	if err != nil {
		t.Fatal(err)
	}
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	if !bytes.Equal(got, want) {
		t.Errorf("parser/keywords.go is not the generator's output: run go test ./cmd/pgsema-gen -update")
	}
}

func TestGenerateKeywordsRefusesBadInput(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	kwlist := write("kwlist.h", `PG_KEYWORD("abort", ABORT_P, UNRESERVED_KEYWORD, BARE_LABEL)
PG_KEYWORD("all", ALL, RESERVED_KEYWORD, AS_LABEL)
`)
	for name, tc := range map[string]struct{ kwlist, tsv, want string }{
		"no kwlist":    {filepath.Join(dir, "none.h"), "", "no such file"},
		"bad category": {write("cat.h", `PG_KEYWORD("abort", ABORT_P, ODD_KEYWORD, BARE_LABEL)`), "", "unknown keyword category"},
		"bad label":    {write("label.h", `PG_KEYWORD("abort", ABORT_P, UNRESERVED_KEYWORD, ODD_LABEL)`), "", "unknown keyword label"},
		"no tsv":       {kwlist, filepath.Join(dir, "none.tsv"), "no such file"},
		"bad header":   {kwlist, write("h.tsv", "word\tcatcode\n"), "header"},
		"short line":   {kwlist, write("s.tsv", "word\tcatcode\tbarelabel\nabort\tU\n"), "bad line"},
		"unknown word": {kwlist, write("u.tsv", "word\tcatcode\tbarelabel\nzzz\tU\tt\n"), "kwlist.h does not have"},
		"bad catcode":  {kwlist, write("c.tsv", "word\tcatcode\tbarelabel\nabort\tX\tt\n"), "bad line"},
		"long catcode": {kwlist, write("l.tsv", "word\tcatcode\tbarelabel\nabort\tUC\tt\n"), "bad line"},
		"bad label16":  {kwlist, write("b.tsv", "word\tcatcode\tbarelabel\nabort\tU\tx\n"), "bad line"},
		"no outdir":    {kwlist, write("ok.tsv", "word\tcatcode\tbarelabel\nabort\tU\tt\nall\tR\tt\n"), "no such file"},
	} {
		t.Run(name, func(t *testing.T) {
			tsv := tc.tsv
			if tsv == "" {
				tsv = filepath.Join(dir, "ok.tsv")
			}
			err := generateKeywords(tc.kwlist, tsv, filepath.Join(dir, "missing"))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("generateKeywords: %v, want an error containing %q", err, tc.want)
			}
		})
	}
}

// TestGenerateKeywordsGivesADifferingKeywordItsOwnToken checks that a
// keyword whose PostgreSQL 16 category differs gets the token _PG16, and
// one PostgreSQL 16 does not have gets IDENT.
func TestGenerateKeywordsGivesADifferingKeywordItsOwnToken(t *testing.T) {
	dir := t.TempDir()
	kwlist := filepath.Join(dir, "kwlist.h")
	tsv := filepath.Join(dir, "k.tsv")
	if err := os.WriteFile(kwlist, []byte(`PG_KEYWORD("all", ALL, RESERVED_KEYWORD, AS_LABEL)
PG_KEYWORD("json", JSON, COL_NAME_KEYWORD, BARE_LABEL)
PG_KEYWORD("path", PATH, UNRESERVED_KEYWORD, BARE_LABEL)
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tsv, []byte("word\tcatcode\tbarelabel\nall\tR\tt\njson\tU\tt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := generateKeywords(kwlist, tsv, dir); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "keywords.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`{"all", ALL, ReservedKeyword, false, ALL_PG16, ReservedKeyword, true}`,
		`{"json", JSON, ColNameKeyword, true, JSON_PG16, UnreservedKeyword, true}`,
		`{"path", PATH, UnreservedKeyword, true, IDENT, UnreservedKeyword, false}`,
		"make(map[string]*Keyword, 2)",
	} {
		if !bytes.Contains(got, []byte(want)) {
			t.Errorf("keywords.go lacks %s", want)
		}
	}
}
