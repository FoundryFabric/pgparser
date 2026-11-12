package main

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/importer"
	goparser "go/parser"
	"go/token"
	"go/types"
	"io/fs"
	"reflect"
	"sort"
	"strings"

	pg_query "github.com/pganalyze/pg_query_go/v5"
	"github.com/pgplex/pgparser/nodes"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// tnode is a parse node in the form both trees are brought to before they
// are compared: its type's name and its fields, keyed by fieldKey. A field
// is nil, a *tnode, a []any list, an int64, a string or a bool.
type tnode struct {
	typ    string
	fields map[string]any
}

// fieldKey makes the fork's Go field names and libpg_query's protobuf field
// names meet: TargetList and target_list are both "targetlist".
func fieldKey(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, "_", ""))
}

// forkRenames maps a fork field, as type.key, to libpg_query's key, where
// the two name the same PostgreSQL field differently.
var forkRenames = map[string]string{
	"String.str":                       "sval",
	"CreateFunctionStmt.isorreplace":   "replace",
	"TransactionStmt.savepoint":        "savepointname",
	"CreateDomainStmt.typname":         "typename",
	"AlterDomainStmt.typname":          "typename",
	"AlterEnumStmt.typname":            "typename",
	"CreateForeignTableStmt.base":      "basestmt",
	"JoinExpr.joinusing":               "joinusingalias",
	"JsonObjectConstructor.uniquekeys": "unique",
	"JsonObjectAgg.uniquekeys":         "unique",
}

// aConstVal is libpg_query's key for A_Const's value, by the value's type:
// the fork holds the value as a node in Val, libpg_query in one of a oneof.
var aConstVal = map[string]string{
	"Integer": "ival", "Float": "fval", "Boolean": "boolval", "String": "sval", "BitString": "bsval",
}

// forkTree brings a fork value to the compared form.
func forkTree(v reflect.Value) any {
	switch v.Kind() {
	case reflect.Interface, reflect.Pointer:
		if v.IsNil() {
			return nil
		}
		if l, ok := v.Interface().(*nodes.List); ok {
			if l == nil {
				return nil
			}
			out := make([]any, len(l.Items))
			for i, it := range l.Items {
				out[i] = forkTree(reflect.ValueOf(&it).Elem())
			}
			return out
		}
		return forkTree(v.Elem())
	case reflect.Struct:
		t := v.Type()
		n := &tnode{typ: t.Name(), fields: map[string]any{}}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			key := fieldKey(f.Name)
			if r, ok := forkRenames[n.typ+"."+key]; ok {
				key = r
			}
			val := forkTree(v.Field(i))
			if n.typ == "A_Const" && key == "val" {
				// The value goes under the key of its type, as in the oneof.
				if c, ok := val.(*tnode); ok {
					n.fields[aConstVal[c.typ]] = c
				}
				continue
			}
			n.fields[key] = val
		}
		normaliseFork(n)
		return n
	case reflect.Slice:
		if v.IsNil() {
			return nil
		}
		out := make([]any, v.Len())
		for i := range out {
			out[i] = forkTree(v.Index(i))
		}
		return out
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int()
	case reflect.Uint8:
		// A C char field is a one-character string in libpg_query's
		// output, and absent (the empty string) when it is 0.
		if v.Uint() == 0 {
			return ""
		}
		return string(rune(v.Uint()))
	case reflect.Uint, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return int64(v.Uint())
	case reflect.String:
		return v.String()
	case reflect.Bool:
		return v.Bool()
	}
	panic(fmt.Sprintf("forkTree: unhandled kind %s", v.Kind()))
}

