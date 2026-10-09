package generate

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mobiletoly/gobetter/internal/source"
)

func fixture(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	writeInput(t, dir, "go.mod", "module fixture\n\ngo 1.26.0\n")
	for name, data := range files {
		writeInput(t, dir, name, data)
	}
	return dir
}
func writeInput(t *testing.T, dir, name, data string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
}
func goCommand(t *testing.T, dir string, env []string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "go", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	data, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go %v: %v\n%s", args, err, data)
	}
	return string(data)
}

type fileRecord struct {
	data []byte
	info os.FileInfo
}

func record(t *testing.T, path string) fileRecord {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return fileRecord{data, info}
}
func preserved(t *testing.T, path string, before fileRecord) {
	t.Helper()
	after := record(t, path)
	if !bytes.Equal(before.data, after.data) || !before.info.ModTime().Equal(after.info.ModTime()) || before.info.Mode() != after.info.Mode() {
		t.Fatalf("changed preserved input/output %s", path)
	}
}

func TestLifecycleRuntimeAndDrift(t *testing.T) {
	dir := fixture(t, map[string]string{
		"model.go": `package models
//gobetter:struct builder,equal,string,getters
type Item struct {
 Value int
 // Note is shown in the item summary.
 note string //gobetter:field getter,optional
 Values []int
}
var _=NewItemBuilder().Value(1).Values(nil).Build()
`,
		"model_test.go": `package models
import "testing"
func TestRuntime(t *testing.T){b:=NewItemBuilder().Value(1);a:=b.Note("x").Values(nil).Build();p:=b.Note("y").Values(nil).BuildPtr();if a.Note()!="x"||p.Note()!="y"||a.String()!=` + "`Item{Value:1, note:\"x\", Values:[]int(nil)}`" + `{t.Fatal(a,p)};c:=a;c.Values=[]int{};if !a.Equal(c){t.Fatal("nil/empty")}}
`,
	})
	input := filepath.Join(dir, "model.go")
	before := record(t, input)
	request := source.Request{Dir: dir}
	r, err := Run(t.Context(), request, Check)
	if err != nil || len(r.Changes) != 1 || r.Changes[0].Kind != "missing" {
		t.Fatalf("missing check: %+v %v", r, err)
	}
	path := source.OutputFilename(input)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("check published output")
	}
	r, err = Run(t.Context(), request, Generate)
	if err != nil || len(r.Written) != 1 {
		t.Fatalf("generate: %+v %v", r, err)
	}
	preserved(t, input, before)
	generated := record(t, path)
	if got := bytes.Count(generated.data, []byte("// Note is shown in the item summary.")); got != 2 || bytes.Contains(generated.data, []byte("//gobetter:field")) {
		t.Fatalf("field comment propagation: found %d copies or a field directive in generated output", got)
	}
	goCommand(t, dir, nil, "test", "-count=1", "./...")
	r, err = Run(t.Context(), request, Generate)
	if err != nil || len(r.Changes) != 0 {
		t.Fatalf("repeat: %+v %v", r, err)
	}
	preserved(t, path, generated)
	r, err = Run(t.Context(), request, Check)
	if err != nil || len(r.Changes) != 0 {
		t.Fatalf("clean check: %+v %v", r, err)
	}
	preserved(t, path, generated)
	writeInput(t, dir, "model_gob.go", string(generated.data)+"\n// stale\n")
	stale := record(t, path)
	r, err = Run(t.Context(), request, Check)
	if err != nil || len(r.Changes) != 1 || r.Changes[0].Kind != "changed" {
		t.Fatalf("stale check: %+v %v", r, err)
	}
	preserved(t, path, stale)
	_, err = Run(t.Context(), request, Generate)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(input); err != nil {
		t.Fatal(err)
	}
	writeInput(t, dir, "anchor.go", "package models\n")
	if err := os.Remove(filepath.Join(dir, "model_test.go")); err != nil {
		t.Fatal(err)
	}
	r, err = Run(t.Context(), request, Check)
	if err != nil || len(r.Changes) != 1 || r.Changes[0].Kind != "obsolete" {
		t.Fatalf("obsolete check: %+v %v", r, err)
	}
	r, err = Run(t.Context(), request, Generate)
	if err != nil || len(r.Removed) != 1 {
		t.Fatalf("obsolete removal: %+v %v", r, err)
	}
}

func TestFailureBeforePublicationAndConvergence(t *testing.T) {
	dir := fixture(t, map[string]string{"a.go": "package models\n//gobetter:struct builder\ntype A struct{Value int}\n", "b.go": "package models\n//gobetter:struct builder\ntype B struct{Value int}\n"})
	request := source.Request{Dir: dir}
	if _, err := Run(t.Context(), request, Generate); err != nil {
		t.Fatal(err)
	}
	a, b := filepath.Join(dir, "a_gob.go"), filepath.Join(dir, "b_gob.go")
	oldA, oldB := record(t, a), record(t, b)
	writeInput(t, dir, "b.go", "package models\n//gobetter:struct equal\ntype B struct{Value func()}\n")
	if _, err := Run(t.Context(), request, Generate); err == nil || !strings.Contains(err.Error(), "unsupported_equality") {
		t.Fatalf("unsupported output: %v", err)
	}
	preserved(t, a, oldA)
	preserved(t, b, oldB)
	writeInput(t, dir, "a.go", "package models\n//gobetter:struct builder\ntype A struct{Value string}\n")
	writeInput(t, dir, "b.go", "package models\n//gobetter:struct builder\ntype B struct{Value string}\n")
	calls := 0
	r, err := run(t.Context(), request, Generate, func(path string, data []byte) error {
		calls++
		if calls == 2 {
			return errors.New("injected rename failure")
		}
		return atomicReplace(path, data)
	})
	if err == nil || len(r.Written) != 1 || !strings.Contains(err.Error(), "b_gob.go") || !strings.Contains(err.Error(), "after 1 writes") {
		t.Fatalf("partial result not truthful: %+v %v", r, err)
	}
	preserved(t, b, oldB)
	r, err = Run(t.Context(), request, Generate)
	if err != nil || len(r.Written) != 1 || r.Written[0] != b {
		t.Fatalf("convergence: %+v %v", r, err)
	}
	goCommand(t, dir, nil, "test", "./...")
}

