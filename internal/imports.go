package python

import (
	"fmt"
	"sort"
	"strings"
)

type importSpec struct {
	Module string
	Name   string
	Alias  string
}

func (i importSpec) String() string {
	if i.Alias != "" {
		if i.Name == "" {
			return fmt.Sprintf("import %s as %s", i.Module, i.Alias)
		}
		return fmt.Sprintf("from %s import %s as %s", i.Module, i.Name, i.Alias)
	}
	if i.Name == "" {
		return "import " + i.Module
	}
	return fmt.Sprintf("from %s import %s", i.Module, i.Name)
}

type importer struct {
	Models  []Struct
	Queries []*Query
	Enums   []Enum
	C       Config
}

func structUses(name string, s *Struct) bool {
	for _, f := range s.Fields {
		if name == pythonTypingModule+".List" && f.Type.IsArray {
			return true
		}
		if name == pythonTypingModule+".Optional" && f.Type.IsNull {
			return true
		}
		if f.Type.InnerType == name {
			return true
		}
	}
	return false
}

func queryValueUses(name string, qv QueryValue) bool {
	if qv.isEmpty() {
		return false
	}
	if name == pythonTypingModule+".List" && qv.Typ.IsArray {
		return true
	}
	if name == pythonTypingModule+".Optional" && qv.Typ.IsNull {
		return true
	}
	if qv.IsStruct() && qv.EmitStruct() {
		return structUses(name, qv.Struct)
	}
	return qv.Typ.InnerType == name
}

func (i *importer) Imports(fileName string) []string {
	if fileName == "models.py" {
		return i.modelImports()
	}
	return i.queryImports(fileName)
}

func (i *importer) modelImportSpecs() (map[string]importSpec, map[string]importSpec) {
	modelUses := func(name string) bool {
		for modelIndex := range i.Models {
			if structUses(name, &i.Models[modelIndex]) {
				return true
			}
		}
		return false
	}

	std := stdImports(modelUses)
	if i.C.EmitPydanticModels {
		std[pythonPydanticModule] = importSpec{Module: pythonPydanticModule}
	} else {
		std["dataclasses"] = importSpec{Module: "dataclasses"}
	}
	if len(i.Enums) > 0 {
		std["enum"] = importSpec{Module: "enum"}
	}

	pkg := make(map[string]importSpec)

	return std, pkg
}

func (i *importer) modelImports() []string {
	std, pkg := i.modelImportSpecs()
	importLines := []string{
		buildImportBlock(std),
		"",
		buildImportBlock(pkg),
	}
	return importLines
}

func (i *importer) queryUses(name, fileName string) bool {
	for _, query := range i.Queries {
		if query.SourceName != fileName {
			continue
		}
		if queryValueUses(name, query.Ret) {
			return true
		}
		for _, arg := range query.Args {
			if queryValueUses(name, arg) {
				return true
			}
		}
	}
	return false
}

func addQueryValueModelImports(std map[string]importSpec, qv QueryValue, conf Config) {
	if !qv.IsStruct() || !qv.EmitStruct() {
		return
	}
	if conf.EmitPydanticModels {
		std[pythonPydanticModule] = importSpec{Module: pythonPydanticModule}
		return
	}
	std["dataclasses"] = importSpec{Module: "dataclasses"}
}

func (i *importer) queryImportSpecs(fileName string) (map[string]importSpec, map[string]importSpec) {
	std := stdImports(func(name string) bool {
		return i.queryUses(name, fileName)
	})

	pkg := make(map[string]importSpec)
	pkg["sqlalchemy"] = importSpec{Module: "sqlalchemy"}
	if i.C.EmitAsyncQuerier {
		pkg["sqlalchemy.ext.asyncio"] = importSpec{Module: "sqlalchemy.ext.asyncio"}
	}

	for _, query := range i.Queries {
		if query.SourceName != fileName {
			continue
		}
		if query.Cmd == queryCmdOne {
			std[pythonTypingModule+".Optional"] = importSpec{Module: pythonTypingModule, Name: "Optional"}
		}
		if query.Cmd == queryCmdMany {
			if i.C.EmitSyncQuerier {
				std[pythonTypingModule+".Iterator"] = importSpec{Module: pythonTypingModule, Name: "Iterator"}
			}
			if i.C.EmitAsyncQuerier {
				std[pythonTypingModule+".AsyncIterator"] = importSpec{Module: pythonTypingModule, Name: "AsyncIterator"}
			}
		}
		addQueryValueModelImports(std, query.Ret, i.C)
		for _, qv := range query.Args {
			addQueryValueModelImports(std, qv, i.C)
		}
	}

	return std, pkg
}