// libpgTree brings a libpg_query message to the compared form.
func libpgTree(m protoreflect.Message) any {
	md := m.Descriptor()
	switch md.Name() {
	case "Node":
		fd := m.WhichOneof(md.Oneofs().Get(0))
		if fd == nil {
			return nil
		}
		return libpgTree(m.Get(fd).Message())
	case "List":
		return libpgValue(md.Fields().ByName("items"), m.Get(md.Fields().ByName("items")))
	}
	n := &tnode{typ: string(md.Name()), fields: map[string]any{}}
	fields := md.Fields()
	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		if fd.Kind() == protoreflect.MessageKind && !fd.IsList() && !m.Has(fd) {
			n.fields[fieldKey(string(fd.Name()))] = nil
			continue
		}
		n.fields[fieldKey(string(fd.Name()))] = libpgValue(fd, m.Get(fd))
	}
	return n
}

func libpgValue(fd protoreflect.FieldDescriptor, v protoreflect.Value) any {
	if fd.IsList() {
		l := v.List()
		out := make([]any, l.Len())
		for i := range out {
			out[i] = libpgScalar(fd, l.Get(i))
		}
		return out
	}
	return libpgScalar(fd, v)
}

func libpgScalar(fd protoreflect.FieldDescriptor, v protoreflect.Value) any {
	switch fd.Kind() {
	case protoreflect.MessageKind:
		return libpgTree(v.Message())
	case protoreflect.EnumKind:
		return libpgEnum(fd.Enum(), v.Enum())
	case protoreflect.Int32Kind, protoreflect.Int64Kind, protoreflect.Sint32Kind, protoreflect.Sint64Kind:
		return v.Int()
	case protoreflect.Uint32Kind, protoreflect.Uint64Kind:
		return int64(v.Uint())
	case protoreflect.StringKind:
		return v.String()
	case protoreflect.BoolKind:
		return v.Bool()
	}
	panic(fmt.Sprintf("libpgTree: unhandled kind %s in %s", fd.Kind(), fd.FullName()))
}

// normaliseFork brings fork nodes that hold the same thing as PostgreSQL
// 16's in another shape to PostgreSQL 16's.
func normaliseFork(n *tnode) {
	switch n.typ {
	case "TypeName":
		// The fork leaves typemod at 0 where PostgreSQL sets -1, no typmod.
		if n.fields["typemod"] == int64(0) {
			n.fields["typemod"] = int64(-1)
		}
	case "MergeWhenClause":
		// PostgreSQL 17 replaced matched with kind; 16 has matched only.
		n.fields["matched"] = n.fields["kind"] == forkEnums["MergeMatchKind"]["MERGE_WHEN_MATCHED"]
		delete(n.fields, "kind")
	case "SelectStmt":
		// The fork has no LIMIT_OPTION_DEFAULT; without LIMIT or OFFSET
		// its LIMIT_OPTION_COUNT is PostgreSQL's LIMIT_OPTION_DEFAULT.
		if n.fields["limitcount"] == nil && n.fields["limitoffset"] == nil && n.fields["limitoption"] == forkEnums["LimitOption"]["LIMIT_OPTION_COUNT"] {
			n.fields["limitoption"] = "LIMIT_OPTION_DEFAULT"
		}
	case "IndexStmt", "Constraint":
		// The fork leaves out the access method PostgreSQL fills in.
		if n.fields["accessmethod"] == "" && (n.typ == "IndexStmt" || n.fields["contype"] == forkEnums["ConstrType"]["CONSTR_EXCLUSION"]) {
			n.fields["accessmethod"] = "btree"
		}
	case "DeallocateStmt":
		// PostgreSQL 17 added isall; 16 has a null name only.
		delete(n.fields, "isall")
	}
}

// forkEnums is each fork enum's members' values, by enum and member name,
// so that enums compare by member: the fork numbers some enums as
// PostgreSQL 17 does, and PostgreSQL's char enums are chars in the fork.
var forkEnums = map[string]map[string]any{
	// The fork spells PARTITION_STRATEGY_* as bare chars.
	"PartitionStrategy": {"PARTITION_STRATEGY_LIST": "l", "PARTITION_STRATEGY_RANGE": "r", "PARTITION_STRATEGY_HASH": "h"},
}

