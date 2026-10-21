package python

import (
	pyast "github.com/sqlc-dev/sqlc-gen-python/internal/ast"
	"github.com/sqlc-dev/sqlc-gen-python/internal/poet"
)

func buildModelsTree(ctx *pyTmplCtx, i *importer) *pyast.Node {
	mod := moduleNode(ctx.SqlcVersion, "")
	std, pkg := i.modelImportSpecs()
	mod.Body = append(mod.Body, buildImportGroup(std), buildImportGroup(pkg))

	for _, e := range ctx.Enums {
		bases := []*pyast.Node{
			poet.Name(pythonStrType),
			poet.Attribute(poet.Name("enum"), "Enum"),
		}
		if i.C.EmitStrEnum {
			// override the bases to emit enum.StrEnum (only support in Python >=3.11)
			bases = []*pyast.Node{
				poet.Attribute(poet.Name("enum"), "StrEnum"),
			}
		}
		def := &pyast.ClassDef{
			Name:  e.Name,
			Bases: bases,
		}
		if e.Comment != "" {
			def.Body = append(def.Body, &pyast.Node{
				Node: &pyast.Node_Expr{
					Expr: &pyast.Expr{
						Value: poet.Constant(e.Comment),
					},
				},
			})
		}
		for _, c := range e.Constants {
			def.Body = append(def.Body, assignNode(c.Name, poet.Constant(c.Value)))
		}
		mod.Body = append(mod.Body, &pyast.Node{
			Node: &pyast.Node_ClassDef{
				ClassDef: def,
			},
		})
	}

	for _, m := range ctx.Models {
		var def *pyast.ClassDef
		if ctx.C.EmitPydanticModels {
			def = pydanticNode(m.Name)
		} else {
			def = dataclassNode(m.Name)
		}
		if m.Comment != "" {
			def.Body = append(def.Body, &pyast.Node{
				Node: &pyast.Node_Expr{
					Expr: &pyast.Expr{
						Value: poet.Constant(m.Comment),
					},
				},
			})
		}
		for _, f := range m.Fields {
			def.Body = append(def.Body, fieldNode(f))
		}
		mod.Body = append(mod.Body, &pyast.Node{
			Node: &pyast.Node_ClassDef{
				ClassDef: def,
			},
		})
	}

	return &pyast.Node{Node: &pyast.Node_Module{Module: mod}}
}
