// Command pg16sweep compares the parser with PostgreSQL 16's over the
// regression statements and their mutations. It is its own module, so that
// the root module's tests need neither pgx nor a database.
//
//	pg16sweep corpus -sql ../../parser/pgregress/testdata/sql -out corpus.jsonl
//	pg16sweep oracle -db postgres://... -corpus corpus.jsonl -out pg16.jsonl
//	pg16sweep fork -corpus corpus.jsonl -out ff.jsonl
//	pg16sweep compare -corpus corpus.jsonl -pg pg16.jsonl -fork ff.jsonl [-other pr.jsonl] -testdata ../../parser/testdata/pg16_sweep.jsonl
//
// The oracle sends each statement S as the simple query "select 1/0;S".
// PostgreSQL parses the whole query string before running any of it, so
// 22012 (division by zero) means its parser accepted S, and any other error
// is its parser's, at a position within S once the prefix is taken off.
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: pg16sweep corpus|oracle|fork|compare [flags]")
		os.Exit(2)
	}
	fs := flag.NewFlagSet(os.Args[1], flag.ExitOnError)
	sqlDir := fs.String("sql", "../../parser/pgregress/testdata/sql", "regression .sql directory")
	probes := fs.String("probes", "probes.sql", "hand-written probe statements, one a line")
	fixtures := fs.String("fixtures", "../../tests/grammar/sql", "grammar fixture .sql directory, read as probes")
	corpus := fs.String("corpus", "corpus.jsonl", "corpus file")
	out := fs.String("out", "", "output file")
	k := fs.Int("k", 2, "mutations per statement")
	db := fs.String("db", "postgres://postgres@127.0.0.1:54316/oracle?sslmode=disable", "PostgreSQL 16 connection string")
	workers := fs.Int("workers", 8, "oracle connections")
	start := fs.Int("start", 0, "first corpus id (fork: resume after a fatal error)")
	pg := fs.String("pg", "pg16.jsonl", "oracle results")
	fork := fs.String("fork", "ff.jsonl", "parser results the test data pins")
	other := fs.String("other", "", "another parser's results to compare, such as an open PR's")
	testdata := fs.String("testdata", "", "test data file to write")
	report := fs.String("report", "", "class report to write")
	sample := fs.Int("sample", 40, "keep one agreeing mutation in this many")
	_ = fs.Parse(os.Args[2:])
	var err error
	switch os.Args[1] {
	case "corpus":
		err = runCorpus(*sqlDir, *probes, *fixtures, *out, *k)
	case "oracle":
		err = runOracle(*db, *corpus, *out, *workers)
	case "fork":
		err = runForkDriver(*corpus, *out, *start)
	case "fork-child":
		err = runForkChild(*corpus, *out, *start)
	case "probe":
		err = runProbe(*db)
	case "compare":
		err = runCompare(*corpus, *pg, *fork, *other, *testdata, *report, *sample)
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
