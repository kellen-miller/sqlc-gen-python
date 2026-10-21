package python

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/sqlc-dev/plugin-sdk-go/metadata"
	"github.com/sqlc-dev/plugin-sdk-go/plugin"
	"github.com/sqlc-dev/plugin-sdk-go/sdk"

	pyast "github.com/sqlc-dev/sqlc-gen-python/internal/ast"
	"github.com/sqlc-dev/sqlc-gen-python/internal/poet"
)

type QueryValue struct {
	Struct *Struct
	Name   string
	Typ    pyType
	Emit   bool
}

func (v QueryValue) Annotation() *pyast.Node {
	if v.Typ != (pyType{}) {
		return v.Typ.Annotation()
	}
	if v.Struct != nil {
		if v.Emit {
			return poet.Name(v.Struct.Name)
		} else {
			return typeRefNode("models", v.Struct.Name)
		}
	}
	panic("no type for QueryValue: " + v.Name)
}

func (v QueryValue) EmitStruct() bool {
	return v.Emit
}

func (v QueryValue) IsStruct() bool {
	return v.Struct != nil
}

func (v QueryValue) isEmpty() bool {
	return v.Typ == (pyType{}) && v.Name == "" && v.Struct == nil
}

func (v QueryValue) RowNode(rowVar string) *pyast.Node {
	if !v.IsStruct() {
		return subscriptNode(
			rowVar,
			poet.Constant(0),
		)
	}
	call := &pyast.Call{
		Func: v.Annotation(),
	}
	for i, f := range v.Struct.Fields {
		call.Keywords = append(call.Keywords, &pyast.Keyword{
			Arg: f.Name,
			Value: subscriptNode(
				rowVar,
				poet.Constant(i),
			),
		})
	}
	return &pyast.Node{
		Node: &pyast.Node_Call{
			Call: call,
		},
	}
}

// A struct used to generate methods and fields on the Queries struct
type Query struct {
	Cmd          string
	Comments     []string
	MethodName   string
	FieldName    string
	ConstantName string
	SQL          string
	SourceName   string
	Args         []QueryValue
	Ret          QueryValue
}

const (
	queryCmdOne                = ":one"
	queryCmdMany               = ":many"
	defaultQueryParameterLimit = 4
)

func (q *Query) AddArgs(args *pyast.Arguments) {
	// A single struct arg does not need to be passed as a keyword argument
	if len(q.Args) == 1 && q.Args[0].IsStruct() {
		args.Args = append(args.Args, &pyast.Arg{
			Arg:        q.Args[0].Name,
			Annotation: q.Args[0].Annotation(),
		})
		return
	}
	for _, a := range q.Args {
		args.KwOnlyArgs = append(args.KwOnlyArgs, &pyast.Arg{
			Arg:        a.Name,
			Annotation: a.Annotation(),
		})
	}
}

func (q *Query) ArgDictNode() *pyast.Node {
	dict := &pyast.Dict{}
	i := 1
	hasStructArgs := false
	for _, a := range q.Args {
		if a.isEmpty() {
			continue
		}
		if a.IsStruct() {
			hasStructArgs = true
			for _, f := range a.Struct.Fields {
				dict.Keys = append(dict.Keys, poet.Constant(fmt.Sprintf("p%v", i)))
				dict.Values = append(dict.Values, typeRefNode(a.Name, f.Name))
				i++
			}
		} else {
			dict.Keys = append(dict.Keys, poet.Constant(fmt.Sprintf("p%v", i)))
			dict.Values = append(dict.Values, poet.Name(a.Name))
			i++
		}
	}
	if len(dict.GetKeys()) == 0 && !hasStructArgs {
		return nil
	}
	return &pyast.Node{
		Node: &pyast.Node_Dict{
			Dict: dict,
		},
	}
}

var matchFirstCap = regexp.MustCompile("(.)([A-Z][a-z]+)")
var matchAllCap = regexp.MustCompile("([a-z0-9])([A-Z])")

func methodName(name string) string {
	snake := matchFirstCap.ReplaceAllString(name, "${1}_${2}")
	snake = matchAllCap.ReplaceAllString(snake, "${1}_${2}")
	return strings.ToLower(snake)
}

type pyColumn struct {
	Column *plugin.Column
	id     int
}

func paramName(p *plugin.Parameter) string {
	if p.GetColumn().GetName() != "" {
		return p.GetColumn().GetName()
	}
	return fmt.Sprintf("dollar_%d", p.GetNumber())
}

func columnsToStruct(req *plugin.GenerateRequest, name string, columns []pyColumn) *Struct {
	gs := Struct{
		Name: name,
	}
	seen := map[string]int{}
	suffixes := map[int]int{}
	for i, c := range columns {
		colName := columnName(c.Column, i)
		fieldName := colName
		// Track suffixes by the ID of the column, so that columns referring to
		// the same numbered parameter can be reused.
		var suffix int
		if o, ok := suffixes[c.id]; ok {
			suffix = o
		} else if v := seen[colName]; v > 0 {
			suffix = v + 1
		}
		suffixes[c.id] = suffix
		if suffix > 0 {
			fieldName = fmt.Sprintf("%s_%d", fieldName, suffix)
		}
		gs.Fields = append(gs.Fields, Field{
			Name: fieldName,
			Type: makePyType(req, c.Column),
		})
		seen[colName]++
	}
	return &gs
}

