package structgen_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/mobiletoly/gobetter/internal/source"
	"github.com/mobiletoly/gobetter/internal/structgen"
)

func TestOutputsRuntimeSemantics(t *testing.T) {
	files := make(map[string]string)
	for _, name := range []string{"models.go", "models_test.go"} {
		data, err := os.ReadFile(filepath.Join("testdata/runtime/outputs", name))
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
		t.Fatalf("output runtime fixture: %v\n%s", err, output)
	}
	assertNoRuntimePublication(t, r)
}

func TestOutputsReviewEmptyEqualityShadowing(t *testing.T) {
	dir := runtimeModule(t, map[string]string{
		"model.go": `package models
var true = false
//gobetter:struct equal
type Empty struct{}
//gobetter:struct equal
type Skipped struct {
 //gobetter:field skip-equal
 Value func()
}
`,
		"model_test.go": `package models
import "testing"
func TestEmptyEquality(t *testing.T) {
 if true {t.Fatal("shadowing fixture must be false")}
 if !(Empty{}).Equal(Empty{}) {t.Fatal("empty equality depends on shadowed true")}
 if !(Skipped{}).Equal(Skipped{Value:func(){}}) {t.Fatal("skipped-only equality depends on shadowed true")}
}
`,
	})
	r, err := source.Load(t.Context(), source.Request{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := structgen.Render(r.Plans)
	if err != nil || len(rendered.Imports) != 0 {
		t.Fatalf("empty equality should need no imports: %v, %v", rendered.Imports, err)
	}
	final := runtimeOverlay(t, r)
	if output, err := runOverlay(t, dir, final, "test", "-count=1", "./..."); err != nil {
		t.Fatalf("shadowed empty equality: %v\n%s", err, output)
	}
	assertNoRuntimePublication(t, r)
}

func TestOutputsReviewConstraintIntersections(t *testing.T) {
	for _, tc := range []struct{ name, declarations, constraint, argument, different string }{
		{"slices", "", "interface{ ~[]int | ~[]string; ~[]int | ~[]byte }", "[]int", "[]int{1}"},
		{"maps", "", "interface{ ~map[string]int | ~[]string; ~map[string]int | ~[]byte }", "map[string]int", "map[string]int{\"a\":1}"},
		{"named_exact", "type Ints []int\ntype Strings []string\n", "interface{ Ints | Strings; ~[]int | ~[]byte }", "Ints", "Ints{1}"},
		{"named_constraints", "type Left interface{~[]int | ~[]string}\ntype Right interface{~[]int | ~[]byte}\n", "interface{Left; Right}", "[]int", "[]int{1}"},
		{"nested_union", "type Left interface{~[]int | ~[]string}\n", "interface{Left | ~[]byte; ~[]int | ~[]float64}", "[]int", "[]int{1}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := runtimeModule(t, map[string]string{
				"model.go":      "package models\n" + tc.declarations + "//gobetter:struct equal\ntype Item[S " + tc.constraint + "] struct {Value S}\n",
				"model_test.go": "package models\nimport \"testing\"\nfunc TestIntersection(t *testing.T){a:=Item[" + tc.argument + "]{};b:=Item[" + tc.argument + "]{Value:" + tc.argument + "{}};if !a.Equal(b){t.Fatal(\"nil/empty intersection\")};b.Value=" + tc.different + ";if a.Equal(b){t.Fatal(\"intersection difference ignored\")}}\n",
			})
			r, err := source.Load(t.Context(), source.Request{Dir: dir})
			if err != nil {
				t.Fatal(err)
			}
			final := runtimeOverlay(t, r)
			if output, err := runOverlay(t, dir, final, "test", "-count=1", "./..."); err != nil {
				t.Fatalf("effective collection intersection: %v\n%s", err, output)
			}
			assertNoRuntimePublication(t, r)
		})
	}
	for _, constraint := range []string{
		"interface{~[]int | ~[]string; ~[]int | ~[]string | ~[]byte}",
		"interface{~[]any | ~[]int; ~[]any | ~[]string}",
		"interface{~[]int | ~map[string]int; ~[]int | ~map[string]int | ~[]byte}",
		"interface{~[]int; ~[]string}",
		"interface{comparable; ~[]int}",
	} {
		t.Run("unsupported_"+constraint, func(t *testing.T) {
			dir := runtimeModule(t, map[string]string{"model.go": "package models\n//gobetter:struct equal\ntype Item[S " + constraint + "] struct {Value S}\n"})
			r, err := source.Load(t.Context(), source.Request{Dir: dir})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := structgen.Render(r.Plans); err == nil || !strings.Contains(err.Error(), "model.go:3:") || !strings.Contains(err.Error(), "unsupported_equality") {
				t.Fatalf("unsupported effective set accepted: %v", err)
			}
			assertNoRuntimePublication(t, r)
		})
	}
}

