package source_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mobiletoly/gobetter/internal/source"
)

func TestCombinedDependencyBootstrapAndLocalRecovery(t *testing.T) {
	dir := module(t, map[string]string{
		"a/model.go": `package a
import "fixture/z"
//gobetter:struct builder
type A struct { Value z.Z }
var _ = NewABuilder().Value(z.NewZBuilder().First(2).A(1).Build()).Build()
`,
		"z/model.go": `package z
//gobetter:struct builder
type Z struct { B int; A int }
var _ = NewZBuilder().First(2).A(1).Build()
`,
		"z/gobetter.json": `{"schema_version":1,"types":[{"package":".","type":"Z","fields":{"B":{"name":"First"}}}]}`,
		"combined.json":   `{"schema_version":1,"types":[{"package":"./z","type":"Z","fields":{"B":{"name":"First"}}}]}`,
	})
	_, err := source.Load(context.Background(), source.Request{Dir: filepath.Join(dir, "a")})
	if err == nil || !strings.Contains(err.Error(), "dependency's own go:generate") {
		t.Fatalf("missing dependency recovery procedure: %v", err)
	}
	combined, err := source.Load(context.Background(), source.Request{Dir: dir, Patterns: []string{"./a", "./z"}, Config: "combined.json"})
	if err != nil {
		t.Fatal(err)
	}
	if len(combined.Bootstrap) != 2 {
		t.Fatal("selected dependency was not included in combined overlay")
	}
	if err := combined.Validate(context.Background(), combined.Bootstrap); err != nil {
		t.Fatal(err)
	}
	compileOverlay(t, dir, combined.Bootstrap)
	dep, err := source.Load(context.Background(), source.Request{Dir: filepath.Join(dir, "z")})
	if err != nil {
		t.Fatal(err)
	}
	p := dep.Plans[0]
	if p.FieldsFor(1)[0].Name != "B" || p.FieldsFor(1)[0].SetterName != "First" {
		t.Fatal("dependency-local configuration was not retained")
	}
	// An executable handwritten fixture represents the dependency's completed
	// generation. Bootstrap bytes are never written to source/output locations.
	path := filepath.Join(dir, "z/model_gob.go")
	fixture := fmt.Sprintf(`%s
%s0.20.0
%smodel.go
package z
type %s struct { value Z }
type %s struct { value Z }
type %s struct { value Z }
func NewZBuilder() %s { return %s{value:Z{}} }
func (b %s) First(value int) %s { b.value.B=value;return %s{value:b.value} }
func (b %s) A(value int) %s { b.value.A=value;return %s{value:b.value} }
func (b %s) Build() Z { return b.value }
func (b %s) BuildPtr() *Z { value:=b.value;return &value }
`, source.GeneratedHeader, source.VersionMetadata, source.SourceMetadata, p.Names.Steps[0], p.Names.Steps[1], p.Names.Done, p.Names.Steps[0], p.Names.Steps[0], p.Names.Steps[0], p.Names.Steps[1], p.Names.Steps[1], p.Names.Steps[1], p.Names.Done, p.Names.Done, p.Names.Done, p.Names.Done)
	if err := dep.Validate(context.Background(), map[string][]byte{path: []byte(fixture)}); err != nil {
		t.Fatal(err)
	}
	write(t, dir, "z/model_gob.go", fixture)
	if _, err := source.Load(context.Background(), source.Request{Dir: filepath.Join(dir, "a")}); err != nil {
		t.Fatalf("dependency-first recovery failed: %v", err)
	}
	// Stale signatures in a selected dependency are replaced in the same overlay.
	write(t, dir, "z/model_gob.go", source.GeneratedHeader+"\n"+source.VersionMetadata+"0.20.0\n"+source.SourceMetadata+"model.go\npackage z\nfunc NewZBuilder() int {return 0}\n")
	_, err = source.Load(context.Background(), source.Request{Dir: filepath.Join(dir, "a")})
	if err == nil || !strings.Contains(err.Error(), "dependency's own go:generate") {
		t.Fatalf("stale dependency recovery procedure missing: %v", err)
	}
	if _, err := source.Load(context.Background(), source.Request{Dir: dir, Patterns: []string{"./a", "./z"}, Config: "combined.json"}); err != nil {
		t.Fatalf("stale combined bootstrap failed: %v", err)
	}
	write(t, dir, "z/bad.go", "package z\nvar _ = MissingUnrelatedDependency\n")
	_, err = source.Load(context.Background(), source.Request{Dir: dir, Patterns: []string{"./a", "./z"}, Config: "combined.json"})
	if err == nil || !strings.Contains(err.Error(), "MissingUnrelatedDependency") {
		t.Fatalf("dependency error hidden: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "a/model_gob.go")); !os.IsNotExist(err) {
		t.Fatal("dependent bootstrap was published")
	}
}

func TestFileModeRetainsAnotherSourceArtifact(t *testing.T) {
	dir := module(t, map[string]string{
		"one.go":     "package models\n//gobetter:struct builder\ntype One struct { Value int }\nvar _=NewOneBuilder().Value(1).Build()\n",
		"two.go":     "package models\n//gobetter:struct builder\ntype Two struct { Value int }\nvar _=NewTwoBuilder().Value(2).Build()\n",
		"two_gob.go": source.GeneratedHeader + "\n" + source.VersionMetadata + "0.20.0\n" + source.SourceMetadata + "two.go\npackage models\ntype twoStage struct{}\nfunc NewTwoBuilder() twoStage {return twoStage{}}\nfunc (twoStage) Value(int) twoStage {return twoStage{}}\nfunc (twoStage) Build() Two {return Two{Value:2}}\n",
	})
	r, err := source.Load(context.Background(), source.Request{Dir: dir, File: "one.go"})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Bootstrap) != 1 || len(r.Plans) != 1 {
		t.Fatal("file mode widened emission scope")
	}
	for path := range r.Bootstrap {
		if strings.HasSuffix(path, "two_gob.go") {
			t.Fatal("another source's sidecar was replaced")
		}
	}
}