func TestFileScopeAndInactiveArtifacts(t *testing.T) {
	dir := fixture(t, map[string]string{"a.go": "package models\n//gobetter:struct builder\ntype A struct{Value int}\nvar _=NewBBuilder().Value(1).Build()\n", "b.go": "package models\n//gobetter:struct builder\ntype B struct{Value int}\n"})
	if _, err := Run(t.Context(), source.Request{Dir: dir}, Generate); err != nil {
		t.Fatal(err)
	}
	b := record(t, filepath.Join(dir, "b_gob.go"))
	writeInput(t, dir, "a.go", "package models\n//gobetter:struct builder\ntype A struct{Value string}\nvar _=NewBBuilder().Value(1).Build()\n")
	if _, err := Run(t.Context(), source.Request{Dir: dir, File: "a.go"}, Generate); err != nil {
		t.Fatal(err)
	}
	preserved(t, filepath.Join(dir, "b_gob.go"), b)
	writeInput(t, dir, "variant.go", "//go:build custom\n\npackage models\n//gobetter:struct builder\ntype Variant struct{Value int}\n")
	if _, err := Run(t.Context(), source.Request{Dir: dir, Tags: []string{"custom"}}, Generate); err != nil {
		t.Fatal(err)
	}
	variant := record(t, filepath.Join(dir, "variant_gob.go"))
	if _, err := Run(t.Context(), source.Request{Dir: dir}, Generate); err != nil {
		t.Fatal(err)
	}
	preserved(t, filepath.Join(dir, "variant_gob.go"), variant)
	goCommand(t, dir, nil, "test", "./...")
}

func TestOwnershipAndConsumerFloor(t *testing.T) {
	for _, kind := range []string{"handwritten", "symlink", "old_go"} {
		t.Run(kind, func(t *testing.T) {
			dir := fixture(t, map[string]string{"model.go": "package models\n//gobetter:struct builder\ntype Item struct{Value int}\n"})
			path := filepath.Join(dir, "model_gob.go")
			if kind == "handwritten" {
				writeInput(t, dir, "model_gob.go", "package models\nfunc Handwritten(){}\n")
			}
			if kind == "symlink" {
				if err := os.Symlink(filepath.Join(dir, "model.go"), path); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "old_go" {
				writeInput(t, dir, "go.mod", "module fixture\n\ngo 1.24.0\n")
			}
			module := record(t, filepath.Join(dir, "go.mod"))
			input := record(t, filepath.Join(dir, "model.go"))
			if _, err := Run(t.Context(), source.Request{Dir: dir}, Generate); err == nil {
				t.Fatal("unsafe target accepted")
			}
			preserved(t, filepath.Join(dir, "go.mod"), module)
			preserved(t, filepath.Join(dir, "model.go"), input)
		})
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	dir := fixture(t, map[string]string{"model.go": "package models\n"})
	if _, err := Run(ctx, source.Request{Dir: dir}, Generate); err == nil {
		t.Fatal("cancelled invocation succeeded")
	}
}

func TestReviewDuplicateArtifactMetadata(t *testing.T) {
	for _, key := range []string{source.SourceMetadata, source.VersionMetadata} {
		keyName := "source"
		valid := "model.go"
		if key == source.VersionMetadata {
			keyName = "version"
			valid = Version
		}
		for _, values := range []struct{ name, first, second string }{
			{"blank_first", "", valid}, {"nonblank_first", valid, valid},
			{"both_blank", "", ""}, {"blank_second", valid, ""},
		} {
			for _, mode := range []Mode{Check, Generate} {
				t.Run(keyName+"/"+values.name+"/"+[]string{"generate", "check"}[mode], func(t *testing.T) {
					dir := fixture(t, map[string]string{"model.go": "package models\n//gobetter:struct builder\ntype Item struct{Value int}\n"})
					lines := []string{source.GeneratedHeader, key + values.first, key + values.second}
					if key == source.SourceMetadata {
						lines = append(lines, source.VersionMetadata+Version)
					} else {
						lines = append(lines, source.SourceMetadata+"model.go")
					}
					writeInput(t, dir, "model_gob.go", strings.Join(lines, "\n")+"\n\npackage models\nfunc Handwritten() int { return 1 }\n")
					path := filepath.Join(dir, "model_gob.go")
					before := record(t, path)
					if _, owned, err := source.InspectArtifact(path, before.data); owned || err == nil || !strings.Contains(err.Error(), "duplicate") {
						t.Errorf("ambiguous metadata authorized ownership: owned=%t err=%v", owned, err)
					}
					input, module := record(t, filepath.Join(dir, "model.go")), record(t, filepath.Join(dir, "go.mod"))
					if report, err := Run(t.Context(), source.Request{Dir: dir}, mode); err == nil || !strings.Contains(err.Error(), "duplicate") || len(report.Written)+len(report.Removed) != 0 {
						t.Errorf("duplicate metadata accepted: %+v %v", report, err)
					}
					preserved(t, path, before)
					preserved(t, filepath.Join(dir, "model.go"), input)
					preserved(t, filepath.Join(dir, "go.mod"), module)
				})
			}
		}
	}
}

// Run FIFO probes in a subprocess: a blocked ReadFile ignores cancellation.
func TestReviewNonregularDestination(t *testing.T) {
	for _, kind := range []string{"fifo", "directory"} {
		for _, mode := range []string{"check", "generate"} {
			for _, pattern := range []string{"", "./models", "fixture/models", "./...", "fixture/...", "./mo..."} {
				t.Run(kind+"/"+mode+"/"+pattern, func(t *testing.T) {
					name := "model.go"
					if pattern != "" {
						name = "models/model.go"
					}
					dir := fixture(t, map[string]string{name: "package models\n//gobetter:struct builder\ntype Item struct{Value int}\n"})
					path := source.OutputFilename(filepath.Join(dir, name))
					if kind == "fifo" {
						mkfifo, err := exec.LookPath("mkfifo")
						if err != nil {
							t.Skip("mkfifo is unavailable")
						}
						if output, err := exec.CommandContext(t.Context(), mkfifo, path).CombinedOutput(); err != nil {
							t.Fatalf("mkfifo: %v\n%s", err, output)
						}
					} else if err := os.Mkdir(path, 0755); err != nil {
						t.Fatal(err)
					}
					before, err := os.Lstat(path)
					if err != nil {
						t.Fatal(err)
					}
					input, module := record(t, filepath.Join(dir, name)), record(t, filepath.Join(dir, "go.mod"))
					binary, err := os.Executable()
					if err != nil {
						t.Fatal(err)
					}
					ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
					defer cancel()
					cmd := exec.CommandContext(ctx, binary, "-test.run=^TestReviewNonregularDestinationProcess$")
					cmd.WaitDelay = time.Second
					cmd.Env = append(os.Environ(), "GOBETTER_TEST_NONREGULAR_DIR="+dir, "GOBETTER_TEST_NONREGULAR_MODE="+mode, "GOBETTER_TEST_NONREGULAR_PATTERN="+pattern)
					output, err := cmd.CombinedOutput()
					if ctx.Err() != nil {
						t.Fatalf("nonregular destination blocked past deadline: %v\n%s", ctx.Err(), output)
					}
					if err != nil {
						t.Fatalf("refusal probe: %v\n%s", err, output)
					}
					after, err := os.Lstat(path)
					if err != nil || after.Mode() != before.Mode() || !after.ModTime().Equal(before.ModTime()) || after.Size() != before.Size() {
						t.Fatalf("nonregular destination changed: %v", err)
					}
					preserved(t, filepath.Join(dir, name), input)
					preserved(t, filepath.Join(dir, "go.mod"), module)
				})
			}
		}
	}
}

func TestReviewNonregularDestinationProcess(t *testing.T) {
	dir := os.Getenv("GOBETTER_TEST_NONREGULAR_DIR")
	if dir == "" {
		return
	}
	mode := Check
	if os.Getenv("GOBETTER_TEST_NONREGULAR_MODE") == "generate" {
		mode = Generate
	}
	request := source.Request{Dir: dir, File: os.Getenv("GOBETTER_TEST_NONREGULAR_FILE")}
	if pattern := os.Getenv("GOBETTER_TEST_NONREGULAR_PATTERN"); pattern != "" {
		request.Patterns = []string{pattern}
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	report, err := Run(ctx, request, mode)
	expected := os.Getenv("GOBETTER_TEST_NONREGULAR_PATH")
	if expected == "" {
		expected = "model_gob.go"
	}
	message := os.Getenv("GOBETTER_TEST_NONREGULAR_MESSAGE")
	if message == "" {
		message = "non-regular"
	}
	if err == nil || !strings.Contains(err.Error(), "output_ownership") || !strings.Contains(err.Error(), message) || !strings.Contains(err.Error(), expected) || len(report.Written)+len(report.Removed) != 0 {
		t.Fatalf("nonregular destination was not refused before opening: %+v %v", report, err)
	}
}

func TestReviewPreflightSymlinkRootsAndTestHeaders(t *testing.T) {
	mkfifo, err := exec.LookPath("mkfifo")
	if err != nil {
		t.Skip("mkfifo is unavailable")
	}
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, input, fifo, pattern, file string
		link                             bool
	}{
		{"symlink_root_output", "models/model.go", "model_gob.go", "./linked/...", "", true},
		{"symlink_child_output", "models/child/model.go", "model_gob.go", "./linked/...", "", true},
		{"symlink_root_test", "models/model.go", "model_test.go", "./linked/...", "", true},
		{"explicit_symlink_test", "models/model.go", "model_test.go", "./linked", "", true},
		{"direct_test", "model.go", "model_test.go", "", "", false},
		{"recursive_test", "models/model.go", "model_test.go", "./...", "", false},
		{"import_path_test", "models/model.go", "model_test.go", "fixture/models", "", false},
		{"file_mode_test", "model.go", "model_test.go", "", "model.go", false},
	} {
		for _, mode := range []string{"check", "generate"} {
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				dir := fixture(t, map[string]string{tc.input: "package models\n//gobetter:struct builder\ntype Item struct{Value int}\n"})
				input := filepath.Join(dir, tc.input)
				path := filepath.Join(filepath.Dir(input), tc.fifo)
				if output, err := exec.CommandContext(t.Context(), mkfifo, path).CombinedOutput(); err != nil {
					t.Fatalf("mkfifo: %v\n%s", err, output)
				}
				expected := path
				var link os.FileInfo
				linked := filepath.Join(dir, "linked")
				if tc.link {
					if err := os.Symlink(filepath.Join(dir, "models"), linked); err != nil {
						t.Fatal(err)
					}
					link, err = os.Lstat(linked)
					if err != nil {
						t.Fatal(err)
					}
					relative, err := filepath.Rel(filepath.Join(dir, "models"), path)
					if err != nil {
						t.Fatal(err)
					}
					expected = filepath.Join(linked, relative)
				}
				before, err := os.Lstat(path)
				if err != nil {
					t.Fatal(err)
				}
				sourceBefore, module := record(t, input), record(t, filepath.Join(dir, "go.mod"))
				ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, binary, "-test.run=^TestReviewNonregularDestinationProcess$")
				cmd.WaitDelay = time.Second
				cmd.Env = append(os.Environ(), "GOBETTER_TEST_NONREGULAR_DIR="+dir, "GOBETTER_TEST_NONREGULAR_MODE="+mode, "GOBETTER_TEST_NONREGULAR_PATTERN="+tc.pattern, "GOBETTER_TEST_NONREGULAR_FILE="+tc.file, "GOBETTER_TEST_NONREGULAR_PATH="+expected)
				output, err := cmd.CombinedOutput()
				if err != nil || ctx.Err() != nil {
					t.Fatalf("bounded preflight refusal: %v context=%v\n%s", err, ctx.Err(), output)
				}
				after, err := os.Lstat(path)
				if err != nil || after.Mode() != before.Mode() || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) {
					t.Fatalf("FIFO changed: %v", err)
				}
				if tc.link {
					after, err := os.Lstat(linked)
					if err != nil || after.Mode() != link.Mode() || !after.ModTime().Equal(link.ModTime()) {
						t.Fatalf("selected root symlink changed: %v", err)
					}
				}
				preserved(t, input, sourceBefore)
				preserved(t, filepath.Join(dir, "go.mod"), module)
				if path != source.OutputFilename(input) {
					if _, err := os.Stat(source.OutputFilename(input)); !os.IsNotExist(err) {
						t.Fatalf("output published despite nonregular test header: %v", err)
					}
				}
			})
		}
	}
}

