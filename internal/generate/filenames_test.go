package generate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mobiletoly/gobetter/internal/source"
	"github.com/mobiletoly/gobetter/internal/structgen"
)

func TestShortGeneratedFilenameLifecycle(t *testing.T) {
	for _, tc := range []struct {
		name, input, output, goos, goarch string
	}{
		{"native", "model.go", "model_gob.go", "", ""},
		{"os", "model_linux.go", "model_gob_linux.go", "linux", ""},
		{"os_arch", "model_linux_amd64.go", "model_gob_linux_amd64.go", "linux", "amd64"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := fixture(t, map[string]string{tc.input: "package models\n//gobetter:struct builder\ntype Item struct {Value int}\nvar _=NewItemBuilder().Value(1).Build()\n"})
			input := filepath.Join(dir, tc.input)
			before := record(t, input)
			path := filepath.Join(dir, tc.output)
			request := source.Request{Dir: dir, GOOS: tc.goos, GOARCH: tc.goarch, CgoEnabled: new(false)}
			report, err := Run(t.Context(), request, Check)
			if err != nil || len(report.Changes) != 1 || report.Changes[0].Path != path || report.Changes[0].Kind != "missing" {
				t.Fatalf("missing short output: %+v %v", report, err)
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatal("check published output")
			}
			report, err = Run(t.Context(), request, Generate)
			if err != nil || len(report.Written) != 1 || report.Written[0] != path {
				t.Fatalf("short generation: %+v %v", report, err)
			}
			generated := record(t, path)
			if _, owned, err := source.InspectArtifact(path, generated.data); err != nil || !owned {
				t.Fatalf("modern short output not owned: %t %v", owned, err)
			}
			for _, mode := range []Mode{Check, Generate} {
				report, err := Run(t.Context(), request, mode)
				if err != nil || len(report.Changes) != 0 || len(report.Written)+len(report.Removed) != 0 {
					t.Fatalf("modern output treated as legacy: %+v %v", report, err)
				}
				preserved(t, path, generated)
			}
			preserved(t, input, before)
			var env []string
			if tc.goos != "" {
				env = append(env, "GOOS="+tc.goos)
			}
			if tc.goarch != "" {
				env = append(env, "GOARCH="+tc.goarch)
			}
			env = append(env, "CGO_ENABLED=0")
			goCommand(t, dir, env, "build", "./...")
		})
	}
}

func TestGeneratedFilenameMigrationRefusals(t *testing.T) {
	for _, tc := range []struct {
		name, input, output, prefix, diagnostic string
		modern                                  bool
	}{
		{"legacy_native", "model.go", "model_gob.go", "", "legacy output format requires explicit removal", false},
		{"legacy_os_old_layout", "model_linux.go", "model_linux_gob.go", "", "legacy output format requires explicit removal", false},
		{"legacy_os_short_layout", "model_linux.go", "model_gob_linux.go", "", "legacy output format requires explicit removal", false},
		{"legacy_os_tagged", "model_linux.go", "model_gob_linux.go", "//go:build linux\n\n", "legacy output format requires explicit removal", false},
		{"legacy_arch_old_layout", "model_amd64.go", "model_amd64_gob.go", "", "legacy output format requires explicit removal", false},
		{"legacy_arch_short_layout", "model_amd64.go", "model_gob_amd64.go", "", "legacy output format requires explicit removal", false},
		{"legacy_os_arch_old_layout", "model_linux_amd64.go", "model_linux_amd64_gob.go", "", "legacy output format requires explicit removal", false},
		{"legacy_os_arch_short_layout", "model_linux_amd64.go", "model_gob_linux_amd64.go", "", "legacy output format requires explicit removal", false},
		{"old_long_name", "model.go", "model_gobetter.go", "", "model_gob.go", true},
		{"old_long_platform_name", "model_linux.go", "model_gobetter_linux.go", "", "model_gob_linux.go", true},
	} {
		for _, mode := range []Mode{Check, Generate} {
			t.Run(tc.name+"/"+[]string{"generate", "check"}[mode], func(t *testing.T) {
				header := tc.prefix + source.GeneratedHeader + "\n"
				if tc.modern {
					header += source.VersionMetadata + Version + "\n" + source.SourceMetadata + tc.input + "\n"
				} else {
					header += "// gobetter:signature=old\n"
				}
				dir := fixture(t, map[string]string{tc.input: "package models\n//gobetter:struct builder\ntype Item struct {Value int}\n", tc.output: header + "\npackage models\nconst Existing = 1\n"})
				path := filepath.Join(dir, tc.output)
				before := record(t, path)
				input := record(t, filepath.Join(dir, tc.input))
				request := source.Request{Dir: dir, File: tc.input, GOOS: "linux", CgoEnabled: new(false)}
				if strings.Contains(tc.input, "_amd64.go") {
					request.GOARCH = "amd64"
				}
				report, err := Run(t.Context(), request, mode)
				if err == nil || !strings.Contains(err.Error(), tc.diagnostic) || len(report.Written)+len(report.Removed) != 0 {
					t.Fatalf("unsafe migration: %+v %v", report, err)
				}
				preserved(t, path, before)
				preserved(t, filepath.Join(dir, tc.input), input)
				expected := source.OutputFilename(filepath.Join(dir, tc.input))
				if expected != path {
					if _, err := os.Stat(expected); !os.IsNotExist(err) {
						t.Fatal("migration failure published a new output")
					}
				}
			})
		}
	}
}

