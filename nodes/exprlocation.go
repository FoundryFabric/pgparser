package nodes

// ExprLocation is where a raw (unanalysed) expression starts in the source
// text, as PostgreSQL 16's exprLocation reports it for raw parse nodes: a
// byte offset, like every ParseLoc, or -1 when it cannot be told.
//
// A node that names a single token is at its own location: a constant, a
// column or parameter reference, ARRAY[...], CASE, ROW(...), COALESCE,
// GREATEST/LEAST, GROUPING, a SQL value function such as CURRENT_DATE,
// DEFAULT, and IS [NOT] JSON (located at its operand). A node whose own
// location is an operator or keyword that can follow its first operand (an
// operator, AND/OR, IS [NOT] NULL, IS TRUE and the like, COLLATE, ::, a
// named argument, a function written in SQL syntax such as
// "x AT TIME ZONE y", a sublink's "x IN (...)") is at the leftmost known
// one of its own location and its first operand's; a subscript or field
// selection ("a[1]", "(a).f") is at its operand. A list is at its first
// member that has a location.
//
// Where the walk meets a node kind it has no rule for (XmlExpr, the JSON
// constructors, ...), the answer is -1, not the location of an operator
// further right: a known offset is always the real start. A nil node, typed
// or not, is absent: it has no location and does not hide its parent's.
//
// The left spine is walked in a loop, so a long left-deep chain such as
// "1 + 2 + ... + n" costs no stack; only a list member is a nested call.
func ExprLocation(n Node) int {
	if loc, known := exprLocation(n); known {
		return loc
	}
	return -1
}

// exprLocation is ExprLocation's walk. known is false when it met a node
// kind with no rule, whose location could precede everything seen so far.
func exprLocation(n Node) (loc int, known bool) {
	loc = -1
	for {
		switch e := n.(type) {
		case nil:
			return loc, true
		case *A_Const:
			if e == nil {
				return loc, true
			}
			return leftmostLoc(loc, int(e.Location)), true
		case *ColumnRef:
			if e == nil {
				return loc, true
			}
			return leftmostLoc(loc, int(e.Location)), true
		case *ParamRef:
			if e == nil {
				return loc, true
			}
			return leftmostLoc(loc, int(e.Location)), true
		case *A_ArrayExpr:
			if e == nil {
				return loc, true
			}
			return leftmostLoc(loc, int(e.Location)), true
		case *CaseExpr:
			if e == nil {
				return loc, true
			}
			return leftmostLoc(loc, int(e.Location)), true
		case *RowExpr:
			if e == nil {
				return loc, true
			}
			return leftmostLoc(loc, int(e.Location)), true
		case *CoalesceExpr:
			if e == nil {
				return loc, true
			}
			return leftmostLoc(loc, int(e.Location)), true
		case *MinMaxExpr:
			if e == nil {
				return loc, true
			}
			return leftmostLoc(loc, int(e.Location)), true
		case *GroupingFunc:
			if e == nil {
				return loc, true
			}
			return leftmostLoc(loc, int(e.Location)), true
		case *SQLValueFunction:
			if e == nil {
				return loc, true
			}
			return leftmostLoc(loc, int(e.Location)), true
		case *SetToDefault:
			if e == nil {
				return loc, true
			}
			return leftmostLoc(loc, int(e.Location)), true
		case *JsonIsPredicate:
			if e == nil {
				return loc, true
			}
			return leftmostLoc(loc, int(e.Location)), true
		case *A_Expr:
			// The right operand cannot precede the operator.
			if e == nil {
				return loc, true
			}
			loc = leftmostLoc(loc, int(e.Location))
			n = e.Lexpr
		case *NullTest:
			if e == nil {
				return loc, true
			}
			loc = leftmostLoc(loc, int(e.Location))
			n = e.Arg
		case *BooleanTest:
			if e == nil {
				return loc, true
			}
			loc = leftmostLoc(loc, int(e.Location))
			n = e.Arg
		case *SubLink:
			if e == nil {
				return loc, true
			}
			loc = leftmostLoc(loc, int(e.Location))
			n = e.Testexpr
		case *CollateClause:
			if e == nil {
				return loc, true
			}
			loc = leftmostLoc(loc, int(e.Location))
			n = e.Arg
		case *NamedArgExpr:
			if e == nil {
				return loc, true
			}
			loc = leftmostLoc(loc, int(e.Location))
			n = e.Arg
		case *A_Indirection:
			if e == nil {
				return loc, true
			}
			n = e.Arg
		case *TypeCast:
			// CAST(x AS t), x::t and t 'literal' put different parts first.
			if e == nil {
				return loc, true
			}
			loc = leftmostLoc(loc, int(e.Location))
			if e.TypeName != nil {
				loc = leftmostLoc(loc, int(e.TypeName.Location))
			}
			n = e.Arg
		case *BoolExpr:
			// NOT's own location precedes its operand; AND/OR's follows the
			// first. The same rule serves both.
			if e == nil {
				return loc, true
			}
			first, ok := listLocation(e.Args)
			return leftmostLoc(loc, int(e.Location), first), ok
		case *FuncCall:
			if e == nil {
				return loc, true
			}
			first, ok := listLocation(e.Args)
			return leftmostLoc(loc, int(e.Location), first), ok
		case *List:
			first, ok := listLocation(e)
			return leftmostLoc(loc, first), ok
		default:
			return -1, false
		}
	}
}

// listLocation is the location of the first member of l that has one, or
// -1. known is false when a member before it is of a kind with no rule.
func listLocation(l *List) (loc int, known bool) {
	if l == nil {
		return -1, true
	}
	for _, item := range l.Items {
		loc, known := exprLocation(item)
		if !known || loc >= 0 {
			return loc, known
		}
	}
	return -1, true
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