func TestOutputsReviewReceiverAliasCollisions(t *testing.T) {
	for _, tc := range []struct {
		name, alias, receiver string
		retained, inactive    bool
		column                int
	}{
		{"value", "Item", "Alias", false, false, 14},
		{"pointer", "Item", "*Alias", false, false, 15},
		{"pointer_alias", "*Item", "Alias", false, false, 14},
		{"retained_value", "Item", "Alias", true, false, 14},
		{"retained_pointer", "Item", "*Alias", true, false, 15},
		{"inactive_source_artifact", "Item", "Alias", true, true, 14},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files := map[string]string{
				"model.go":   "package models\n//gobetter:struct string\ntype Item struct {Value int}\n",
				"aliases.go": "package models\ntype Middle = " + tc.alias + "\n",
			}
			methodFile, methodLine := "method.go", 3
			if tc.retained {
				files["other.go"] = "package models\ntype Other struct{}\n"
				if tc.inactive {
					files["other.go"] = "//go:build custom\n\n" + files["other.go"]
				}
				methodFile, methodLine = "other_gob.go", 7
				files[methodFile] = source.GeneratedHeader + "\n" + source.VersionMetadata + "0.20.0\n" + source.SourceMetadata + "other.go\npackage models\ntype Alias = Middle\n"
			} else {
				files["alias.go"] = "package models\ntype Alias = Middle\n"
				files[methodFile] = "package models\n"
			}
			files[methodFile] += "//line renamed.go:200:1\nfunc (" + tc.receiver + ") String() string {return \"authored\"}\n"
			dir := runtimeModule(t, files)
			request := source.Request{Dir: dir}
			if tc.retained && !tc.inactive {
				request.File = "model.go"
			}
			_, err := source.Load(t.Context(), request)
			var ds structgen.Diagnostics
			if !errors.As(err, &ds) || len(ds) != 1 {
				t.Fatalf("expected one pre-emission collision: %v", err)
			}
			d := ds[0]
			if d.Code != structgen.SymbolCollision || d.Position.Filename != filepath.Join(dir, "model.go") || d.Position.Line != 3 || d.Position.Column != 6 || !strings.Contains(d.Message, "String") || !strings.Contains(d.Message, filepath.Join(dir, methodFile)+":"+strconv.Itoa(methodLine)+":"+strconv.Itoa(tc.column)) {
				t.Fatalf("alias collision lost physical request/method positions: %v", d)
			}
			if _, err := os.Stat(filepath.Join(dir, "model_gob.go")); !os.IsNotExist(err) {
				t.Fatal("collision published a bootstrap sidecar")
			}
		})
	}
	t.Run("parse_local_chain", func(t *testing.T) {
		f, err := source.Parse("model.go", []byte("package models\ntype Item struct{}\ntype Middle = Item\ntype Alias = Middle\n//line renamed.go:200\nfunc (*Alias) String() string {return \"authored\"}\n"))
		if err != nil {
			t.Fatal(err)
		}
		methods := f.Targets[0].ExistingMethods
		if len(methods) != 1 || methods[0].Name != "String" || methods[0].Position.Filename != "model.go" || methods[0].Position.Line != 6 || methods[0].Position.Column != 15 {
			t.Fatalf("parsed alias method missing physical position: %v", methods)
		}
	})
	t.Run("masked_same_source_artifact", func(t *testing.T) {
		artifact := source.GeneratedHeader + "\n" + source.VersionMetadata + "0.20.0\n" + source.SourceMetadata + "model.go\npackage models\ntype Alias = Item\nfunc (Alias) String() string {return \"stale\"}\n"
		dir := runtimeModule(t, map[string]string{
			"model.go":      "package models\n//gobetter:struct string\ntype Item struct {Value int}\n",
			"model_gob.go":  artifact,
			"model_test.go": "package models\nimport \"testing\"\nfunc TestReplacement(t *testing.T){if (Item{Value:1}).String()!=\"Item{Value:1}\" {t.Fatal(\"stale alias method retained\")}}\n",
		})
		path := filepath.Join(dir, "model_gob.go")
		before, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		r, err := source.Load(t.Context(), source.Request{Dir: dir})
		if err != nil {
			t.Fatal(err)
		}
		final := runtimeOverlay(t, r)
		if output, err := runOverlay(t, dir, final, "test", "-count=1", "./..."); err != nil {
			t.Fatalf("masked alias method: %v\n%s", err, output)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		after, err := os.Stat(path)
		if err != nil || string(data) != artifact || !before.ModTime().Equal(after.ModTime()) {
			t.Fatalf("masked artifact changed on disk: %v", err)
		}
	})
}

