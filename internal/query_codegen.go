package python

import (
	"fmt"

	pyast "github.com/sqlc-dev/sqlc-gen-python/internal/ast"
	"github.com/sqlc-dev/sqlc-gen-python/internal/poet"
)

func querierClassDef() *pyast.ClassDef {
	return querierClass("Querier", typeRefNode("sqlalchemy", "engine", "Connection"))
}

func asyncQuerierClassDef() *pyast.ClassDef {
	return querierClass("AsyncQuerier", typeRefNode("sqlalchemy", "ext", "asyncio", "AsyncConnection"))
}

func querierClass(name string, connectionType *pyast.Node) *pyast.ClassDef {
	return &pyast.ClassDef{
		Name: name,
		Body: []*pyast.Node{
			poet.Node(&pyast.FunctionDef{
				Name: "__init__",
				Args: &pyast.Arguments{
					Args: []*pyast.Arg{
						{Arg: pythonSelfName},
						{Arg: "conn", Annotation: connectionType},
					},
				},
				Body: []*pyast.Node{
					poet.Node(&pyast.Assign{
						Targets: []*pyast.Node{
							poet.Attribute(poet.Name(pythonSelfName), "_conn"),
						},
						Value: poet.Name("conn"),
					}),
				},
			}),
		},
	}
}

func buildQueryTree(ctx *pyTmplCtx, i *importer, source string) *pyast.Node {
	mod := moduleNode(ctx.SqlcVersion, source)
	std, pkg := i.queryImportSpecs(source)
	mod.Body = append(mod.Body,
		buildImportGroup(std),
		buildImportGroup(pkg),
		queryModelsImportNode(ctx.C.Package),
	)
	mod.Body = append(mod.Body, buildQueryDeclarations(ctx, source)...)
	if ctx.C.EmitSyncQuerier {
		mod.Body = append(mod.Body, buildSyncQuerier(ctx, source))
	}
	if ctx.C.EmitAsyncQuerier {
		mod.Body = append(mod.Body, buildAsyncQuerier(ctx, source))
	}
	return poet.Node(mod)
}

func queryModelsImportNode(packageName string) *pyast.Node {
	return &pyast.Node{
		Node: &pyast.Node_ImportGroup{
			ImportGroup: &pyast.ImportGroup{
				Imports: []*pyast.Node{
					{
						Node: &pyast.Node_ImportFrom{
							ImportFrom: &pyast.ImportFrom{
								Module: packageName,
								Names:  []*pyast.Node{poet.Alias("models")},
							},
						},
					},
				},
			},
		},
	}
}

func buildQueryDeclarations(ctx *pyTmplCtx, source string) []*pyast.Node {
	var declarations []*pyast.Node
	for _, query := range ctx.Queries {
		if query.SourceName != source {
			continue
		}
		queryText := fmt.Sprintf("-- name: %s \\\\%s\n%s\n", query.MethodName, query.Cmd, query.SQL)
		declarations = append(declarations, assignNode(query.ConstantName, poet.Constant(queryText)))
		for _, arg := range query.Args {
			if !arg.EmitStruct() {
				continue
			}
			declarations = append(declarations, queryStructNode(ctx, arg.Struct))
		}
		if query.Ret.EmitStruct() {
			declarations = append(declarations, queryStructNode(ctx, query.Ret.Struct))
		}
	}
	return declarations
}

func queryStructNode(ctx *pyTmplCtx, model *Struct) *pyast.Node {
	var definition *pyast.ClassDef
	if ctx.C.EmitPydanticModels {
		definition = pydanticNode(model.Name)
	} else {
		definition = dataclassNode(model.Name)
	}
	for _, field := range model.Fields {
		definition.Body = append(definition.Body, fieldNode(field))
	}
	return poet.Node(definition)
}

func buildSyncQuerier(ctx *pyTmplCtx, source string) *pyast.Node {
	return buildQuerier(ctx, source, querierClassDef(), syncQueryMethod)
}

func buildAsyncQuerier(ctx *pyTmplCtx, source string) *pyast.Node {
	return buildQuerier(ctx, source, asyncQuerierClassDef(), asyncQueryMethod)
}

