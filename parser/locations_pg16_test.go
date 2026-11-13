package parser

import (
	"bufio"
	"encoding/json"
	"os"
	"slices"
	"sort"
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// locationsCase is one statement of testdata/locations_pg16.jsonl: for each
// node kind named, the sorted locations of that kind's nodes in the tree
// libpg_query, PostgreSQL 16's parser built as a library, gave it.
type locationsCase struct {
	SQL       string                      `json:"sql"`
	Locations map[string][]nodes.ParseLoc `json:"locations"`
}

// TestLocationsAreLibpgQuerys pins the locations of the nodes PostgreSQL 16
// places at a token of their rule (DefElem and the like) to libpg_query's,
// for the statements of each rule that makes one.
func TestLocationsAreLibpgQuerys(t *testing.T) {
	f, err := os.Open("testdata/locations_pg16.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var cases []locationsCase
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var c locationsCase
		if err := json.Unmarshal(sc.Bytes(), &c); err != nil {
			t.Fatal(err)
		}
		cases = append(cases, c)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if len(cases) != 336 {
		t.Fatalf("%d cases, want 336", len(cases))
	}
	for _, c := range cases {
		tree, err := Parse(c.SQL)
		if err != nil {
			t.Errorf("Parse(%q): %v", c.SQL, err)
			continue
		}
		for kind, want := range c.Locations {
			got := locationsOf(tree, kind)
			sort.Slice(got, func(i, j int) bool { return got[i] < got[j] })
			if !slices.Equal(got, want) {
				t.Errorf("%q: %s locations %v, libpg_query's are %v", c.SQL, kind, got, want)
			}
		}
	}
}
