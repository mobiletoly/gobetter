package structgen_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/mobiletoly/gobetter/internal/source"
	"github.com/mobiletoly/gobetter/internal/structgen"
)

func runtimeModule(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	writeRuntimeInput(t, dir, "go.mod", "module fixture\n\ngo 1.26.0\n")
	for name, text := range files {
		writeRuntimeInput(t, dir, name, text)
	}
	return dir
}

func writeRuntimeInput(t *testing.T, dir, name, text string) {
	t.Helper()
	filename := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte(text), 0644); err != nil {
		t.Fatal(err)
	}
}

// runtimeOverlay combines rendered declarations with the bootstrap import
// envelope so renderer tests can compile and execute output without publishing it.
func runtimeOverlay(t *testing.T, r *source.Result) map[string][]byte {
	t.Helper()
	final := make(map[string][]byte)
	for path, bootstrap := range r.Bootstrap {
		var plans []structgen.Plan
		for _, p := range r.Plans {
			if source.OutputFilename(p.Target.SourceFile) == path {
				plans = append(plans, p)
			}
		}
		code, err := structgen.Render(plans)
		if err != nil {
			t.Fatal(err)
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, bootstrap, parser.ParseComments)
		if err != nil {
			t.Fatal(err)
		}
		end := fset.PositionFor(file.Name.End(), false).Offset
		for _, decl := range file.Decls {
			if group, ok := decl.(*ast.GenDecl); ok && group.Tok == token.IMPORT {
				end = fset.PositionFor(group.End(), false).Offset
			}
		}
		var extra strings.Builder
		for _, binding := range code.Imports {
			extra.WriteString("import " + binding.Name + " " + strconv.Quote(binding.Path) + "\n")
		}
		final[path], err = format.Source(append(append(bytes.Clone(bootstrap[:end]), '\n', '\n'), []byte(extra.String()+code.Code)...))
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := r.Validate(context.Background(), final); err != nil {
		t.Fatal(err)
	}
	return final
}

func runOverlay(t *testing.T, dir string, overlay map[string][]byte, arguments ...string) ([]byte, error) {
	t.Helper()
	temp := t.TempDir()
	replacements := make(map[string]string)
	for path, data := range overlay {
		file := filepath.Join(temp, strconv.Itoa(len(replacements))+".go")
		if err := os.WriteFile(file, data, 0600); err != nil {
			t.Fatal(err)
		}
		replacements[path] = file
	}
	data, err := json.Marshal(struct{ Replace map[string]string }{replacements})
	if err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(temp, "overlay.json")
	if err := os.WriteFile(manifest, data, 0600); err != nil {
		t.Fatal(err)
	}
	args := append([]string{arguments[0], "-mod=readonly", "-overlay=" + manifest}, arguments[1:]...)
	cmd := exec.CommandContext(t.Context(), "go", args...)
	cmd.Dir = dir
	return cmd.CombinedOutput()
}

func assertNoRuntimePublication(t *testing.T, r *source.Result) {
	t.Helper()
	for path := range r.Bootstrap {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("overlay published at %s", path)
		}
	}
}

