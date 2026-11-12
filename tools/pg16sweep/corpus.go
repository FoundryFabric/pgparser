package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math/rand/v2"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/pgplex/pgparser/parser/pgregress"
)

// item is one corpus statement: a regression statement, or a mutation of
// one when Edit is set.
type item struct {
	ID   int     `json:"id"`
	File string  `json:"file"`
	Stmt int     `json:"stmt"`
	Sum  string  `json:"sum"`
	Op   string  `json:"op,omitempty"`
	Edit *[3]any `json:"edit,omitempty"`
	SQL  string  `json:"sql"`
}

// probeFile is the File of a hand-written probe statement (probes.sql, one
// statement a line), which the test data carries as text.
const probeFile = "probes.sql"

// stmtSum is the fnv-1a 32 hash of a regression statement's text, in hex.
// The test checks it so that a change to the extraction is seen.
func stmtSum(sql string) string {
	h := fnv.New32a()
	h.Write([]byte(sql))
	return fmt.Sprintf("%08x", h.Sum32())
}

type tok struct {
	start, end int
	word       bool // an identifier, keyword or quoted identifier
}

// tokenize splits sql into tokens roughly as scan.l does; comments and
// whitespace are not tokens. It is only used to place mutations.
func tokenize(s string) []tok {
	var toks []tok
	n := len(s)
	isWord := func(c byte) bool {
		return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '$' || c >= 0x80
	}
	opChars := "+-*/<>=~!@#%^&|`?"
	quoted := func(i int, q byte, backslash bool) int {
		i++
		for i < n {
			switch {
			case backslash && s[i] == '\\':
				i += 2
				continue
			case s[i] == q:
				if i+1 < n && s[i+1] == q {
					i += 2
					continue
				}
				return i + 1
			}
			i++
		}
		return n
	}
	i := 0
	for i < n {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v':
			i++
		case c == '-' && i+1 < n && s[i+1] == '-':
			for i < n && s[i] != '\n' {
				i++
			}
		case c == '/' && i+1 < n && s[i+1] == '*':
			depth := 0
			for i < n {
				if i+1 < n && s[i] == '/' && s[i+1] == '*' {
					depth++
					i += 2
				} else if i+1 < n && s[i] == '*' && s[i+1] == '/' {
					depth--
					i += 2
					if depth == 0 {
						break
					}
				} else {
					i++
				}
			}
		case c == '\'':
			e := quoted(i, '\'', false)
			toks = append(toks, tok{i, e, false})
			i = e
		case c == '"':
			e := quoted(i, '"', false)
			toks = append(toks, tok{i, e, true})
			i = e
		case (c == 'E' || c == 'e') && i+1 < n && s[i+1] == '\'':
			e := quoted(i+1, '\'', true)
			toks = append(toks, tok{i, e, false})
			i = e
		case strings.IndexByte("BbXxNn", c) >= 0 && i+1 < n && s[i+1] == '\'':
			e := quoted(i+1, '\'', false)
			toks = append(toks, tok{i, e, false})
			i = e
		case (c == 'U' || c == 'u') && i+2 < n && s[i+1] == '&' && (s[i+2] == '\'' || s[i+2] == '"'):
			e := quoted(i+2, s[i+2], false)
			toks = append(toks, tok{i, e, s[i+2] == '"'})
			i = e
		case c == '$':
			j := i + 1
			for j < n && (isWord(s[j]) && s[j] != '$') && !(s[j] >= '0' && s[j] <= '9' && j == i+1) {
				j++
			}
			if j < n && s[j] == '$' {
				tag := s[i : j+1]
				k := strings.Index(s[j+1:], tag)
				e := n
				if k >= 0 {
					e = j + 1 + k + len(tag)
				}
				toks = append(toks, tok{i, e, false})
				i = e
				continue
			}
			j = i + 1
			for j < n && s[j] >= '0' && s[j] <= '9' {
				j++
			}
			toks = append(toks, tok{i, j, false})
			i = j
		case c >= '0' && c <= '9' || c == '.' && i+1 < n && s[i+1] >= '0' && s[i+1] <= '9':
			j := i + 1
			for j < n && (isWord(s[j]) || s[j] == '.' && !(j+1 < n && s[j+1] == '.')) {
				j++
			}
			toks = append(toks, tok{i, j, false})
			i = j
		case isWord(c):
			j := i + 1
			for j < n && isWord(s[j]) {
				j++
			}
			toks = append(toks, tok{i, j, true})
			i = j
		case c == ':' && i+1 < n && (s[i+1] == ':' || s[i+1] == '='):
			toks = append(toks, tok{i, i + 2, false})
			i += 2
		case strings.IndexByte(opChars, c) >= 0:
			j := i + 1
			for j < n && strings.IndexByte(opChars, s[j]) >= 0 && !(s[j] == '-' && j+1 < n && s[j+1] == '-') && !(s[j] == '/' && j+1 < n && s[j+1] == '*') {
				j++
			}
			toks = append(toks, tok{i, j, false})
			i = j
		default:
			_, w := utf8.DecodeRuneInString(s[i:])
			toks = append(toks, tok{i, i + w, false})
			i += w
		}
	}
	return toks
}

