package source_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/mobiletoly/gobetter/internal/source"
)

func TestOutputFilenameShortSuffix(t *testing.T) {
	for input, want := range map[string]string{
		"person.go":               "person_gob.go",
		"person_linux.go":         "person_gob_linux.go",
		"person_amd64.go":         "person_gob_amd64.go",
		"person_linux_amd64.go":   "person_gob_linux_amd64.go",
		"person_windows_arm64.go": "person_gob_windows_arm64.go",
		"my_person.go":            "my_person_gob.go",
		"person_custom.go":        "person_custom_gob.go",
		"linux.go":                "linux_gob.go",
		"models/person_darwin.go": "models/person_gob_darwin.go",
	} {
		t.Run(input, func(t *testing.T) {
			if got := source.OutputFilename(filepath.FromSlash(input)); got != filepath.FromSlash(want) {
				t.Fatalf("output = %q, want %q", got, want)
			}
		})
	}
}

func TestShortArtifactOwnership(t *testing.T) {
	dir := t.TempDir()
	modern := source.GeneratedHeader + "\n" + source.VersionMetadata + "0.20.0\n" + source.SourceMetadata + "model_linux.go\n\npackage models\n"
	for _, tc := range []struct {
		name, filename, data string
		owned                bool
		errorContains        string
	}{
		{"modern", "model_gob_linux.go", modern, true, ""},
		{"tagged_modern", "model_gob_linux.go", "//go:build linux\n\n" + modern, true, ""},
		{"legacy", "model_linux_gob.go", source.GeneratedHeader + "\n// gobetter:signature=old\n\npackage models\n", false, ""},
		{"handwritten", "model_gob_linux.go", "package models\n", false, ""},
		{"old_long_name", "model_gobetter_linux.go", modern, false, "model_gob_linux.go"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(dir, tc.filename)
			artifact, owned, err := source.InspectArtifact(path, []byte(tc.data))
			if owned != tc.owned {
				t.Fatalf("owned = %t, want %t; %v", owned, tc.owned, err)
			}
			if tc.errorContains != "" {
				if err == nil || !strings.Contains(err.Error(), tc.errorContains) {
					t.Fatalf("missing expected filename diagnostic: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if owned && (artifact.Path != path || artifact.Source != filepath.Join(dir, "model_linux.go") || artifact.Version != "0.20.0") {
				t.Fatalf("metadata changed: %+v", artifact)
			}
		})
	}
}