func TestReviewPreflightRegularSymlinkRoot(t *testing.T) {
	dir := fixture(t, map[string]string{"models/model.go": "package models\n//gobetter:struct builder\ntype Item struct{Value int}\nvar _=NewItemBuilder().Value(1).Build()\n"})
	linked := filepath.Join(dir, "linked")
	if err := os.Symlink(filepath.Join(dir, "models"), linked); err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(dir, "models/model.go")
	before := record(t, input)
	request := source.Request{Dir: dir, Patterns: []string{"./linked/..."}}
	if report, err := Run(t.Context(), request, Check); err != nil || len(report.Changes) != 1 || report.Changes[0].Kind != "missing" {
		t.Fatalf("valid symlink root check: %+v %v", report, err)
	}
	if report, err := Run(t.Context(), request, Generate); err != nil || len(report.Written) != 1 {
		t.Fatalf("valid symlink root generation: %+v %v", report, err)
	}
	output := source.OutputFilename(input)
	generated := record(t, output)
	if report, err := Run(t.Context(), request, Check); err != nil || len(report.Changes) != 0 {
		t.Fatalf("valid symlink root clean check: %+v %v", report, err)
	}
	preserved(t, input, before)
	preserved(t, output, generated)
	goCommand(t, dir, nil, "build", "-mod=readonly", "./linked/...")
}

