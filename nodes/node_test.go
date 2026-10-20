package nodes

import "testing"

// TestListElems pins Elems on a nil list, an empty one and a populated one:
// a nil list has no members and does not panic, and a populated list's
// members are its own slice, in order.
func TestListElems(t *testing.T) {
	var absent *List
	if got := absent.Elems(); got != nil {
		t.Errorf("nil list: Elems() = %v, want nil", got)
	}
	if got := (&List{}).Elems(); len(got) != 0 {
		t.Errorf("empty list: Elems() = %v, want no members", got)
	}
	a, b := &String{Str: "a"}, &Integer{Ival: 1}
	l := &List{Items: []Node{a, b}}
	got := l.Elems()
	if len(got) != 2 || got[0] != a || got[1] != b {
		t.Fatalf("populated list: Elems() = %v, want [a 1]", got)
	}
	got[0] = b
	if l.Items[0] != b {
		t.Errorf("Elems() is a copy; want the list's own slice")
	}
}
