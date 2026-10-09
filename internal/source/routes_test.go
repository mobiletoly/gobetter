package source_test

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/mobiletoly/gobetter/internal/config"
	"github.com/mobiletoly/gobetter/internal/source"
	"github.com/mobiletoly/gobetter/internal/structgen"
)

func TestGetterOnlyAdapterRoutes(t *testing.T) {
	module := t.TempDir()
	fileName := filepath.Join(module, "models.go")
	cases := []struct {
		name, src, json string
		invocation      structgen.Invocation
	}{
		{"annotation", "package models\n//gobetter:struct getters\ntype Secret struct {\n//gobetter:field getter\nvalue int\n}", "", structgen.Invocation{}},
		{"configuration", "package models\ntype Secret struct {value int}", `{"schema_version":1,"types":[{"package":".","type":"Secret","derive":["getters"],"fields":{"value":{"getter":true}}}]}`, structgen.Invocation{}},
		{"explicit CLI output override", "package models\ntype Secret struct {\n//gobetter:field getter\nvalue int\n}", "", structgen.Invocation{Selection: structgen.Exported, Options: structgen.TypeOptions{Derive: new(structgen.Getters)}}},
	}
	var first behavior
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file, err := source.Parse(fileName, []byte(tc.src))
			if err != nil {
				t.Fatal(err)
			}
			var overrides []structgen.TypeOverride
			if tc.json != "" {
				configFile := filepath.Join(module, "gobetter.json")
				document, err := config.Decode(configFile, []byte(tc.json))
				if err != nil {
					t.Fatal(err)
				}
				overrides, err = config.Bind(document, configFile, module, []config.PackageInventory{{Dir: module, Active: file.Targets}})
				if err != nil {
					t.Fatal(err)
				}
			}
			plans, err := structgen.MakePlans(file.Targets, overrides, tc.invocation)
			if err != nil {
				t.Fatal(err)
			}
			if len(plans) != 1 {
				t.Fatalf("wrong selection: %+v", plans)
			}
			p := plans[0]
			if p.Derive != structgen.Getters || p.Constructor != "" || len(p.FieldsFor(structgen.Builder)) != 0 || len(p.FieldsFor(structgen.Equal)) != 0 || len(p.FieldsFor(structgen.String)) != 0 || len(p.FieldsFor(structgen.Getters)) != 1 {
				t.Fatalf("getter-only contract: %+v", p)
			}
			if i == 0 {
				first = describe(t, p)
			} else if !reflect.DeepEqual(first, describe(t, p)) {
				t.Fatal("getter-only routes differ")
			}
		})
	}
}

func TestEffectivePrecedenceAcrossAdapters(t *testing.T) {
	module := t.TempDir()
	src := `package models
//gobetter:struct equal,constructor=exported
type secret struct {
    //gobetter:field getter,optional,skip-builder,skip-equal,skip-string
    value int
}
`
	file, err := source.Parse(filepath.Join(module, "models.go"), []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	json := `{"schema_version":1,"types":[{"package":".","type":"secret","derive":["string"],"constructor":"package","fields":{"value":{"getter":false,"optional":false,"skip_builder":false,"skip_equal":false,"skip_string":false}}}]}`
	configFile := filepath.Join(module, "gobetter.json")
	document, err := config.Decode(configFile, []byte(json))
	if err != nil {
		t.Fatal(err)
	}
	overrides, err := config.Bind(document, configFile, module, []config.PackageInventory{{Dir: module, Active: file.Targets}})
	if err != nil {
		t.Fatal(err)
	}
	plans, err := structgen.MakePlans(file.Targets, overrides, structgen.Invocation{Selection: structgen.Exported, Options: structgen.TypeOptions{Derive: new(structgen.Builder), Constructor: new(structgen.ExportedConstructor)}})
	if err != nil {
		t.Fatal(err)
	}
	p := plans[0]
	if p.Derive != structgen.Builder || p.Constructor != structgen.ExportedConstructor || len(p.FieldsFor(structgen.Builder)) != 1 || len(p.FieldsFor(structgen.Getters)) != 0 {
		t.Fatalf("effective precedence/selection: %+v", p)
	}
	f := p.Fields[0]
	if f.Getter || f.Optional || f.SkipBuilder || f.SkipEqual || f.SkipString {
		t.Fatalf("false did not clear annotation: %+v", f)
	}
}

func TestOuterAnonymousFieldGetterAndNestedConfigRejection(t *testing.T) {
	module := t.TempDir()
	src := "package models\n//gobetter:struct getters\ntype Outer struct {\n//gobetter:field getter\nconfig *struct { value int }\n}\n"
	file, err := source.Parse(filepath.Join(module, "models.go"), []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	plans, err := structgen.MakePlans(file.Targets, nil, structgen.Invocation{})
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 1 || plans[0].FieldsFor(structgen.Getters)[0].GetterName != "Config" {
		t.Fatalf("outer getter missing: %+v", plans)
	}
	configFile := filepath.Join(module, "gobetter.json")
	document, err := config.Decode(configFile, []byte(`{"schema_version":1,"types":[{"package":".","type":"Outer.config","derive":["getters"],"fields":{"value":{"getter":true}}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	overrides, err := config.Bind(document, configFile, module, []config.PackageInventory{{Dir: module, Active: file.Targets}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = structgen.MakePlans(file.Targets, overrides, structgen.Invocation{})
	requireCode(t, err, structgen.UnsupportedTarget)
}

func TestRelativeSourceAndConfigIdentities(t *testing.T) {
	file, err := source.Parse("models.go", []byte("package models\ntype T struct {value int}\n"))
	if err != nil {
		t.Fatal(err)
	}
	dir, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	document, err := config.Decode("gobetter.json", []byte(`{"schema_version":1,"types":[{"package":".","type":"T"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	overrides, err := config.Bind(document, "gobetter.json", dir, []config.PackageInventory{{Dir: ".", Active: file.Targets}})
	if err != nil {
		t.Fatal(err)
	}
	plans, err := structgen.MakePlans(file.Targets, overrides, structgen.Invocation{})
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 1 || plans[0].Target.PackageDir != dir {
		t.Fatalf("relative identities failed to match: %+v", plans)
	}
}
