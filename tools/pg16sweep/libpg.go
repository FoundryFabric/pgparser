package main

import (
	"errors"
	"fmt"
	"os"
	"runtime/debug"
	"sort"
	"strings"
	"time"

	pg_query "github.com/pganalyze/pg_query_go/v5"
	pgqparser "github.com/pganalyze/pg_query_go/v5/parser"
	"github.com/pgplex/pgparser/nodes"
)

// classOraclesDisagree holds the statements PostgreSQL 16 and libpg_query
// give a different verdict on. They are not counted against the fork.
const classOraclesDisagree = "oracles-disagree"

// parseLibpg parses sql with libpg_query. Its errors carry no SQLSTATE, and
// their cursor position is 1-based and in characters, as PostgreSQL's.
func parseLibpg(id int, sql string) (r result, tree *pg_query.ParseResult) {
	r = result{ID: id, Pos: -1}
	defer func() {
		if p := recover(); p != nil {
			r.Panic = fmt.Sprint(p) + "\n" + string(debug.Stack())
			tree = nil
		}
	}()
	t0 := time.Now()
	tree, err := pg_query.Parse(sql)
	r.Micros = time.Since(t0).Microseconds()
	var pe *pgqparser.Error
	switch {
	case err == nil:
		r.OK = true
	case errors.As(err, &pe):
		r.Msg, r.Pos = pe.Message, byteOffset(sql, pe.Cursorpos)
	default:
		r.Msg = err.Error()
	}
	return r, tree
}

// oracleKind is how PostgreSQL 16's verdict on a statement compares with
// libpg_query's.
func oracleKind(pg, lq result) string {
	switch {
	case pg.OK != lq.OK:
		return classOraclesDisagree
	case pg.OK:
		return "both accept"
	case pg.Msg != lq.Msg:
		return "both refuse, different message"
	case pg.Pos != lq.Pos:
		return "both refuse, different position"
	}
	return "both refuse alike"
}

type treeFinding struct {
	it    item
	first treeMismatch
	n     int
}

// runLibpg compares the fork with libpg_query over the corpus: verdicts as
// compare does with PostgreSQL 16's, and, where both accept, the trees. With
// pgPath it also compares libpg_query's verdicts with PostgreSQL 16's, and
// leaves out of the fork's counts the statements those two disagree on.
func runLibpg(corpusPath, pgPath, nodesDir, out, reportPath string, examples int) error {
	items, err := readCorpus(corpusPath)
	if err != nil {
		return err
	}
	if err := loadForkEnums(nodesDir); err != nil {
		return err
	}
	var pgs map[int]result
	if pgPath != "" {
		if pgs, err = readResults(pgPath); err != nil {
			return err
		}
	}
	began := time.Now()
	results := make([]result, 0, len(items))
	oracles := map[string]int{}
	var apart []finding
	kinds := map[string]int{}
	var findings []finding
	parents := map[string]*finding{}
	byClass := map[string][]treeFinding{}
	var compared, alike int
	for i, it := range items {
		lq, ltree := parseLibpg(it.ID, it.SQL)
		results = append(results, lq)
		ftree, fk := parseTree(it.ID, it.SQL)
		if i%10000 == 0 {
			fmt.Fprintf(os.Stderr, "libpg: %d/%d %s\n", i, len(items), time.Since(began).Round(time.Second))
		}
		counted := true
		if pgs != nil {
			pg, ok := pgs[it.ID]
			if ok && pg.Unclassified == "" {
				k := oracleKind(pg, lq)
				oracles[k]++
				if k == classOraclesDisagree {
					apart = append(apart, finding{it: it, pg: pg, fk: lq, kind: k, class: k})
					counted = false
				}
			}
		}
		// libpg_query gives no SQLSTATE, so the fork's is not compared.
		fkNoCode := fk
		fkNoCode.Code = ""
		f := finding{it: it, pg: lq, fk: fkNoCode, kind: disagreement(lq, fkNoCode)}
		f.fk.Code = fk.Code
		if counted {
			kinds[f.kind]++
		}
		key := it.File + ":" + fmt.Sprint(it.Stmt)
		if counted && f.kind != kindAgree {
			var parent *finding
			if it.Edit != nil {
				parent = parents[key]
			}
			f.class = classify(f, parent)
			findings = append(findings, f)
			if it.Edit == nil && it.File != probeFile {
				p := f
				parents[key] = &p
			}
		}
		if !lq.OK || !fk.OK || ltree == nil || ftree == nil {
			continue
		}
		compared++
		fv, lv := treesOf(ftree, ltree)
		ms := compareTrees(fv, lv)
		if len(ms) == 0 {
			alike++
			continue
		}
		counts := map[string]int{}
		firsts := map[string]treeMismatch{}
		for _, m := range ms {
			c := m.class()
			if counts[c] == 0 {
				firsts[c] = m
			}
			counts[c]++
		}
		for c, n := range counts {
			byClass[c] = append(byClass[c], treeFinding{it: it, first: firsts[c], n: n})
		}
	}
	fmt.Fprintf(os.Stderr, "libpg: %d statements in %s\n", len(items), time.Since(began).Round(time.Millisecond))
	if out != "" {
		if err := writeResults(out, results); err != nil {
			return err
		}
	}
	if pgs != nil {
		printCounts("oracles", oracles)
	}
	printCounts("fork vs libpg_query", kinds)
	fmt.Printf("trees: %d compared, %d alike, %d mismatch classes\n", compared, alike, len(byClass))
	if reportPath == "" {
		return nil
	}
	w, err := os.Create(reportPath)
	if err != nil {
		return err
	}
	defer w.Close()
	if pgs != nil {
		fmt.Fprintf(w, "##### PostgreSQL 16 against libpg_query\n")
		if err := writeClasses(w, apart, "pg", "pg16", "libpg"); err != nil {
			return err
		}
	}
	fmt.Fprintf(w, "##### the fork against libpg_query\n")
	if err := writeClasses(w, findings, "libpg", "libpg", "fork"); err != nil {
		return err
	}
	fmt.Fprintf(w, "##### trees, where both accept: %d compared, %d alike\n", compared, alike)
	writeTreeClasses(w, byClass, examples)
	return nil
}

