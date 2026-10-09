package source_test

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mobiletoly/gobetter/internal/config"
	"github.com/mobiletoly/gobetter/internal/source"
	"github.com/mobiletoly/gobetter/internal/structgen"
)

func requireCode(t *testing.T, err error, code structgen.Code) {
	t.Helper()
	ds, ok := err.(structgen.Diagnostics)
	if !ok {
		t.Fatalf("expected %s, got %T: %v", code, err, err)
	}
	for _, d := range ds {
		if d.Code == code {
			if d.Position.Filename == "" || d.Position.Line < 1 || d.Position.Column < 1 {
				t.Fatalf("unpositioned error: %+v", d)
			}
			return
		}
	}
	t.Fatalf("missing %s: %v", code, err)
}

func TestDirectiveAttachmentAndRejections(t *testing.T) {
	cases := []struct {
		name, src string
		code      structgen.Code
	}{
		{"inline type", "type T struct{} //gobetter:struct builder", structgen.DirectivePlacement},
		{"inline struct on field", "type T struct { x int //gobetter:struct builder\n}", structgen.DirectivePlacement},
		{"inline field on type", "type T struct{} //gobetter:field getter", structgen.DirectivePlacement},
		{"inline grouped field", "//gobetter:struct builder\ntype T struct { A, B int //gobetter:field optional\n}", structgen.UnsupportedTarget},
		{"inline blank field", "//gobetter:struct builder\ntype T struct { _ int //gobetter:field optional\n}", structgen.UnsupportedTarget},
		{"inline field duplicate", "//gobetter:struct builder\ntype T struct {\n//gobetter:field getter\nx int //gobetter:field getter\n}", structgen.DirectiveDuplicate},
		{"inline field unknown", "//gobetter:struct builder\ntype T struct { x int //gobetter:field unknown\n}", structgen.InvalidOption},
		{"inline field with space", "//gobetter:struct builder\ntype T struct { x int // gobetter:field optional\n}", structgen.DirectiveSyntax},
		{"block", "/*gobetter:struct builder*/\ntype T struct{}", structgen.DirectivePlacement},
		{"multiline block", "/*\n * gobetter:struct builder\n */\ntype T struct{}", structgen.DirectivePlacement},
		{"blank attachment", "//gobetter:struct builder\n\ntype T struct{}", structgen.DirectivePlacement},
		{"intervening declaration", "//gobetter:struct builder\nvar x int\ntype T struct{}", structgen.DirectivePlacement},
		{"wrong attachment", "//gobetter:field getter\ntype T struct{}", structgen.DirectivePlacement},
		{"grouped types", "//gobetter:struct builder\ntype (A struct{}; B struct{})", structgen.DirectivePlacement},
		{"space after slashes", "// gobetter:struct builder\ntype T struct{}", structgen.DirectiveSyntax},
		{"unknown envelope", "//gobetter:unknown builder\ntype T struct{}", structgen.DirectiveSyntax},
		{"space after colon", "//gobetter: struct builder\ntype T struct{}", structgen.DirectiveSyntax},
		{"unknown output", "//gobetter:struct build\ntype T struct{}", structgen.InvalidOption},
		{"invalid order", "//gobetter:struct order=abc\ntype T struct{}", structgen.InvalidOption},
		{"invalid constructor", "//gobetter:struct constructor=none\ntype T struct{}", structgen.InvalidOption},
		{"empty CSV token", "//gobetter:struct builder,,equal\ntype T struct{}", structgen.DirectiveSyntax},
		{"stacked conflict", "//gobetter:struct constructor=package\n//gobetter:struct constructor=exported\ntype T struct{}", structgen.DirectiveDuplicate},
		{"stacked output duplicate", "//gobetter:struct builder\n//gobetter:struct builder\ntype T struct{}", structgen.DirectiveDuplicate},
		{"field grouped", "//gobetter:struct builder\ntype T struct {\n//gobetter:field optional\nA,B int\n}", structgen.UnsupportedTarget},
		{"field separated by blank", "//gobetter:struct builder\ntype T struct {\n//gobetter:field getter\n\nx int\n}", structgen.DirectivePlacement},
		{"field unknown", "//gobetter:struct builder\ntype T struct {\n//gobetter:field opt\nx int\n}", structgen.InvalidOption},
		{"field boolean assignment", "//gobetter:struct builder\ntype T struct {\n//gobetter:field getter=false\nx int\n}", structgen.InvalidOption},
		{"field keyword name", "//gobetter:struct builder\ntype T struct {\n//gobetter:field name=func\nx int\n}", structgen.InvalidOption},
		{"blank field", "//gobetter:struct builder\ntype T struct {\n//gobetter:field optional\n_ int\n}", structgen.UnsupportedTarget},
		{"nested wrong type", "type T struct {\n//gobetter:struct builder\nx []int\n}", structgen.UnsupportedTarget},
		{"named alias", "//gobetter:struct builder\ntype T = struct{}", structgen.UnsupportedTarget},
		{"nonstruct", "//gobetter:struct builder\ntype T int", structgen.UnsupportedTarget},
		{"legacy inline", "type T struct{} //+gob:Constructor", structgen.LegacyDirective},
		{"legacy block", "/* +gob:getter */\ntype T struct{}", structgen.LegacyDirective},
		{"unused field", "type T struct {\n//gobetter:field getter\nx int\n}", structgen.UnusedFieldDirective},
		{"empty getters", "//gobetter:struct getters\ntype T struct {x int}", structgen.EmptyGetters},
		{"nested methods", "type T struct {\n//gobetter:struct getters\nx struct {\n//gobetter:field getter\ny int\n}\n}", structgen.UnsupportedTarget},
		{"syntax", "type T struct {", structgen.SourceSyntax},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file, err := source.Parse("models.go", []byte("package models\n"+tc.src+"\n"))
			if err == nil {
				_, err = structgen.MakePlans(file.Targets, nil, structgen.Invocation{})
			}
			requireCode(t, err, tc.code)
		})
	}
}