func TestLegacyFilenameOutsideFileScope(t *testing.T) {
	dir := fixture(t, map[string]string{
		"model.go":           "package models\n//gobetter:struct builder\ntype Item struct {Value int}\n",
		"other_linux.go":     "package models\ntype Other struct{}\n",
		"other_gob_linux.go": source.GeneratedHeader + "\n// gobetter:signature=historical\n\npackage models\nconst Legacy = 1\n",
	})
	path := filepath.Join(dir, "other_gob_linux.go")
	before := record(t, path)
	request := source.Request{Dir: dir, File: "model.go", GOOS: "linux", CgoEnabled: new(false)}
	if _, err := Run(t.Context(), request, Generate); err != nil {
		t.Fatal(err)
	}
	preserved(t, path, before)
	if _, err := Run(t.Context(), source.Request{Dir: dir, GOOS: "linux", CgoEnabled: new(false)}, Check); err == nil || !strings.Contains(err.Error(), "legacy output format requires explicit removal") {
		t.Fatalf("package mode failed to diagnose retained legacy output: %v", err)
	}
	preserved(t, path, before)
}

func TestReviewInactiveLegacyBuildContext(t *testing.T) {
	for _, layout := range []struct{ name, filename, header string }{
		{"platform", "model_gob_linux.go", ""},
		{"tagged", "model_gob_linux.go", "//go:build linux\n\n"},
		{"old_layout_tagged", "model_linux_gob.go", "//go:build linux\n\n"},
		{"active_artifact_inactive_source", "model_linux_gob.go", ""},
	} {
		for _, goos := range []string{"darwin", "linux"} {
			for _, mode := range []Mode{Check, Generate} {
				t.Run(layout.name+"/"+goos+"/"+[]string{"generate", "check"}[mode], func(t *testing.T) {
					dir := fixture(t, map[string]string{
						"anchor.go":      "package models\n//gobetter:struct builder\ntype Anchor struct {Value int}\nvar _=NewAnchorBuilder().Value(1).Build()\n",
						"model_linux.go": "package models\ntype LinuxModel struct{Value int}\n",
						layout.filename:  layout.header + source.GeneratedHeader + "\n// gobetter:signature=historical\n\npackage models\ntype Retained struct{Value int}\n",
					})
					paths := []string{"go.mod", "anchor.go", "model_linux.go", layout.filename}
					before := make(map[string]fileRecord)
					for _, name := range paths {
						before[name] = record(t, filepath.Join(dir, name))
					}
					request := source.Request{Dir: dir, GOOS: goos, GOARCH: "arm64", CgoEnabled: new(false)}
					request.Invocation.Selection = structgen.All
					report, err := Run(t.Context(), request, mode)
					if goos == "linux" {
						if err == nil || !strings.Contains(err.Error(), "legacy output format requires explicit removal") || len(report.Written)+len(report.Removed) != 0 {
							t.Fatalf("active legacy output accepted: %+v %v", report, err)
						}
						if _, err := os.Stat(filepath.Join(dir, "anchor_gob.go")); !os.IsNotExist(err) {
							t.Fatal("refusal published unrelated output")
						}
					} else {
						if err != nil || len(report.Changes) != 1 || report.Changes[0].Path != filepath.Join(dir, "anchor_gob.go") || len(report.Removed) != 0 {
							t.Fatalf("inactive legacy output blocked another context: %+v %v", report, err)
						}
						if mode == Check {
							if len(report.Written) != 0 {
								t.Fatal("check published output")
							}
							if _, err := os.Stat(filepath.Join(dir, "anchor_gob.go")); !os.IsNotExist(err) {
								t.Fatal("check created output")
							}
							if _, err := Run(t.Context(), request, Generate); err != nil {
								t.Fatal(err)
							}
						}
						r, err := source.Load(t.Context(), request)
						if err != nil {
							t.Fatal(err)
						}
						for _, p := range r.Plans {
							if p.Target.SourceFile != filepath.Join(dir, "anchor.go") {
								t.Fatalf("legacy declarations became targets: %+v", p.Target)
							}
						}
						for _, a := range r.Packages[0].Artifacts {
							if a.Path == filepath.Join(dir, layout.filename) {
								t.Fatal("legacy output gained cleanup ownership")
							}
						}
						output := record(t, filepath.Join(dir, "anchor_gob.go"))
						for _, repeat := range []Mode{Check, Generate} {
							clean, err := Run(t.Context(), request, repeat)
							if err != nil || len(clean.Changes)+len(clean.Written)+len(clean.Removed) != 0 {
								t.Fatalf("repeat scope drift: %+v %v", clean, err)
							}
							preserved(t, filepath.Join(dir, "anchor_gob.go"), output)
						}
						goCommand(t, dir, []string{"GOOS=darwin", "GOARCH=arm64", "CGO_ENABLED=0"}, "build", "./...")
					}
					for name, original := range before {
						preserved(t, filepath.Join(dir, name), original)
					}
				})
			}
		}
	}
}