// pg16Reserved is PostgreSQL 16's reserved keywords (kwlist.h,
// RESERVED_KEYWORD).
var pg16Reserved = strings.Fields(`all analyse analyze and any array as asc asymmetric both case cast check
collate column constraint create current_catalog current_date current_role current_time
current_timestamp current_user default deferrable desc distinct do else end except false fetch
for foreign from grant group having in initially intersect into lateral leading limit localtime
localtimestamp not null offset on only or order placing primary references returning select
session_user some symmetric system_user table then to trailing true union unique user using
variadic when where window with`)

var mutationOps = []string{"delete", "duplicate", "swap", "insert", "dotted", "typmod", "array", "reserved"}

var insertions = []string{".", "[1]", "[1:2]", ".*", " * ", " ( ", " ) ", " , "}

type edit struct {
	op       string
	at, del  int
	ins      string
	resulted string
}

// mutate returns up to k distinct mutations of sql, chosen by a generator
// seeded from the statement's file and index, so the corpus is the same on
// every run.
func mutate(file string, idx int, sql string, k int) []edit {
	toks := tokenize(sql)
	if len(toks) == 0 {
		return nil
	}
	h := fnv.New64a()
	h.Write([]byte(file))
	r := rand.New(rand.NewPCG(h.Sum64(), uint64(idx)))
	var words, afterCast []int
	for i, t := range toks {
		if t.word {
			words = append(words, i)
			if i > 0 && sql[toks[i-1].start:toks[i-1].end] == "::" {
				afterCast = append(afterCast, i)
			}
		}
	}
	pickWord := func(preferCast bool) (int, bool) {
		if preferCast && len(afterCast) > 0 && r.IntN(2) == 0 {
			return afterCast[r.IntN(len(afterCast))], true
		}
		if len(words) == 0 {
			return 0, false
		}
		return words[r.IntN(len(words))], true
	}
	seen := map[string]bool{sql: true}
	var out []edit
	for attempt := 0; len(out) < k && attempt < 8*k; attempt++ {
		op := mutationOps[r.IntN(len(mutationOps))]
		ti := r.IntN(len(toks))
		t := toks[ti]
		var e edit
		e.op = op
		switch op {
		case "delete":
			e.at, e.del = t.start, t.end-t.start
		case "duplicate":
			e.at, e.ins = t.end, " "+sql[t.start:t.end]
		case "swap":
			if ti+1 >= len(toks) {
				continue
			}
			u := toks[ti+1]
			e.at, e.del = t.start, u.end-t.start
			e.ins = sql[u.start:u.end] + sql[t.end:u.start] + sql[t.start:t.end]
		case "insert":
			ins := insertions[r.IntN(len(insertions))]
			e.op = "insert" + strings.TrimSpace(ins)
			e.at, e.ins = t.end, ins
		case "dotted", "typmod", "array":
			wi, ok := pickWord(op != "dotted")
			if !ok {
				continue
			}
			e.at = toks[wi].end
			e.ins = map[string]string{"dotted": ".x", "typmod": "(3)", "array": "[]"}[op]
		case "reserved":
			wi, ok := pickWord(false)
			if !ok {
				continue
			}
			w := toks[wi]
			kw := pg16Reserved[r.IntN(len(pg16Reserved))]
			if strings.EqualFold(sql[w.start:w.end], kw) {
				continue
			}
			e.at, e.del, e.ins = w.start, w.end-w.start, kw
		}
		e.resulted = sql[:e.at] + e.ins + sql[e.at+e.del:]
		if seen[e.resulted] || strings.TrimSpace(e.resulted) == "" {
			continue
		}
		seen[e.resulted] = true
		out = append(out, e)
	}
	return out
}