func TestOutputsUnsupportedEquality(t *testing.T) {
	for _, fieldType := range []string{"func()", "any", "NamedInterface", "[2]any", "struct {Value any}", "[]any", "[]func()", "map[string]any", "map[string][]int", "map[string]struct {Value any}"} {
		t.Run(fieldType, func(t *testing.T) {
			dir := runtimeModule(t, map[string]string{"model.go": "package models\ntype NamedInterface interface {M()}\n//gobetter:struct equal\ntype Item struct { Value " + fieldType + " }\n"})
			r, err := source.Load(t.Context(), source.Request{Dir: dir})
			if err != nil {
				t.Fatal(err)
			}
			_, err = structgen.Render(r.Plans)
			if err == nil || !strings.Contains(err.Error(), "model.go:4:20: unsupported_equality") || !strings.Contains(err.Error(), "skip-equal") {
				t.Fatalf("unsupported equality not positioned/actionable: %v", err)
			}
			assertNoRuntimePublication(t, r)
		})
	}
	for _, constraint := range []string{"any", "~[]int | ~[]string", "~map[string][]int"} {
		t.Run(constraint, func(t *testing.T) {
			dir := runtimeModule(t, map[string]string{"model.go": "package models\n//gobetter:struct equal\ntype Box[T " + constraint + "] struct {Value T}\n"})
			r, err := source.Load(t.Context(), source.Request{Dir: dir})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = structgen.Render(r.Plans); err == nil || !strings.Contains(err.Error(), "unsupported_equality") {
				t.Fatalf("unsupported generic operation accepted: %v", err)
			}
		})
	}
}

func TestOutputsMemberCollisions(t *testing.T) {
	for _, tc := range []struct{ name, model, extra, symbol string }{
		{"equal_field", "//gobetter:struct equal\ntype Item struct {Equal int}\n", "", "Equal"},
		{"string_field", "//gobetter:struct string\ntype Item struct {String int}\n", "", "String"},
		{"existing_value_method", "//gobetter:struct string\ntype Item struct {Value int}\n", "func (Item) String() string {return \"authored\"}\n", "String"},
		{"existing_pointer_generic_method", "//gobetter:struct equal\ntype Item[T comparable] struct {Value T}\n", "func (*Item[T]) Equal(Item[T]) bool {return false}\n", "Equal"},
		{"getter_existing_field", "//gobetter:struct getters\ntype Item struct {Value int\n//gobetter:field getter\nvalue int}\n", "", "Value"},
		{"duplicate_getters", "//gobetter:struct getters\ntype Item struct {\n//gobetter:field getter,name=Same\na int\n//gobetter:field getter,name=Same\nb int}\n", "", "Same"},
		{"duplicate_optional", "//gobetter:struct builder\ntype Item struct {\n//gobetter:field optional,name=Same\na int\n//gobetter:field optional,name=Same\nb int}\n", "", "Same"},
		{"constructor", "//gobetter:struct builder\ntype Item struct{Value int}\n", "func NewItemBuilder(){}\n", "NewItemBuilder"},
		{"stage", "//gobetter:struct builder\ntype Item struct{Value int}\n", "type gobetter_Item_step0 int\n", "gobetter_Item_step0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := runtimeModule(t, map[string]string{"model.go": "package models\n" + tc.model, "other.go": "package models\n" + tc.extra})
			_, err := source.Load(t.Context(), source.Request{Dir: dir})
			if err == nil || !strings.Contains(err.Error(), "model.go:") || !strings.Contains(err.Error(), "symbol_collision") || !strings.Contains(err.Error(), tc.symbol) {
				t.Fatalf("missing originating collision diagnostic: %v", err)
			}
		})
	}
}

func TestOutputsConfigurationParity(t *testing.T) {
	for _, route := range []string{"annotation", "configuration", "invocation"} {
		t.Run(route, func(t *testing.T) {
			model := "package models\n"
			if route == "annotation" {
				model += "//gobetter:struct equal,string\n"
			}
			model += "type Item struct { Z int; A string }\n"
			dir := runtimeModule(t, map[string]string{"model.go": model})
			request := source.Request{Dir: dir}
			if route == "configuration" {
				writeRuntimeInput(t, dir, "gobetter.json", `{"schema_version":1,"types":[{"package":".","type":"Item","derive":["equal","string"]}]}`)
			}
			if route == "invocation" {
				request.Invocation = structgen.Invocation{Selection: structgen.Exported, Options: structgen.TypeOptions{Derive: new(structgen.Equal | structgen.String)}}
			}
			r, err := source.Load(t.Context(), request)
			if err != nil {
				t.Fatal(err)
			}
			final := runtimeOverlay(t, r)
			writeRuntimeInput(t, dir, "model_test.go", "package models\nimport \"testing\"\nfunc TestOutput(t *testing.T){v:=Item{Z:2,A:\"x\"};if v.String()!=`Item{Z:2, A:\"x\"}` || !v.Equal(v){t.Fatal(v.String())}}\n")
			if output, err := runOverlay(t, dir, final, "test", "-count=1", "./..."); err != nil {
				t.Fatalf("output adapter: %v\n%s", err, output)
			}
		})
	}
}