func TestReviewPreflightHeaderScope(t *testing.T) {
	t.Run("regular_tests", func(t *testing.T) {
		dir := fixture(t, map[string]string{
			"model.go":      "package models\n//gobetter:struct builder\ntype Item struct{Value int}\n",
			"model_test.go": "package models\n//gobetter:struct builder\ntype TestOnly struct{Value int}\nvar _=MissingTestOnlyIdentifier\n",
		})
		path := filepath.Join(dir, "model_test.go")
		before := record(t, path)
		if report, err := Run(t.Context(), source.Request{Dir: dir}, Generate); err != nil || len(report.Written) != 1 {
			t.Fatalf("test declarations entered generation: %+v %v", report, err)
		}
		if _, err := os.Stat(source.OutputFilename(path)); !os.IsNotExist(err) {
			t.Fatalf("test sidecar published: %v", err)
		}
		if _, err := Run(t.Context(), source.Request{Dir: dir, File: "model_test.go"}, Check); err == nil || !strings.Contains(err.Error(), "test-file input is unsupported") {
			t.Fatalf("explicit test-file input accepted: %v", err)
		}
		preserved(t, path, before)
		goCommand(t, dir, nil, "build", "-mod=readonly", "./...")
	})
	t.Run("ignored_headers_and_descendant_symlink", func(t *testing.T) {
		mkfifo, err := exec.LookPath("mkfifo")
		if err != nil {
			t.Skip("mkfifo is unavailable")
		}
		dir := fixture(t, map[string]string{
			"model.go":                 "package models\n//gobetter:struct builder\ntype Item struct{Value int}\n",
			"testdata/target/model.go": "package ignored\n",
		})
		linked := filepath.Join(dir, "linked")
		if err := os.Symlink(filepath.Join(dir, "testdata/target"), linked); err != nil {
			t.Fatal(err)
		}
		before := make(map[string]os.FileInfo)
		for _, name := range []string{".ignored_test.go", "_ignored_test.go", "testdata/target/model_test.go"} {
			path := filepath.Join(dir, name)
			if output, err := exec.CommandContext(t.Context(), mkfifo, path).CombinedOutput(); err != nil {
				t.Fatalf("mkfifo: %v\n%s", err, output)
			}
			info, err := os.Lstat(path)
			if err != nil {
				t.Fatal(err)
			}
			before[path] = info
		}
		info, err := os.Lstat(linked)
		if err != nil {
			t.Fatal(err)
		}
		before[linked] = info
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		if report, err := Run(ctx, source.Request{Dir: dir, Patterns: []string{"./..."}}, Check); err != nil || len(report.Changes) != 1 || report.Changes[0].Kind != "missing" {
			t.Fatalf("preflight widened header/link scope: %+v %v", report, err)
		}
		for path, info := range before {
			after, err := os.Lstat(path)
			if err != nil || after.Mode() != info.Mode() || after.Size() != info.Size() || !after.ModTime().Equal(info.ModTime()) {
				t.Fatalf("excluded entry changed: %s %v", path, err)
			}
		}
	})
}

func TestReviewNonregularScope(t *testing.T) {
	mkfifo, err := exec.LookPath("mkfifo")
	if err != nil {
		t.Skip("mkfifo is unavailable")
	}
	for _, scope := range []string{"explicit", "recursive"} {
		t.Run(scope, func(t *testing.T) {
			dir := fixture(t, map[string]string{"selected/model.go": "package models\n//gobetter:struct builder\ntype Item struct{Value int}\n"})
			patterns := []string{"./selected"}
			excluded := []string{"unselected"}
			if scope == "recursive" {
				patterns = []string{"./..."}
				excluded = []string{"vendor/dependency", "testdata", ".hidden", "_hidden", "nested"}
				writeInput(t, dir, "nested/go.mod", "module nested\n\ngo 1.26.0\n")
			}
			before := make(map[string]os.FileInfo)
			for _, name := range excluded {
				path := filepath.Join(dir, name, "model_gob.go")
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					t.Fatal(err)
				}
				if data, err := exec.CommandContext(t.Context(), mkfifo, path).CombinedOutput(); err != nil {
					t.Fatalf("mkfifo: %v\n%s", err, data)
				}
				info, err := os.Lstat(path)
				if err != nil {
					t.Fatal(err)
				}
				before[path] = info
			}
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			report, err := Run(ctx, source.Request{Dir: dir, Patterns: patterns}, Check)
			if err != nil || len(report.Changes) != 1 || report.Changes[0].Kind != "missing" {
				t.Fatalf("scope widened to excluded FIFO: %+v %v", report, err)
			}
			for path, info := range before {
				after, err := os.Lstat(path)
				if err != nil || after.Mode() != info.Mode() || !after.ModTime().Equal(info.ModTime()) || after.Size() != info.Size() {
					t.Fatalf("excluded FIFO changed: %s %v", path, err)
				}
			}
		})
	}
	t.Run("go_named_directory", func(t *testing.T) {
		dir := fixture(t, map[string]string{
			"model.go":        "package models\n//gobetter:struct builder\ntype Item struct{Value int}\n",
			"sub.go/model.go": "package sub\n//gobetter:struct builder\ntype Item struct{Value int}\n",
		})
		for _, tc := range []struct {
			patterns []string
			count    int
		}{{nil, 1}, {[]string{"./..."}, 2}} {
			report, err := Run(t.Context(), source.Request{Dir: dir, Patterns: tc.patterns}, Check)
			if err != nil || len(report.Changes) != tc.count {
				t.Fatalf("directory ending in .go treated as a source: %+v %v", report, err)
			}
		}
	})
}