func buildQuerier(ctx *pyTmplCtx, source string, class *pyast.ClassDef, method func(*Query) *pyast.Node) *pyast.Node {
	for _, query := range ctx.Queries {
		if query.SourceName != source {
			continue
		}
		class.Body = append(class.Body, method(query))
	}
	return poet.Node(class)
}

func syncQueryMethod(query *Query) *pyast.Node {
	method := &pyast.FunctionDef{
		Name: query.MethodName,
		Args: &pyast.Arguments{Args: []*pyast.Arg{{Arg: pythonSelfName}}},
	}
	query.AddArgs(method.GetArgs())
	exec := connMethodNode("execute", query.ConstantName, query.ArgDictNode())
	switch query.Cmd {
	case queryCmdOne:
		method.Body = append(method.Body,
			assignNode("row", poet.Node(&pyast.Call{Func: poet.Attribute(exec, "first")})),
			missingRowNode(),
			poet.Return(query.Ret.RowNode("row")),
		)
		method.Returns = subscriptNode("Optional", query.Ret.Annotation())
	case queryCmdMany:
		method.Body = append(method.Body,
			assignNode("result", exec),
			poet.Node(&pyast.For{
				Target: poet.Name("row"),
				Iter:   poet.Name("result"),
				Body:   []*pyast.Node{poet.Expr(poet.Yield(query.Ret.RowNode("row")))},
			}),
		)
		method.Returns = subscriptNode("Iterator", query.Ret.Annotation())
	case ":exec":
		method.Body = append(method.Body, exec)
		method.Returns = poet.Constant(nil)
	case ":execrows":
		method.Body = append(method.Body,
			assignNode("result", exec),
			poet.Return(poet.Attribute(poet.Name("result"), "rowcount")),
		)
		method.Returns = poet.Name("int")
	case ":execresult":
		method.Body = append(method.Body, poet.Return(exec))
		method.Returns = typeRefNode("sqlalchemy", "engine", "Result")
	default:
		panic("unknown cmd " + query.Cmd)
	}
	return poet.Node(method)
}

func asyncQueryMethod(query *Query) *pyast.Node {
	method := &pyast.AsyncFunctionDef{
		Name: query.MethodName,
		Args: &pyast.Arguments{Args: []*pyast.Arg{{Arg: pythonSelfName}}},
	}
	query.AddArgs(method.GetArgs())
	exec := connMethodNode("execute", query.ConstantName, query.ArgDictNode())
	switch query.Cmd {
	case queryCmdOne:
		method.Body = append(method.Body,
			assignNode("row", poet.Node(&pyast.Call{Func: poet.Attribute(poet.Await(exec), "first")})),
			missingRowNode(),
			poet.Return(query.Ret.RowNode("row")),
		)
		method.Returns = subscriptNode("Optional", query.Ret.Annotation())
	case queryCmdMany:
		stream := connMethodNode("stream", query.ConstantName, query.ArgDictNode())
		method.Body = append(method.Body,
			assignNode("result", poet.Await(stream)),
			poet.Node(&pyast.AsyncFor{
				Target: poet.Name("row"),
				Iter:   poet.Name("result"),
				Body:   []*pyast.Node{poet.Expr(poet.Yield(query.Ret.RowNode("row")))},
			}),
		)
		method.Returns = subscriptNode("AsyncIterator", query.Ret.Annotation())
	case ":exec":
		method.Body = append(method.Body, poet.Await(exec))
		method.Returns = poet.Constant(nil)
	case ":execrows":
		method.Body = append(method.Body,
			assignNode("result", poet.Await(exec)),
			poet.Return(poet.Attribute(poet.Name("result"), "rowcount")),
		)
		method.Returns = poet.Name("int")
	case ":execresult":
		method.Body = append(method.Body, poet.Return(poet.Await(exec)))
		method.Returns = typeRefNode("sqlalchemy", "engine", "Result")
	default:
		panic("unknown cmd " + query.Cmd)
	}
	return poet.Node(method)
}

func missingRowNode() *pyast.Node {
	return poet.Node(&pyast.If{
		Test: poet.Node(&pyast.Compare{
			Left: poet.Name("row"),
			Ops:  []*pyast.Node{poet.Is()},
			Comparators: []*pyast.Node{
				poet.Constant(nil),
			},
		}),
		Body: []*pyast.Node{poet.Return(poet.Constant(nil))},
	})
}