func TestReviewHistoricalLegacyFileScope(t *testing.T) {
	for _, layout := range []struct{ name, input, output, header string }{
		{"native", "other.go", "other_gob.go", ""},
		{"platform", "other_linux.go", "other_gob_linux.go", ""},
		{"old_layout", "other_linux.go", "other_linux_gob.go", ""},
		{"tagged", "other_linux.go", "other_gob_linux.go", "//go:build linux\n\n"},
	} {
		t.Run(layout.name, func(t *testing.T) {
			dir := fixture(t, map[string]string{
				"model.go":    "package models\n//gobetter:struct builder\ntype Item struct {Value LegacyValue}\nvar _=NewItemBuilder().Value(LegacyValue(1)).Build()\nvar _=NewOtherBuilder().Value\n",
				layout.input:  "package models\ntype Other struct {Value int}\n",
				layout.output: layout.header + source.GeneratedHeader + "\n// gobetter:signature=historical\n\npackage models\nimport clock \"time\"\ntype LegacyValue int\nfunc NewOtherBuilder() *Other { return &Other{Value:int(clock.Second)} }\n",
			})
			before := make(map[string]fileRecord)
			for _, name := range []string{"go.mod", "model.go", layout.input, layout.output} {
				before[name] = record(t, filepath.Join(dir, name))
			}
			request := source.Request{Dir: dir, File: "model.go", GOOS: "linux", GOARCH: "arm64", CgoEnabled: new(false)}
			request.Invocation.Selection = structgen.All
			expected := filepath.Join(dir, "model_gob.go")
			report, err := Run(t.Context(), request, Check)
			if err != nil || len(report.Changes) != 1 || report.Changes[0] != (Change{expected, "missing"}) || len(report.Written)+len(report.Removed) != 0 {
				t.Fatalf("historical file-scope check: %+v %v", report, err)
			}
			if _, err := os.Stat(expected); !os.IsNotExist(err) {
				t.Fatal("check created output")
			}
			for name, original := range before {
				preserved(t, filepath.Join(dir, name), original)
			}
			report, err = Run(t.Context(), request, Generate)
			if err != nil || len(report.Changes) != 1 || report.Changes[0] != (Change{expected, "missing"}) || len(report.Written) != 1 || report.Written[0] != expected || len(report.Removed) != 0 {
				t.Fatalf("historical file-scope generation: %+v %v", report, err)
			}
			r, err := source.Load(t.Context(), request)
			if err != nil {
				t.Fatal(err)
			}
			for _, plan := range r.Plans {
				if plan.Target.SourceFile != filepath.Join(dir, "model.go") {
					t.Fatalf("legacy declaration selected: %+v", plan.Target)
				}
			}
			for _, artifact := range r.Packages[0].Artifacts {
				if artifact.Path == filepath.Join(dir, layout.output) {
					t.Fatal("legacy bytes gained cleanup ownership")
				}
			}
			generated := record(t, expected)
			for _, mode := range []Mode{Check, Generate} {
				clean, err := Run(t.Context(), request, mode)
				if err != nil || len(clean.Changes)+len(clean.Written)+len(clean.Removed) != 0 {
					t.Fatalf("repeat file mode: %+v %v", clean, err)
				}
				preserved(t, expected, generated)
			}
			goCommand(t, dir, []string{"GOOS=linux", "GOARCH=arm64", "CGO_ENABLED=0"}, "build", "./...")
			for _, mode := range []Mode{Check, Generate} {
				refusal, err := Run(t.Context(), source.Request{Dir: dir, GOOS: "linux", GOARCH: "arm64", CgoEnabled: new(false)}, mode)
				if err == nil || !strings.Contains(err.Error(), "legacy output format requires explicit removal") || len(refusal.Written)+len(refusal.Removed) != 0 {
					t.Fatalf("package mode failed to refuse active legacy format: %+v %v", refusal, err)
				}
				for name, original := range before {
					preserved(t, filepath.Join(dir, name), original)
				}
				preserved(t, expected, generated)
			}
		})
	}
}

