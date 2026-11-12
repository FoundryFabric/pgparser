// Command parsercover measures how much of the parser the parser package's
// tests reach, grammar actions included, and checks that a change reaches
// no less than its base did.
//
// go test -cover alone cannot see the grammar actions: goyacc's //line
// directives attribute their code to gram.y and its driver's to "yaccpar".
// run copies a source tree, generates parser/parser.go from parser/gram.y
// with goyacc -l (no line directives) and tools/yaccmaxdepth, as go generate
// does otherwise, and runs the parser package's tests twice: with a
// coverage profile, and under gobco for condition coverage. It writes
// summary.txt (statements and conditions per file, and the rules whose
// action no test reduces) and facts.txt, every statement block and
// condition outcome with whether a test reached it.
//
//	parsercover run -src . -out .cover/head
//	parsercover check -base .cover/base/facts.txt -head .cover/head/facts.txt
//
// A fact's key holds its file, the function or rule it is in and its
// normalised source text, never a line or rule number, since every grammar
// edit renumbers both. check fails for each fact the head does not reach
// that the base reached or did not have.
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: parsercover run|check [flags]")
		os.Exit(2)
	}
	fs := flag.NewFlagSet(os.Args[1], flag.ExitOnError)
	src := fs.String("src", ".", "source tree to measure")
	out := fs.String("out", ".cover/head", "directory for summary.txt and facts.txt")
	base := fs.String("base", "", "the base's facts.txt")
	head := fs.String("head", "", "the head's facts.txt")
	_ = fs.Parse(os.Args[2:])
	var err error
	switch os.Args[1] {
	case "run":
		err = run(*src, *out)
	case "check":
		err = check(*base, *head, os.Stdout)
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "parsercover:", err)
		os.Exit(1)
	}
}
