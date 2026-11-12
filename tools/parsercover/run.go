package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// The tools run pins: goyacc as make check-parser does, and gobco.
const (
	goyacc = "golang.org/x/tools/cmd/goyacc@v0.40.0"
	gobco  = "github.com/rillig/gobco@v1.3.4"
)

// run measures the tree at src and writes the results to out. Its work
// directory, gobco's temporary directories included, is removed after.
func run(src, out string) (err error) {
	began := time.Now()
	work, err := os.MkdirTemp("", "parsercover-")
	if err != nil {
		return err
	}
	defer func() {
		if rerr := os.RemoveAll(work); err == nil {
			err = rerr
		}
	}()
	tree := filepath.Join(work, "tree")
	for _, d := range []string{"go.mod", "nodes", "parser", "tools/yaccmaxdepth"} {
		if err := copyTree(filepath.Join(src, d), filepath.Join(tree, d)); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	// A failed run leaves no results behind for check to read.
	for _, name := range []string{"facts.txt", "summary.txt"} {
		if err := os.Remove(filepath.Join(out, name)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	pdir := filepath.Join(tree, "parser")
	gen, err := command(pdir, nil, "go", "run", goyacc, "-l", "-o", "parser.go", "-p", "pg", "gram.y")
	if err != nil {
		return err
	}
	if _, err := command(tree, nil, "go", "run", "./tools/yaccmaxdepth", "-p", "pg", "parser/parser.go"); err != nil {
		return err
	}
	profile := filepath.Join(work, "cover.out")
	if _, err := command(tree, nil, "go", "test", "-count=1", "-coverprofile="+profile, "./parser"); err != nil {
		return err
	}
	tmp := filepath.Join(work, "tmp")
	if err := os.Mkdir(tmp, 0o755); err != nil {
		return err
	}
	stats := filepath.Join(work, "gobco.json")
	if _, err := command(tree, []string{"TMPDIR=" + tmp}, "go", "run", gobco, "-stats", stats, "./parser"); err != nil {
		return err
	}
	gram, err := os.ReadFile(filepath.Join(pdir, "gram.y"))
	if err != nil {
		return err
	}
	rules, err := grammarRules(gram)
	if err != nil {
		return err
	}
	srcs := map[string]*source{}
	names, err := filepath.Glob(filepath.Join(pdir, "*.go"))
	if err != nil {
		return err
	}
	for _, name := range names {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		s, err := loadSource(name, rules)
		if err != nil {
			return err
		}
		srcs["parser/"+filepath.Base(name)] = s
	}
	stmts, err := profileFacts(profile, srcs)
	if err != nil {
		return err
	}
	conds, never, err := conditionFacts(stats, srcs, rules)
	if err != nil {
		return err
	}
	if err := writeFacts(filepath.Join(out, "facts.txt"), append(stmts, conds...)); err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(out, "summary.txt"))
	if err != nil {
		return err
	}
	summarise(f, stmts, conds, never, deadRules(gen))
	if err := f.Close(); err != nil {
		return err
	}
	b, err := os.ReadFile(filepath.Join(out, "summary.txt"))
	if err != nil {
		return err
	}
	fmt.Printf("%s(%s, written to %s)\n", headLines(string(b), 16), time.Since(began).Round(time.Second), out)
	return nil
}

// command runs name in dir and returns its standard output, failing with
// its output when it fails.
func command(dir string, env []string, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	var stdout, all strings.Builder
	cmd.Stdout = io.MultiWriter(&stdout, &all)
	cmd.Stderr = &all
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s %s: %v\n%s", name, strings.Join(args, " "), err, headLines(all.String(), 40))
	}
	return stdout.String(), nil
}

func headLines(s string, n int) string {
	lines := strings.SplitAfter(s, "\n")
	if len(lines) > n {
		lines = append(lines[:n], "...\n")
	}
	return strings.Join(lines, "")
}

// deadRules returns the rules goyacc reports as never reduced in any state.
func deadRules(goyaccOut string) []string {
	var dead []string
	for _, l := range strings.Split(goyaccOut, "\n") {
		if strings.HasPrefix(l, "rule ") && strings.HasSuffix(l, "never reduced") {
			dead = append(dead, strings.Join(strings.Fields(strings.TrimSuffix(strings.TrimPrefix(l, "rule "), "never reduced")), " "))
		}
	}
	return dead
}

func copyTree(src, dst string) error {
	return filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		to := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(to, 0o755)
		}
		// The generated files are generated again.
		if base := filepath.Base(p); filepath.Base(filepath.Dir(p)) == "parser" && (base == "parser.go" || base == "y.output") {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
			return err
		}
		return os.WriteFile(to, b, 0o644)
	})
}

