package printer

import (
	"strconv"
	"strings"

	"github.com/sqlc-dev/sqlc-gen-python/internal/ast"
)

type writer struct {
	options Options
	src     []byte
}

type Options struct {
}

type PrintResult struct {
	Python []byte
}

const inlineDictLimit = 3

func Print(node *ast.Node, options Options) PrintResult {
	w := writer{options: options}
	w.printNode(node, 0)
	return PrintResult{
		Python: w.src,
	}
}

func (w *writer) print(text string) {
	w.src = append(w.src, text...)
}

func (w *writer) printIndent(indent int32) {
	for i, n := 0, int(indent); i < n; i++ {
		w.src = append(w.src, "    "...)
	}
}

func (w *writer) printNode(node *ast.Node, indent int32) {
	if w.printDefinitionNode(node, indent) {
		return
	}
	w.printExpressionNode(node, indent)
}

func (w *writer) printDefinitionNode(node *ast.Node, indent int32) bool {
	switch n := node.GetNode().(type) {
	case *ast.Node_AnnAssign:
		w.printAnnAssign(n.AnnAssign, indent)
	case *ast.Node_Assign:
		w.printAssign(n.Assign, indent)
	case *ast.Node_AsyncFor:
		w.printAsyncFor(n.AsyncFor, indent)
	case *ast.Node_AsyncFunctionDef:
		w.printAsyncFunctionDef(n.AsyncFunctionDef, indent)
	case *ast.Node_ClassDef:
		w.printClassDef(n.ClassDef, indent)
	case *ast.Node_For:
		w.printFor(n.For, indent)
	case *ast.Node_FunctionDef:
		w.printFunctionDef(n.FunctionDef, indent)
	case *ast.Node_If:
		w.printIf(n.If, indent)
	case *ast.Node_Import:
		w.printImport(n.Import, indent)
	case *ast.Node_ImportFrom:
		w.printImportFrom(n.ImportFrom, indent)
	case *ast.Node_ImportGroup:
		w.printImportGroup(n.ImportGroup, indent)
	case *ast.Node_Module:
		w.printModule(n.Module, indent)
	case *ast.Node_Pass:
		w.print("pass")
	case *ast.Node_Return:
		w.printReturn(n.Return, indent)
	default:
		return false
	}
	return true
}

func (w *writer) printExpressionNode(node *ast.Node, indent int32) {
	switch n := node.GetNode().(type) {
	case *ast.Node_Alias:
		w.print(n.Alias.GetName())
	case *ast.Node_Attribute:
		w.printAttribute(n.Attribute, indent)
	case *ast.Node_Await:
		w.printAwait(n.Await, indent)
	case *ast.Node_Call:
		w.printCall(n.Call, indent)
	case *ast.Node_Comment:
		w.printComment(n.Comment)
	case *ast.Node_Compare:
		w.printCompare(n.Compare, indent)
	case *ast.Node_Constant:
		w.printConstant(n.Constant)
	case *ast.Node_Dict:
		w.printDict(n.Dict, indent)
	case *ast.Node_Expr:
		w.printNode(n.Expr.GetValue(), indent)
	case *ast.Node_Is:
		w.print("is")
	case *ast.Node_Keyword:
		w.printKeyword(n.Keyword, indent)
	case *ast.Node_Name:
		w.print(n.Name.GetId())
	case *ast.Node_Subscript:
		w.printSubscript(n.Subscript, indent)
	case *ast.Node_Yield:
		w.printYield(n.Yield, indent)
	default:
		panic(n)
	}
}

func (w *writer) printAnnAssign(aa *ast.AnnAssign, indent int32) {
	if aa.GetComment() != "" {
		w.print("# ")
		w.print(aa.GetComment())
		w.print("\n")
		w.printIndent(indent)
	}
	w.printName(aa.GetTarget())
	w.print(": ")
	w.printNode(aa.GetAnnotation(), indent)
}

func (w *writer) printArg(a *ast.Arg, indent int32) {
	w.print(a.GetArg())
	if a.GetAnnotation() != nil {
		w.print(": ")
		w.printNode(a.GetAnnotation(), indent)
	}
}

func (w *writer) printAssign(a *ast.Assign, indent int32) {
	for i, name := range a.GetTargets() {
		w.printNode(name, indent)
		if i != len(a.GetTargets())-1 {
			w.print(", ")
		}
	}
	w.print(" = ")
	w.printNode(a.GetValue(), indent)
}

func (w *writer) printAsyncFor(n *ast.AsyncFor, indent int32) {
	w.print("async ")
	w.printFor(&ast.For{
		Target: n.GetTarget(),
		Iter:   n.GetIter(),
		Body:   n.GetBody(),
	}, indent)
}