func TestReviewArtifactOnlyCleanup(t *testing.T) {
	for _, selection := range []string{"current", "explicit", "recursive"} {
		t.Run(selection, func(t *testing.T) {
			name := "model.go"
			var patterns []string
			if selection != "current" {
				name = "models/model.go"
				patterns = []string{"./models"}
				if selection == "recursive" {
					patterns = []string{"./..."}
				}
			}
			dir := fixture(t, map[string]string{name: "package models\n//gobetter:struct builder\ntype Item struct{Value int}\nvar _=NewItemBuilder().Value(1).Build()\n"})
			request := source.Request{Dir: dir, Patterns: patterns}
			if _, err := Run(t.Context(), request, Generate); err != nil {
				t.Fatal(err)
			}
			path := source.OutputFilename(filepath.Join(dir, name))
			before, module := record(t, path), record(t, filepath.Join(dir, "go.mod"))
			if err := os.Remove(filepath.Join(dir, name)); err != nil {
				t.Fatal(err)
			}
			report, err := Run(t.Context(), request, Check)
			if err != nil || len(report.Changes) != 1 || report.Changes[0] != (Change{path, "obsolete"}) || len(report.Written)+len(report.Removed) != 0 {
				t.Fatalf("last-source obsolete check: %+v %v", report, err)
			}
			preserved(t, path, before)
			if _, err := Run(t.Context(), source.Request{Dir: dir, File: name}, Generate); err == nil {
				t.Fatal("deleted file became a file-mode cleanup target")
			}
			preserved(t, path, before)
			report, err = Run(t.Context(), request, Generate)
			if err != nil || len(report.Removed) != 1 || report.Removed[0] != path || len(report.Written) != 0 {
				t.Fatalf("last-source obsolete removal: %+v %v", report, err)
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("orphan artifact remains: %v", err)
			}
			preserved(t, filepath.Join(dir, "go.mod"), module)
		})
	}
	t.Run("inactive_source", func(t *testing.T) {
		dir := fixture(t, map[string]string{
			"model.go":     "//go:build custom\n\npackage models\n",
			"model_gob.go": source.GeneratedHeader + "\n" + source.VersionMetadata + Version + "\n" + source.SourceMetadata + "model.go\n\npackage models\nconst Retained = 1\n",
		})
		path := filepath.Join(dir, "model_gob.go")
		before, input := record(t, path), record(t, filepath.Join(dir, "model.go"))
		for _, mode := range []Mode{Check, Generate} {
			report, err := Run(t.Context(), source.Request{Dir: dir}, mode)
			if err != nil || len(report.Changes) != 0 {
				t.Fatalf("inactive source artifact became obsolete: %+v %v", report, err)
			}
			preserved(t, path, before)
			preserved(t, filepath.Join(dir, "model.go"), input)
		}
	})
	t.Run("unrelated_error", func(t *testing.T) {
		dir := fixture(t, map[string]string{
			"obsolete/model.go": "package models\n//gobetter:struct builder\ntype Item struct{Value int}\n",
			"live/model.go":     "package live\nvar _=MissingAuthoredIdentifier\n",
		})
		if _, err := Run(t.Context(), source.Request{Dir: dir, Patterns: []string{"./obsolete"}}, Generate); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, "obsolete/model_gob.go")
		before := record(t, path)
		if err := os.Remove(filepath.Join(dir, "obsolete/model.go")); err != nil {
			t.Fatal(err)
		}
		for _, mode := range []Mode{Check, Generate} {
			report, err := Run(t.Context(), source.Request{Dir: dir, Patterns: []string{"./..."}}, mode)
			if err == nil || !strings.Contains(err.Error(), "MissingAuthoredIdentifier") || len(report.Written)+len(report.Removed) != 0 {
				t.Fatalf("unrelated error was hidden for cleanup: %+v %v", report, err)
			}
			preserved(t, path, before)
		}
	})
}

func TestCgoAndBuildHeaders(t *testing.T) {
	dir := fixture(t, map[string]string{
		"anchor.go": "package models\n",
		"model.go":  "// License\n//go:build custom\n// +build custom\n\npackage models\n/* #include <stdint.h> */\nimport \"C\"\nimport clock \"time\"\n//gobetter:struct builder\ntype Item struct{Value clock.Time}\n",
	})
	request := source.Request{Dir: dir, Tags: []string{"custom"}, CgoEnabled: new(true)}
	if _, err := Run(t.Context(), request, Generate); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "model_gob.go")
	output := record(t, path)
	if !strings.Contains(string(output.data), "//go:build custom && cgo") || !strings.Contains(string(output.data), "// +build custom,cgo") || strings.Contains(string(output.data), `import "C"`) {
		t.Fatalf("cgo constraints/preamble: %s", output.data)
	}
	goCommand(t, dir, []string{"CGO_ENABLED=1"}, "test", "-tags=custom", "./...")
	request.CgoEnabled = new(false)
	if _, err := Run(t.Context(), request, Generate); err != nil {
		t.Fatal(err)
	}
	preserved(t, path, output)
	goCommand(t, dir, []string{"CGO_ENABLED=0"}, "test", "-tags=custom", "./...")
	writeInput(t, dir, "model.go", "package models\n/* #include <stdint.h> */\nimport \"C\"\n//gobetter:struct builder\ntype Item struct{Value C.int}\n")
	request.CgoEnabled = new(true)
	request.Tags = nil
	if _, err := Run(t.Context(), request, Generate); err == nil || !strings.Contains(err.Error(), "unsupported_c_signature") {
		t.Fatalf("C signature accepted: %v", err)
	}
	preserved(t, path, output)
	// Separate ordinary package qualifier from a type parameter with the same
	// spelling (the latter cannot qualify Time in valid Go).
	writeInput(t, dir, "model.go", "package models\nimport C \"time\"\n//gobetter:struct builder\ntype Item struct{At C.Time}\n//gobetter:struct builder\ntype Box[C any] struct{Value C}\n")
	if _, err := Run(t.Context(), request, Generate); err != nil {
		t.Fatal(err)
	}
	goCommand(t, dir, nil, "test", "./...")
}

func TestConfigurationParityAndContexts(t *testing.T) {
	var expected []byte
	for _, annotated := range []bool{true, false} {
		model := "package models\n"
		if annotated {
			model += "//gobetter:struct builder,equal,string\n"
		} else {
			model = "// Code generated by upstream; DO NOT EDIT.\n" + model
		}
		model += "type Item struct {\n"
		for _, field := range []string{"Leading string", "Z int", "Middle string", "A string", "Tail string"} {
			model += field
			if annotated && (strings.HasPrefix(field, "Leading ") || strings.HasPrefix(field, "Middle ") || strings.HasPrefix(field, "Tail ")) {
				model += " //gobetter:field optional"
			}
			model += "\n"
		}
		model += "}\n"
		dir := fixture(t, map[string]string{"model.go": model})
		if !annotated {
			writeInput(t, dir, "gobetter.json", `{"schema_version":1,"types":[{"package":".","type":"Item","derive":["builder","equal","string"],"fields":{"Leading":{"optional":true},"Middle":{"optional":true},"Tail":{"optional":true}}}]}`)
		}
		before := record(t, filepath.Join(dir, "model.go"))
		if _, err := Run(t.Context(), source.Request{Dir: dir}, Generate); err != nil {
			t.Fatal(err)
		}
		preserved(t, filepath.Join(dir, "model.go"), before)
		data := record(t, filepath.Join(dir, "model_gob.go")).data
		if expected != nil && !bytes.Equal(expected, data) {
			t.Fatal("annotation/config output differs")
		}
		expected = data
		writeInput(t, dir, "model_test.go", `package models
import "testing"
func TestWindows(t *testing.T) {
 value:=NewItemBuilder().Leading("l").Z(1).Middle("old").Middle("m").A("a").Tail("t").Build()
 if value.Leading!="l" || value.Middle!="m" || value.Tail!="t" || !value.Equal(value) || value.String()!=`+"`Item{Leading:\"l\", Z:1, Middle:\"m\", A:\"a\", Tail:\"t\"}`"+` {t.Fatal(value)}
 omitted:=NewItemBuilder().Z(1).A("a").BuildPtr()
 if omitted.Leading!="" || omitted.Middle!="" || omitted.Tail!="" {t.Fatal(omitted)}
}
`)
		goCommand(t, dir, nil, "test", "./...")
	}
	dir := fixture(t, map[string]string{"anchor.go": "package models\n", "person_windows.go": "package models\n//gobetter:struct builder\ntype Person struct{Value int}\n"})
	if _, err := Run(t.Context(), source.Request{Dir: dir, GOOS: "windows", GOARCH: "amd64", CgoEnabled: new(false)}, Generate); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "person_gob_windows.go")
	before := record(t, path)
	if _, err := Run(t.Context(), source.Request{Dir: dir}, Generate); err != nil {
		t.Fatal(err)
	}
	preserved(t, path, before)
	goCommand(t, dir, []string{"GOOS=windows", "GOARCH=amd64", "CGO_ENABLED=0"}, "build", "./...")
	writeInput(t, dir, "go.mod", "module fixture\n\ngo 1.26\n")
	if _, err := Run(t.Context(), source.Request{Dir: dir}, Check); err != nil {
		t.Fatal("Go 1.26 language declaration rejected:", err)
	}
}