func (i *importer) queryImports(fileName string) []string {
	std, pkg := i.queryImportSpecs(fileName)

	modelImportStr := fmt.Sprintf("from %s import models", i.C.Package)

	importLines := []string{
		buildImportBlock(std),
		"",
		buildImportBlock(pkg),
		"",
		modelImportStr,
	}
	return importLines
}

type importFromSpec struct {
	Module string
	Alias  string
	Names  []string
}

func buildImportBlock2(pkgs map[string]importSpec) []importFromSpec {
	pkgImports := make([]importFromSpec, 0)
	fromImports := make(map[string][]string)
	for _, is := range pkgs {
		if is.Name == "" || is.Alias != "" {
			pkgImports = append(pkgImports, importFromSpec{
				Module: is.Module,
				Names:  []string{is.Name},
				Alias:  is.Alias,
			})
		} else {
			names, ok := fromImports[is.Module]
			if !ok {
				names = make([]string, 0, 1)
			}
			names = append(names, is.Name)
			fromImports[is.Module] = names
		}
	}
	for modName, names := range fromImports {
		sort.Strings(names)
		pkgImports = append(pkgImports, importFromSpec{
			Module: modName,
			Names:  names,
		})
	}
	sort.Slice(pkgImports, func(i, j int) bool {
		if pkgImports[i].Module != pkgImports[j].Module {
			return pkgImports[i].Module < pkgImports[j].Module
		}
		return pkgImports[i].Names[0] < pkgImports[j].Names[0]
	})
	return pkgImports
}

func buildImportBlock(pkgs map[string]importSpec) string {
	pkgImports := make([]importSpec, 0)
	fromImports := make(map[string][]string)
	for _, is := range pkgs {
		if is.Name == "" || is.Alias != "" {
			pkgImports = append(pkgImports, is)
		} else {
			names, ok := fromImports[is.Module]
			if !ok {
				names = make([]string, 0, 1)
			}
			names = append(names, is.Name)
			fromImports[is.Module] = names
		}
	}

	importStrings := make([]string, 0, len(pkgImports)+len(fromImports))
	for _, is := range pkgImports {
		importStrings = append(importStrings, is.String())
	}
	for modName, names := range fromImports {
		sort.Strings(names)
		nameString := strings.Join(names, ", ")
		importStrings = append(importStrings, fmt.Sprintf("from %s import %s", modName, nameString))
	}
	sort.Strings(importStrings)
	return strings.Join(importStrings, "\n")
}

func stdImports(uses func(name string) bool) map[string]importSpec {
	std := make(map[string]importSpec)
	if uses("decimal.Decimal") {
		std["decimal"] = importSpec{Module: "decimal"}
	}
	if uses("datetime.date") || uses("datetime.time") || uses("datetime.datetime") || uses("datetime.timedelta") {
		std["datetime"] = importSpec{Module: "datetime"}
	}
	if uses("uuid.UUID") {
		std["uuid"] = importSpec{Module: "uuid"}
	}
	if uses(pythonTypingModule + ".List") {
		std[pythonTypingModule+".List"] = importSpec{Module: pythonTypingModule, Name: "List"}
	}
	if uses(pythonTypingModule + ".Optional") {
		std[pythonTypingModule+".Optional"] = importSpec{Module: pythonTypingModule, Name: "Optional"}
	}
	if uses(pythonAnyType) {
		std[pythonTypingModule+".Any"] = importSpec{Module: pythonTypingModule, Name: pythonAnyType}
	}
	return std
}