func TestOutputsImportBindingsAndIntersections(t *testing.T) {
	dir := runtimeModule(t, map[string]string{
		"library/types.go": "package gobetterFmt\nconst Value=1\n",
		"models.go": `package models
import "fixture/library"
var _=gobetterFmt.Value
//gobetter:struct string
type First[gobetterFmt any] struct {Value gobetterFmt}
//gobetter:struct string
type Second struct {Value int}
type Both interface { ~[]int | ~[]string }
type Ints interface { ~[]int }
//gobetter:struct equal
type Restricted[S interface {Both; Ints}] struct {Value S}
`,
		"models_test.go": `package models
import "testing"
func TestOutput(t *testing.T){
 if (First[int]{Value:1}).String()!="First{Value:1}" || (Second{Value:2}).String()!="Second{Value:2}" {t.Fatal("import/binder collision")}
 if !(Restricted[[]int]{}).Equal(Restricted[[]int]{Value:[]int{}}) {t.Fatal("intersection not supported")}
}
`,
	})
	r, err := source.Load(t.Context(), source.Request{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	final := runtimeOverlay(t, r)
	if output, err := runOverlay(t, dir, final, "test", "-count=1", "./..."); err != nil {
		t.Fatalf("bindings/constraint intersection: %v\n%s", err, output)
	}
	assertNoRuntimePublication(t, r)
}

func TestOutputsReviewMethodConstraints(t *testing.T) {
	for _, tc := range []struct{ name, declarations, term string }{
		{"unnamed_slice", "", "[]int"},
		{"unnamed_map", "", "map[string]int"},
		{"named_slice", "type Named []int\n", "Named"},
		{"named_map", "type Named map[string]int\n", "Named"},
		{"wrong_signature", "type Named []int\nfunc (Named) M(int) {}\n", "Named"},
		{"pointer_only", "type Named []int\nfunc (*Named) M() {}\n", "Named"},
		{"comparable_exact", "type Named int\n", "Named"},
		{"method_reduced_mixed_shape", "type Named []int\nfunc (Named) M(){}\ntype Other []string\n", "Named | Other"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			declaration := "type Item[S interface{ " + tc.term + "; M() }] struct { Value S }"
			model := "package models\n" + tc.declarations + "//gobetter:struct equal\n" + declaration + "\n"
			dir := runtimeModule(t, map[string]string{"model.go": model})
			r, err := source.Load(t.Context(), source.Request{Dir: dir})
			if err != nil {
				t.Fatal(err)
			}
			_, err = structgen.Render(r.Plans)
			var ds structgen.Diagnostics
			if !errors.As(err, &ds) || len(ds) != 1 || ds[0].Code != structgen.UnsupportedEquality || ds[0].Position.Filename != filepath.Join(dir, "model.go") || ds[0].Position.Line != strings.Count(model, "\n") || ds[0].Position.Column != strings.Index(declaration, "Value")+1 || !strings.Contains(ds[0].Message, "skip-equal") {
				t.Fatalf("impossible exact term lacks field-positioned rejection: %v", err)
			}
			assertNoRuntimePublication(t, r)
		})
	}
	for _, tc := range []struct{ name, declarations, constraint, argument, different string }{
		{"exact_slice", "type Named []int\nfunc (Named) M(){panic(\"method must not run\")}\n", "interface{Named; M()}", "Named", "Named{1}"},
		{"exact_map", "type Named map[string]int\nfunc (Named) M(){panic(\"method must not run\")}\n", "interface{Named; M()}", "Named", "Named{\"a\":1}"},
		{"tilde_slice", "type Named []int\nfunc (Named) M(){panic(\"method must not run\")}\n", "interface{~[]int; M()}", "Named", "Named{1}"},
		{"tilde_map", "type Named map[string]int\nfunc (Named) M(){panic(\"method must not run\")}\n", "interface{~map[string]int; M()}", "Named", "Named{\"a\":1}"},
		{"embedded_method", "type Named []int\nfunc (Named) M(){panic(\"method must not run\")}\ntype Required interface{M()}\n", "interface{Named; Required}", "Named", "Named{1}"},
		{"union_with_impossible_exact", "type Named []int\nfunc (Named) M(){panic(\"method must not run\")}\ntype Other []int\n", "interface{Named | Other; M()}", "Named", "Named{1}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := runtimeModule(t, map[string]string{
				"model.go":      "package models\n" + tc.declarations + "//gobetter:struct equal\ntype Item[S " + tc.constraint + "] struct {Value S}\n",
				"model_test.go": "package models\nimport \"testing\"\nfunc TestMethodConstraint(t *testing.T){a:=Item[" + tc.argument + "]{};b:=Item[" + tc.argument + "]{Value:" + tc.argument + "{}};if !a.Equal(b){t.Fatal(\"nil/empty equality\")};b.Value=" + tc.different + ";if a.Equal(b){t.Fatal(\"difference ignored\")}}\n",
			})
			r, err := source.Load(t.Context(), source.Request{Dir: dir})
			if err != nil {
				t.Fatal(err)
			}
			final := runtimeOverlay(t, r)
			if output, err := runOverlay(t, dir, final, "test", "-count=1", "./..."); err != nil {
				t.Fatalf("valid method constraint: %v\n%s", err, output)
			}
			assertNoRuntimePublication(t, r)
		})
	}
}

