// Command yaccmaxdepth bounds the parse stack of a parser goyacc generated,
// as Bison's YYMAXDEPTH bounds PostgreSQL's.
//
// goyacc's driver doubles its stack whenever it fills, without limit, and
// goyacc has no option to change that, so this rewrites the one place the
// driver pushes a state: the parse fails once the stack would hold
// <prefix>MaxDepth states, calling <prefix>StackExhausted(<prefix>lex) and
// returning 1, where Bison raises "memory exhausted". The grammar's own code
// defines both names. It runs after goyacc in parser/parse.go's go:generate
// and in make check-parser:
//
//	go run ../tools/yaccmaxdepth -p pg parser.go
//
// It refuses a file that does not hold the push, followed by the stack's
// growth, exactly once: one a goyacc other than the pinned one laid out
// differently, or one already patched.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"strings"
)

func main() {
	prefix := flag.String("p", "yy", "the prefix given to goyacc's -p")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: yaccmaxdepth [-p prefix] parser.go")
		os.Exit(2)
	}
	if err := patchFile(flag.Arg(0), *prefix); err != nil {
		fmt.Fprintln(os.Stderr, "yaccmaxdepth:", err)
		os.Exit(1)
	}
}

// patchFile rewrites the generated parser at path in place.
func patchFile(path, prefix string) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	out, err := patch(src, prefix)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return os.WriteFile(path, out, 0o644)
}

// patch returns src with the depth check inserted after the driver's push.
func patch(src []byte, prefix string) ([]byte, error) {
	r := strings.NewReplacer("$", prefix)
	push := []byte(r.Replace("\t/* put a state and value onto the stack */\n" +
		"\tif $Debug >= 4 {\n" +
		"\t\t__yyfmt__.Printf(\"char %v in %v\\n\", $Tokname($token), $Statname($state))\n" +
		"\t}\n" +
		"\n" +
		"\t$p++\n"))
	grow := []byte(r.Replace("\tif $p >= len($S) {\n"))
	check := []byte(r.Replace("\t// The stack holds $p+1 states; Bison refuses its YYMAXDEPTH-th.\n" +
		"\tif $p+1 >= $MaxDepth {\n" +
		"\t\t$StackExhausted($lex)\n" +
		"\t\tgoto ret1\n" +
		"\t}\n"))
	// The push is followed by the stack's growth until it is patched.
	at := append(append([]byte{}, push...), grow...)
	if n := bytes.Count(src, at); n != 1 {
		return nil, fmt.Errorf("found the driver's push %d times, want once", n)
	}
	i := bytes.Index(src, at) + len(push)
	out := make([]byte, 0, len(src)+len(check))
	out = append(out, src[:i]...)
	out = append(out, check...)
	return append(out, src[i:]...), nil
}