func writeFacts(path string, facts []fact) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	for _, ft := range facts {
		c := 0
		if ft.covered {
			c = 1
		}
		fmt.Fprintf(w, "%d\t%d\t%s\t%s\n", c, ft.stmts, ft.file, ft.key)
	}
	if err := w.Flush(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func readFacts(path string) (map[string]fact, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	facts := map[string]fact{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		parts := strings.SplitN(sc.Text(), "\t", 4)
		if len(parts) != 4 {
			return nil, fmt.Errorf("%s: bad line %q", path, sc.Text())
		}
		n, _ := strconv.Atoi(parts[1])
		facts[parts[3]] = fact{key: parts[3], file: parts[2], stmts: n, covered: parts[0] == "1"}
	}
	return facts, sc.Err()
}

type tally struct{ hit, all int }

func (t tally) String() string {
	if t.all == 0 {
		return "0/0"
	}
	return fmt.Sprintf("%d/%d (%.1f%%)", t.hit, t.all, 100*float64(t.hit)/float64(t.all))
}

func summarise(w io.Writer, stmts, conds []fact, never, dead []string) {
	per := func(facts []fact, weight func(fact) int) (map[string]tally, tally) {
		m := map[string]tally{}
		var total tally
		for _, f := range facts {
			t := m[f.file]
			n := weight(f)
			t.all += n
			total.all += n
			if f.covered {
				t.hit += n
				total.hit += n
			}
			m[f.file] = t
		}
		return m, total
	}
	files := func(m map[string]tally) []string {
		var fs []string
		for f := range m {
			fs = append(fs, f)
		}
		sort.Strings(fs)
		return fs
	}
	sm, st := per(stmts, func(f fact) int { return f.stmts })
	fmt.Fprintln(w, "statements (go test -coverprofile; parser.go generated with goyacc -l)")
	for _, f := range files(sm) {
		fmt.Fprintf(w, "  %-22s %s\n", f, sm[f])
	}
	fmt.Fprintf(w, "  %-22s %s\n", "total", st)
	// A rule's case condition has two outcomes in gobco's count; its false
	// one is always reached.
	var outcomes []fact
	var actions int
	for _, c := range conds {
		outcomes = append(outcomes, c)
		if strings.HasPrefix(c.key, "R ") {
			actions++
			outcomes = append(outcomes, fact{file: c.file, covered: true})
		}
	}
	cm, ct := per(outcomes, func(fact) int { return 1 })
	fmt.Fprintln(w, "conditions (gobco; each condition's true and false outcomes)")
	for _, f := range files(cm) {
		fmt.Fprintf(w, "  %-22s %s\n", f, cm[f])
	}
	fmt.Fprintf(w, "  %-22s %s\n", "total", ct)
	fmt.Fprintf(w, "rule actions never reduced: %d of %d\n", len(never), actions)
	fmt.Fprintf(w, "rules goyacc never reduces in any state: %d\n", len(dead))
	for _, r := range dead {
		fmt.Fprintf(w, "  %s\n", r)
	}
	fmt.Fprintln(w, "rule actions never reduced, by rule:")
	for _, r := range never {
		fmt.Fprintf(w, "  %s\n", r)
	}
}

// check writes each fact the head does not reach that the base reached or
// did not have, and fails when there is one.
func check(basePath, headPath string, w io.Writer) error {
	if basePath == "" || headPath == "" {
		return fmt.Errorf("check needs -base and -head")
	}
	base, err := readFacts(basePath)
	if err != nil {
		return err
	}
	head, err := readFacts(headPath)
	if err != nil {
		return err
	}
	var lost, added []string
	for k, h := range head {
		if h.covered {
			continue
		}
		b, ok := base[k]
		switch {
		case !ok:
			added = append(added, k)
		case b.covered:
			lost = append(lost, k)
		}
	}
	sort.Strings(lost)
	sort.Strings(added)
	for _, k := range lost {
		fmt.Fprintf(w, "no longer reached: %s\n", k)
	}
	for _, k := range added {
		fmt.Fprintf(w, "new and not reached: %s\n", k)
	}
	if n := len(lost) + len(added); n > 0 {
		return fmt.Errorf("%d facts the base reached or did not have are not reached (%d no longer, %d new)", n, len(lost), len(added))
	}
	fmt.Fprintf(w, "coverage check passed: every fact the head does not reach, the base did not reach either (%d facts)\n", len(head))
	return nil
}