var postgresPlaceholderRegexp = regexp.MustCompile(`\B\$(\d+)\b`)

// Sqlalchemy uses ":name" for placeholders, so "$N" is converted to ":pN"
// This also means ":" has special meaning to sqlalchemy, so it must be escaped.
func sqlalchemySQL(s, engine string) string {
	s = strings.ReplaceAll(s, ":", `\\:`)
	if engine == "postgresql" {
		return postgresPlaceholderRegexp.ReplaceAllString(s, ":p$1")
	}
	return s
}

func buildQueries(conf Config, req *plugin.GenerateRequest, structs []Struct) ([]*Query, error) {
	queries := make([]*Query, 0, len(req.GetQueries()))
	for _, query := range req.GetQueries() {
		if query.GetName() == "" || query.GetCmd() == "" {
			continue
		}
		built, err := buildQuery(conf, req, query, structs)
		if err != nil {
			return nil, err
		}
		queries = append(queries, built)
	}
	sort.Slice(queries, func(i, j int) bool { return queries[i].MethodName < queries[j].MethodName })
	return queries, nil
}

func buildQuery(conf Config, req *plugin.GenerateRequest, source *plugin.Query, structs []Struct) (*Query, error) {
	if source.GetCmd() == metadata.CmdCopyFrom {
		return nil, errors.New("support for CopyFrom in Python is not implemented")
	}
	queryMethodName := methodName(source.GetName())
	query := &Query{
		Cmd:          source.GetCmd(),
		Comments:     source.GetComments(),
		MethodName:   queryMethodName,
		FieldName:    sdk.LowerTitle(source.GetName()) + "Stmt",
		ConstantName: strings.ToUpper(queryMethodName),
		SQL:          sqlalchemySQL(source.GetText(), req.GetSettings().GetEngine()),
		SourceName:   source.GetFilename(),
	}
	args, err := buildQueryArgs(conf, req, source)
	if err != nil {
		return nil, err
	}
	query.Args = args
	query.Ret = buildQueryResult(req, source, structs)
	return query, nil
}

func buildQueryArgs(conf Config, req *plugin.GenerateRequest, query *plugin.Query) ([]QueryValue, error) {
	parameterLimit := int32(defaultQueryParameterLimit)
	if conf.QueryParameterLimit != nil {
		parameterLimit = *conf.QueryParameterLimit
	}
	if parameterLimit < 0 {
		return nil, errors.New("invalid query parameter limit")
	}
	params := query.GetParams()
	if len(params) > int(parameterLimit) || parameterLimit == 0 {
		columns := make([]pyColumn, 0, len(params))
		for _, parameter := range params {
			columns = append(columns, pyColumn{
				Column: parameter.GetColumn(),
				id:     int(parameter.GetNumber()),
			})
		}
		return []QueryValue{{
			Emit:   true,
			Name:   "arg",
			Struct: columnsToStruct(req, query.GetName()+"Params", columns),
		}}, nil
	}
	args := make([]QueryValue, 0, len(params))
	for _, parameter := range params {
		args = append(args, QueryValue{
			Name: paramName(parameter),
			Typ:  makePyType(req, parameter.GetColumn()),
		})
	}
	return args, nil
}

func buildQueryResult(req *plugin.GenerateRequest, query *plugin.Query, structs []Struct) QueryValue {
	columns := query.GetColumns()
	switch len(columns) {
	case 0:
		return QueryValue{}
	case 1:
		column := columns[0]
		return QueryValue{
			Name: columnName(column, 0),
			Typ:  makePyType(req, column),
		}
	default:
		for structIndex := range structs {
			model := &structs[structIndex]
			if modelMatchesColumns(req, model, columns) {
				return QueryValue{Name: "i", Struct: model}
			}
		}
		generated := make([]pyColumn, 0, len(columns))
		for index, column := range columns {
			generated = append(generated, pyColumn{
				Column: column,
				id:     index,
			})
		}
		return QueryValue{
			Emit:   true,
			Name:   "i",
			Struct: columnsToStruct(req, query.GetName()+"Row", generated),
		}
	}
}

func modelMatchesColumns(req *plugin.GenerateRequest, model *Struct, columns []*plugin.Column) bool {
	if len(model.Fields) != len(columns) {
		return false
	}
	for index, field := range model.Fields {
		column := columns[index]
		modelType := makePyType(req, column)
		modelType.InnerType = strings.TrimPrefix(modelType.InnerType, "models.")
		if field.Name != columnName(column, index) || field.Type != modelType {
			return false
		}
	}
	return sdk.SameTableName(columns[0].GetTable(), model.Table, req.GetCatalog().GetDefaultSchema())
}
