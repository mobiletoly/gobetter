package structgen_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/mobiletoly/gobetter/internal/source"
	"github.com/mobiletoly/gobetter/internal/structgen"
)

func TestOptionalWindowsRuntimeAndCallChains(t *testing.T) {
	files := make(map[string]string)
	for _, name := range []string{"models.go", "models_test.go"} {
		data, err := os.ReadFile(filepath.Join("testdata/runtime/windows", name))
		if err != nil {
			t.Fatal(err)
		}
		files[name] = string(data)
	}
	dir := runtimeModule(t, files)
	r, err := source.Load(t.Context(), source.Request{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	final := runtimeOverlay(t, r)
	if output, err := runOverlay(t, dir, final, "test", "-count=1", "./..."); err != nil {
		t.Fatalf("window runtime fixture: %v\n%s", err, output)
	}
	for _, tc := range []struct{ name, call, diagnostic string }{
		{"all_omitted", `NewWindowBuilder().First(1).Next("n").Build()`, ""},
		{"all_windows", `NewWindowBuilder().Leading("l").First(1).Alias("a").Middle("m").Alias("b").Next("n").Tail("t").BuildPtr()`, ""},
		{"reverse_order", `NewWindowBuilder().First(1).Middle("m").Alias("a").Next("n").Build()`, ""},
		{"zero_required", `NewOptionalBuilder().B("b").A("a").B("c").BuildPtr()`, ""},
		{"only_skipped", `NewSkippedBuilder().BuildPtr()`, ""},
		{"leading_closed", `NewWindowBuilder().First(1).Leading("l")`, "Leading"},
		{"middle_too_early", `NewWindowBuilder().Middle("m")`, "Middle"},
		{"middle_closed", `NewWindowBuilder().First(1).Next("n").Middle("m")`, "Middle"},
		{"alias_closed", `NewWindowBuilder().First(1).Next("n").Alias("a")`, "Alias"},
		{"tail_too_early", `NewWindowBuilder().First(1).Tail("t")`, "Tail"},
		{"initial_build", `NewWindowBuilder().Build()`, "Build"},
		{"initial_build_ptr", `NewWindowBuilder().Leading("l").BuildPtr()`, "BuildPtr"},
		{"window_build", `NewWindowBuilder().First(1).Middle("m").Build()`, "Build"},
		{"window_build_ptr", `NewWindowBuilder().First(1).Alias("a").BuildPtr()`, "BuildPtr"},
		{"required_out_of_order", `NewWindowBuilder().Next("n")`, "Next"},
		{"required_repeated", `NewWindowBuilder().First(1).First(2)`, "First"},
		{"skipped", `NewWindowBuilder().First(1).Ignored(2)`, "Ignored"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writeRuntimeInput(t, dir, "caller.go", "package models\nvar _ = "+tc.call+"\n")
			output, err := runOverlay(t, dir, final, "build", "./...")
			if tc.diagnostic == "" {
				if err != nil {
					t.Fatalf("accepted chain: %v\n%s", err, output)
				}
			} else if err == nil || !strings.Contains(string(output), tc.diagnostic+" undefined") {
				t.Fatalf("rejected chain must identify unavailable %s: %v\n%s", tc.diagnostic, err, output)
			}
		})
	}
	assertNoRuntimePublication(t, r)
}

func TestSourceOrderMethodEmission(t *testing.T) {
	dir := runtimeModule(t, map[string]string{"model.go": `package models
//gobetter:struct builder,equal,string
type Item struct {
	Z int
	z int //gobetter:field getter,optional,name=Zulu
	Ignored int //gobetter:field skip-builder,skip-equal,skip-string
	a int //gobetter:field getter,name=Alpha
	A int
}
`})
	r, err := source.Load(t.Context(), source.Request{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := structgen.Render(r.Plans)
	if err != nil {
		t.Fatal(err)
	}
	file, err := parser.ParseFile(token.NewFileSet(), "generated.go", "package models\n"+rendered.Code, 0)
	if err != nil {
		t.Fatal(err)
	}
	var setters, getters []string
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil {
			continue
		}
		if _, pointer := fn.Recv.List[0].Type.(*ast.StarExpr); pointer {
			getters = append(getters, fn.Name.Name)
		} else if len(fn.Type.Params.List) > 0 && fn.Name.Name != "Equal" {
			setters = append(setters, fn.Name.Name)
		}
	}
	if !slices.Equal(setters, []string{"Z", "Zulu", "Alpha", "A"}) || !slices.Equal(getters, []string{"Zulu", "Alpha"}) {
		t.Fatalf("source-order emission or one-setter-per-field contract: setters=%v getters=%v", setters, getters)
	}
}

func TestBuilderWideSetterCollisionsBeforeTypeLoading(t *testing.T) {
	for _, route := range []string{"annotation", "configuration"} {
		for _, optionals := range [][2]bool{{false, false}, {true, false}, {false, true}, {true, true}} {
			name := route
			for _, optional := range optionals {
				if optional {
					name += "/optional"
				} else {
					name += "/required"
				}
			}
			t.Run(name, func(t *testing.T) {
				fields := []string{"first", "second"}
				model := "package models\n//gobetter:struct builder\ntype Item struct {\n"
				for i, field := range fields {
					options := "name=Same"
					if optionals[i] {
						options += ",optional"
					}
					if route == "annotation" {
						model += "//gobetter:field " + options + "\n"
					}
					model += field + " MissingType\n"
					if i == 0 {
						model += "Between int\n"
					}
				}
				model += "}\n"
				dir := runtimeModule(t, map[string]string{"model.go": model})
				if route == "configuration" {
					config := `{"schema_version":1,"types":[{"package":".","type":"Item","fields":{`
					for i, field := range fields {
						if i > 0 {
							config += ","
						}
						config += `"` + field + `":{"name":"Same","optional":`
						if optionals[i] {
							config += "true}"
						} else {
							config += "false}"
						}
					}
					writeRuntimeInput(t, dir, "gobetter.json", config+"}}]}")
				}
				for range 2 {
					_, err := source.Load(t.Context(), source.Request{Dir: dir})
					ds, ok := err.(structgen.Diagnostics)
					line, firstLine := 8, 5
					if route == "configuration" {
						line, firstLine = 6, 4
					}
					if !ok || len(ds) != 1 || ds[0].Code != structgen.SymbolCollision || ds[0].Position.Line != line || ds[0].Position.Column != 1 || !strings.Contains(ds[0].Message, "setter Same for field second conflicts with field first at "+filepath.Join(dir, "model.go")+":"+strconv.Itoa(firstLine)+":1") {
						t.Fatalf("expected deterministic collision before MissingType loading: %v", err)
					}
				}
			})
		}
	}
}

func TestOptionalBuildNamesReservedInEveryWindow(t *testing.T) {
	for _, name := range []string{"Build", "BuildPtr"} {
		for _, tc := range []struct {
			window, fields string
			line           int
		}{
			{"leading", name + " int //gobetter:field optional\nRequired int\n", 4},
			{"middle", "First int\n" + name + " int //gobetter:field optional\nLast int\n", 5},
			{"trailing", "Required int\n" + name + " int //gobetter:field optional\n", 5},
			{"zero_required", name + " int //gobetter:field optional\n", 4},
			{"leading_comment", "//gobetter:field optional\n" + name + " int\n", 5},
		} {
			t.Run(name+"/"+tc.window, func(t *testing.T) {
				dir := runtimeModule(t, map[string]string{"model.go": "package models\n//gobetter:struct builder\ntype Item struct {\n" + tc.fields + "}\n"})
				_, err := source.Load(t.Context(), source.Request{Dir: dir})
				ds, ok := err.(structgen.Diagnostics)
				if !ok || len(ds) != 1 {
					t.Fatalf("expected one reserved-name diagnostic: %v", err)
				}
				diagnostic := ds[0]
				pos := diagnostic.Position
				if diagnostic.Code != structgen.SymbolCollision || pos.Filename != filepath.Join(dir, "model.go") || pos.Line != tc.line || pos.Column != 1 {
					t.Fatalf("reserved-name diagnostic lost its physical position: %v", diagnostic)
				}
				message := "optional setter " + name + " for field " + name + " uses a reserved build-method name; supply an explicit name"
				if !strings.Contains(diagnostic.Message, message) || strings.Contains(diagnostic.Message, "final-stage") {
					t.Fatalf("incorrect reserved-name diagnostic: %v", diagnostic)
				}
			})
		}
	}
}

func TestSetterNameParticipationAndBuilderScope(t *testing.T) {
	for _, tc := range []struct {
		name, model string
		collision   bool
	}{
		{"default_capitalization", "//gobetter:struct builder\ntype Item struct {value int; Value int}\n", true},
		{"renamed", "//gobetter:struct builder\ntype Item struct {\nvalue int //gobetter:field name=PrivateValue\nValue int\n}\n", false},
		{"skipped", "//gobetter:struct builder\ntype Item struct {\nvalue int //gobetter:field skip-builder\nValue int\n}\n", false},
		{"no_builder", "//gobetter:struct string\ntype Item struct {value int; Value int}\n", false},
		{"different_builders", "//gobetter:struct builder\ntype First struct {Value int}\n//gobetter:struct builder\ntype Second struct {Value int}\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file, err := source.Parse("model.go", []byte("package models\n"+tc.model))
			if err != nil {
				t.Fatal(err)
			}
			_, err = structgen.MakePlans(file.Targets, nil, structgen.Invocation{})
			if !tc.collision {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			ds, ok := err.(structgen.Diagnostics)
			if !ok || len(ds) != 1 || ds[0].Code != structgen.SymbolCollision || ds[0].Position.Filename != "model.go" || ds[0].Position.Line != 3 || !strings.Contains(ds[0].Message, "setter Value for field Value conflicts with field value") {
				t.Fatalf("default effective names must collide: %v", err)
			}
		})
	}
}
