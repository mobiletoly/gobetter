package source_test

import (
	"context"
	"go/parser"
	"go/token"
	"maps"
	"strconv"
	"testing"

	"github.com/mobiletoly/gobetter/internal/source"
)

func bootstrapImports(t *testing.T, r *source.Result) map[string]string {
	t.Helper()
	if len(r.Bootstrap) != 1 {
		t.Fatalf("expected one bootstrap artifact, got %d", len(r.Bootstrap))
	}
	bindings := make(map[string]string)
	for path, code := range r.Bootstrap {
		file, err := parser.ParseFile(token.NewFileSet(), path, code, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, spec := range file.Imports {
			path, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			name := ""
			if spec.Name != nil {
				name = spec.Name.Name
			}
			bindings[path] = name
		}
	}
	return bindings
}

func TestImportNamesAndDotBinding(t *testing.T) {
	dir := module(t, map[string]string{
		"library/types.go": "package differently\ntype Kind int\n",
		"model.go":         "package models\nimport . \"fixture/library\"\n//gobetter:struct builder\ntype Item struct { Value Kind }\nvar _=NewItemBuilder().Value(Kind(1)).Build()\n",
	})
	r, err := source.Load(context.Background(), source.Request{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if imports := bootstrapImports(t, r); !maps.Equal(imports, map[string]string{"fixture/library": "."}) {
		t.Fatalf("dot import was not preserved in emitted signatures: %v", imports)
	}
	compileOverlay(t, dir, r.Bootstrap)
	// A source import may be used only in authored code, outside output signatures.
	write(t, dir, "model.go", "package models\nimport . \"fixture/library\"\n//gobetter:struct builder\ntype Item struct { Value int }\nvar _=Kind(1)\nvar _=NewItemBuilder().Value(1).Build()\n")
	r, err = source.Load(context.Background(), source.Request{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if imports := bootstrapImports(t, r); len(imports) != 0 {
		t.Fatalf("authored-only dot import leaked into emitted signatures: %v", imports)
	}
	write(t, dir, "model.go", "package models\nimport \"fixture/library\"\n//gobetter:struct builder\ntype Item struct { Value differently.Kind }\nvar _=NewItemBuilder().Value(differently.Kind(1)).Build()\n")
	r, err = source.Load(context.Background(), source.Request{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if imports := bootstrapImports(t, r); !maps.Equal(imports, map[string]string{"fixture/library": "differently"}) {
		t.Fatalf("emitted import did not use the declared package name: %v", imports)
	}
	compileOverlay(t, dir, r.Bootstrap)
}
