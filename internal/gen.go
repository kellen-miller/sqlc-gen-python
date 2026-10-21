package python

import (
	"context"
	json "encoding/json"
	"fmt"
	"strings"

	"github.com/sqlc-dev/plugin-sdk-go/plugin"

	pyprint "github.com/sqlc-dev/sqlc-gen-python/internal/printer"
)

type pyTmplCtx struct {
	SqlcVersion string
	Models      []Struct
	Queries     []*Query
	Enums       []Enum
	C           Config
}

func HashComment(s string) string {
	return "# " + strings.ReplaceAll(s, "\n", "\n# ")
}

func Generate(_ context.Context, req *plugin.GenerateRequest) (*plugin.GenerateResponse, error) {
	var conf Config
	if len(req.GetPluginOptions()) > 0 {
		if err := json.Unmarshal(req.GetPluginOptions(), &conf); err != nil {
			return nil, fmt.Errorf("unmarshal plugin options: %w", err)
		}
	}

	enums := buildEnums(req)
	models := buildModels(conf, req)
	queries, err := buildQueries(conf, req, models)
	if err != nil {
		return nil, err
	}

	i := &importer{
		Models:  models,
		Queries: queries,
		Enums:   enums,
		C:       conf,
	}

	tctx := pyTmplCtx{
		Models:      models,
		Queries:     queries,
		Enums:       enums,
		SqlcVersion: req.GetSqlcVersion(),
		C:           conf,
	}

	output := map[string]string{}
	modelsResult := pyprint.Print(buildModelsTree(&tctx, i), pyprint.Options{})
	output["models.py"] = string(modelsResult.Python)

	files := map[string]bool{}
	for _, q := range queries {
		files[q.SourceName] = true
	}

	for source := range files {
		queryResult := pyprint.Print(buildQueryTree(&tctx, i, source), pyprint.Options{})
		if !strings.HasSuffix(source, ".py") {
			source = strings.TrimSuffix(source, ".sql") + ".py"
		}
		output[source] = string(queryResult.Python)
	}

	resp := plugin.GenerateResponse{}

	for filename, code := range output {
		resp.Files = append(resp.Files, &plugin.File{
			Name:     filename,
			Contents: []byte(code),
		})
	}

	return &resp, nil
}
