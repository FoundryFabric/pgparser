package main

import (
	"regexp"
	"strings"
)

var nearRE = regexp.MustCompile(`at or near "((?:[^"]|"")*)"`)

func nearToken(r result) string {
	if m := nearRE.FindStringSubmatch(r.Msg); m != nil {
		return m[1]
	}
	return ""
}

func ci(pattern string) *regexp.Regexp { return regexp.MustCompile(`(?is)` + pattern) }

// actionChecks maps the message of an error PostgreSQL 16 raises from a
// grammar action (or from the scanner) and the fork does not to its class.
var actionChecks = []struct {
	msg   *regexp.Regexp
	class string
}{
	{ci(`^(invalid publication object list|invalid table name|invalid schema name|WHERE clause not allowed for schema|column specification not allowed for schema)`), "publication-object-list"},
	{ci(`^(constraint declared INITIALLY DEFERRED must be DEFERRABLE|FOREIGN KEY constraints cannot be marked|a column list with SET NULL is only supported|for a generated column, GENERATED ALWAYS must be specified)`), "constraint-attribute-checks"},
	{ci(`^(WITH TIES cannot be specified without ORDER BY|SKIP LOCKED and WITH TIES|cannot use multiple ORDER BY clauses with WITHIN GROUP)`), "select-clause-checks"},
	{ci(`^(CREATE SCHEMA IF NOT EXISTS cannot include|WHERE clause not allowed with COPY TO|unrecognized partitioning strategy|column number must be in range|unrecognized JSON encoding|duplicate trigger events specified|option name .* cannot be used in XMLTABLE|unrecognized column option|frame (starting|ending) )`), "grammar-action-checks"},
	{ci(`^(UESCAPE must be followed by a simple string literal|invalid Unicode escape character)`), "uescape"},
	{ci(`^unrecognized role option`), "role-option-error-order"},
	{ci(`cannot be used as a role name here`), "role-name-checks"},
	{ci(`is not yet implemented$`), "not-yet-implemented-errors"},
}

var (
	reSQLJSONQuery = ci(`\bjson_(table|query|value|exists)\s*\(`)
	reSQLJSONCtor  = ci(`\bjson(_scalar|_serialize)?\s*\(`)
	reMerge        = ci(`\bmerge\b`)
	reSQLJSONName  = ci(`\bjson(_scalar|_serialize|_table|_query|_value|_exists)?\s*\([^()]*$`)
	reColOptions   = ci(`\balter\s+(column\s+)?\S+\s+(set|reset)\s*\(`)
	rePG17DDL      = []*regexp.Regexp{
		ci(`set\s+access\s+method\s+default`),
		ci(`set\s+expression\b`),
		ci(`\bat\s+local\b`),
		ci(`alter\s+domain\s+\S+\s+add\s+(constraint\s+\S+\s+)?not\s+null`),
		ci(`alter\s+operator\b.*\bset\s*\(\s*(merges|hashes)\s*\)`),
	}
	reOIDS        = ci(`\bwith\s+oids\b`)
	reCTE         = ci(`^\s*(explain\s*(\([^)]*\))?\s*)?(create\s+(temporary\s+)?view\s+\S+\s+as\s+)?with\b`)
	reUtilOpt     = ci(`^\s*(explain|vacuum|analyze|reindex|cluster)\s*\(`)
	reDropType    = ci(`^\s*drop\s+(type|domain)\b`)
	reDropAgg     = ci(`^\s*drop\s+aggregate\b`)
	reAlterRole   = ci(`^\s*alter\s+(role|user)\b`)
	reReindex     = ci(`^\s*reindex\b`)
	reCopy        = ci(`^\s*copy\b`)
	reRefresh     = ci(`^\s*alter\s+database\s+\S+\s+refresh\b`)
	reAlterExt    = ci(`^\s*alter\s+extension\s+\S+\s+(add|drop)\b`)
	reAlterCol    = ci(`^\s*alter\s+(table|foreign\s+table|view|materialized\s+view|index)\s+(if\s+exists\s+)?(only\s+)?\S+\s+alter\s+\S+\s+set\b`)
	reAttach      = ci(`^\s*alter\s+table\b.*\b(attach|detach)\s+partition\b`)
	reDropRoutine = ci(`^\s*drop\s+(function|procedure|routine)\b`)
	tfnKeywords   = map[string]bool{"authorization": true, "binary": true, "collation": true, "concurrently": true, "cross": true, "current_schema": true, "freeze": true, "full": true, "ilike": true, "inner": true, "is": true, "isnull": true, "join": true, "left": true, "like": true, "natural": true, "notnull": true, "outer": true, "overlaps": true, "right": true, "similar": true, "tablesample": true, "verbose": true}
	reJSONName    = ci(`\bjson(_scalar|_serialize|_table|_query|_value|_exists)?\s*(\(\s*\d*\s*\)|\(\s*$|\.|\[|$)`)
)

