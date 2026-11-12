package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
	"regexp"
	"sort"
	"strings"
)

// disagreement kinds, worst first.
const (
	kindAgree    = ""
	kindCrash    = "crash"    // a panic, fatal error or timeout
	kindAccepts  = "accepts"  // the parser accepts what PostgreSQL 16 refuses
	kindRefuses  = "refuses"  // the parser refuses what PostgreSQL 16 accepts
	kindMessage  = "message"  // both refuse, with a different SQLSTATE or message
	kindPosition = "position" // both refuse with the same message at a different place
)

func disagreement(pg, fk result) string {
	switch {
	case fk.Panic != "" || fk.Fatal || fk.Timeout:
		return kindCrash
	case pg.OK && fk.OK:
		return kindAgree
	case fk.OK:
		return kindAccepts
	case pg.OK:
		return kindRefuses
	case pg.Code != fk.Code || pg.Msg != fk.Msg:
		return kindMessage
	case pg.Pos >= 0 && pg.Pos != fk.Pos:
		return kindPosition
	}
	return kindAgree
}

func describe(r result) string {
	switch {
	case r.Panic != "":
		return "panic: " + strings.SplitN(r.Panic, "\n", 2)[0]
	case r.Fatal:
		return "fatal error"
	case r.Timeout:
		return "timeout"
	case r.OK:
		return "ok"
	}
	return fmt.Sprintf("%s %s @%d", r.Code, r.Msg, r.Pos)
}

var quotedTok = regexp.MustCompile(`"(?:[^"]|"")*"`)

// signature groups disagreements for reading: kind, both messages with the
// token kept, and the statement's first two words.
func signature(kind string, it item, pg, fk result, pgName, fkName string) string {
	lead := strings.Fields(strings.ToLower(stripComments(it.SQL)))
	if len(lead) > 2 {
		lead = lead[:2]
	}
	return kind + " | " + pgName + ": " + msgShape(pg) + " | " + fkName + ": " + msgShape(fk) + " | " + strings.Join(lead, " ")
}

func msgShape(r result) string {
	if r.OK {
		return "ok"
	}
	if r.Panic != "" || r.Fatal || r.Timeout {
		return describe(r)
	}
	return r.Code + " " + r.Msg
}

var commentRE = regexp.MustCompile(`(?s)--[^\n]*|/\*.*?\*/`)

func stripComments(s string) string { return commentRE.ReplaceAllString(s, " ") }

type row struct {
	SQL   string  `json:"sql,omitempty"`
	File  string  `json:"file,omitempty"`
	Stmt  int     `json:"stmt,omitempty"`
	Sum   string  `json:"sum,omitempty"`
	Edit  *[3]any `json:"edit,omitempty"`
	Code  string  `json:"code,omitempty"`
	Msg   string  `json:"msg,omitempty"`
	Pos   *int    `json:"pos,omitempty"`
	Class string  `json:"class,omitempty"`
}

type finding struct {
	it        item
	pg, fk    result
	kind      string
	class     string
	otherKind string
	hasOther  bool
}

func runCompare(corpusPath, pgPath, forkPath, otherPath, testdataPath, reportPath string, sample int) error {
	items, err := readCorpus(corpusPath)
	if err != nil {
		return err
	}
	pgs, err := readResults(pgPath)
	if err != nil {
		return err
	}
	fks, err := readResults(forkPath)
	if err != nil {
		return err
	}
	var others map[int]result
	if otherPath != "" {
		if others, err = readResults(otherPath); err != nil {
			return err
		}
	}
	type tally struct{ stmts, agree, unclassified int }
	var regress, muts, probes tally
	kinds := map[string][3]int{}
	otherKinds := map[string][3]int{}
	var findings []finding
	var rows []row
	var slowest result
	parents := map[string]*finding{}
	for _, it := range items {
		pg, fk := pgs[it.ID], fks[it.ID]
		t := &regress
		side := 0
		switch {
		case it.File == probeFile:
			t, side = &probes, 2
		case it.Edit != nil:
			t, side = &muts, 1
		}
		t.stmts++
		if pg.Unclassified != "" {
			t.unclassified++
			continue
		}
		if fk.Micros > slowest.Micros {
			slowest = fk
		}
		f := finding{it: it, pg: pg, fk: fk, kind: disagreement(pg, fk)}
		c := kinds[f.kind]
		c[side]++
		kinds[f.kind] = c
		if others != nil {
			f.hasOther = true
			f.otherKind = disagreement(pg, others[it.ID])
			c := otherKinds[f.otherKind]
			c[side]++
			otherKinds[f.otherKind] = c
		}
		key := it.File + ":" + fmt.Sprint(it.Stmt)
		if f.kind != kindAgree {
			var parent *finding
			if it.Edit != nil {
				parent = parents[key]
			}
			f.class = classify(f, parent)
			findings = append(findings, f)
		} else {
			t.agree++
		}
		if it.Edit == nil && it.File != probeFile && f.kind != kindAgree {
			p := f
			parents[key] = &p
		}
		// Every regression statement and probe is kept, and of the
		// mutations every disagreement, every one PostgreSQL 16 accepts or
		// refuses with something other than a syntax error, and a sample
		// of the rest.
		keep := it.Edit == nil || f.kind != kindAgree || (f.hasOther && f.otherKind != kindAgree) ||
			pg.OK || !strings.HasPrefix(pg.Msg, "syntax error") || keepSample(it.ID, sample)
		if keep && f.kind != kindCrash {
			r := row{File: it.File, Stmt: it.Stmt, Sum: it.Sum, Edit: it.Edit, Class: f.class}
			if it.Edit != nil {
				// The statement's own row checks its hash.
				r.Sum = ""
			}
			if it.File == probeFile {
				r = row{SQL: it.SQL, Class: f.class}
			}
			if !pg.OK {
				r.Code, r.Msg = pg.Code, pg.Msg
				p := pg.Pos
				r.Pos = &p
			}
			rows = append(rows, r)
		}
	}
	fmt.Printf("regression: %d statements, %d unclassified by the oracle\n", regress.stmts, regress.unclassified)
	fmt.Printf("mutations:  %d statements, %d unclassified by the oracle\n", muts.stmts, muts.unclassified)
	fmt.Printf("probes:     %d statements, %d unclassified by the oracle\n", probes.stmts, probes.unclassified)
	printKinds := func(name string, m map[string][3]int) {
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
			fmt.Printf("  %s %-9s regression %6d  mutations %6d  probes %4d\n", name, label, m[k][0], m[k][1], m[k][2])
		}
	}
	printKinds("fork ", kinds)
	if others != nil {
		printKinds("other", otherKinds)
	}
	fmt.Printf("slowest fork parse: id %d, %dus\n", slowest.ID, slowest.Micros)
	if reportPath != "" {
		if err := writeReport(reportPath, findings); err != nil {
			return err
		}
		if err := writeFindings(reportPath+".jsonl", findings); err != nil {
			return err
		}
	}
	if testdataPath != "" {
		f, err := os.Create(testdataPath)
		if err != nil {
			return err
		}
		defer f.Close()
		w := bufio.NewWriter(f)
		enc := json.NewEncoder(w)
		enc.SetEscapeHTML(false)
		for _, r := range rows {
			if err := enc.Encode(r); err != nil {
				return err
			}
		}
		fmt.Printf("test data: %d rows\n", len(rows))
		return w.Flush()
	}
	return nil
}

