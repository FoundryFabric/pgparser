package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// TestPatchCommittedParser requires the committed parser to carry the check
// exactly once, right after the push, and patch to refuse it again.
func TestPatchCommittedParser(t *testing.T) {
	src, err := os.ReadFile("../../parser/parser.go")
	if err != nil {
		t.Fatal(err)
	}
	want := "\tpgp++\n\t// The stack holds pgp+1 states; Bison refuses its YYMAXDEPTH-th.\n\tif pgp+1 >= pgMaxDepth {\n\t\tpgStackExhausted(pglex)\n\t\tgoto ret1\n\t}\n\tif pgp >= len(pgS) {\n"
	if n := strings.Count(string(src), want); n != 1 {
		t.Fatalf("parser.go holds the depth check %d times, want once", n)
	}
	if _, err := patch(src, "pg"); err == nil {
		t.Fatal("patched an already patched parser")
	}
}

// TestPatchUnpatched removes the check from the committed parser and
// requires patch to put it back as it was.
func TestPatchUnpatched(t *testing.T) {
	src, err := os.ReadFile("../../parser/parser.go")
	if err != nil {
		t.Fatal(err)
	}
	check := "\t// The stack holds pgp+1 states; Bison refuses its YYMAXDEPTH-th.\n\tif pgp+1 >= pgMaxDepth {\n\t\tpgStackExhausted(pglex)\n\t\tgoto ret1\n\t}\n"
	unpatched := bytes.Replace(src, []byte(check), nil, 1)
	if bytes.Equal(unpatched, src) {
		t.Fatal("no check to remove")
	}
	got, err := patch(unpatched, "pg")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, src) {
		t.Fatal("patch did not reproduce the committed parser")
	}
	if _, err := patch(unpatched, "yy"); err == nil {
		t.Fatal("patched with the wrong prefix")
	}
	if _, err := patch(append(append([]byte{}, unpatched...), unpatched...), "pg"); err == nil {
		t.Fatal("patched a file holding the push twice")
	}
}
