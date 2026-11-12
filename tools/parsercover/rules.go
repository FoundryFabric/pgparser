package main

import (
	"bytes"
	"fmt"
	"strings"
)

// grammarRules returns the text of each rule of a goyacc grammar, "lhs: rhs"
// or "lhs: <empty>", indexed by goyacc's rule number: the rules in the order
// the rules section holds them, from 1 ($accept is 0).
func grammarRules(src []byte) ([]string, error) {
	sep := []byte("\n%%\n")
	start := bytes.Index(src, sep)
	if start < 0 {
		return nil, fmt.Errorf("no %%%% before the rules")
	}
	body := src[start+len(sep):]
	if end := bytes.Index(body, sep); end >= 0 {
		body = body[:end]
	}
	rules := []string{"$accept"}
	var lhs, pending string
	var rhs []string
	end := func() {
		if pending != "" {
			rhs = append(rhs, pending)
			pending = ""
		}
		text := lhs + ": <empty>"
		if len(rhs) > 0 {
			text = lhs + ": " + strings.Join(rhs, " ")
		}
		rules = append(rules, text)
		rhs = nil
	}
	for i := 0; i < len(body); {
		c := body[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case bytes.HasPrefix(body[i:], []byte("/*")):
			j := bytes.Index(body[i+2:], []byte("*/"))
			if j < 0 {
				return nil, fmt.Errorf("unterminated comment")
			}
			i += j + 4
		case bytes.HasPrefix(body[i:], []byte("//")):
			i = skipLine(body, i)
		case c == '{':
			j, err := skipAction(body, i)
			if err != nil {
				return nil, err
			}
			i = j
		case c == '\'':
			j, err := skipQuoted(body, i)
			if err != nil {
				return nil, err
			}
			if pending != "" {
				rhs = append(rhs, pending)
			}
			pending = string(body[i:j])
			i = j
		case c == ':':
			lhs, pending, rhs = pending, "", nil
			i++
		case c == '|' || c == ';':
			end()
			i++
		case bytes.HasPrefix(body[i:], []byte("%prec")):
			i += len("%prec")
			for i < len(body) && (body[i] == ' ' || body[i] == '\t') {
				i++
			}
			for i < len(body) && isIdent(body[i]) {
				i++
			}
		case isIdent(c):
			j := i
			for j < len(body) && isIdent(body[j]) {
				j++
			}
			if pending != "" {
				rhs = append(rhs, pending)
			}
			pending = string(body[i:j])
			i = j
		default:
			return nil, fmt.Errorf("unexpected %q in the rules", c)
		}
	}
	return rules, nil
}

func isIdent(c byte) bool {
	return c == '_' || c == '.' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

func skipLine(b []byte, i int) int {
	j := bytes.IndexByte(b[i:], '\n')
	if j < 0 {
		return len(b)
	}
	return i + j
}

// skipQuoted returns the offset after the Go or grammar literal at b[i]:
// 'c', "s" or `s`.
func skipQuoted(b []byte, i int) (int, error) {
	q := b[i]
	for j := i + 1; j < len(b); j++ {
		switch {
		case b[j] == '\\' && q != '`':
			j++
		case b[j] == q:
			return j + 1, nil
		}
	}
	return 0, fmt.Errorf("unterminated %c literal", q)
}

// skipAction returns the offset after the action whose { is at b[i].
func skipAction(b []byte, i int) (int, error) {
	depth := 0
	for j := i; j < len(b); {
		switch {
		case b[j] == '"' || b[j] == '`' || b[j] == '\'':
			k, err := skipQuoted(b, j)
			if err != nil {
				return 0, err
			}
			j = k
			continue
		case bytes.HasPrefix(b[j:], []byte("//")):
			j = skipLine(b, j)
			continue
		case bytes.HasPrefix(b[j:], []byte("/*")):
			k := bytes.Index(b[j+2:], []byte("*/"))
			if k < 0 {
				return 0, fmt.Errorf("unterminated comment")
			}
			j += k + 4
			continue
		case b[j] == '{':
			depth++
		case b[j] == '}':
			depth--
			if depth == 0 {
				return j + 1, nil
			}
		}
		j++
	}
	return 0, fmt.Errorf("unterminated action")
}