// loadForkEnums reads the fork's enum members from the nodes package source
// in dir.
func loadForkEnums(dir string) error {
	fset := token.NewFileSet()
	pkgs, err := goparser.ParseDir(fset, dir, func(fi fs.FileInfo) bool { return !strings.HasSuffix(fi.Name(), "_test.go") }, 0)
	if err != nil {
		return err
	}
	p, ok := pkgs["nodes"]
	if !ok {
		return fmt.Errorf("%s: no package nodes", dir)
	}
	var files []*ast.File
	for _, f := range p.Files {
		files = append(files, f)
	}
	conf := types.Config{Importer: importer.ForCompiler(fset, "source", nil)}
	pkg, err := conf.Check("nodes", fset, files, nil)
	if err != nil {
		return err
	}
	for _, name := range pkg.Scope().Names() {
		c, ok := pkg.Scope().Lookup(name).(*types.Const)
		if !ok {
			continue
		}
		named, ok := c.Type().(*types.Named)
		if !ok {
			continue
		}
		v, ok := constant.Int64Val(c.Val())
		if !ok {
			continue
		}
		var val any = v
		if b, ok := named.Underlying().(*types.Basic); ok && b.Kind() == types.Byte {
			val = string(rune(v))
		}
		e := named.Obj().Name()
		if forkEnums[e] == nil {
			forkEnums[e] = map[string]any{}
		}
		forkEnums[e][name] = val
	}
	return nil
}

// libpgEnum is a libpg_query enum value as the fork holds the member of
// that name. A member the fork has not got stays its name.
func libpgEnum(ed protoreflect.EnumDescriptor, num protoreflect.EnumNumber) any {
	name := string(ed.Values().ByNumber(num).Name())
	members, ok := forkEnums[string(ed.Name())]
	if !ok {
		// libpg_query numbers each enum from 1, keeping 0 for undefined.
		return int64(num) - 1
	}
	if v, ok := members[name]; ok {
		return v
	}
	return name
}

// treesOf brings each tree's statements to the compared form.
func treesOf(fork *nodes.List, libpg *pg_query.ParseResult) (any, any) {
	lq := make([]any, len(libpg.Stmts))
	for i, s := range libpg.Stmts {
		// The fork's Parse returns the statements without their RawStmt.
		lq[i] = libpgTree(s.Stmt.ProtoReflect())
	}
	return forkTree(reflect.ValueOf(fork)), lq
}

// mismatch kinds, worst first.
const (
	treeNodeType   = "node-type"   // a different node type in the same place
	treePresence   = "presence"    // a node or field one tree has and the other has not
	treeListLength = "list-length" // a list of a different length
	treeName       = "name"        // a different string: a name part, a type name, a literal
	treeValue      = "value"       // a different number, flag or enum
	treeLocation   = "location"    // both have a location, at different places
	treeUnlocated  = "unlocated"   // the fork has none (-1 or 0) where libpg_query has one
	treeOverplaced = "overplaced"  // the fork has a location where libpg_query has -1
)

var treeKindRank = map[string]int{treeNodeType: 0, treePresence: 1, treeListLength: 2, treeName: 3, treeValue: 4, treeLocation: 5, treeUnlocated: 6, treeOverplaced: 7}

// locationKind tells a missing location from a wrong one.
func locationKind(fork, libpg any) string {
	f, ok1 := fork.(int64)
	l, ok2 := libpg.(int64)
	switch {
	case !ok1 || !ok2:
		return treeLocation
	case l == -1:
		return treeOverplaced
	case f == -1 || f == 0 && l > 0:
		return treeUnlocated
	}
	return treeLocation
}

// treeMismatch is one place where the trees differ. Where names the field
// that holds the node and the node's own field, as Owner.field/Type.field.
type treeMismatch struct {
	kind, where string
	fork, libpg any
}

func (m treeMismatch) class() string { return m.kind + " " + m.where }

// compareTrees lists the places where the fork's tree differs from
// libpg_query's. It does not descend below a node type or list length that
// differs.
func compareTrees(fork, libpg any) []treeMismatch {
	var out []treeMismatch
	compareValue("", "", fork, libpg, &out)
	return out
}