func (w *writer) printAsyncFunctionDef(afd *ast.AsyncFunctionDef, indent int32) {
	w.print("async ")
	w.printFunctionDef(&ast.FunctionDef{
		Name:    afd.GetName(),
		Args:    afd.GetArgs(),
		Body:    afd.GetBody(),
		Returns: afd.GetReturns(),
	}, indent)
}

func (w *writer) printAttribute(a *ast.Attribute, indent int32) {
	if _, ok := a.GetValue().GetNode().(*ast.Node_Await); ok {
		w.print("(")
		w.printNode(a.GetValue(), indent)
		w.print(")")
	} else {
		w.printNode(a.GetValue(), indent)
	}
	w.print(".")
	w.print(a.GetAttr())
}

func (w *writer) printAwait(n *ast.Await, indent int32) {
	w.print("await ")
	w.printNode(n.GetValue(), indent)
}

func (w *writer) printCall(c *ast.Call, indent int32) {
	w.printNode(c.GetFunc(), indent)
	w.print("(")
	for i, a := range c.GetArgs() {
		w.printNode(a, indent)
		if i != len(c.GetArgs())-1 {
			w.print(", ")
		}
	}
	for _, kw := range c.GetKeywords() {
		w.print("\n")
		w.printIndent(indent + 1)
		w.printKeyword(kw, indent+1)
		w.print(",")
	}
	if len(c.GetKeywords()) > 0 {
		w.print("\n")
		w.printIndent(indent)
	}
	w.print(")")
}

func (w *writer) printClassDef(cd *ast.ClassDef, indent int32) {
	w.printClassHeader(cd, indent)
	w.printClassBody(cd, indent)
}

func (w *writer) printClassHeader(cd *ast.ClassDef, indent int32) {
	for _, node := range cd.GetDecoratorList() {
		w.print("@")
		w.printNode(node, indent)
		w.print("\n")
	}
	w.print("class ")
	w.print(cd.GetName())
	if len(cd.GetBases()) > 0 {
		w.print("(")
		for i, node := range cd.GetBases() {
			w.printNode(node, indent)
			if i != len(cd.GetBases())-1 {
				w.print(", ")
			}
		}
		w.print(")")
	}
	w.print(":\n")
}

func (w *writer) printClassBody(cd *ast.ClassDef, indent int32) {
	if len(cd.GetBody()) == 0 {
		w.printIndent(indent + 1)
		w.print("pass\n")
		return
	}
	for i, node := range cd.GetBody() {
		if i > 0 && isClassMethod(node) {
			w.print("\n")
		}
		w.printIndent(indent + 1)
		if i == 0 && w.printClassDocstring(node) {
			continue
		}
		w.printNode(node, indent+1)
		w.print("\n")
	}
}

func isClassMethod(node *ast.Node) bool {
	switch node.GetNode().(type) {
	case *ast.Node_FunctionDef, *ast.Node_AsyncFunctionDef:
		return true
	default:
		return false
	}
}

func (w *writer) printClassDocstring(node *ast.Node) bool {
	expr, ok := node.GetNode().(*ast.Node_Expr)
	if !ok {
		return false
	}
	constant, ok := expr.Expr.GetValue().GetNode().(*ast.Node_Constant)
	if !ok {
		return false
	}
	w.print(`""`)
	w.printConstant(constant.Constant)
	w.print(`""`)
	w.print("\n")
	return true
}

func (w *writer) printConstant(c *ast.Constant) {
	switch n := c.GetValue().(type) {
	case *ast.Constant_Int:
		w.print(strconv.Itoa(int(n.Int)))

	case *ast.Constant_None:
		w.print("None")

	case *ast.Constant_Str:
		str := `"`
		if strings.Contains(n.Str, "\n") {
			str = `"""`
		}
		w.print(str)
		w.print(n.Str)
		w.print(str)

	default:
		panic(n)
	}
}

func (w *writer) printComment(c *ast.Comment) {
	w.print("# ")
	w.print(c.GetText())
	w.print("\n")
}

func (w *writer) printCompare(c *ast.Compare, indent int32) {
	w.printNode(c.GetLeft(), indent)
	w.print(" ")
	for _, node := range c.GetOps() {
		w.printNode(node, indent)
		w.print(" ")
	}
	for _, node := range c.GetComparators() {
		w.printNode(node, indent)
	}
}