// runCorpus writes the corpus: every regression statement extract.go finds
// and k mutations of each. Statements the oracle cannot classify are
// counted and left out.
func runCorpus(sqlDir, probes, fixtures, out string, k int) error {
	files, err := filepath.Glob(filepath.Join(sqlDir, "*.sql"))
	if err != nil {
		return err
	}
	sort.Strings(files)
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	excluded := map[string]int{}
	id, stmts, muts := 0, 0, 0
	for _, path := range files {
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		base := filepath.Base(path)
		for i, st := range pgregress.ExtractStatements(base, content) {
			switch {
			case st.HasPsqlVar:
				excluded["psql variable"]++
				continue
			case !utf8.ValidString(st.SQL):
				excluded["invalid UTF-8"]++
				continue
			case strings.ContainsRune(st.SQL, 0):
				excluded["NUL byte"]++
				continue
			case strings.TrimSpace(st.SQL) == "":
				excluded["empty"]++
				continue
			}
			sum := stmtSum(st.SQL)
			if err := enc.Encode(item{ID: id, File: base, Stmt: i, Sum: sum, SQL: st.SQL}); err != nil {
				return err
			}
			id++
			stmts++
			for _, e := range mutate(base, i, st.SQL, k) {
				ed := [3]any{e.at, e.del, e.ins}
				if err := enc.Encode(item{ID: id, File: base, Stmt: i, Sum: sum, Op: e.op, Edit: &ed, SQL: e.resulted}); err != nil {
					return err
				}
				id++
				muts++
			}
		}
	}
	np := 0
	if probes != "" {
		content, err := os.ReadFile(probes)
		if err != nil {
			return err
		}
		for _, line := range strings.Split(string(content), "\n") {
			if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "--") {
				continue
			}
			if err := enc.Encode(item{ID: id, File: probeFile, SQL: line}); err != nil {
				return err
			}
			id++
			np++
		}
	}
	if fixtures != "" {
		paths, err := filepath.Glob(filepath.Join(fixtures, "*.sql"))
		if err != nil {
			return err
		}
		sort.Strings(paths)
		for _, path := range paths {
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for _, st := range pgregress.ExtractStatements(filepath.Base(path), content) {
				if strings.TrimSpace(stripComments(st.SQL)) == "" {
					continue
				}
				if err := enc.Encode(item{ID: id, File: probeFile, SQL: st.SQL}); err != nil {
					return err
				}
				id++
				np++
			}
		}
	}
	fmt.Fprintf(os.Stderr, "corpus: %d probes, %d regression statements, %d mutations, excluded %v\n", np, stmts, muts, excluded)
	return w.Flush()
}

func readCorpus(path string) ([]item, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var items []item
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<26)
	for sc.Scan() {
		var it item
		if err := json.Unmarshal(sc.Bytes(), &it); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, sc.Err()
}
