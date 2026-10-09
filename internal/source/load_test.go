package source_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"go/types"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mobiletoly/gobetter/internal/source"
)

func module(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	write(t, dir, "go.mod", "module fixture\n\ngo 1.26.0\n")
	for name, text := range files {
		write(t, dir, name, text)
	}
	return dir
}
func write(t *testing.T, dir, name, text string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadSignatureBootstrapAndTypeFidelity(t *testing.T) {
	dir := module(t, map[string]string{
		"models.go": `package models
import clock "time"
//gobetter:struct builder,getters
type Box[Box interface { Self() Box }] struct {
    //gobetter:field getter
    value [3]Box
    When clock.Time
    Nested *struct { Box Box ` + "`json:\"Box\"`" + ` }
}
type Self int
func (v Self) Self() Self { return v }
var _ = NewBoxBuilder[Self]().Value([3]Self{}).When(clock.Time{}).Nested(nil).Build()
var _ = (&Box[Self]{}).Value()
`,
	})
	before, err := os.ReadFile(filepath.Join(dir, "models.go"))
	if err != nil {
		t.Fatal(err)
	}
	r, err := source.Load(context.Background(), source.Request{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Plans) != 2 || len(r.Bootstrap) != 1 {
		t.Fatalf("plans/overlay: %+v", r)
	}
	p := r.Plans[0]
	if p.Target.TypeParameters[0].Binder == "Box" || p.Target.TypeParameters[0].Object == nil {
		t.Fatal("generic binding was not preserved/renamed/resolved")
	}
	if _, ok := p.Fields[0].Type.(*types.Array); !ok {
		t.Fatalf("fixed array became %T", p.Fields[0].Type)
	}
	for path, code := range r.Bootstrap {
		if !strings.Contains(string(code), `clock "time"`) || !strings.Contains(string(code), "[3]gobetterT") || !strings.Contains(string(code), `json:"Box"`) {
			t.Fatalf("type syntax lost:\n%s", code)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("bootstrap was published at %s", path)
		}
	}
	if err := r.Validate(context.Background(), r.Bootstrap); err != nil {
		t.Fatalf("combined overlay validation: %v", err)
	}
	compileOverlay(t, dir, r.Bootstrap)
	after, err := os.ReadFile(filepath.Join(dir, "models.go"))
	if err != nil || sha256.Sum256(before) != sha256.Sum256(after) {
		t.Fatal("source changed")
	}
}

// Native compiler qualification uses Go's normal overlay facility. Replacement
// bytes live outside the fixture source directory and no bootstrap is executed.
func compileOverlay(t *testing.T, dir string, overlay map[string][]byte) {
	t.Helper()
	temp := t.TempDir()
	replacements := make(map[string]string)
	i := 0
	for path, data := range overlay {
		replacement := filepath.Join(temp, fmt.Sprintf("%d.go", i))
		i++
		if err := os.WriteFile(replacement, data, 0600); err != nil {
			t.Fatal(err)
		}
		replacements[path] = replacement
	}
	data, err := json.Marshal(struct{ Replace map[string]string }{Replace: replacements})
	if err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(temp, "overlay.json")
	if err := os.WriteFile(manifest, data, 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "build", "-mod=readonly", "-overlay="+manifest, "./...")
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("native overlay compilation: %v\n%s", err, output)
	}
}

func TestLoadStaleArtifactAndUnrelatedErrors(t *testing.T) {
	dir := module(t, map[string]string{"models.go": "package models\n//gobetter:struct builder\ntype Item struct { Value [2]int }\nvar _ = NewItemBuilder().Value([2]int{}).Build()\n",
		"models_gob.go": source.GeneratedHeader + "\n" + source.VersionMetadata + "0.20.0\n" + source.SourceMetadata + "models.go\npackage models\nfunc NewItemBuilder() int {return 0}\n"})
	artifact := filepath.Join(dir, "models_gob.go")
	before, _ := os.ReadFile(artifact)
	r, err := source.Load(context.Background(), source.Request{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Validate(context.Background(), map[string][]byte{artifact: []byte("package models\nfunc Broken( {")}); err == nil {
		t.Fatal("invalid final artifact was accepted")
	}
	after, _ := os.ReadFile(artifact)
	if sha256.Sum256(before) != sha256.Sum256(after) {
		t.Fatal("stale output was changed on disk")
	}
	write(t, dir, "bad.go", "package models\nvar bad = MissingUnrelated\n")
	_, err = source.Load(context.Background(), source.Request{Dir: dir})
	if err == nil || !strings.Contains(err.Error(), "MissingUnrelated") {
		t.Fatalf("unrelated source error hidden: %v", err)
	}
}

func TestDiscoveryPatternsFileModeAndInactiveConfig(t *testing.T) {
	dir := module(t, map[string]string{
		"model.go":            "package models\n//gobetter:struct builder\ntype Item struct { Value int }\n",
		"variant_linux.go":    "package models\n//gobetter:struct builder\ntype Variant struct { Linux int }\n",
		"variant_windows.go":  "package models\n//gobetter:struct builder\ntype Variant struct { Windows string }\n",
		"sub/model.go":        "package sub\n//gobetter:struct builder\ntype Child struct { Value int }\n",
		"gobetter.json":       `{"schema_version":1,"types":[{"package":".","type":"Variant","fields":{"Linux":{"optional":true}}},{"package":".","type":"Inactive","fields":{"typo":{"getter":true}}}]}`,
		"inactive_windows.go": "package models\ntype Inactive struct { Value int }\n",
	})
	r, err := source.Load(context.Background(), source.Request{Dir: dir, GOOS: "linux", CgoEnabled: new(false)})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Packages) != 1 || len(r.Plans) != 2 {
		t.Fatalf(". traversed subpackages: %+v", r)
	}
	r, err = source.Load(context.Background(), source.Request{Dir: dir, GOOS: "linux", CgoEnabled: new(false), Patterns: []string{"./...", "."}})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Packages) != 2 {
		t.Fatal("recursive pattern was not deduplicated")
	}
	r, err = source.Load(context.Background(), source.Request{Dir: dir, GOOS: "linux", CgoEnabled: new(false), File: "model.go"})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Plans) != 1 {
		t.Fatal("file mode selected another source")
	}
	_, err = source.Load(context.Background(), source.Request{Dir: dir, GOOS: "linux", CgoEnabled: new(false), File: "variant_windows.go"})
	if err == nil || !strings.Contains(err.Error(), "not active") {
		t.Fatalf("inactive explicit file accepted: %v", err)
	}
	_, err = source.Load(context.Background(), source.Request{Dir: dir, GOOS: "windows", CgoEnabled: new(false)})
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("active config field semantics skipped: %v", err)
	}
	_, err = source.Load(context.Background(), source.Request{Dir: dir, Patterns: []string{"./missing"}})
	if err == nil {
		t.Fatal("unmatched pattern accepted")
	}
	_, err = source.Load(context.Background(), source.Request{Dir: dir, File: "model.go", Patterns: []string{"."}})
	if err == nil {
		t.Fatal("file/pattern combination accepted")
	}
	if got := source.OutputFilename("person_linux_amd64.go"); got != "person_gob_linux_amd64.go" {
		t.Fatalf("platform suffix: %s", got)
	}
}