// classify names the grammar cause of a disagreement, or "" when no rule
// covers it. parent is the disagreement of the regression statement a
// mutation was made from, when that statement disagrees too.
func classify(f finding, parent *finding) string {
	sql := stripComments(f.it.SQL)
	pg, fk := f.pg, f.fk
	if f.kind == kindCrash {
		return "crash"
	}
	// The fork refuses a token PostgreSQL 16 takes as a bare column label
	// (target_el: a_expr BareColLabel): putting AS before it gets the
	// fork past it.
	if !fk.OK && (pg.OK || pg.Pos > fk.Pos) && (bareLabelProbe(f.it.SQL, fk.Pos) || bareLabelProbe(f.it.SQL, previousWord(f.it.SQL, fk.Pos))) {
		return "bare-column-label"
	}
	if !pg.OK {
		for _, a := range actionChecks {
			if a.msg.MatchString(pg.Msg) {
				return a.class
			}
		}
	}
	tok := strings.ToUpper(nearToken(pg))
	upTo := sql
	if !pg.OK && pg.Pos >= 0 && pg.Pos <= len(f.it.SQL) {
		upTo = stripComments(f.it.SQL[:pg.Pos])
	}
	// A mutation of a statement that already disagrees, refused by
	// PostgreSQL 16 where it refused the statement, has the statement's
	// cause.
	if parent != nil && !pg.OK && parent.class != "" && !parent.pg.OK && parent.pg.Msg == pg.Msg {
		return parent.class
	}
	switch {
	case !pg.OK && reMerge.MatchString(upTo) && (tok == "RETURNING" || tok == "BY" || strings.Contains(strings.ToLower(upTo), "merge_action")):
		return "pg17-merge"
	case !pg.OK && (fk.OK || fk.Pos >= pg.Pos) && (reSQLJSONQuery.MatchString(upTo) || reSQLJSONCtor.MatchString(upTo)):
		return "pg17-sqljson"
	case (pg.OK || pg.Pos > fk.Pos) && !fk.OK && (reSQLJSONQuery.MatchString(sql) || reSQLJSONCtor.MatchString(sql)) && reSQLJSONName.MatchString(windowBefore(f.it.SQL, fk.Pos, 40)):
		return "pg17-json-names"
	case (pg.OK || pg.Pos > fk.Pos) && !fk.OK && fk.Pos >= 0 && reJSONName.MatchString(windowBefore(f.it.SQL, fk.Pos, 24)):
		return "pg17-json-names"
	case !pg.OK && reOIDS.MatchString(sql):
		return "with-oids"
	}
	for _, re := range rePG17DDL {
		if !pg.OK && re.MatchString(sql) {
			return "pg17-ddl"
		}
	}
	switch {
	case !pg.OK && (fk.OK || fk.Pos > pg.Pos) && reCTE.MatchString(sql):
		return "cte-column-list-parentheses"
	case !pg.OK && (fk.OK || fk.Pos > pg.Pos) && reUtilOpt.MatchString(sql) && (tok == "(" || tok == "*" || tok == "," || tok == "DEFAULT"):
		return "utility-option-argument"
	case reDropType.MatchString(sql) && (fk.OK || pg.OK || fk.Pos != pg.Pos):
		return "drop-type-typename"
	case !pg.OK && fk.OK && reDropAgg.MatchString(sql):
		return "drop-aggregate-arguments"
	case pg.OK && reAlterRole.MatchString(sql):
		return "alter-role-no-options"
	case pg.OK && reReindex.MatchString(sql):
		return "reindex-optional-name"
	case reCopy.MatchString(sql) && (pg.OK || fk.OK || fk.Pos > pg.Pos):
		return "copy-generic-options"
	case (pg.OK || pg.Pos > fk.Pos) && !fk.OK && reColOptions.MatchString(sql) && nearToken(fk) == ".":
		return "column-options-qualified-name"
	case (pg.OK || pg.Pos > fk.Pos) && !fk.OK && reAlterCol.MatchString(sql):
		return "alter-column-optional-keyword"
	case !pg.OK && (fk.OK || fk.Pos > pg.Pos) && reAttach.MatchString(sql) && nearToken(pg) == ",":
		return "partition-command-in-list"
	case (pg.OK || pg.Pos > fk.Pos) && !fk.OK && tfnKeywords[strings.ToLower(nearToken(fk))]:
		return "role-spec-keywords"
	case !pg.OK && !fk.OK && reDropRoutine.MatchString(sql) && nearToken(fk) == "[":
		return "drop-routine-list-name"
	case (pg.OK || pg.Pos > fk.Pos) && !fk.OK && reAlterExt.MatchString(sql):
		return "alter-extension-member"
	case (pg.OK || pg.Pos > fk.Pos) && !fk.OK && reRefresh.MatchString(sql):
		return "alter-database-refresh-collation"
	case !pg.OK && !fk.OK && strings.HasPrefix(pg.Msg, "missing argument"):
		return "operator-missing-argument-message"
	case !pg.OK && !fk.OK && strings.HasPrefix(pg.Msg, "unrecognized row security option"):
		return "policy-option-message"
	}
	return ""
}

// bareLabelProbe reports whether the parser, refusing sql at the word at
// pos, gets past it once AS is put before it.
func bareLabelProbe(sql string, pos int) bool {
	if pos < 0 || pos >= len(sql) {
		return false
	}
	c := sql[pos]
	if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_') {
		return false
	}
	end := pos
	for end < len(sql) && (isWordByte(sql[end])) {
		end++
	}
	before := strings.ToUpper(strings.TrimSpace(sql[:pos]))
	if strings.HasSuffix(before, " AS") {
		return false
	}
	r := parseOne(0, sql[:pos]+"AS "+sql[pos:])
	return r.OK || r.Pos >= end+3
}

// previousWord is where the word before pos starts, or -1.
func previousWord(s string, pos int) int {
	if pos > len(s) {
		pos = len(s)
	}
	i := pos - 1
	for i >= 0 && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n') {
		i--
	}
	end := i + 1
	for i >= 0 && isWordByte(s[i]) {
		i--
	}
	if i+1 == end {
		return -1
	}
	return i + 1
}

func isWordByte(c byte) bool {
	return c == '_' || c == '$' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c >= 0x80
}

func windowBefore(s string, pos, n int) string {
	if pos > len(s) {
		pos = len(s)
	}
	from := pos - n
	if from < 0 {
		from = 0
	}
	return stripComments(s[from:pos])
}