func (w *writer) printDict(d *ast.Dict, indent int32) {
	if len(d.GetKeys()) != len(d.GetValues()) {
		panic(`dict keys and values are not the same length`)
	}
	w.print("{")
	split := len(d.GetKeys()) > inlineDictLimit
	keyIndent := indent
	if split {
		keyIndent += 1
	}
	for i := range d.GetKeys() {
		if split {
			w.print("\n")
			w.printIndent(keyIndent)
		}
		w.printNode(d.GetKeys()[i], keyIndent)
		w.print(": ")
		w.printNode(d.GetValues()[i], keyIndent)
		if i != len(d.GetKeys())-1 || split {
			if split {
				w.print(",")
			} else {
				w.print(", ")
			}
		}
	}
	if split {
		w.print("\n")
		w.printIndent(indent)
	}
	w.print("}")
}

func (w *writer) printFor(n *ast.For, indent int32) {
	w.print("for ")
	w.printNode(n.GetTarget(), indent)
	w.print(" in ")
	w.printNode(n.GetIter(), indent)
	w.print(":\n")
	for i, node := range n.GetBody() {
		w.printIndent(indent + 1)
		w.printNode(node, indent+1)
		if i != len(n.GetBody())-1 {
			w.print("\n")
		}
	}
}

func (w *writer) printIf(i *ast.If, indent int32) {
	w.print("if ")
	w.printNode(i.GetTest(), indent)
	w.print(":\n")
	for j, node := range i.GetBody() {
		w.printIndent(indent + 1)
		w.printNode(node, indent+1)
		if j != len(i.GetBody())-1 {
			w.print("\n")
		}
	}
}

func (w *writer) printFunctionDef(fd *ast.FunctionDef, indent int32) {
	w.print("def ")
	w.print(fd.GetName())
	w.print("(")
	if fd.GetArgs() != nil {
		for i, arg := range fd.GetArgs().GetArgs() {
			w.printArg(arg, indent)
			if i != len(fd.GetArgs().GetArgs())-1 {
				w.print(", ")
			}
		}
		if len(fd.GetArgs().GetKwOnlyArgs()) > 0 {
			w.print(", *, ")
			for i, arg := range fd.GetArgs().GetKwOnlyArgs() {
				w.printArg(arg, indent)
				if i != len(fd.GetArgs().GetKwOnlyArgs())-1 {
					w.print(", ")
				}
			}
		}
	}
	w.print(")")
	if fd.GetReturns() != nil {
		w.print(" -> ")
		w.printNode(fd.GetReturns(), indent)
	}
	w.print(":\n")
	for i, node := range fd.GetBody() {
		w.printIndent(indent + 1)
		w.printNode(node, indent+1)
		if i != len(fd.GetBody())-1 {
			w.print("\n")
		}
	}
}

func (w *writer) printImport(imp *ast.Import, indent int32) {
	w.print("import ")
	for i, node := range imp.GetNames() {
		w.printNode(node, indent)
		if i != len(imp.GetNames())-1 {
			w.print(", ")
		}
	}
	w.print("\n")
}

func (w *writer) printImportFrom(imp *ast.ImportFrom, indent int32) {
	w.print("from ")
	w.print(imp.GetModule())
	w.print(" import ")
	for i, node := range imp.GetNames() {
		w.printNode(node, indent)
		if i != len(imp.GetNames())-1 {
			w.print(", ")
		}
	}
	w.print("\n")
}

func (w *writer) printImportGroup(n *ast.ImportGroup, indent int32) {
	if len(n.GetImports()) == 0 {
		return
	}
	for _, node := range n.GetImports() {
		w.printNode(node, indent)
	}
	w.print("\n")
}

func (w *writer) printKeyword(k *ast.Keyword, indent int32) {
	w.print(k.GetArg())
	w.print("=")
	w.printNode(k.GetValue(), indent)
}

func (w *writer) printModule(mod *ast.Module, indent int32) {
	for i, node := range mod.GetBody() {
		prevIsImport := false
		if i > 0 {
			_, isImport := mod.GetBody()[i-1].GetNode().(*ast.Node_ImportGroup)
			prevIsImport = isImport
		}
		_, isClassDef := node.GetNode().(*ast.Node_ClassDef)
		_, isAssign := node.GetNode().(*ast.Node_Assign)
		if isClassDef || isAssign {
			if prevIsImport {
				w.print("\n")
			} else {
				w.print("\n\n")
			}
		}
		w.printNode(node, indent)
		if isAssign {
			w.print("\n")
		}
	}
}

func (w *writer) printName(n *ast.Name) {
	w.print(n.GetId())
}

func (w *writer) printReturn(r *ast.Return, indent int32) {
	w.print("return ")
	w.printNode(r.GetValue(), indent)
}

func (w *writer) printSubscript(ss *ast.Subscript, indent int32) {
	w.printName(ss.GetValue())
	w.print("[")
	w.printNode(ss.GetSlice(), indent)
	w.print("]")
}

func (w *writer) printYield(n *ast.Yield, indent int32) {
	w.print("yield ")
	w.printNode(n.GetValue(), indent)
}