func TestOutputsReviewRetainedImportSymbols(t *testing.T) {
	for _, inactive := range []bool{false, true} {
		for _, tc := range []struct {
			name, importSpec, use, library, symbol string
			alias, blank                           bool
		}{
			{"constructor", `NewItemBuilder "time"`, "NewItemBuilder.Time{}", "", "NewItemBuilder", false, false},
			{"done", `gobetter_Item_done "time"`, "gobetter_Item_done.Time{}", "", "gobetter_Item_done", false, false},
			{"step", `gobetter_Item_step0 "time"`, "gobetter_Item_step0.Time{}", "", "gobetter_Item_step0", false, false},
			{"default_name", `"fixture/library"`, "gobetter_Item_done.Value", "package gobetter_Item_done\nconst Value=1\n", "gobetter_Item_done", false, false},
			{"dot_constructor", `. "fixture/library"`, "NewItemBuilder()", "package library\nfunc NewItemBuilder() int{return 1}\n", "NewItemBuilder", false, false},
			{"alias_suffix", `ItemConfig "time"`, "ItemConfig.Time{}", "", "ItemConfig_2", true, false},
			{"blank_import", `_ "fixture/library"`, "1", "package gobetter_Item_done\nconst Value=1\n", "", false, true},
		} {
			t.Run(tc.name+"_inactive_"+strconv.FormatBool(inactive), func(t *testing.T) {
				model := "package models\n//gobetter:struct builder\ntype Item struct {Value int}\n"
				if tc.alias {
					model = "package models\n//gobetter:struct builder\ntype Item struct {Config struct {Value int}}\n"
				}
				artifact := source.GeneratedHeader + "\n" + source.VersionMetadata + "0.20.0\n" + source.SourceMetadata + "other.go\npackage models\nimport " + tc.importSpec + "\nvar _=" + tc.use + "\n"
				files := map[string]string{"model.go": model, "other.go": "package models\ntype Other struct{}\n", "other_gob.go": artifact}
				if inactive {
					files["other.go"] = "//go:build custom\n\n" + files["other.go"]
				}
				if tc.library != "" {
					files["library/types.go"] = tc.library
				}
				dir := runtimeModule(t, files)
				path := filepath.Join(dir, "other_gob.go")
				before, err := os.Stat(path)
				if err != nil {
					t.Fatal(err)
				}
				request := source.Request{Dir: dir}
				if !inactive {
					request.File = "model.go"
				}
				r, err := source.Load(t.Context(), request)
				if tc.alias || tc.blank {
					if err != nil {
						t.Fatal(err)
					}
					call := "NewItemBuilder().Value(1).Build()"
					member := "Value"
					if tc.alias {
						found := false
						for _, p := range r.Plans {
							if p.Target.Kind == structgen.NestedAlias {
								found = p.Target.Name == tc.symbol
							}
						}
						if !found {
							t.Fatalf("retained import did not reserve alias spelling: %+v", r.Plans)
						}
						call = "NewItemBuilder().Config(NewItemConfig_2Builder().Value(1).Build()).Build()"
						member = "Config.Value"
					}
					final := runtimeOverlay(t, r)
					writeRuntimeInput(t, dir, "model_test.go", "package models\nimport \"testing\"\nfunc TestRetained(t *testing.T){v:="+call+";if v."+member+"!=1 {t.Fatal(\"builder assignment\")}}\n")
					if output, err := runOverlay(t, dir, final, "test", "-count=1", "./..."); err != nil {
						t.Fatalf("retained import control: %v\n%s", err, output)
					}
					assertNoRuntimePublication(t, r)
				} else {
					var ds structgen.Diagnostics
					if !errors.As(err, &ds) || len(ds) != 1 || ds[0].Code != structgen.SymbolCollision || ds[0].Position.Filename != filepath.Join(dir, "model.go") || ds[0].Position.Line != 3 || ds[0].Position.Column != 6 || !strings.Contains(ds[0].Message, tc.symbol) {
						t.Fatalf("retained import lacks originating collision: %v", err)
					}
					if _, err := os.Stat(filepath.Join(dir, "model_gob.go")); !os.IsNotExist(err) {
						t.Fatal("collision published bootstrap")
					}
				}
				data, readErr := os.ReadFile(path)
				after, statErr := os.Stat(path)
				if readErr != nil || statErr != nil || string(data) != artifact || !before.ModTime().Equal(after.ModTime()) {
					t.Fatalf("retained artifact changed: %v, %v", readErr, statErr)
				}
			})
		}
	}
	t.Run("masked_import", func(t *testing.T) {
		artifact := source.GeneratedHeader + "\n" + source.VersionMetadata + "0.20.0\n" + source.SourceMetadata + "model.go\npackage models\nimport gobetter_Item_done \"time\"\nvar _=gobetter_Item_done.Time{}\n"
		dir := runtimeModule(t, map[string]string{
			"model.go":      "package models\n//gobetter:struct builder\ntype Item struct {Value int}\n",
			"model_gob.go":  artifact,
			"model_test.go": "package models\nimport \"testing\"\nfunc TestMasked(t *testing.T){if NewItemBuilder().Value(1).Build().Value!=1 {t.Fatal(\"builder\")}}\n",
		})
		path := filepath.Join(dir, "model_gob.go")
		before, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		r, err := source.Load(t.Context(), source.Request{Dir: dir})
		if err != nil {
			t.Fatal(err)
		}
		final := runtimeOverlay(t, r)
		if output, err := runOverlay(t, dir, final, "test", "-count=1", "./..."); err != nil {
			t.Fatalf("masked import reserved a stale binding: %v\n%s", err, output)
		}
		data, readErr := os.ReadFile(path)
		after, statErr := os.Stat(path)
		if readErr != nil || statErr != nil || string(data) != artifact || !before.ModTime().Equal(after.ModTime()) {
			t.Fatalf("masked artifact changed: %v, %v", readErr, statErr)
		}
	})
}