func compareValue(owner, key string, a, b any, out *[]treeMismatch) {
	at := func(kind string) {
		*out = append(*out, treeMismatch{kind: kind, where: owner, fork: a, libpg: b})
	}
	// An absent list and an empty one are the same: libpg_query's
	// repeated fields cannot tell them apart.
	if isEmpty(a) && isEmpty(b) {
		return
	}
	switch x := a.(type) {
	case *tnode:
		y, ok := b.(*tnode)
		switch {
		case !ok && b == nil:
			at(treePresence)
		case !ok:
			at(treeNodeType)
		case x.typ != y.typ:
			at(treeNodeType)
		default:
			compareNode(owner, x, y, out)
		}
	case []any:
		y, ok := b.([]any)
		switch {
		case !ok && b == nil:
			at(treePresence)
		case !ok:
			at(treeNodeType)
		case len(x) != len(y):
			at(treeListLength)
		default:
			for i := range x {
				compareValue(owner, key, x[i], y[i], out)
			}
		}
	case nil:
		at(treePresence)
	default:
		if a == b {
			return
		}
		switch {
		case key == "location" || strings.HasSuffix(key, "location"):
			at(locationKind(a, b))
		case isString(a) && isString(b):
			at(treeName)
		default:
			at(treeValue)
		}
	}
}

func compareNode(owner string, a, b *tnode, out *[]treeMismatch) {
	keys := make([]string, 0, len(a.fields)+len(b.fields))
	for k := range a.fields {
		keys = append(keys, k)
	}
	for k := range b.fields {
		if _, ok := a.fields[k]; !ok {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		x, inA := a.fields[k]
		y, inB := b.fields[k]
		where := a.typ + "." + k
		if owner != "" {
			where = owner + "/" + where
		}
		// A field only one tree has is taken as its zero value in the
		// other: libpg_query leaves out fields at their zero value, and
		// the fork has fields PostgreSQL 16 has not.
		if !inA && isZero(y) || !inB && isZero(x) {
			continue
		}
		if !inA || !inB {
			*out = append(*out, treeMismatch{kind: treePresence, where: where, fork: x, libpg: y})
			continue
		}
		if _, ok := x.(*tnode); ok {
			compareValue(a.typ+"."+k, k, x, y, out)
			continue
		}
		if _, ok := x.([]any); ok {
			compareValue(a.typ+"."+k, k, x, y, out)
			continue
		}
		if _, ok := y.(*tnode); ok {
			compareValue(a.typ+"."+k, k, x, y, out)
			continue
		}
		compareScalar(where, k, x, y, out)
	}
}

func compareScalar(where, key string, a, b any, out *[]treeMismatch) {
	if a == b || isEmpty(a) && isEmpty(b) {
		return
	}
	kind := treeValue
	switch {
	case key == "location" || strings.HasSuffix(key, "location"):
		kind = locationKind(a, b)
	case a == nil || b == nil:
		kind = treePresence
	case isString(a) && isString(b):
		kind = treeName
	}
	*out = append(*out, treeMismatch{kind: kind, where: where, fork: a, libpg: b})
}

func isString(v any) bool { _, ok := v.(string); return ok }

func isEmpty(v any) bool {
	if v == nil {
		return true
	}
	l, ok := v.([]any)
	return ok && len(l) == 0
}

func isZero(v any) bool {
	switch x := v.(type) {
	case int64:
		return x == 0
	case string:
		return x == ""
	case bool:
		return !x
	}
	return isEmpty(v)
}

// shortValue is a mismatch's value for the report.
func shortValue(v any) string {
	switch x := v.(type) {
	case nil:
		return "nil"
	case *tnode:
		return x.typ
	case []any:
		return fmt.Sprintf("list of %d", len(x))
	case string:
		return fmt.Sprintf("%q", x)
	}
	return fmt.Sprint(v)
}
