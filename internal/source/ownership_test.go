package source_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mobiletoly/gobetter/internal/source"
)

func TestOwnershipAndModuleBoundaries(t *testing.T) {
	dir := module(t, map[string]string{"model.go": "package models\n//gobetter:struct builder\ntype Item struct {Value int}\n"})
	path := filepath.Join(dir, "model_gob.go")
	write(t, dir, "model_gob.go", "package models\n// handwritten file\n")
	if _, err := source.Load(context.Background(), source.Request{Dir: dir}); err == nil || !strings.Contains(err.Error(), "non-owned") {
		t.Fatalf("non-owned output accepted: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "model.go"), path); err != nil {
		t.Fatal(err)
	}
	if _, err := source.Load(context.Background(), source.Request{Dir: dir}); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("output symlink followed: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	write(t, dir, "nested/go.mod", "module nested\n\ngo 1.26.0\n")
	write(t, dir, "nested/model.go", "package nested\ntype Item struct{}\n")
	if _, err := source.Load(context.Background(), source.Request{Dir: dir, Patterns: []string{"./nested"}}); err == nil {
		t.Fatal("nested module became an input-module target")
	}
	write(t, dir, "model_gob.go", source.GeneratedHeader+"\npackage models\n")
	if _, err := source.Load(context.Background(), source.Request{Dir: dir}); err == nil || !strings.Contains(err.Error(), "legacy") {
		t.Fatalf("legacy output was silently accepted: %v", err)
	}
}

func TestInactiveArtifactIsPreserved(t *testing.T) {
	dir := module(t, map[string]string{
		"anchor.go":       "package models\n//gobetter:struct builder\ntype Item struct {Value int}\nvar _=Retained()\n",
		"inactive.go":     "//go:build custom\n\npackage models\ntype Inactive struct{}\n",
		"inactive_gob.go": source.GeneratedHeader + "\n" + source.VersionMetadata + "0.20.0\n" + source.SourceMetadata + "inactive.go\npackage models\nfunc Retained() int {return 1}\n",
	})
	r, err := source.Load(context.Background(), source.Request{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Bootstrap) != 1 {
		t.Fatal("inactive source artifact was included in emission scope")
	}
}

func TestReviewOwnershipMetadataAfterPackageClause(t *testing.T) {
	handwritten := "package\tmodels\nconst quoted = `\n" + source.GeneratedHeader + "\n" + source.VersionMetadata + "0.20.0\n" + source.SourceMetadata + "model.go\n`\nfunc Handwritten() int { return 1 }\n"
	dir := module(t, map[string]string{"anchor.go": "package models\n//gobetter:struct builder\ntype Item struct {Value int}\nvar _=Handwritten()\n", "model_gob.go": handwritten + "var _=MissingHandwrittenError\n"})
	_, err := source.Load(context.Background(), source.Request{Dir: dir})
	if err == nil || !strings.Contains(err.Error(), "MissingHandwrittenError") {
		t.Fatalf("handwritten error was masked: %v", err)
	}
	write(t, dir, "model_gob.go", handwritten)
	if _, err := source.Load(context.Background(), source.Request{Dir: dir}); err != nil {
		t.Fatalf("handwritten declaration disappeared: %v", err)
	}
}
