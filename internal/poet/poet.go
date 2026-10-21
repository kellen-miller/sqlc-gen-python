package poet

import (
	"google.golang.org/protobuf/proto"

	"github.com/sqlc-dev/sqlc-gen-python/internal/ast"
)

func Nodes(nodes ...proto.Message) []*ast.Node {
	list := make([]*ast.Node, len(nodes))
	for i := range nodes {
		list[i] = Node(nodes[i])
	}
	return list
}

func Node(node proto.Message) *ast.Node {
	if wrapped := wrapDefinition(node); wrapped != nil {
		return wrapped
	}
	if wrapped := wrapExpression(node); wrapped != nil {
		return wrapped
	}
	panic(node)
}

func wrapDefinition(node proto.Message) *ast.Node {
	switch n := node.(type) {
	case *ast.AnnAssign:
		return &ast.Node{Node: &ast.Node_AnnAssign{AnnAssign: n}}
	case *ast.Assign:
		return &ast.Node{Node: &ast.Node_Assign{Assign: n}}
	case *ast.AsyncFor:
		return &ast.Node{Node: &ast.Node_AsyncFor{AsyncFor: n}}
	case *ast.AsyncFunctionDef:
		return &ast.Node{Node: &ast.Node_AsyncFunctionDef{AsyncFunctionDef: n}}
	case *ast.ClassDef:
		return &ast.Node{Node: &ast.Node_ClassDef{ClassDef: n}}
	case *ast.For:
		return &ast.Node{Node: &ast.Node_For{For: n}}
	case *ast.FunctionDef:
		return &ast.Node{Node: &ast.Node_FunctionDef{FunctionDef: n}}
	case *ast.If:
		return &ast.Node{Node: &ast.Node_If{If: n}}
	case *ast.Module:
		return &ast.Node{Node: &ast.Node_Module{Module: n}}
	default:
		return nil
	}
}

func wrapExpression(node proto.Message) *ast.Node {
	switch n := node.(type) {
	case *ast.Alias:
		return &ast.Node{Node: &ast.Node_Alias{Alias: n}}
	case *ast.Await:
		return &ast.Node{Node: &ast.Node_Await{Await: n}}
	case *ast.Attribute:
		return &ast.Node{Node: &ast.Node_Attribute{Attribute: n}}
	case *ast.Call:
		return &ast.Node{Node: &ast.Node_Call{Call: n}}
	case *ast.Comment:
		return &ast.Node{Node: &ast.Node_Comment{Comment: n}}
	case *ast.Compare:
		return &ast.Node{Node: &ast.Node_Compare{Compare: n}}
	case *ast.Expr:
		return &ast.Node{Node: &ast.Node_Expr{Expr: n}}
	case *ast.Yield:
		return &ast.Node{Node: &ast.Node_Yield{Yield: n}}
	default:
		return nil
	}
}

func Constant(value any) *ast.Node {
	switch n := value.(type) {
	case string:
		return &ast.Node{
			Node: &ast.Node_Constant{
				Constant: &ast.Constant{
					Value: &ast.Constant_Str{Str: n},
				},
			},
		}
	case int:
		if int64(n) < -1<<31 || int64(n) > 1<<31-1 {
			panic("integer does not fit Python AST int")
		}
		return &ast.Node{
			Node: &ast.Node_Constant{
				Constant: &ast.Constant{
					Value: &ast.Constant_Int{Int: int32(n)},
				},
			},
		}
	case nil:
		return &ast.Node{
			Node: &ast.Node_Constant{
				Constant: &ast.Constant{
					Value: &ast.Constant_None{},
				},
			},
		}
	default:
		panic("unknown constant type")
	}
}
