package python

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestBuildImportBlock2SortsByModuleBeforeName(t *testing.T) {
	imports := map[string]importSpec{
		"alpha-module": {Module: "alpha-module", Name: "Zulu"},
		"zulu-module":  {Module: "zulu-module", Name: "Alpha"},
	}

	got := buildImportBlock2(imports)
	want := []importFromSpec{
		{Module: "alpha-module", Names: []string{"Zulu"}},
		{Module: "zulu-module", Names: []string{"Alpha"}},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("import order mismatch (-want +got):\n%s", diff)
	}
}
