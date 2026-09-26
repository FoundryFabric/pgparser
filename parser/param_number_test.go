package parser

import (
	"reflect"
	"testing"

	"github.com/pgplex/pgparser/nodes"
)

// collectParamRefs walks v's fields in struct declaration order and returns
// every *nodes.ParamRef reachable from it. Declaration order matches SQL
// source-text order for the two shapes this test exercises (a target list,
// and A_Expr's Lexpr before Rexpr), so it is enough to observe the property
// this test pins without a full source-order walker.
func collectParamRefs(root nodes.Node) []*nodes.ParamRef {
	var out []*nodes.ParamRef
	var walk func(v reflect.Value)
	walk = func(v reflect.Value) {
		if !v.IsValid() {
			return
		}
		switch v.Kind() { //nolint:exhaustive // only the kinds that can carry a Node
		case reflect.Ptr:
			if v.IsNil() {
				return
			}
			if pr, ok := v.Interface().(*nodes.ParamRef); ok {
				out = append(out, pr)
				return
			}
			walk(v.Elem())
		case reflect.Interface:
			if v.IsNil() {
				return
			}
			walk(v.Elem())
		case reflect.Struct:
			for i := 0; i < v.NumField(); i++ {
				walk(v.Field(i))
			}
		case reflect.Slice, reflect.Array:
			for i := 0; i < v.Len(); i++ {
				walk(v.Index(i))
			}
		}
	}
	walk(reflect.ValueOf(root))
	return out
}

// TestLexCarriesParamNumber pins parserLexer.Lex copying a $N placeholder's
// scanned number into the grammar's semantic value for the PARAM token, the
// same way it already does for ICONST. Without that, every ParamRef.Number
// is 0 regardless of which $N appeared in the source.
func TestLexCarriesParamNumber(t *testing.T) {
	t.Run("binary expression, out-of-order params", func(t *testing.T) {
		list, err := Parse("SELECT $2::int - $1::int")
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		if list == nil || len(list.Items) != 1 {
			t.Fatalf("expected 1 statement, got %#v", list)
		}
		sel, ok := list.Items[0].(*nodes.SelectStmt)
		if !ok {
			t.Fatalf("expected *nodes.SelectStmt, got %T", list.Items[0])
		}
		refs := collectParamRefs(sel.TargetList)
		if len(refs) != 2 {
			t.Fatalf("expected 2 ParamRefs, got %d", len(refs))
		}
		got := []int{refs[0].Number, refs[1].Number}
		want := []int{2, 1}
		if got[0] != want[0] || got[1] != want[1] {
			t.Errorf("Numbers = %v, want %v", got, want)
		}
	})

	t.Run("multi-digit parameter", func(t *testing.T) {
		list, err := Parse("SELECT $10")
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		if list == nil || len(list.Items) != 1 {
			t.Fatalf("expected 1 statement, got %#v", list)
		}
		sel, ok := list.Items[0].(*nodes.SelectStmt)
		if !ok {
			t.Fatalf("expected *nodes.SelectStmt, got %T", list.Items[0])
		}
		refs := collectParamRefs(sel.TargetList)
		if len(refs) != 1 {
			t.Fatalf("expected 1 ParamRef, got %d", len(refs))
		}
		got := []int{refs[0].Number}
		want := []int{10}
		if got[0] != want[0] {
			t.Errorf("Numbers = %v, want %v", got, want)
		}
	})
}