func TestOutputsReviewEffectiveDependencyExports(t *testing.T) {
	header := func(sourceName, body string) string {
		return source.GeneratedHeader + "\n" + source.VersionMetadata + "0.20.0\n" + source.SourceMetadata + sourceName + "\n" + body
	}
	for _, scenario := range []string{"fresh_constructor", "removed_constructor", "fresh_alias", "removed_alias", "retained_constructor", "authored_constructor"} {
		t.Run(scenario, func(t *testing.T) {
			collision := scenario == "fresh_constructor" || scenario == "retained_constructor" || scenario == "authored_constructor"
			alias := scenario == "fresh_alias" || scenario == "removed_alias"
			files := map[string]string{
				"a/model.go":        "package a\n//gobetter:struct builder\ntype Item struct {Value int}\n",
				"a/retained.go":     "//go:build custom\n\npackage a\ntype Retained struct{}\n",
				"a/retained_gob.go": header("retained.go", "package a\nimport . \"fixture/z\"\nvar _=Anchor\n"),
				"z/model.go":        "package z\nconst Anchor=1\n//gobetter:struct string\ntype item struct {Value int}\n",
			}
			switch scenario {
			case "fresh_constructor":
				files["z/model.go"] = "package z\nconst Anchor=1\n//gobetter:struct builder,constructor=exported\ntype item struct {Value int}\n"
			case "removed_constructor":
				files["z/model_gob.go"] = header("model.go", "package z\nfunc NewItemBuilder() int {return 0}\n")
			case "fresh_alias":
				files["z/model.go"] = "package z\nconst Anchor=1\n//gobetter:struct builder\ntype Item struct {ConfigDetail struct {Value int}}\n"
			case "removed_alias":
				files["z/model_gob.go"] = header("model.go", "package z\ntype ItemConfigDetail = struct {Value string}\n")
			case "retained_constructor":
				files["z/kept.go"] = "//go:build custom\n\npackage z\ntype Kept struct{}\n"
				files["z/kept_gob.go"] = header("kept.go", "package z\nfunc NewItemBuilder() int {return 0}\n")
			case "authored_constructor":
				files["z/other.go"] = "package z\nfunc NewItemBuilder() int {return 0}\n"
			}
			if alias {
				files["a/model.go"] = "package a\n//gobetter:struct builder\ntype ItemConfig struct {Detail struct {Value int}}\n"
			}
			dir := runtimeModule(t, files)
			infos := make(map[string]os.FileInfo)
			for name := range files {
				if strings.Contains(name, "_gob.go") {
					info, err := os.Stat(filepath.Join(dir, name))
					if err != nil {
						t.Fatal(err)
					}
					infos[name] = info
				}
			}
			var previous map[string][]byte
			for _, patterns := range [][]string{{"./a", "./z"}, {"./z", "./a"}} {
				r, err := source.Load(t.Context(), source.Request{Dir: dir, Patterns: patterns})
				if collision {
					var ds structgen.Diagnostics
					if !errors.As(err, &ds) || len(ds) != 1 || ds[0].Code != structgen.SymbolCollision || ds[0].Position.Filename != filepath.Join(dir, "a/model.go") || ds[0].Position.Line != 3 || ds[0].Position.Column != 6 || !strings.Contains(ds[0].Message, "NewItemBuilder") {
						t.Fatalf("effective dependency constructor lacks originating diagnostic: %v", err)
					}
					if _, err := os.Stat(filepath.Join(dir, "a/model_gob.go")); !os.IsNotExist(err) {
						t.Fatal("collision published a bootstrap")
					}
					continue
				}
				if err != nil {
					t.Fatal(err)
				}
				if r.Plans[0].Target.PackageDir != filepath.Join(dir, "a") {
					t.Fatal("dependency-first allocation changed result plan ordering")
				}
				if alias {
					want := "ItemConfigDetail"
					if scenario == "fresh_alias" {
						want += "_2"
					}
					found := false
					for _, p := range r.Plans {
						if p.Target.PackageDir == filepath.Join(dir, "a") && p.Target.Kind == structgen.NestedAlias {
							found = p.Target.Name == want
						}
					}
					if !found {
						t.Fatalf("effective exports did not allocate %s: %+v", want, r.Plans)
					}
				}
				final := runtimeOverlay(t, r)
				if previous != nil {
					if len(previous) != len(final) {
						t.Fatal("artifact inventory depends on pattern order")
					}
					for path, data := range final {
						if !bytes.Equal(data, previous[path]) {
							t.Fatalf("artifact %s depends on pattern order", path)
						}
					}
				}
				previous = final
			}
			if !collision {
				call, member := "NewItemBuilder().Value(3).Build()", "Value"
				if alias {
					constructor := "NewItemConfigDetailBuilder"
					if scenario == "fresh_alias" {
						constructor = "NewItemConfigDetail_2Builder"
					}
					call, member = "NewItemConfigBuilder().Detail("+constructor+"().Value(3).Build()).Build()", "Detail.Value"
				}
				writeRuntimeInput(t, dir, "a/model_test.go", "package a\nimport \"testing\"\nfunc TestEffective(t *testing.T){v:="+call+";if v."+member+"!=3 {t.Fatal(\"assignment\")}}\n")
				if scenario == "fresh_alias" {
					writeRuntimeInput(t, dir, "z/model_test.go", "package z\nimport \"testing\"\nfunc TestDependency(t *testing.T){v:=NewItemBuilder().ConfigDetail(NewItemConfigDetailBuilder().Value(5).Build()).Build();if v.ConfigDetail.Value!=5 {t.Fatal(\"dependency assignment\")}}\n")
				}
				if output, err := runOverlay(t, dir, previous, "test", "-count=1", "./..."); err != nil {
					t.Fatalf("effective dependency runtime: %v\n%s", err, output)
				}
			}
			for name, before := range infos {
				data, readErr := os.ReadFile(filepath.Join(dir, name))
				after, statErr := os.Stat(filepath.Join(dir, name))
				if readErr != nil || statErr != nil || string(data) != files[name] || !before.ModTime().Equal(after.ModTime()) {
					t.Fatalf("retained/masked artifact %s changed: %v, %v", name, readErr, statErr)
				}
			}
			for _, name := range []string{"a/model_gob.go", "z/model_gob.go"} {
				if _, existed := files[name]; !existed {
					if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
						t.Fatalf("published new sidecar %s", name)
					}
				}
			}
		})
	}
	t.Run("transitive_imports_are_not_exports", func(t *testing.T) {
		dir := runtimeModule(t, map[string]string{
			"a/model.go":        "package a\n//gobetter:struct builder\ntype Item struct {Value int}\n",
			"a/retained.go":     "//go:build custom\n\npackage a\ntype Retained struct{}\n",
			"a/retained_gob.go": header("retained.go", "package a\nimport . \"fixture/m\"\nvar _=MiddleAnchor\n"),
			"m/model.go":        "package m\nconst MiddleAnchor=1\n//gobetter:struct string\ntype middle struct {Value int}\n",
			"m/retained.go":     "//go:build custom\n\npackage m\ntype Retained struct{}\n",
			"m/retained_gob.go": header("retained.go", "package m\nimport . \"fixture/z\"\nvar _=Anchor\n"),
			"z/model.go":        "package z\nconst Anchor=1\n//gobetter:struct builder,constructor=exported\ntype item struct {Value int}\n",
			"a/model_test.go":   "package a\nimport \"testing\"\nfunc TestTransitive(t *testing.T){if NewItemBuilder().Value(3).Build().Value!=3 {t.Fatal(\"builder\")}}\n",
		})
		r, err := source.Load(t.Context(), source.Request{Dir: dir, Patterns: []string{"./a", "./m", "./z"}})
		if err != nil {
			t.Fatal(err)
		}
		final := runtimeOverlay(t, r)
		if output, err := runOverlay(t, dir, final, "test", "-count=1", "./..."); err != nil {
			t.Fatalf("transitive import names were re-exported: %v\n%s", err, output)
		}
		assertNoRuntimePublication(t, r)
	})
}