func TestRuntimeBuildersSnapshotsAndGetters(t *testing.T) {
	files := make(map[string]string)
	for _, name := range []string{"models.go", "models_test.go"} {
		data, err := os.ReadFile(filepath.Join("testdata/runtime/basic", name))
		if err != nil {
			t.Fatal(err)
		}
		files[name] = string(data)
	}
	dir := runtimeModule(t, files)
	before, _ := os.ReadFile(filepath.Join(dir, "models.go"))
	info, _ := os.Stat(filepath.Join(dir, "models.go"))
	r, err := source.Load(t.Context(), source.Request{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	final := runtimeOverlay(t, r)
	if output, err := runOverlay(t, dir, final, "test", "-count=1", "./..."); err != nil {
		t.Fatalf("runtime fixture: %v\n%s", err, output)
	}
	after, _ := os.ReadFile(filepath.Join(dir, "models.go"))
	afterInfo, _ := os.Stat(filepath.Join(dir, "models.go"))
	if sha256.Sum256(before) != sha256.Sum256(after) || !info.ModTime().Equal(afterInfo.ModTime()) {
		t.Fatal("runtime generation changed source bytes/mtime")
	}
	assertNoRuntimePublication(t, r)
}

func TestRuntimeFieldCommentsOnSetterAndGetter(t *testing.T) {
	dir := runtimeModule(t, map[string]string{"model.go": `package models
//gobetter:struct builder,getters
type User struct {
	// Email is used for account notifications.
	//gobetter:field getter,name=Email
	// Empty values disable notifications.
	email string // Keep the original spelling.
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
	file, err := parser.ParseFile(token.NewFileSet(), "generated.go", "package models\n"+rendered.Code, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"// Email is used for account notifications.",
		"// Empty values disable notifications.",
		"// Keep the original spelling.",
	}
	methods := 0
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || fn.Name.Name != "Email" {
			continue
		}
		methods++
		var got []string
		if fn.Doc != nil {
			for _, comment := range fn.Doc.List {
				got = append(got, comment.Text)
			}
		}
		if !slices.Equal(got, want) {
			t.Errorf("Email method %d comments: got %q, want %q", methods, got, want)
		}
	}
	if methods != 2 {
		t.Fatalf("found %d Email methods, want setter and getter", methods)
	}
}

func TestRuntimeRequiredSetterHints(t *testing.T) {
	dir := runtimeModule(t, map[string]string{"model.go": `package models
//gobetter:struct builder,getters
type Person struct {
	First int
	// Name is shown to readers.
	Name string
	// Optional comments.
	Comment string //gobetter:field optional
	// DOB is the recorded date of birth.
	dob string //gobetter:field getter,name=DOB
	Age int
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
	file, err := parser.ParseFile(token.NewFileSet(), "generated.go", "package models\n"+rendered.Code, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{
		"First/1":   {"// Next required setter: Name"},
		"Age/1":     nil,
		"Name/1":    {"// Name is shown to readers.", "//", "// Next required setter: DOB"},
		"Comment/1": {"// Optional comments."},
		"DOB/1":     {"// DOB is the recorded date of birth.", "//", "// Next required setter: Age"},
		"DOB/0":     {"// DOB is the recorded date of birth."},
	}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil {
			continue
		}
		key := fn.Name.Name + "/" + strconv.Itoa(len(fn.Type.Params.List))
		expected, check := want[key]
		if !check {
			continue
		}
		var got []string
		if fn.Doc != nil {
			for _, comment := range fn.Doc.List {
				got = append(got, comment.Text)
			}
		}
		if !slices.Equal(got, expected) {
			t.Errorf("%s comments: got %q, want %q", key, got, expected)
		}
		delete(want, key)
	}
	if len(want) != 0 {
		t.Fatalf("missing methods: %v", want)
	}
}

func TestRuntimeGenericNestedAndEmbedded(t *testing.T) {
	files := make(map[string]string)
	for _, name := range []string{"models.go", "models_test.go"} {
		data, err := os.ReadFile(filepath.Join("testdata/runtime/generic", name))
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
		t.Fatalf("generic runtime fixture: %v\n%s", err, output)
	}
	assertNoRuntimePublication(t, r)
}

func TestRuntimeRequiredStagesCompileFailures(t *testing.T) {
	for _, tc := range []struct{ name, call, diagnostic string }{
		{"valid", `NewOrderedBuilder().B(1).A(2).Note("a").Note("b").Build()`, ""},
		{"early_build", `NewOrderedBuilder().Build()`, "Build"},
		{"missing_required", `NewOrderedBuilder().B(1).BuildPtr()`, "BuildPtr"},
		{"wrong_order", `NewOrderedBuilder().A(1)`, "A"},
		{"early_optional", `NewOrderedBuilder().Note("a")`, "Note"},
		{"wrong_type", `NewOrderedBuilder().B("wrong")`, "cannot use"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := runtimeModule(t, map[string]string{"model.go": "package models\n//gobetter:struct builder\ntype Ordered struct { B int; A int;\n//gobetter:field optional\nNote string }\n"})
			r, err := source.Load(t.Context(), source.Request{Dir: dir})
			if err != nil {
				t.Fatal(err)
			}
			final := runtimeOverlay(t, r)
			writeRuntimeInput(t, dir, "caller.go", "package models\nvar _="+tc.call+"\n")
			output, err := runOverlay(t, dir, final, "build", "./...")
			if tc.diagnostic == "" {
				if err != nil {
					t.Fatalf("valid chain failed: %v\n%s", err, output)
				}
			} else if err == nil || !strings.Contains(string(output), tc.diagnostic) {
				t.Fatalf("invalid chain did not fail as expected: %v\n%s", err, output)
			}
			assertNoRuntimePublication(t, r)
		})
	}
}

func TestRuntimeExternalNameOverrides(t *testing.T) {
	for _, route := range []string{"annotation", "configuration"} {
		t.Run(route, func(t *testing.T) {
			model := "package models\n//gobetter:struct builder,getters\ntype Exposed struct {\n//gobetter:field getter\n_id int\n//gobetter:field getter\n字 int\n}\n"
			dir := runtimeModule(t, map[string]string{"models/model.go": model})
			_, err := source.Load(t.Context(), source.Request{Dir: filepath.Join(dir, "models")})
			if err == nil || !strings.Contains(err.Error(), "invalid_method_name") || !strings.Contains(err.Error(), "_id") || !strings.Contains(err.Error(), "字") {
				t.Fatalf("non-exported defaults not diagnosed: %v", err)
			}
			if route == "annotation" {
				model = strings.Replace(model, "//gobetter:field getter\n_id", "//gobetter:field getter,name=ID\n_id", 1)
				model = strings.Replace(model, "//gobetter:field getter\n字", "//gobetter:field getter,name=Glyph\n字", 1)
				writeRuntimeInput(t, dir, "models/model.go", model)
			} else {
				writeRuntimeInput(t, dir, "models/gobetter.json", `{"schema_version":1,"types":[{"package":".","type":"Exposed","fields":{"_id":{"name":"ID"},"字":{"name":"Glyph"}}}]}`)
			}
			r, err := source.Load(t.Context(), source.Request{Dir: filepath.Join(dir, "models")})
			if err != nil {
				t.Fatal(err)
			}
			final := runtimeOverlay(t, r)
			writeRuntimeInput(t, dir, "client/main.go", `package main
import (
 "fixture/models"
 "reflect"
)
func main() {
 value:=models.NewExposedBuilder().ID(12).Glyph(13).Build()
 if value.ID()!=12 || value.Glyph()!=13 {panic("inaccessible or wrong getter/setter")}
 typ:=reflect.TypeOf(value)
 if typ.Field(0).Name!="_id" || typ.Field(1).Name!="字" {panic("source member renamed")}
}
`)
			if output, err := runOverlay(t, dir, final, "run", "./client"); err != nil {
				t.Fatalf("external caller: %v\n%s", err, output)
			}
			assertNoRuntimePublication(t, r)
		})
	}
}

func TestRuntimeGetterOnly(t *testing.T) {
	dir := runtimeModule(t, map[string]string{
		"model.go": `package models
//gobetter:struct getters
type Secret struct {
 //gobetter:field getter
 hidden int
}
`,
		"model_test.go": `package models
import "testing"
func TestGetter(t *testing.T) {
 value := Secret{hidden: 8}
 if value.Hidden() != 8 {
  t.Fatal("wrong getter")
 }
}
`,
	})
	r, err := source.Load(t.Context(), source.Request{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	final := runtimeOverlay(t, r)
	for _, data := range final {
		file, err := parser.ParseFile(token.NewFileSet(), "", data, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(file.Decls) != 1 {
			t.Fatalf("getter-only output must contain one method, got %d declarations", len(file.Decls))
		}
		method, ok := file.Decls[0].(*ast.FuncDecl)
		if !ok || method.Recv == nil || method.Name.Name != "Hidden" {
			t.Fatal("getter-only output contains another generated declaration")
		}
	}
	if output, err := runOverlay(t, dir, final, "test", "-count=1", "./..."); err != nil {
		t.Fatalf("getter-only runtime: %v\n%s", err, output)
	}
	assertNoRuntimePublication(t, r)
}

func TestRuntimeRenderBoundaries(t *testing.T) {
	dir := runtimeModule(t, map[string]string{"model.go": "package models\n//gobetter:struct builder\ntype Empty struct{}\n"})
	r, err := source.Load(t.Context(), source.Request{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	r.Plans[0].Target.Object = nil
	if _, err := structgen.Render(r.Plans); err == nil || !strings.Contains(err.Error(), "unresolved_plan") {
		t.Fatalf("unresolved empty target rendered: %v", err)
	}
	for _, derive := range []string{"getters", "equal", "string"} {
		writeRuntimeInput(t, dir, "model.go", "package models\n//gobetter:struct builder\ntype Outer struct {\n//gobetter:struct "+derive+"\nNested struct {Value int}\n}\n")
		if _, err := source.Load(t.Context(), source.Request{Dir: dir}); err == nil || !strings.Contains(err.Error(), "use a named defined struct") {
			t.Fatalf("alias method request accepted: %v", err)
		}
	}
}
