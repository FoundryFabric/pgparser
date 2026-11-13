package parser

import (
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// typeNameCase is one statement of testdata/generictype_pg16.json and what
// PostgreSQL 16.15 did with it. A statement PostgreSQL refuses at parse time
// has its syntax error and 0-based byte position. Any other statement it
// parses (pg16After is what analysis then said), and types is its TypeName
// nodes in tree order. Where PostgreSQL's later error named a type or gave a
// position, pg16Name and pg16Position hold it, and a TypeName must carry
// that name or sit at that position. The other TypeName facts (parts beyond
// the error text, typmod count, array bounds, SETOF, %TYPE and a Location
// PostgreSQL did not show) are the values an oracle pass against PostgreSQL
// 16.15 recorded as agreeing with it.
type typeNameCase struct {
	SQL          string `json:"sql"`
	SyntaxError  string `json:"syntaxError"`
	Position     int    `json:"position"`
	Divergence   string `json:"divergence"`
	PG16After    string `json:"pg16After"`
	PG16Name     string `json:"pg16Name"`
	PG16Position *int   `json:"pg16Position"`
	Types        []struct {
		Names    []string       `json:"names"`
		Typmods  int            `json:"typmods"`
		Bounds   []int64        `json:"bounds"`
		Setof    bool           `json:"setof"`
		PctType  bool           `json:"pctType"`
		Location nodes.ParseLoc `json:"location"`
	} `json:"types"`
}

// TestTypeNamesArePostgreSQL16s pins PostgreSQL 16's answers for 323 type
// name statements: casts, CAST, column, parameter, return, sequence,
// XMLSERIALIZE and DDL type names of one to eight parts, quoted and keyword
// parts, typmods, array bounds, SETOF and %TYPE. A case with a divergence is
// a known disagreement: it is skipped while the parser still disagrees, and
// fails once it agrees so that the note is removed.
func TestTypeNamesArePostgreSQL16s(t *testing.T) {
	data, err := os.ReadFile("testdata/generictype_pg16.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []typeNameCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		t.Run(c.SQL, func(t *testing.T) {
			problem := typeNameDisagreement(c)
			switch {
			case c.Divergence != "" && problem != "":
				t.Skip("known divergence from PostgreSQL 16: " + c.Divergence + "; " + problem)
			case c.Divergence != "":
				t.Errorf("Parse(%q) now agrees with PostgreSQL 16: drop its divergence note", c.SQL)
			case problem != "":
				t.Errorf("Parse(%q): %s", c.SQL, problem)
			}
		})
	}
}

// typeNameDisagreement is how Parse(c.SQL) differs from PostgreSQL 16's
// answer in c, or "" when it agrees.
func typeNameDisagreement(c typeNameCase) string {
	stmts, err := parse16(c.SQL)
	if c.SyntaxError != "" {
		var pe *ParseError
		if !errors.As(err, &pe) || pe.Code != codeSyntaxError || pe.Message != c.SyntaxError || pe.Position != c.Position {
			return "got " + describeParseResult(err) + ", want 42601 " + c.SyntaxError + " at " + strconv.Itoa(c.Position)
		}
		return ""
	}
	if err != nil {
		return "got " + describeParseResult(err) + ", want it parsed"
	}
	got := typeNames(stmts)
	if len(got) != len(c.Types) {
		return strconv.Itoa(len(got)) + " TypeName nodes, want " + strconv.Itoa(len(c.Types))
	}
	nameSeen, positionSeen := c.PG16Name == "", c.PG16Position == nil
	for i, want := range c.Types {
		tn := got[i]
		var parts []string
		if tn.Names != nil {
			for _, item := range tn.Names.Items {
				s, ok := item.(*nodes.String)
				if !ok {
					return "TypeName " + strconv.Itoa(i) + " has a part that is not a name"
				}
				parts = append(parts, s.Str)
			}
		}
		typmods := 0
		if tn.Typmods != nil {
			typmods = len(tn.Typmods.Items)
		}
		var bounds []int64
		if tn.ArrayBounds != nil {
			for _, item := range tn.ArrayBounds.Items {
				bounds = append(bounds, item.(*nodes.Integer).Ival)
			}
		}
		if !reflect.DeepEqual(parts, want.Names) || typmods != want.Typmods || !reflect.DeepEqual(bounds, want.Bounds) ||
			tn.Setof != want.Setof || tn.PctType != want.PctType || tn.Location != want.Location {
			gotJSON, _ := json.Marshal([]any{parts, typmods, bounds, tn.Setof, tn.PctType, tn.Location})
			wantJSON, _ := json.Marshal([]any{want.Names, want.Typmods, want.Bounds, want.Setof, want.PctType, want.Location})
			return "TypeName " + strconv.Itoa(i) + " [names typmods bounds setof pctType location] " + string(gotJSON) + ", want " + string(wantJSON)
		}
		joined := strings.Join(parts, ".")
		if c.PG16Name == joined || c.PG16Name == joined+strings.Repeat("[]", len(bounds)) || c.PG16Name == joined+"%TYPE" {
			nameSeen = true
		}
		if c.PG16Position != nil && int(tn.Location) == *c.PG16Position {
			positionSeen = true
		}
	}
	if !nameSeen {
		return "no TypeName is PostgreSQL 16's " + c.PG16Name
	}
	if !positionSeen {
		return "no TypeName is at PostgreSQL 16's position " + strconv.Itoa(*c.PG16Position)
	}
	return ""
}

func describeParseResult(err error) string {
	if err == nil {
		return "no error"
	}
	return err.Error()
}