func TestReviewDotImportedPlannedDependencyAliases(t *testing.T) {
	dir := module(t, map[string]string{
		"z/model.go": "package z\n//gobetter:struct builder\ntype Z struct {Config struct {Value int}}\n",
		"a/model.go": "package a\nimport . \"fixture/z\"\n//gobetter:struct builder\ntype A struct {Data ZConfig}\nvar _=NewABuilder().Data(ZConfig{Value:1}).Build()\n",
	})
	for _, stale := range []bool{false, true} {
		if stale {
			write(t, dir, "z/model_gob.go", source.GeneratedHeader+"\n"+source.VersionMetadata+"0.20.0\n"+source.SourceMetadata+"model.go\npackage z\ntype ZConfig=struct {Value string}\n")
		}
		r, err := source.Load(context.Background(), source.Request{Dir: dir, Patterns: []string{"./a", "./z"}})
		if err != nil {
			t.Fatalf("stale=%t: %v", stale, err)
		}
		compileOverlay(t, dir, r.Bootstrap)
	}
}

func TestReviewImportedDependencyWithoutSelfCall(t *testing.T) {
	for _, qualifier := range []string{"z", "dep"} {
		t.Run(qualifier, func(t *testing.T) {
			importText := `"fixture/z"`
			if qualifier == "dep" {
				importText = `dep "fixture/z"`
			}
			dir := module(t, map[string]string{"z/model.go": "package z\n//gobetter:struct builder\ntype Z struct {Value int}\n", "a/model.go": "package a\nimport " + importText + "\nvar _=" + qualifier + ".NewZBuilder().Value(1).Build()\n"})
			_, err := source.Load(context.Background(), source.Request{Dir: filepath.Join(dir, "a")})
			if err == nil || !strings.Contains(err.Error(), qualifier+".NewZBuilder") || !strings.Contains(err.Error(), "dependency's own go:generate") || !strings.Contains(err.Error(), "fixture/z") {
				t.Fatalf("importing-package recovery guidance missing: %v", err)
			}
			if _, err := os.Stat(filepath.Join(dir, "z/model_gob.go")); !os.IsNotExist(err) {
				t.Fatal("dependency scope was widened")
			}
			write(t, dir, "a/model.go", "package a\nimport "+importText+"\nvar _="+qualifier+".MissingOrdinaryFunction()\n")
			_, err = source.Load(context.Background(), source.Request{Dir: filepath.Join(dir, "a")})
			if err == nil || !strings.Contains(err.Error(), "MissingOrdinaryFunction") || strings.Contains(err.Error(), "go:generate") {
				t.Fatalf("ordinary error was misclassified: %v", err)
			}
		})
	}
	t.Run("same_message_different_imports", func(t *testing.T) {
		for _, mapping := range []string{"", "//line shared.go:100:1\n"} {
			t.Run(fmt.Sprintf("mapped=%t", mapping != ""), func(t *testing.T) {
				dir := module(t, map[string]string{
					"z/model.go":     "package z\n//gobetter:struct builder\ntype Z struct {Value int}\n",
					"ordinary/z.go":  "package z\n",
					"a/generated.go": "package a\nimport \"fixture/z\"\n" + mapping + "var _=z.NewZBuilder()\n",
					"a/ordinary.go":  "package a\nimport \"fixture/ordinary\"\n" + mapping + "var _=z.NewZBuilder()\n",
				})
				_, err := source.Load(context.Background(), source.Request{Dir: filepath.Join(dir, "a")})
				if err == nil || strings.Count(err.Error(), "undefined: z.NewZBuilder") != 2 || strings.Count(err.Error(), "dependency's own go:generate") != 1 || strings.Contains(err.Error(), "missing/stale generated dependency fixture/ordinary") {
					t.Fatalf("same-message errors lost their import/position identity: %v", err)
				}
				if mapping != "" {
					for _, filename := range []string{"generated.go", "ordinary.go"} {
						if !strings.Contains(err.Error(), filepath.Join(dir, "a", filename)+":4:") {
							t.Fatalf("physical diagnostic origin %s missing: %v", filename, err)
						}
					}
					if !strings.Contains(err.Error(), "physical source: "+filepath.Join(dir, "a/generated.go")+":4:9\nmissing/stale generated dependency fixture/z:") || strings.Contains(err.Error(), "physical source: "+filepath.Join(dir, "a/ordinary.go")+":4:9\nmissing/stale generated dependency") {
						t.Fatalf("guidance attached to the wrong physical caller: %v", err)
					}
				}
			})
		}
	})
}