func TestCombinedPublicationAndPrevalidation(t *testing.T) {
	dir := fixture(t, map[string]string{
		"a/model.go": "package a\nimport . \"fixture/z\"\n//gobetter:struct builder\ntype A struct{Config ZConfig}\nvar _=NewABuilder().Config(NewZConfigBuilder().Value(1).Build()).Build()\n",
		"z/model.go": "package z\n//gobetter:struct builder\ntype Z struct{Config struct{Value int}}\n",
	})
	if _, err := Run(t.Context(), source.Request{Dir: dir, Patterns: []string{"./a", "./z"}}, Generate); err != nil {
		t.Fatal(err)
	}
	goCommand(t, dir, nil, "test", "./...")
	a := record(t, filepath.Join(dir, "a/model_gob.go"))
	z := record(t, filepath.Join(dir, "z/model_gob.go"))
	writeInput(t, dir, "a/invalid.go", "package a\nvar _=UnrelatedSourceError\n")
	if _, err := Run(t.Context(), source.Request{Dir: dir, Patterns: []string{"./a", "./z"}}, Generate); err == nil || !strings.Contains(err.Error(), "UnrelatedSourceError") {
		t.Fatal("unrelated failure hidden:", err)
	}
	preserved(t, filepath.Join(dir, "a/model_gob.go"), a)
	preserved(t, filepath.Join(dir, "z/model_gob.go"), z)
}

func TestCgoOutputBoundaries(t *testing.T) {
	for _, declaration := range []string{
		"type Item struct{Value C.int}",
		"type Item struct{Nested struct{Value C.int}}",
		"type Item[T interface{~C.int}] struct{Value T}",
	} {
		t.Run(declaration, func(t *testing.T) {
			dir := fixture(t, map[string]string{"anchor.go": "package models\n", "model.go": "package models\n/* #include <stdint.h> */\nimport \"C\"\n//gobetter:struct builder\n" + declaration + "\n"})
			if _, err := Run(t.Context(), source.Request{Dir: dir, CgoEnabled: new(true)}, Generate); err == nil || !strings.Contains(err.Error(), "unsupported_c_signature") {
				t.Fatalf("unsupported C output: %v", err)
			}
			if _, err := os.Stat(filepath.Join(dir, "model_gob.go")); !os.IsNotExist(err) {
				t.Fatal("published C signature")
			}
		})
	}
	dir := fixture(t, map[string]string{"model.go": `package models
/* #include <stdint.h> */
import "C"
type GoInt C.int
//gobetter:struct builder
type Item struct{Value GoInt; Other struct{
 //gobetter:field name=Value
 _Ctype_int int
}}
`})
	if _, err := Run(t.Context(), source.Request{Dir: dir, CgoEnabled: new(true)}, Generate); err != nil {
		t.Fatal(err)
	}
	goCommand(t, dir, []string{"CGO_ENABLED=1"}, "test", "./...")
}

func boundedOwnershipProbe(t *testing.T, dir, command, pattern, file, path, message string) {
	t.Helper()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "-test.run=^TestReviewNonregularDestinationProcess$")
	cmd.WaitDelay = time.Second
	cmd.Env = append(os.Environ(), "GOBETTER_TEST_NONREGULAR_DIR="+dir, "GOBETTER_TEST_NONREGULAR_MODE="+command, "GOBETTER_TEST_NONREGULAR_PATTERN="+pattern, "GOBETTER_TEST_NONREGULAR_FILE="+file, "GOBETTER_TEST_NONREGULAR_PATH="+path, "GOBETTER_TEST_NONREGULAR_MESSAGE="+message)
	output, err := cmd.CombinedOutput()
	if err != nil || ctx.Err() != nil {
		t.Fatalf("bounded ownership refusal: %v context=%v\n%s", err, ctx.Err(), output)
	}
}

func mkfifo(t *testing.T, path string) {
	t.Helper()
	command, err := exec.LookPath("mkfifo")
	if err != nil {
		t.Skip("mkfifo is unavailable")
	}
	if output, err := exec.CommandContext(t.Context(), command, path).CombinedOutput(); err != nil {
		t.Fatalf("mkfifo: %v\n%s", err, output)
	}
}

func entryRecord(t *testing.T, path string) os.FileInfo {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info
}

func entryPreserved(t *testing.T, path string, before os.FileInfo) {
	t.Helper()
	after := entryRecord(t, path)
	if before.Mode() != after.Mode() || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		t.Fatalf("changed entry metadata: %s", path)
	}
}

func TestReviewPreflightGoModDirectory(t *testing.T) {
	for _, command := range []string{"check", "generate"} {
		t.Run(command, func(t *testing.T) {
			dir := fixture(t, map[string]string{"child/model.go": "package models\n//gobetter:struct builder\ntype Item struct{Value int}\n"})
			marker := filepath.Join(dir, "child/go.mod")
			if err := os.Mkdir(marker, 0755); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "child/model_test.go")
			mkfifo(t, path)
			fifo, directory := entryRecord(t, path), entryRecord(t, marker)
			input, module := record(t, filepath.Join(dir, "child/model.go")), record(t, filepath.Join(dir, "go.mod"))
			boundedOwnershipProbe(t, dir, command, "./...", "", path, "non-regular")
			entryPreserved(t, path, fifo)
			entryPreserved(t, marker, directory)
			preserved(t, filepath.Join(dir, "child/model.go"), input)
			preserved(t, filepath.Join(dir, "go.mod"), module)
			if _, err := os.Stat(filepath.Join(dir, "child/model_gob.go")); !os.IsNotExist(err) {
				t.Fatalf("output published: %v", err)
			}
		})
	}
	t.Run("regular_child", func(t *testing.T) {
		dir := fixture(t, map[string]string{"child/model.go": "package models\n//gobetter:struct builder\ntype Item struct{Value int}\n"})
		marker := filepath.Join(dir, "child/go.mod")
		if err := os.Mkdir(marker, 0755); err != nil {
			t.Fatal(err)
		}
		before := entryRecord(t, marker)
		if report, err := Run(t.Context(), source.Request{Dir: dir, Patterns: []string{"./..."}}, Generate); err != nil || len(report.Written) != 1 {
			t.Fatalf("go.mod directory pruned valid child: %+v %v", report, err)
		}
		entryPreserved(t, marker, before)
		goCommand(t, dir, nil, "build", "-mod=readonly", "./...")
	})
}

