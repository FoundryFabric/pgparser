package nodes

// ExprLocation is where a raw (unanalysed) expression starts in the source
// text, as PostgreSQL 16's exprLocation reports it for raw parse nodes: a
// byte offset, like every ParseLoc, or -1 when the tree does not know.
//
// A node that names a single token (a constant, a column or parameter
// reference, ARRAY[...], CASE, ROW(...)) is at its own location. A node
// whose own location is an operator or keyword that can follow its first
// operand (an operator, AND/OR, IS [NOT] NULL, IS TRUE and the like, ::, a
// function written in SQL syntax such as "x AT TIME ZONE y", a sublink's
// "x IN (...)") is at the leftmost known one of its own location and its
// first operand's. A list is at its first member that has a location.
// Every other node kind is -1.
//
// The left spine is walked in a loop, so a long left-deep chain such as
// "1 + 2 + ... + n" costs no stack; only a list member is a nested call.
func ExprLocation(n Node) int {
	loc := -1
	for n != nil {
		switch e := n.(type) {
		case *A_Const:
			return leftmostLoc(loc, int(e.Location))
		case *ColumnRef:
			return leftmostLoc(loc, int(e.Location))
		case *ParamRef:
			return leftmostLoc(loc, int(e.Location))
		case *A_ArrayExpr:
			return leftmostLoc(loc, int(e.Location))
		case *CaseExpr:
			return leftmostLoc(loc, int(e.Location))
		case *RowExpr:
			return leftmostLoc(loc, int(e.Location))
		case *A_Expr:
			// The right operand cannot precede the operator.
			loc = leftmostLoc(loc, int(e.Location))
			n = e.Lexpr
		case *NullTest:
			loc = leftmostLoc(loc, int(e.Location))
			n = e.Arg
		case *BooleanTest:
			loc = leftmostLoc(loc, int(e.Location))
			n = e.Arg
		case *SubLink:
			loc = leftmostLoc(loc, int(e.Location))
			n = e.Testexpr
		case *TypeCast:
			// CAST(x AS t), x::t and t 'literal' put different parts first.
			loc = leftmostLoc(loc, int(e.Location))
			if e.TypeName != nil {
				loc = leftmostLoc(loc, int(e.TypeName.Location))
			}
			n = e.Arg
		case *BoolExpr:
			// NOT's own location precedes its operand; AND/OR's follows the
			// first. The same rule serves both.
			return leftmostLoc(loc, int(e.Location), listLocation(e.Args))
		case *FuncCall:
			return leftmostLoc(loc, int(e.Location), listLocation(e.Args))
		case *List:
			return leftmostLoc(loc, listLocation(e))
		default:
			return loc
		}
	}
	return loc
}

// listLocation is the location of the first member of l that has one, or
// -1.
func listLocation(l *List) int {
	if l == nil {
		return -1
	}
	for _, item := range l.Items {
		if loc := ExprLocation(item); loc >= 0 {
			return loc
		}
	}
	return -1
}

// leftmostLoc is the smallest of locs that is known (not negative), or -1
// when none is.
func leftmostLoc(locs ...int) int {
	out := -1
	for _, l := range locs {
		if l >= 0 && (out < 0 || l < out) {
			out = l
		}
	}
	return out
}