func TestReviewDependencyConfigurationAttribution(t *testing.T) {
	for _, tc := range []struct {
		name, configuration, directive string
		guidance                       bool
	}{
		{"string_replaces_builder", `{"schema_version":1,"types":[{"package":".","type":"Z","derive":["string"]}]}`, "//go:generate go tool gobetter generate .\n", false},
		{"package_constructor", `{"schema_version":1,"types":[{"package":".","type":"Z","constructor":"package"}]}`, "", false},
		{"builder_with_configuration", `{"schema_version":1,"types":[{"package":".","type":"Z","derive":["builder"]}]}`, "//go:generate go tool gobetter generate .\n", true},
		{"malformed_configuration", `{"schema_version":`, "", false},
		{"unknown_selector", `{"schema_version":1,"types":[{"package":".","type":"Unknown"}]}`, "", false},
		{"unknown_directive_flags", `{"schema_version":1,"types":[{"package":".","type":"Z","derive":["builder"]}]}`, "//go:generate go tool gobetter generate -derive=string .\n", false},
		{"unknown_directive_tab_flags", `{"schema_version":1,"types":[{"package":".","type":"Z","derive":["builder"]}]}`, "//go:generate\tgo tool gobetter generate -derive=string .\n", false},
	} {
		for _, selfCall := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/self_call=%t", tc.name, selfCall), func(t *testing.T) {
				model := "package z\n" + tc.directive + "//gobetter:struct builder\ntype Z struct {Value int}\n"
				if selfCall {
					model += "var _=NewZBuilder().Value(1).Build()\n"
				}
				dir := module(t, map[string]string{
					"z/model.go":      model,
					"z/gobetter.json": tc.configuration,
					"a/model.go":      "package a\nimport \"fixture/z\"\nvar _=z.NewZBuilder()\n",
				})
				_, err := source.Load(context.Background(), source.Request{Dir: filepath.Join(dir, "a")})
				if err == nil || !strings.Contains(err.Error(), "NewZBuilder") || strings.Contains(err.Error(), "dependency's own go:generate") != tc.guidance {
					t.Fatalf("dependency settings misattributed: %v", err)
				}
				for _, pkg := range []string{"a", "z"} {
					if _, err := os.Stat(filepath.Join(dir, pkg, "model_gob.go")); !os.IsNotExist(err) {
						t.Fatal("generation scope widened")
					}
				}
			})
		}
	}
}

