package main

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// TestGrammarRulesMatchYOutput requires grammarRules to number and spell the
// committed grammar's rules as goyacc's y.output does, wherever y.output
// shows a rule completed: "lhs: rhs.    (N)".
func TestGrammarRulesMatchYOutput(t *testing.T) {
	gram, err := os.ReadFile("../../parser/gram.y")
	if err != nil {
		t.Fatal(err)
	}
	rules, err := grammarRules(gram)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Open("../../parser/y.output")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	done := regexp.MustCompile(`^\t(\S+):\s+(.*?)\s*\.\s+\((\d+)\)$`)
	seen := map[int]bool{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		m := done.FindStringSubmatch(sc.Text())
		if m == nil {
			continue
		}
		n, _ := strconv.Atoi(m[3])
		if seen[n] {
			continue
		}
		seen[n] = true
		want := m[1] + ": <empty>"
		if m[2] != "" {
			want = m[1] + ": " + strings.Join(strings.Fields(m[2]), " ")
		}
		if n >= len(rules) || rules[n] != want {
			t.Fatalf("rule %d: y.output has %q, grammarRules has %q", n, want, rules[n%len(rules)])
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	// Only the rules goyacc never reduces are missing from y.output.
	if missing := len(rules) - 1 - len(seen); missing < 0 || missing > 20 {
		t.Fatalf("y.output completes %d of %d rules", len(seen), len(rules)-1)
	}
}

// TestCheck requires check to fail for a fact the head no longer reaches and
// for a new fact it does not reach, and to pass otherwise.
func TestCheck(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, facts ...fact) string {
		p := filepath.Join(dir, name)
		if err := writeFacts(p, facts); err != nil {
			t.Fatal(err)
		}
		return p
	}
	base := write("base", fact{key: "a", covered: true}, fact{key: "b"}, fact{key: "gone", covered: true})
	for _, c := range []struct {
		name string
		head []fact
		ok   bool
	}{
		{"same", []fact{{key: "a", covered: true}, {key: "b"}}, true},
		{"more", []fact{{key: "a", covered: true}, {key: "b", covered: true}, {key: "new", covered: true}}, true},
		{"lost", []fact{{key: "a"}, {key: "b"}}, false},
		{"new", []fact{{key: "a", covered: true}, {key: "new"}}, false},
	} {
		var out strings.Builder
		err := check(base, write(c.name, c.head...), &out)
		if (err == nil) != c.ok {
			t.Errorf("%s: check = %v, want ok %v\n%s", c.name, err, c.ok, out.String())
		}
	}
}