func printCounts(name string, m map[string]int) {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	for _, k := range ks {
		label := k
		if label == "" {
			label = "agree"
		}
		fmt.Printf("  %s: %-32s %7d\n", name, label, m[k])
	}
}

// writeTreeClasses lists the tree mismatch classes worst kind first, then
// by size, each with its examples shortest statements.
func writeTreeClasses(w *os.File, byClass map[string][]treeFinding, examples int) {
	var classes []string
	for c := range byClass {
		classes = append(classes, c)
	}
	kindOf := func(c string) int { return treeKindRank[strings.SplitN(c, " ", 2)[0]] }
	sort.Slice(classes, func(i, j int) bool {
		a, b := classes[i], classes[j]
		if kindOf(a) != kindOf(b) {
			return kindOf(a) < kindOf(b)
		}
		if len(byClass[a]) != len(byClass[b]) {
			return len(byClass[a]) > len(byClass[b])
		}
		return a < b
	})
	for _, c := range classes {
		fs := byClass[c]
		reg, probes := 0, 0
		for _, f := range fs {
			switch {
			case f.it.File == probeFile:
				probes++
			case f.it.Edit == nil:
				reg++
			}
		}
		fmt.Fprintf(w, "=== tree %q: %d (regression %d, mutations %d, probes %d)\n", c, len(fs), reg, len(fs)-reg-probes, probes)
		sort.Slice(fs, func(i, j int) bool { return len(fs[i].it.SQL) < len(fs[j].it.SQL) })
		for i := 0; i < len(fs) && i < examples; i++ {
			fmt.Fprintf(w, "       [%d %s:%d %s] %s\n         fork: %s  libpg: %s  (%d in the statement)\n", fs[i].it.ID, fs[i].it.File, fs[i].it.Stmt, fs[i].it.Op, oneLine(fs[i].it.SQL), shortValue(fs[i].first.fork), shortValue(fs[i].first.libpg), fs[i].n)
		}
	}
}

func oneLine(sql string) string {
	sql = strings.Join(strings.Fields(sql), " ")
	if len(sql) > 220 {
		sql = sql[:220] + "..."
	}
	return sql
}

// parseTree is parseOne keeping the tree.
func parseTree(id int, sql string) (*nodes.List, result) {
	var tree *nodes.List
	r := parseWith(id, sql, func(s string) error {
		t, err := parseFork(s)
		tree = t
		return err
	})
	if !r.OK {
		return nil, r
	}
	return tree, r
}
