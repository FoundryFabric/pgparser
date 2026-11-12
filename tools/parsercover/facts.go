package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// fact is one thing the tests either reach or do not: a statement block of
// the coverage profile, or one outcome of a condition gobco counts. Its key
// names it without line or rule numbers, so that the same code has the same
// key in another version of the grammar.
type fact struct {
	key     string
	file    string
	stmts   int // a block's statements; 0 for a condition outcome
	covered bool
}

// span is a stretch of a source file that names the code in it: a function,
// or a grammar action, the case of the parser's action switch for one rule.
type span struct {
	start, end int
	name       string
}

// source is one parsed file of the generated package.
type source struct {
	src   []byte
	file  *token.File
	spans []span
}

// loadSource parses path and records its functions and, in the parser
// goyacc generated, the actions of the rules.
func loadSource(path string, rules []string) (*source, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, src, 0)
	if err != nil {
		return nil, err
	}
	s := &source{src: src, file: fset.File(f.Pos())}
	off := func(p token.Pos) int { return s.file.Offset(p) }
	ast.Inspect(f, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncDecl:
			name := n.Name.Name
			if n.Recv != nil && len(n.Recv.List) == 1 {
				name = types(n.Recv.List[0].Type) + "." + name
			}
			s.spans = append(s.spans, span{off(n.Pos()), off(n.End()), "func " + name})
		case *ast.SwitchStmt:
			if id, ok := n.Tag.(*ast.Ident); !ok || id.Name != "pgnt" {
				return true
			}
			for _, c := range n.Body.List {
				cc := c.(*ast.CaseClause)
				if len(cc.List) != 1 {
					continue
				}
				lit, ok := cc.List[0].(*ast.BasicLit)
				if !ok {
					continue
				}
				r, err := strconv.Atoi(lit.Value)
				if err != nil || r <= 0 || r >= len(rules) {
					continue
				}
				s.spans = append(s.spans, span{off(cc.Pos()), off(cc.End()), "rule " + rules[r]})
			}
		}
		return true
	})
	return s, nil
}

func types(e ast.Expr) string {
	switch e := e.(type) {
	case *ast.StarExpr:
		return "*" + types(e.X)
	case *ast.Ident:
		return e.Name
	}
	return "?"
}

// offset turns a 1-based line and byte column into a file offset.
func (s *source) offset(line, col int) int {
	return s.file.Offset(s.file.LineStart(line)) + col - 1
}

// context names the innermost span holding offset o.
func (s *source) context(o int) string {
	best := span{start: -1, end: 1 << 62}
	for _, sp := range s.spans {
		if sp.start <= o && o < sp.end && sp.end-sp.start < best.end-best.start {
			best = sp
		}
	}
	if best.start < 0 {
		return "top level"
	}
	return best.name
}

func normalise(b []byte) string {
	return strings.Join(strings.Fields(string(b)), " ")
}

// numbered gives facts with the same key an occurrence suffix, in the order
// they appear, so that identical code in one function or action stays apart.
func numbered(facts []fact) []fact {
	seen := map[string]int{}
	for i := range facts {
		k := facts[i].key
		facts[i].key = fmt.Sprintf("%s #%d", k, seen[k])
		seen[k]++
	}
	return facts
}

var profileLine = regexp.MustCompile(`^(.+):(\d+)\.(\d+),(\d+)\.(\d+) (\d+) (\d+)$`)

// profileFacts reads a coverage profile of the parser package.
func profileFacts(path string, srcs map[string]*source) ([]fact, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var facts []fact
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		m := profileLine.FindStringSubmatch(sc.Text())
		if m == nil {
			continue
		}
		file := "parser/" + filepath.Base(m[1])
		s := srcs[file]
		if s == nil {
			return nil, fmt.Errorf("%s: no source for %s", path, m[1])
		}
		n := atoi(m[2], m[3], m[4], m[5], m[6], m[7])
		a, b := s.offset(n[0], n[1]), s.offset(n[2], n[3])
		facts = append(facts, fact{
			key:     fmt.Sprintf("S %s: %s: %s", file, s.context(a), normalise(s.src[a:b])),
			file:    file,
			stmts:   n[4],
			covered: n[5] > 0,
		})
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(facts, func(i, j int) bool { return facts[i].key < facts[j].key })
	return numbered(facts), nil
}

func atoi(ss ...string) []int {
	out := make([]int, len(ss))
	for i, s := range ss {
		out[i], _ = strconv.Atoi(s)
	}
	return out
}

// gobcoCondition is one condition of gobco's -stats file.
type gobcoCondition struct {
	Start      string
	Code       string
	TrueCount  int
	FalseCount int
}

var reducedCode = regexp.MustCompile(`^pgnt == (\d+)$`)

// conditionFacts reads gobco's -stats file. The condition of a rule's case
// in the action switch, pgnt == N, becomes the fact that the rule is
// reduced; it has no false outcome worth a fact, since another rule's
// reduction always makes it false.
func conditionFacts(path string, srcs map[string]*source, rules []string) ([]fact, []string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	var conds []gobcoCondition
	if err := json.Unmarshal(b, &conds); err != nil {
		return nil, nil, err
	}
	var facts []fact
	var never []string
	for _, c := range conds {
		parts := strings.Split(c.Start, ":")
		if len(parts) != 3 {
			return nil, nil, fmt.Errorf("%s: bad start %q", path, c.Start)
		}
		file := parts[0]
		s := srcs[file]
		if s == nil {
			return nil, nil, fmt.Errorf("%s: no source for %s", path, file)
		}
		if m := reducedCode.FindStringSubmatch(c.Code); m != nil && file == "parser/parser.go" {
			r, _ := strconv.Atoi(m[1])
			if r <= 0 || r >= len(rules) {
				return nil, nil, fmt.Errorf("%s: no rule %d", path, r)
			}
			facts = append(facts, fact{key: "R " + rules[r], file: file, covered: c.TrueCount > 0})
			if c.TrueCount == 0 {
				never = append(never, rules[r])
			}
			continue
		}
		n := atoi(parts[1], parts[2])
		ctx := s.context(s.offset(n[0], n[1]))
		code := normalise([]byte(c.Code))
		facts = append(facts,
			fact{key: fmt.Sprintf("C %s: %s: %s is true", file, ctx, code), file: file, covered: c.TrueCount > 0},
			fact{key: fmt.Sprintf("C %s: %s: %s is false", file, ctx, code), file: file, covered: c.FalseCount > 0})
	}
	sort.SliceStable(facts, func(i, j int) bool { return facts[i].key < facts[j].key })
	sort.Strings(never)
	return numbered(facts), never, nil
}