func TestReviewPreflightDirectoryLinks(t *testing.T) {
	for _, name := range []string{"helpers_test.go", "helpers.go"} {
		for _, mode := range []Mode{Check, Generate} {
			t.Run(name+"/"+[]string{"generate", "check"}[mode], func(t *testing.T) {
				dir := fixture(t, map[string]string{"model.go": "package models\n//gobetter:struct builder\ntype Item struct{Value int}\n", "testdata/helpers/value.txt": "preserved directory target\n"})
				path := filepath.Join(dir, name)
				target := filepath.Join(dir, "testdata/helpers")
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
				link, directory := entryRecord(t, path), entryRecord(t, target)
				data := record(t, filepath.Join(target, "value.txt"))
				input, module := record(t, filepath.Join(dir, "model.go")), record(t, filepath.Join(dir, "go.mod"))
				report, err := Run(t.Context(), source.Request{Dir: dir}, mode)
				if err != nil || len(report.Changes) != 1 || report.Changes[0].Kind != "missing" {
					t.Fatalf("directory link rejected: %+v %v", report, err)
				}
				if mode == Check && len(report.Written) != 0 || mode == Generate && len(report.Written) != 1 {
					t.Fatalf("wrong publication: %+v", report)
				}
				entryPreserved(t, path, link)
				entryPreserved(t, target, directory)
				preserved(t, filepath.Join(target, "value.txt"), data)
				preserved(t, filepath.Join(dir, "model.go"), input)
				preserved(t, filepath.Join(dir, "go.mod"), module)
				if actual, err := os.Readlink(path); err != nil || actual != target {
					t.Fatalf("directory link retargeted: %q %v", actual, err)
				}
				if mode == Generate {
					goCommand(t, dir, nil, "build", "-mod=readonly", "./...")
				}
			})
		}
	}
	for _, kind := range []string{"file", "fifo", "output_directory"} {
		for _, command := range []string{"check", "generate"} {
			t.Run(kind+"/"+command, func(t *testing.T) {
				dir := fixture(t, map[string]string{"model.go": "package models\n//gobetter:struct builder\ntype Item struct{Value int}\n"})
				target := filepath.Join(dir, "target.txt")
				if kind == "fifo" {
					mkfifo(t, target)
				} else if kind == "file" {
					writeInput(t, dir, "target.txt", "package models\n")
				} else if err := os.Mkdir(target, 0755); err != nil {
					t.Fatal(err)
				}
				name := "helpers_test.go"
				if kind == "output_directory" {
					name = "model_gob.go"
				}
				path := filepath.Join(dir, name)
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
				before, link := entryRecord(t, target), entryRecord(t, path)
				boundedOwnershipProbe(t, dir, command, "", "", path, "symlink")
				entryPreserved(t, target, before)
				entryPreserved(t, path, link)
			})
		}
	}
}

func TestReviewPreflightIgnoreDirectives(t *testing.T) {
	for _, tc := range []struct {
		name, directives, neighbor string
		ignored                    []string
	}{
		{"root_relative", "ignore ./fixtures\n", "pkg/fixtures/model.go", []string{"fixtures"}},
		{"any_component", "ignore fixtures\n", "pkg/fixtures_extra/model.go", []string{"fixtures", "pkg/fixtures"}},
		{"relative_tree", "ignore ./generated/fixtures\n", "other/generated/fixtures/model.go", []string{"generated/fixtures"}},
		{"any_tree", "ignore generated/fixtures\n", "generated/fixtures_extra/model.go", []string{"generated/fixtures", "other/generated/fixtures"}},
		{"block_and_quoted", "ignore (\n \"./fixtures with space\"\n cache\n)\n", "pkg/cache_extra/model.go", []string{"fixtures with space", "pkg/cache"}},
	} {
		for _, pattern := range []string{"./...", "fixture/..."} {
			label := "local"
			if pattern == "fixture/..." {
				label = "import_path"
			}
			t.Run(tc.name+"/"+label, func(t *testing.T) {
				dir := fixture(t, map[string]string{"selected/model.go": "package models\n//gobetter:struct builder\ntype Item struct{Value int}\n", tc.neighbor: "package neighbor\n//gobetter:struct builder\ntype Other struct{Value int}\n"})
				writeInput(t, dir, "go.mod", "module fixture\n\ngo 1.26.0\n\n"+tc.directives)
				inputs := make(map[string]fileRecord)
				entries := make(map[string]os.FileInfo)
				for _, name := range tc.ignored {
					input := filepath.Join(dir, name, "model.go")
					writeInput(t, dir, filepath.Join(name, "model.go"), "package ignored\n//gobetter:struct builder\ntype Ignored struct{Value int}\n")
					inputs[input] = record(t, input)
					path := filepath.Join(dir, name, "model_test.go")
					mkfifo(t, path)
					entries[path] = entryRecord(t, path)
				}
				for _, name := range []string{"go.mod", "selected/model.go", tc.neighbor} {
					path := filepath.Join(dir, name)
					inputs[path] = record(t, path)
				}
				request := source.Request{Dir: dir, Patterns: []string{pattern}}
				for _, mode := range []Mode{Check, Generate} {
					ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
					report, err := Run(ctx, request, mode)
					cancel()
					if err != nil || len(report.Changes) != 2 {
						t.Fatalf("ignore traversal disagrees with Go: %+v %v", report, err)
					}
					if mode == Check && len(report.Written) != 0 || mode == Generate && len(report.Written) != 2 {
						t.Fatalf("wrong ignore-scope publication: %+v", report)
					}
					for path, before := range inputs {
						preserved(t, path, before)
					}
					for path, before := range entries {
						entryPreserved(t, path, before)
					}
					for _, name := range tc.ignored {
						if _, err := os.Stat(filepath.Join(dir, name, "model_gob.go")); !os.IsNotExist(err) {
							t.Fatalf("ignored sidecar published: %v", err)
						}
					}
				}
				goCommand(t, dir, nil, "build", "-mod=readonly", pattern)
			})
		}
	}
	t.Run("explicit_selection", func(t *testing.T) {
		dir := fixture(t, map[string]string{"fixtures/model.go": "package models\n//gobetter:struct builder\ntype Item struct{Value int}\n"})
		writeInput(t, dir, "go.mod", "module fixture\n\ngo 1.26.0\nignore ./fixtures\n")
		path := filepath.Join(dir, "fixtures/model_test.go")
		mkfifo(t, path)
		before := entryRecord(t, path)
		for _, command := range []string{"check", "generate"} {
			for _, pattern := range []string{"./fixtures", "fixture/fixtures"} {
				boundedOwnershipProbe(t, dir, command, pattern, "", path, "non-regular")
			}
			boundedOwnershipProbe(t, dir, command, "", "fixtures/model.go", path, "non-regular")
		}
		entryPreserved(t, path, before)
	})
	t.Run("recursive_ignored_root", func(t *testing.T) {
		dir := fixture(t, map[string]string{"fixtures/model.go": "package models\n"})
		writeInput(t, dir, "go.mod", "module fixture\n\ngo 1.26.0\nignore ./fixtures\n")
		path := filepath.Join(dir, "fixtures/model_test.go")
		mkfifo(t, path)
		before := entryRecord(t, path)
		for _, mode := range []Mode{Check, Generate} {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			report, err := Run(ctx, source.Request{Dir: dir, Patterns: []string{"./fixtures/..."}}, mode)
			cancel()
			if err == nil || strings.Contains(err.Error(), "output_ownership") || errors.Is(err, context.DeadlineExceeded) || len(report.Written)+len(report.Removed) != 0 {
				t.Fatalf("ignored recursive root was inspected: %+v %v", report, err)
			}
			entryPreserved(t, path, before)
		}
	})
}

