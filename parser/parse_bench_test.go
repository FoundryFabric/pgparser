package parser_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pgplex/pgparser/parser"
	"github.com/pgplex/pgparser/parser/pgregress"
)

// benchStatements is every statement of the regression corpus.
func benchStatements(b *testing.B) []string {
	b.Helper()
	files, err := filepath.Glob("pgregress/testdata/sql/*.sql")
	if err != nil || len(files) == 0 {
		b.Fatalf("no corpus: %v", err)
	}
	var stmts []string
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			b.Fatal(err)
		}
		for _, s := range pgregress.ExtractStatements(filepath.Base(f), data) {
			stmts = append(stmts, s.SQL)
		}
	}
	return stmts
}

// BenchmarkParseCorpus parses every statement of the regression corpus with
// Parse and with a Parser for each version.
func BenchmarkParseCorpus(b *testing.B) {
	stmts := benchStatements(b)
	var size int64
	for _, s := range stmts {
		size += int64(len(s))
	}
	run := func(name string, parse func(string) error) {
		b.Run(name, func(b *testing.B) {
			b.SetBytes(size)
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				for _, s := range stmts {
					_ = parse(s)
				}
			}
		})
	}
	run("Parse", func(s string) error { _, err := parser.Parse(s); return err })
	for _, v := range []parser.Version{parser.PostgreSQL17, parser.PostgreSQL16} {
		p, err := parser.New(v)
		if err != nil {
			b.Fatal(err)
		}
		run("PostgreSQL"+[...]string{16: "16", 17: "17"}[v], func(s string) error { _, err := p.Parse(s); return err })
	}
}