func TestReviewLegacyContextErrorsAndCollisions(t *testing.T) {
	for _, tc := range []struct{ name, model, legacy, diagnostic string }{
		{"type_error", "builder", "var _=MissingLegacySymbol\n", "MissingLegacySymbol"},
		{"syntax_error", "builder", "//line renamed.go:200\nfunc Broken( {\n", "source_syntax"},
		{"constructor", "builder", "func NewItemBuilder() int {return 1}\n", "symbol_collision"},
		{"method", "equal", "func (Item) Equal(Item) bool {return true}\n", "symbol_collision"},
		{"import", "builder", "import gobetter_Item_done \"time\"\nvar _=gobetter_Item_done.Second\n", "symbol_collision"},
	} {
		for _, mode := range []Mode{Check, Generate} {
			t.Run(tc.name+"/"+[]string{"generate", "check"}[mode], func(t *testing.T) {
				dir := fixture(t, map[string]string{
					"model.go":     "package models\n//gobetter:struct " + tc.model + "\ntype Item struct {Value int}\n",
					"other.go":     "package models\ntype Other struct{}\n",
					"other_gob.go": source.GeneratedHeader + "\n// gobetter:signature=historical\n\npackage models\n" + tc.legacy,
				})
				path := filepath.Join(dir, "other_gob.go")
				before := record(t, path)
				request := source.Request{Dir: dir, File: "model.go", CgoEnabled: new(false)}
				report, err := Run(t.Context(), request, mode)
				if err == nil || !strings.Contains(err.Error(), tc.diagnostic) || strings.Contains(err.Error(), "directive_syntax") || len(report.Written)+len(report.Removed) != 0 {
					t.Fatalf("retained Go context mishandled: %+v %v", report, err)
				}
				if tc.diagnostic == "symbol_collision" && !strings.Contains(err.Error(), filepath.Join(dir, "model.go")+":3:6") {
					t.Fatalf("collision lost originating request: %v", err)
				}
				if tc.name == "syntax_error" && (!strings.Contains(err.Error(), path+":6:") || strings.Contains(err.Error(), "renamed.go:")) {
					t.Fatalf("syntax error lost physical position: %v", err)
				}
				preserved(t, path, before)
				if _, err := os.Stat(filepath.Join(dir, "model_gob.go")); !os.IsNotExist(err) {
					t.Fatal("context failure published output")
				}
			})
		}
	}
}
