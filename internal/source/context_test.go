package source_test

import (
	"context"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mobiletoly/gobetter/internal/source"
)

func TestBuildTagsAndUnrecognizableInactiveInput(t *testing.T) {
	dir := module(t, map[string]string{
		"anchor.go":         "package models\n",
		"tagged.go":         "// License first.\n//go:build custom\n\npackage models\n//gobetter:struct builder\ntype Tagged struct { Value int }\nvar _=NewTaggedBuilder().Value(1).Build()\n",
		"broken_windows.go": "package models\ntype Broken struct {",
	})
	r, err := source.Load(context.Background(), source.Request{Dir: dir, GOOS: "linux", CgoEnabled: new(false), Tags: []string{"custom"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Plans) != 1 || r.Plans[0].Target.BuildConstraint == nil || r.Plans[0].Target.BuildConstraint.String() != "custom" {
		t.Fatal("leading license/build header lost")
	}
	if _, err := source.Load(context.Background(), source.Request{Dir: dir, GOOS: "linux", CgoEnabled: new(false), File: "tagged.go"}); err == nil {
		t.Fatal("tag-excluded file accepted")
	}
	write(t, dir, "gobetter.json", `{"schema_version":1,"types":[{"package":".","type":"Unknown"}]}`)
	_, err = source.Load(context.Background(), source.Request{Dir: dir, GOOS: "linux", CgoEnabled: new(false)})
	if err == nil || !strings.Contains(err.Error(), "inactive declaration inventory is incomplete") {
		t.Fatalf("unrecognizable inactive input was guessed: %v", err)
	}
}

func TestParenthesizedStructAcrossBuildContexts(t *testing.T) {
	dir := module(t, map[string]string{
		"anchor.go": "package models\n",
		"tagged.go": `//go:build custom

package models
type Outer (struct { Child struct { Value int } })
`,
		"gobetter.json": `{"schema_version":1,"types":[{"package":".","type":"Outer.Child","fields":{"Missing":{"optional":true}}}]}`,
	})
	r, err := source.Load(t.Context(), source.Request{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Plans) != 0 || len(r.Bootstrap) != 0 {
		t.Fatal("inactive nested declaration was selected")
	}
	write(t, dir, "gobetter.json", `{"schema_version":1,"types":[{"package":".","type":"Outer.Child","fields":{"Value":{"optional":true}}}]}`)
	r, err = source.Load(t.Context(), source.Request{Dir: dir, Tags: []string{"custom"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Plans) != 1 || r.Plans[0].Target.Path != "Outer.Child" || !r.Plans[0].Fields[0].Optional || len(r.Bootstrap) != 1 {
		t.Fatalf("active nested configuration was not applied: %+v", r.Plans)
	}
}

func TestCgoInactiveInventory(t *testing.T) {
	dir := module(t, map[string]string{
		"anchor.go":     "package models\n",
		"native.go":     "package models\n/*\n#include <stdint.h>\n*/\nimport \"C\"\n//gobetter:struct builder\ntype Native struct { Value int }\nvar _=NewNativeBuilder().Value(1).Build()\n",
		"gobetter.json": `{"schema_version":1,"types":[{"package":".","type":"Native","fields":{"Value":{"optional":false}}}]}`,
	})
	r, err := source.Load(context.Background(), source.Request{Dir: dir, CgoEnabled: new(false)})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Plans) != 0 || len(r.Bootstrap) != 0 {
		t.Fatal("inactive cgo source was selected")
	}
	r, err = source.Load(context.Background(), source.Request{Dir: dir, CgoEnabled: new(true)})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Plans) != 1 || !strings.Contains(r.Plans[0].Target.BuildConstraint.String(), "cgo") {
		t.Fatal("implicit cgo constraint not retained")
	}
}

func TestLoaderTemporaryCleanupAndCancellation(t *testing.T) {
	loaderTemp := t.TempDir()
	t.Setenv("TMPDIR", loaderTemp)
	dir := module(t, map[string]string{"model.go": "package models\n//gobetter:struct builder\ntype Item struct {Value int}\nvar _=NewItemBuilder().Value(1).Build()\n"})
	r, err := source.Load(context.Background(), source.Request{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	check := func() {
		t.Helper()
		entries, err := os.ReadDir(loaderTemp)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), "gocommand-") {
				t.Fatalf("loader overlay temporary leaked: %s", entry.Name())
			}
		}
		if _, err := os.Stat(filepath.Join(dir, "model_gob.go")); !os.IsNotExist(err) {
			t.Fatal("bootstrap reached an output location")
		}
	}
	check()
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := r.Validate(cancelled, r.Bootstrap); err == nil {
		t.Fatal("cancelled validation succeeded")
	}
	check()
	for path := range r.Bootstrap {
		if err := r.Validate(context.Background(), map[string][]byte{path: []byte("package models\nvar _=UnknownFailure\n")}); err == nil {
			t.Fatal("bad overlay accepted")
		}
	}
	check()
}

func TestReviewArchitectureFeatureTags(t *testing.T) {
	t.Setenv("GOAMD64", "v1")
	t.Setenv("GOARM64", "v8.0")
	for _, tc := range []struct{ arch, tag string }{{"amd64", "amd64.v1"}, {"arm64", "arm64.v8.0"}} {
		t.Run(tc.arch, func(t *testing.T) {
			dir := module(t, map[string]string{"anchor.go": "package models\n", "model.go": "//go:build " + tc.tag + "\n\npackage models\n//gobetter:struct builder\ntype Item struct {Value int}\nvar _=NewItemBuilder().Value(1).Build()\n"})
			r, err := source.Load(context.Background(), source.Request{Dir: dir, GOOS: "linux", GOARCH: tc.arch, CgoEnabled: new(false)})
			if err != nil {
				t.Fatal(err)
			}
			if len(r.Plans) != 1 || len(r.Bootstrap) != 1 {
				t.Fatal("Go-selected architecture-feature source omitted")
			}
			if err := r.Validate(context.Background(), r.Bootstrap); err != nil {
				t.Fatal(err)
			}
			t.Setenv("GOOS", "linux")
			t.Setenv("GOARCH", tc.arch)
			t.Setenv("CGO_ENABLED", "0")
			compileOverlay(t, dir, r.Bootstrap)
		})
	}
}

func TestReviewBuildDirectivesInsideBlockComments(t *testing.T) {
	for _, tc := range []struct {
		name, header, constraint string
		compile                  bool
	}{
		{"block_contents", "/* License\n//go:build custom\n// +build custom\n*/\n", "", true},
		{"trailing_go_build", "/* License */ //go:build custom\n", "", false},
		{"legacy_after_block", "/* License */\n// +build custom\n", "", true},
		{"go_build_after_block", "/* License */\n//go:build custom\n", "custom", true},
		{"legacy_header", "// License\n// +build custom\n", "custom", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := module(t, map[string]string{"model.go": tc.header + "\npackage models\n//gobetter:struct builder\ntype Item struct {Value int}\nvar _=NewItemBuilder().Value(1).Build()\n"})
			var tags []string
			if tc.constraint != "" {
				tags = []string{"custom"}
			}
			r, err := source.Load(context.Background(), source.Request{Dir: dir, Tags: tags})
			if err != nil {
				t.Fatal(err)
			}
			if len(r.Plans) != 1 {
				t.Fatal("active target omitted")
			}
			constraint := ""
			if r.Plans[0].Target.BuildConstraint != nil {
				constraint = r.Plans[0].Target.BuildConstraint.String()
			}
			if constraint != tc.constraint {
				t.Fatalf("constraint = %q, want %q", constraint, tc.constraint)
			}
			if err := r.Validate(context.Background(), r.Bootstrap); err != nil {
				t.Fatal(err)
			}
			// A trailing //go:build is rejected by the compiler as misplaced;
			// its original diagnostic is outside this constraint-extraction check.
			if tc.compile {
				if len(tags) != 0 {
					t.Setenv("GOFLAGS", "-tags=custom")
				}
				compileOverlay(t, dir, r.Bootstrap)
			}
		})
	}
}

func TestCgoOrdinaryImports(t *testing.T) {
	dir := module(t, map[string]string{"anchor.go": "package models\n", "native.go": `package models
/* #include <stdint.h> */
import "C"
import clock "time"
import "fmt"
//gobetter:struct builder
type Native struct { When clock.Time; Printer fmt.Stringer }
var _=NewNativeBuilder().When(clock.Time{}).Printer(nil).Build()
`})
	r, err := source.Load(context.Background(), source.Request{Dir: dir, CgoEnabled: new(true)})
	if err != nil {
		t.Fatal(err)
	}
	if imports := bootstrapImports(t, r); !maps.Equal(imports, map[string]string{"fmt": "fmt", "time": "clock"}) {
		t.Fatalf("ordinary cgo imports were not preserved in emitted signatures: %v", imports)
	}
	for _, field := range r.Plans[0].Fields {
		if field.Type == nil {
			t.Fatal("ordinary cgo field type missing")
		}
	}
	compileOverlay(t, dir, r.Bootstrap)
}
