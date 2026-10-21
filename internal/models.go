package python

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/sqlc-dev/plugin-sdk-go/plugin"
	pyast "github.com/sqlc-dev/sqlc-gen-python/internal/ast"
	"github.com/sqlc-dev/sqlc-gen-python/internal/inflection"
	"github.com/sqlc-dev/sqlc-gen-python/internal/poet"
)

const (
	pythonAnyType        = "Any"
	pythonStrType        = "str"
	pythonPydanticModule = "pydantic"
	pythonTypingModule   = "typing"
	pythonSelfName       = "self"
	postgresCatalog      = "pg_catalog"
	informationSchema    = "information_schema"
)

type Constant struct {
	Name  string
	Type  string
	Value string
}

type Enum struct {
	Name      string
	Comment   string
	Constants []Constant
}

type pyType struct {
	InnerType string
	IsArray   bool
	IsNull    bool
}

func (t pyType) Annotation() *pyast.Node {
	ann := poet.Name(t.InnerType)
	if t.IsArray {
		ann = subscriptNode("List", ann)
	}
	if t.IsNull {
		ann = subscriptNode("Optional", ann)
	}
	return ann
}

type Field struct {
	Name    string
	Comment string
	Type    pyType
}

type Struct struct {
	Table   *plugin.Identifier
	Name    string
	Comment string
	Fields  []Field
}

var pyIdentPattern = regexp.MustCompile("[^a-zA-Z0-9_]+")

func makePyType(req *plugin.GenerateRequest, col *plugin.Column) pyType {
	typ := pyInnerType(req, col)
	return pyType{
		InnerType: typ,
		IsArray:   col.GetIsArray(),
		IsNull:    !col.GetNotNull(),
	}
}

func pyInnerType(req *plugin.GenerateRequest, col *plugin.Column) string {
	switch req.GetSettings().GetEngine() {
	case "postgresql":
		return postgresType(req, col)
	default:
		fmt.Fprintln(os.Stderr, "unsupported engine type")
		return pythonAnyType
	}
}

func modelName(name string) string {
	var out strings.Builder
	for p := range strings.SplitSeq(name, "_") {
		out.WriteString(cases.Title(language.Und, cases.NoLower).String(p))
	}
	return out.String()
}

func pyEnumValueName(value string) string {
	id := strings.ReplaceAll(value, "-", "_")
	id = strings.ReplaceAll(id, ":", "_")
	id = strings.ReplaceAll(id, "/", "_")
	id = pyIdentPattern.ReplaceAllString(id, "")
	return strings.ToUpper(id)
}

func buildEnums(req *plugin.GenerateRequest) []Enum {
	var enums []Enum
	for _, schema := range req.GetCatalog().GetSchemas() {
		if schema.GetName() == postgresCatalog || schema.GetName() == informationSchema {
			continue
		}
		for _, enum := range schema.GetEnums() {
			var enumName string
			if schema.GetName() == req.GetCatalog().GetDefaultSchema() {
				enumName = enum.GetName()
			} else {
				enumName = schema.GetName() + "_" + enum.GetName()
			}
			e := Enum{
				Name:    modelName(enumName),
				Comment: enum.GetComment(),
			}
			for _, v := range enum.GetVals() {
				e.Constants = append(e.Constants, Constant{
					Name:  pyEnumValueName(v),
					Value: v,
					Type:  e.Name,
				})
			}
			enums = append(enums, e)
		}
	}
	if len(enums) > 0 {
		sort.Slice(enums, func(i, j int) bool { return enums[i].Name < enums[j].Name })
	}
	return enums
}

func buildModels(conf Config, req *plugin.GenerateRequest) []Struct {
	var structs []Struct
	for _, schema := range req.GetCatalog().GetSchemas() {
		if schema.GetName() == postgresCatalog || schema.GetName() == informationSchema {
			continue
		}
		for _, table := range schema.GetTables() {
			var tableName string
			if schema.GetName() == req.GetCatalog().GetDefaultSchema() {
				tableName = table.GetRel().GetName()
			} else {
				tableName = schema.GetName() + "_" + table.GetRel().GetName()
			}
			structName := tableName
			if !conf.EmitExactTableNames {
				structName = inflection.Singular(inflection.SingularParams{
					Name:       structName,
					Exclusions: conf.InflectionExcludeTableNames,
				})
			}
			s := Struct{
				Table:   &plugin.Identifier{Schema: schema.GetName(), Name: table.GetRel().GetName()},
				Name:    modelName(structName),
				Comment: table.GetComment(),
			}
			for _, column := range table.GetColumns() {
				typ := makePyType(req, column)
				typ.InnerType = strings.TrimPrefix(typ.InnerType, "models.")
				s.Fields = append(s.Fields, Field{
					Name:    column.GetName(),
					Type:    typ,
					Comment: column.GetComment(),
				})
			}
			structs = append(structs, s)
		}
	}
	if len(structs) > 0 {
		sort.Slice(structs, func(i, j int) bool { return structs[i].Name < structs[j].Name })
	}
	return structs
}

func columnName(c *plugin.Column, pos int) string {
	if c.GetName() != "" {
		return c.GetName()
	}
	return fmt.Sprintf("column_%d", pos+1)
}