func TestRemovedOrderDirectiveDiagnostics(t *testing.T) {
	for _, value := range []string{"source", "name"} {
		for _, pointer := range []string{"named", "", "*"} {
			src := "package models\n//gobetter:struct builder,order=" + value + "\ntype Item struct{Value int}\n"
			line := 2
			if pointer != "named" {
				src = "package models\ntype Item struct {\n//gobetter:struct builder,order=" + value + "\nValue " + pointer + "struct{A int}\n}\n"
				line = 3
			}
			_, err := source.Parse("model.go", []byte(src))
			ds, ok := err.(structgen.Diagnostics)
			if !ok || len(ds) != 1 || ds[0].Code != structgen.InvalidOption || ds[0].Position.Filename != "model.go" || ds[0].Position.Line != line || ds[0].Position.Column != 27 || ds[0].Message != "unknown or invalid struct option order; use builder,equal,string,getters,constructor=auto|exported|package" {
				t.Fatalf("removed order=%s input must fail at directive boundary: %v", value, err)
			}
		}
	}
}

func TestInlineFieldDirectiveOptions(t *testing.T) {
	src := `package models
//gobetter:struct builder,getters
type User struct {
	// Email is used for notifications.
	//gobetter:field optional
	email string //gobetter:field getter,name=Email
}
`
	file, err := source.Parse("models.go", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	plans, err := structgen.MakePlans(file.Targets, nil, structgen.Invocation{})
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 1 || len(plans[0].Fields) != 1 {
		t.Fatalf("unexpected inline field inventory: %+v", plans)
	}
	field := plans[0].Fields[0]
	if !field.Annotated || !field.Optional || !field.Getter || field.SetterName != "Email" || field.GetterName != "Email" {
		t.Fatalf("inline field settings were not applied: %+v", field)
	}
}

func TestStackedDirectivesAndPhysicalPositions(t *testing.T) {
	src := "package models\n//line renamed.go:200\n// Doc before.\n//gobetter:struct builder\n// More docs.\n//gobetter:struct equal, constructor = auto\ntype T struct {\n//gobetter:field name = ID\n//gobetter:field getter, optional\nid string\n}\n"
	file, err := source.Parse("actual.go", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	plans, err := structgen.MakePlans(file.Targets, nil, structgen.Invocation{})
	if err != nil {
		t.Fatal(err)
	}
	p := plans[0]
	if p.Derive != structgen.Builder|structgen.Equal || !p.Fields[0].Getter || !p.Fields[0].Optional || p.Fields[0].SetterName != "ID" {
		t.Fatalf("stacked settings: %+v", p)
	}
	if p.Target.Position.Filename != "actual.go" || p.Target.Position.Line != 7 || p.Fields[0].Position.Line != 10 {
		t.Fatal("logical //line positions replaced physical origin")
	}
}

func TestSyntaxErrorsUsePhysicalPositions(t *testing.T) {
	cases := []struct {
		name, source, message string
		line, column          int
	}{
		{"lexical line mapping", "package models\n//line renamed.go:200\ntype T struct { @ }", "illegal character", 3, 17},
		{"lexical line and column mapping", "package models\n//line renamed.go:200:11\ntype T struct { @ }", "illegal character", 3, 17},
		{"parser line mapping", "package models\n//line renamed.go:200\ntype T struct { value [] }", "expected type", 3, 26},
		{"parser line and column mapping", "package models\n//line renamed.go:200:11\ntype T struct { value [] }", "expected type", 3, 26},
		{"parser end of file", "package models\n//line renamed.go:200\ntype T struct {", "expected '}'", 3, 16},
		{"early package lexical failure", "//line renamed.go:200\npackage @", "illegal character", 2, 9},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file, err := source.Parse("actual.go", []byte(tc.source))
			if file != nil {
				t.Fatal("syntax failure returned a usable source inventory")
			}
			ds, ok := err.(structgen.Diagnostics)
			if !ok || len(ds) == 0 {
				t.Fatalf("expected syntax diagnostics, got %T: %v", err, err)
			}
			found := false
			for _, diagnostic := range ds {
				pos := diagnostic.Position
				if diagnostic.Code != structgen.SourceSyntax || pos.Filename != "actual.go" || pos.Line < 1 || pos.Column < 1 {
					t.Fatalf("syntax diagnostic lost its physical origin: %+v", diagnostic)
				}
				if strings.Contains(diagnostic.Message, tc.message) {
					found = true
					if pos.Line != tc.line || pos.Column != tc.column {
						t.Fatalf("physical position %s, want actual.go:%d:%d", pos, tc.line, tc.column)
					}
				}
			}
			if !found {
				t.Fatalf("missing %q syntax diagnostic: %v", tc.message, ds)
			}
		})
	}
}