func TestReviewPrivateConstructorRecovery(t *testing.T) {
	for _, tc := range []struct {
		name     string
		external bool
		selfCall bool
		guidance int
	}{
		{"external_private_call", true, false, 0},
		{"dependency_private_self_call", false, true, 1},
		{"both_origins", true, true, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model := "package z\n//gobetter:struct builder\ntype Z struct {Value int}\n"
			if tc.selfCall {
				model += "var _=newZBuilder().Value(1).Build()\n"
			}
			caller := "var _=z.Z{}\n"
			if tc.external {
				caller = "var _=z.newZBuilder()\n"
			}
			dir := module(t, map[string]string{
				"z/model.go":      model,
				"z/gobetter.json": `{"schema_version":1,"types":[{"package":".","type":"Z","constructor":"package"}]}`,
				"a/model.go":      "package a\nimport \"fixture/z\"\n" + caller,
			})
			_, err := source.Load(context.Background(), source.Request{Dir: filepath.Join(dir, "a")})
			if err == nil || strings.Count(err.Error(), "dependency's own go:generate") != tc.guidance {
				t.Fatalf("private constructor guidance: %v", err)
			}
			if tc.external && (!strings.Contains(err.Error(), "undefined: z.newZBuilder") || strings.Contains(err.Error(), "undefined: z.newZBuilder\nmissing/stale generated dependency")) {
				t.Fatalf("external inaccessible call lost its original diagnostic or received guidance: %v", err)
			}
			if tc.selfCall {
				if !strings.Contains(err.Error(), "undefined: newZBuilder\nmissing/stale generated dependency fixture/z:") {
					t.Fatalf("private self-call recovery was lost: %v", err)
				}
				dependency, err := source.Load(context.Background(), source.Request{Dir: filepath.Join(dir, "z")})
				if err != nil {
					t.Fatal(err)
				}
				if dependency.Plans[0].Names.Constructor != "newZBuilder" {
					t.Fatal("dependency's private constructor setting changed")
				}
				if err := dependency.Validate(context.Background(), dependency.Bootstrap); err != nil {
					t.Fatal(err)
				}
				compileOverlay(t, filepath.Join(dir, "z"), dependency.Bootstrap)
			}
			for _, pkg := range []string{"a", "z"} {
				if _, err := os.Stat(filepath.Join(dir, pkg, "model_gob.go")); !os.IsNotExist(err) {
					t.Fatal("generation scope widened or bootstrap published")
				}
			}
		})
	}
}