func TestOutputsReviewEffectiveImportGraph(t *testing.T) {
	header := func(sourceName, body string) string {
		return source.GeneratedHeader + "\n" + source.VersionMetadata + "0.20.0\n" + source.SourceMetadata + sourceName + "\n" + body
	}
	for _, scenario := range []string{"fresh_constructor", "fresh_alias", "default_binding", "authored_cycle", "retained_cycle"} {
		t.Run(scenario, func(t *testing.T) {
			files := map[string]string{
				"a/model.go":        "package a\n//gobetter:struct builder\ntype Item struct {Value int}\n",
				"a/retained.go":     "//go:build custom\n\npackage a\ntype Retained struct{}\n",
				"a/retained_gob.go": header("retained.go", "package a\nimport . \"fixture/z\"\nvar _=Anchor\n"),
				"z/model.go":        "package z\nconst Anchor=1\n//gobetter:struct builder,constructor=exported\ntype item struct {Value int}\n",
				"z/model_gob.go":    header("model.go", "package z\nimport \"fixture/a\"\nvar _=a.Item{}\n"),
			}
			symbol, line := "NewItemBuilder", 3
			switch scenario {
			case "fresh_alias":
				files["a/model.go"] = "package a\n//gobetter:struct builder\ntype ItemConfig struct {Detail struct {Value int}}\n"
				files["z/model.go"] = "package z\nconst Anchor=1\n//gobetter:struct builder\ntype Item struct {ConfigDetail struct {Value int}}\n"
				files["z/model_gob.go"] = header("model.go", "package z\nimport \"fixture/a\"\nvar _=a.ItemConfig{}\n")
			case "default_binding":
				delete(files, "a/retained.go")
				delete(files, "a/retained_gob.go")
				files["a/model.go"] = "package a\nimport \"fixture/z\"\n//gobetter:struct builder\ntype Item struct {Value int}\nvar _=gobetter_Item_done.Anchor\n"
				files["z/model.go"] = "package gobetter_Item_done\nconst Anchor=1\n//gobetter:struct string\ntype item struct {Value int}\n"
				files["z/model_gob.go"] = header("model.go", "package gobetter_Item_done\nimport \"fixture/a\"\nvar _=a.Item{}\n")
				symbol, line = "gobetter_Item_done", 4
			case "authored_cycle":
				delete(files, "z/model_gob.go")
				files["a/model.go"] += "const Anchor=1\n"
				files["z/other.go"] = "package z\nimport \"fixture/a\"\nvar _=a.Anchor\n"
			case "retained_cycle":
				delete(files, "z/model_gob.go")
				files["a/model.go"] += "const Anchor=1\n"
				files["z/kept.go"] = "//go:build custom\n\npackage z\ntype Kept struct{}\n"
				files["z/kept_gob.go"] = header("kept.go", "package z\nimport \"fixture/a\"\nvar _=a.Anchor\n")
			}
			dir := runtimeModule(t, files)
			infos := make(map[string]os.FileInfo)
			for name := range files {
				info, err := os.Stat(filepath.Join(dir, name))
				if err != nil {
					t.Fatal(err)
				}
				infos[name] = info
			}
			var previous map[string][]byte
			for i, patterns := range [][]string{{"./a", "./z"}, {"./z", "./a"}} {
				t.Run(strconv.Itoa(i), func(t *testing.T) {
					r, err := source.Load(t.Context(), source.Request{Dir: dir, Patterns: patterns})
					if strings.HasSuffix(scenario, "_cycle") {
						file, line := "z/other.go", 2
						if scenario == "retained_cycle" {
							file, line = "z/kept_gob.go", 5
						}
						var ds structgen.Diagnostics
						if !errors.As(err, &ds) || len(ds) != 1 || ds[0].Code != structgen.PackageLoad || ds[0].Position.Filename != filepath.Join(dir, file) || ds[0].Position.Line != line || ds[0].Position.Column != 8 || !strings.Contains(ds[0].Message, "import cycle not allowed") || !strings.Contains(ds[0].Message, "fixture/a -> fixture/z -> fixture/a") {
							t.Fatalf("genuine effective cycle not preserved: %v", err)
						}
					} else if scenario != "fresh_alias" {
						var ds structgen.Diagnostics
						if !errors.As(err, &ds) || len(ds) != 1 || ds[0].Code != structgen.SymbolCollision || ds[0].Position.Filename != filepath.Join(dir, "a/model.go") || ds[0].Position.Line != line || ds[0].Position.Column != 6 || !strings.Contains(ds[0].Message, symbol) {
							t.Fatalf("stale disk cycle lost originating collision: %v", err)
						}
					} else {
						if err != nil {
							t.Fatal(err)
						}
						found := false
						for _, p := range r.Plans {
							if p.Target.PackageDir == filepath.Join(dir, "a") && p.Target.Kind == structgen.NestedAlias {
								found = p.Target.Name == "ItemConfigDetail_2"
							}
						}
						if !found || r.Plans[0].Target.PackageDir != filepath.Join(dir, "a") {
							t.Fatal("effective graph did not preserve alias suffix/result ordering")
						}
						final := runtimeOverlay(t, r)
						for path, data := range final {
							if previous != nil && !bytes.Equal(data, previous[path]) {
								t.Fatalf("output %s depends on pattern order", path)
							}
						}
						previous = final
						writeRuntimeInput(t, dir, "a/model_test.go", "package a\nimport \"testing\"\nfunc TestGraph(t *testing.T){v:=NewItemConfigBuilder().Detail(NewItemConfigDetail_2Builder().Value(3).Build()).Build();if v.Detail.Value!=3 {t.Fatal(\"assignment\")}}\n")
						if output, err := runOverlay(t, dir, final, "test", "-count=1", "./..."); err != nil {
							t.Fatalf("effective graph runtime: %v\n%s", err, output)
						}
					}
					for name, before := range infos {
						data, readErr := os.ReadFile(filepath.Join(dir, name))
						after, statErr := os.Stat(filepath.Join(dir, name))
						if readErr != nil || statErr != nil || string(data) != files[name] || !before.ModTime().Equal(after.ModTime()) {
							t.Fatalf("source/artifact %s changed: %v, %v", name, readErr, statErr)
						}
					}
					if _, err := os.Stat(filepath.Join(dir, "a/model_gob.go")); !os.IsNotExist(err) {
						t.Fatal("published bootstrap")
					}
				})
			}
		})
	}
}
