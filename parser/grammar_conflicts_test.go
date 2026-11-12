package parser

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"testing"
)

// The grammar's conflicts, as goyacc reports them for gram.y. A change to
// either count must change these on purpose. make check-parser checks that
// y.output is goyacc's output for gram.y.
const (
	pinnedShiftReduce  = 10
	pinnedReduceReduce = 21
)

var (
	conflictSummary = regexp.MustCompile(`^(\d+) shift/reduce, (\d+) reduce/reduce conflicts reported$`)
	conflictLine    = regexp.MustCompile(`^ ?\d+: (shift/reduce|reduce/reduce) conflict `)
)

// yaccConflicts counts the conflicts in a goyacc y.output: the summary line
// goyacc writes when there are any, and the conflict lines of the states,
// which must agree with it.
func yaccConflicts(path string) (shiftReduce, reduceReduce int, err error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	summary := false
	sumSR, sumRR := 0, 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if m := conflictLine.FindStringSubmatch(line); m != nil {
			if m[1] == "shift/reduce" {
				shiftReduce++
			} else {
				reduceReduce++
			}
			continue
		}
		if m := conflictSummary.FindStringSubmatch(line); m != nil {
			summary = true
			sumSR, _ = strconv.Atoi(m[1])
			sumRR, _ = strconv.Atoi(m[2])
		}
	}
	if err := sc.Err(); err != nil {
		return 0, 0, err
	}
	if summary != (shiftReduce+reduceReduce > 0) || sumSR != shiftReduce || sumRR != reduceReduce {
		return 0, 0, fmt.Errorf("%s: summary %d shift/reduce, %d reduce/reduce; states list %d and %d", path, sumSR, sumRR, shiftReduce, reduceReduce)
	}
	return shiftReduce, reduceReduce, nil
}

// TestGrammarConflictsArePinned fails when gram.y's conflict counts are not
// the pinned ones, so that a new conflict, or a removed one, is a deliberate
// change.
func TestGrammarConflictsArePinned(t *testing.T) {
	sr, rr, err := yaccConflicts("y.output")
	if err != nil {
		t.Fatal(err)
	}
	if sr != pinnedShiftReduce || rr != pinnedReduceReduce {
		t.Errorf("gram.y has %d shift/reduce and %d reduce/reduce conflicts; pinned are %d and %d", sr, rr, pinnedShiftReduce, pinnedReduceReduce)
	}
}

// TestYaccConflictsCountsAndChecks covers yaccConflicts on outputs it must
// read and ones it must refuse.
func TestYaccConflictsCountsAndChecks(t *testing.T) {
	cases := []struct {
		name, text string
		sr, rr     int
		bad        bool
	}{
		{"none", "state 0\n\t$accept: .stmtblock $end \n", 0, 0, false},
		{"some", "1098: shift/reduce conflict (shift 1754(0), red'n 1725(0)) on CASCADE\n 1098: reduce/reduce conflict  (red'ns 1725 and 433) on $end\n 1098: reduce/reduce conflict  (red'ns 1725 and 433) on ';'\n1 shift/reduce, 2 reduce/reduce conflicts reported\n", 1, 2, false},
		{"summary differs", "1098: shift/reduce conflict (shift 1754(0), red'n 1725(0)) on CASCADE\n2 shift/reduce, 0 reduce/reduce conflicts reported\n", 0, 0, true},
		{"no summary", " 1: reduce/reduce conflict  (red'ns 1 and 2) on ';'\n", 0, 0, true},
		{"summary only", "0 shift/reduce, 1 reduce/reduce conflicts reported\n", 0, 0, true},
	}
	for _, c := range cases {
		path := t.TempDir() + "/y.output"
		if err := os.WriteFile(path, []byte(c.text), 0o644); err != nil {
			t.Fatal(err)
		}
		sr, rr, err := yaccConflicts(path)
		if (err != nil) != c.bad || sr != c.sr || rr != c.rr {
			t.Errorf("%s: yaccConflicts = %d, %d, %v; want %d, %d, error %v", c.name, sr, rr, err, c.sr, c.rr, c.bad)
		}
	}
	if _, _, err := yaccConflicts(t.TempDir() + "/missing"); err == nil {
		t.Error("yaccConflicts of a missing file: no error")
	}
}