func TestASTInventoryRetainsIdentityAndSource(t *testing.T) {
	src := []byte("// Code generated by upstream; DO NOT EDIT.\npackage models\ntype Base[T any] struct{}\ntype Outer[T any] struct {\nA,B [3]T\n_ int\n*Base[T]\nConfig *struct {Value T `json:\"value\"`}\n}\n")
	before := bytes.Clone(src)
	file, err := source.Parse("models.go", src)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(src, before) || !ast.IsGenerated(file.AST) {
		t.Fatal("generated input changed or was excluded")
	}
	outer := file.Targets[1]
	if got := []string{outer.Fields[0].Name, outer.Fields[1].Name, outer.Fields[2].Name, outer.Fields[3].Name}; !reflect.DeepEqual(got, []string{"A", "B", "Base", "Config"}) {
		t.Fatalf("canonical fields: %v", got)
	}
	array := outer.Fields[0].TypeExpr.(*ast.ArrayType)
	if array.Len.(*ast.BasicLit).Value != "3" || outer.Fields[0].AST != outer.Fields[1].AST || !outer.Fields[2].Embedded {
		t.Fatal("AST fidelity/group identity lost")
	}
	nested := file.Targets[2]
	if nested.Path != "Outer.Config" || nested.Name != "" || nested.TypeParameters[0].Ident != outer.TypeParameters[0].Ident || nested.Fields[0].AST.Tag.Value != "`json:\"value\"`" {
		t.Fatal("nested source identity/constraint/tag lost")
	}
	if _, err := source.Parse("models_test.go", []byte("package models")); err == nil {
		t.Fatal("test-file input accepted")
	}
}