func TestReviewPackageLocalGoModDirectories(t *testing.T) {
	for _, name := range []string{"child", "child/inner"} {
		t.Run(name, func(t *testing.T) {
			dir := fixture(t, map[string]string{filepath.Join(name, "model.go"): "package models\n//gobetter:struct builder\ntype Item struct{Value int}\nvar _=NewItemBuilder().Value(1).Build()\n"})
			entries := make(map[string]os.FileInfo)
			for _, directory := range []string{"child", name} {
				path := filepath.Join(dir, directory, "go.mod")
				if err := os.MkdirAll(path, 0755); err != nil {
					t.Fatal(err)
				}
				entries[path] = entryRecord(t, path)
			}
			local := filepath.Join(dir, name)
			input := filepath.Join(local, "model.go")
			module := record(t, filepath.Join(dir, "go.mod"))
			before := record(t, input)
			request := source.Request{Dir: local, Patterns: []string{"."}}
			report, err := Run(t.Context(), request, Check)
			if err != nil || len(report.Changes) != 1 || report.Changes[0].Kind != "missing" || len(report.Written)+len(report.Removed) != 0 {
				t.Fatalf("package-local check missed parent module: %+v %v", report, err)
			}
			path := source.OutputFilename(input)
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("check published output: %v", err)
			}
			report, err = Run(t.Context(), request, Generate)
			if err != nil || len(report.Written) != 1 || report.Written[0] != path {
				t.Fatalf("package-local generation missed parent module: %+v %v", report, err)
			}
			generated := record(t, path)
			for _, mode := range []Mode{Check, Generate} {
				report, err := Run(t.Context(), request, mode)
				if err != nil || len(report.Changes) != 0 {
					t.Fatalf("package-local repeat: %+v %v", report, err)
				}
				preserved(t, path, generated)
			}
			preserved(t, input, before)
			preserved(t, filepath.Join(dir, "go.mod"), module)
			for path, info := range entries {
				entryPreserved(t, path, info)
			}
			goCommand(t, local, nil, "build", "-mod=readonly", ".")
		})
	}
	t.Run("parent_floor_stays_strict", func(t *testing.T) {
		dir := fixture(t, map[string]string{"child/model.go": "package models\n//gobetter:struct builder\ntype Item struct{Value int}\n"})
		writeInput(t, dir, "go.mod", "module fixture\n\ngo 1.25.0\n")
		marker := filepath.Join(dir, "child/go.mod")
		if err := os.Mkdir(marker, 0755); err != nil {
			t.Fatal(err)
		}
		before := entryRecord(t, marker)
		module := record(t, filepath.Join(dir, "go.mod"))
		for _, mode := range []Mode{Check, Generate} {
			report, err := Run(t.Context(), source.Request{Dir: filepath.Join(dir, "child"), Patterns: []string{"."}}, mode)
			if err == nil || !strings.Contains(err.Error(), "consumer_go_version") || !strings.Contains(err.Error(), filepath.Join(dir, "go.mod")) || len(report.Written)+len(report.Removed) != 0 {
				t.Fatalf("wrong parent-floor diagnostic: %+v %v", report, err)
			}
		}
		entryPreserved(t, marker, before)
		preserved(t, filepath.Join(dir, "go.mod"), module)
	})
}

func TestReviewPreflightModuleRootIgnore(t *testing.T) {
	for _, directive := range []struct{ name, pattern string }{{"dot", "."}, {"slash", "./"}} {
		t.Run(directive.name, func(t *testing.T) {
			dir := fixture(t, map[string]string{"model.go": "package models\n//gobetter:struct builder\ntype Item struct{Value int}\n"})
			writeInput(t, dir, "go.mod", "module fixture\n\ngo 1.26.0\nignore "+directive.pattern+"\n")
			path := filepath.Join(dir, "model_test.go")
			mkfifo(t, path)
			before := entryRecord(t, path)
			module, input := record(t, filepath.Join(dir, "go.mod")), record(t, filepath.Join(dir, "model.go"))
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, "go", "list", "-e", "-mod=readonly", "./...")
			cmd.Dir = dir
			data, err := cmd.CombinedOutput()
			if err != nil || ctx.Err() != nil || !strings.Contains(string(data), "matched no packages") {
				t.Fatalf("pinned Go root-ignore control: %v context=%v\n%s", err, ctx.Err(), data)
			}
			for _, mode := range []Mode{Check, Generate} {
				ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
				report, err := Run(ctx, source.Request{Dir: dir, Patterns: []string{"./..."}}, mode)
				expired := ctx.Err()
				cancel()
				if err == nil || expired != nil || strings.Contains(err.Error(), "output_ownership") || errors.Is(err, context.DeadlineExceeded) || len(report.Written)+len(report.Removed) != 0 {
					t.Fatalf("ignored module root was inspected: %+v %v context=%v", report, err, expired)
				}
				entryPreserved(t, path, before)
				preserved(t, filepath.Join(dir, "go.mod"), module)
				preserved(t, filepath.Join(dir, "model.go"), input)
			}
			for _, command := range []string{"check", "generate"} {
				boundedOwnershipProbe(t, dir, command, ".", "", path, "non-regular")
				boundedOwnershipProbe(t, dir, command, "fixture", "", path, "non-regular")
				boundedOwnershipProbe(t, dir, command, "", "model.go", path, "non-regular")
			}
			// Go's import-path matcher does not apply ignores to the module
			// root itself, unlike the local recursive-directory matcher.
			for _, command := range []string{"check", "generate"} {
				boundedOwnershipProbe(t, dir, command, "fixture/...", "", path, "non-regular")
			}
			entryPreserved(t, path, before)
			if _, err := os.Stat(filepath.Join(dir, "model_gob.go")); !os.IsNotExist(err) {
				t.Fatalf("root-ignore probe published output: %v", err)
			}
		})
	}
}