func keepSample(id, sample int) bool {
	h := fnv.New32a()
	fmt.Fprint(h, id)
	return h.Sum32()%uint32(sample) == 0
}

// writeReport lists the disagreements by class, then by signature, with the
// shortest examples first.
func writeReport(path string, findings []finding) error {
	w, err := os.Create(path)
	if err != nil {
		return err
	}
	defer w.Close()
	return writeClasses(w, findings, "pg", "pg16", "fork")
}

// writeClasses is writeReport's listing, with the names its two parsers'
// verdicts are shown under (the first in signatures, then in examples).
func writeClasses(w *os.File, findings []finding, pgSig, pgName, fkName string) error {
	byClass := map[string][]finding{}
	for _, f := range findings {
		byClass[f.class] = append(byClass[f.class], f)
	}
	var classes []string
	for c := range byClass {
		classes = append(classes, c)
	}
	sort.Slice(classes, func(i, j int) bool { return len(byClass[classes[i]]) > len(byClass[classes[j]]) })
	for _, c := range classes {
		fs := byClass[c]
		n15 := 0
		reg, probes := 0, 0
		kinds := map[string]int{}
		for _, f := range fs {
			if f.hasOther && f.otherKind == kindAgree {
				n15++
			}
			switch {
			case f.it.File == probeFile:
				probes++
			case f.it.Edit == nil:
				reg++
			}
			kinds[f.kind]++
		}
		fmt.Fprintf(w, "=== class %q: %d (regression %d, mutations %d, probes %d), kinds %v, other branch agrees on %d\n", c, len(fs), reg, len(fs)-reg-probes, probes, kinds, n15)
		bySig := map[string][]finding{}
		for _, f := range fs {
			s := signature(f.kind, f.it, f.pg, f.fk, pgSig, fkName)
			bySig[s] = append(bySig[s], f)
		}
		var sigs []string
		for s := range bySig {
			sigs = append(sigs, s)
		}
		sort.Slice(sigs, func(i, j int) bool {
			if len(bySig[sigs[i]]) != len(bySig[sigs[j]]) {
				return len(bySig[sigs[i]]) > len(bySig[sigs[j]])
			}
			return sigs[i] < sigs[j]
		})
		for _, s := range sigs {
			ex := bySig[s]
			sort.Slice(ex, func(i, j int) bool { return len(ex[i].it.SQL) < len(ex[j].it.SQL) })
			fmt.Fprintf(w, "  -- %d  %s\n", len(ex), s)
			for i := 0; i < len(ex) && i < 3; i++ {
				sql := strings.Join(strings.Fields(ex[i].it.SQL), " ")
				if len(sql) > 220 {
					sql = sql[:220] + "..."
				}
				fmt.Fprintf(w, "       [%d %s:%d %s] %s\n         %s: %s\n         %s: %s\n", ex[i].it.ID, ex[i].it.File, ex[i].it.Stmt, ex[i].it.Op, sql, pgName, describe(ex[i].pg), fkName, describe(ex[i].fk))
			}
		}
	}
	return nil
}

// writeFindings writes one line per disagreement: its corpus id, kind and
// class, and the other parser's kind.
func writeFindings(path string, findings []finding) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	for _, x := range findings {
		fmt.Fprintf(w, "{\"id\":%d,\"kind\":%q,\"class\":%q,\"other\":%q}\n", x.it.ID, x.kind, x.class, x.otherKind)
	}
	return w.Flush()
}