type behavior struct {
	Derive                          structgen.Outputs
	Constructor                     structgen.Constructor
	Fields                          []string
	Builder, Equal, String, Getters []string
}

func describe(t *testing.T, plan structgen.Plan) behavior {
	t.Helper()
	b := behavior{Derive: plan.Derive, Constructor: plan.Constructor}
	for _, field := range plan.Fields {
		var typeText bytes.Buffer
		if err := format.Node(&typeText, token.NewFileSet(), field.TypeExpr); err != nil {
			t.Fatal(err)
		}
		b.Fields = append(b.Fields, fmt.Sprintf("%s|%s|%s|%s|optional=%t,getter=%t,skip-builder=%t,skip-equal=%t,skip-string=%t", field.Name, typeText.String(), field.SetterName, field.GetterName, field.Optional, field.Getter, field.SkipBuilder, field.SkipEqual, field.SkipString))
	}
	for i, output := range []structgen.Outputs{structgen.Builder, structgen.Equal, structgen.String, structgen.Getters} {
		var fields []string
		for _, field := range plan.FieldsFor(output) {
			fields = append(fields, field.Name)
		}
		switch i {
		case 0:
			b.Builder = fields
		case 1:
			b.Equal = fields
		case 2:
			b.String = fields
		case 3:
			b.Getters = fields
		}
	}
	return b
}

func TestAnnotationConfigurationParity(t *testing.T) {
	dir, err := filepath.Abs("testdata/parity")
	if err != nil {
		t.Fatal(err)
	}
	read := func(name string) []byte {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	annotated, err := source.Parse(filepath.Join(dir, "annotated.go"), read("annotated.go"))
	if err != nil {
		t.Fatal(err)
	}
	plainBytes := read("plain.go")
	plain, err := source.Parse(filepath.Join(dir, "plain.go"), plainBytes)
	if err != nil {
		t.Fatal(err)
	}
	configFile := filepath.Join(dir, "gobetter.json")
	document, err := config.Decode(configFile, read("gobetter.json"))
	if err != nil {
		t.Fatal(err)
	}
	overrides, err := config.Bind(document, configFile, dir, []config.PackageInventory{{Dir: dir, Active: plain.Targets}})
	if err != nil {
		t.Fatal(err)
	}
	a, err := structgen.MakePlans(annotated.Targets, nil, structgen.Invocation{})
	if err != nil {
		t.Fatal(err)
	}
	b, err := structgen.MakePlans(plain.Targets, overrides, structgen.Invocation{})
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != 1 || len(b) != 1 || !reflect.DeepEqual(describe(t, a[0]), describe(t, b[0])) {
		t.Fatalf("adapter plans differ: %+v %+v", a, b)
	}
	if !b[0].Fields[1].Optional {
		t.Fatal("optional setting lost")
	}
	if got := describe(t, b[0]); !reflect.DeepEqual(got.Builder, []string{"Z", "id", "A", "B"}) {
		t.Fatalf("source-order view: %+v", got)
	}
	if !bytes.Equal(plainBytes, read("plain.go")) {
		t.Fatal("upstream source bytes changed")
	}
}
