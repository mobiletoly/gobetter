package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestProcessExitCodes(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "gobetter")
	build := exec.CommandContext(t.Context(), "go", "build", "-o", binary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, output)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module fixture\n\ngo 1.26.0\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "model.go"), []byte("package models\n//gobetter:struct builder\ntype Item struct{Value int}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	assertExit := func(args []string, expected int, message string) {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), binary, args...)
		cmd.Dir = dir
		data, err := cmd.CombinedOutput()
		code := 0
		if err != nil {
			exit, ok := err.(*exec.ExitError)
			if !ok {
				t.Fatal(err)
			}
			code = exit.ExitCode()
		}
		if code != expected || !strings.Contains(string(data), message) {
			t.Fatalf("%v: code %d %s", args, code, data)
		}
	}
	assertExit([]string{"version"}, 0, "gobetter version 0.20.0")
	assertExit([]string{"check"}, 1, "missing owned output")
	assertExit([]string{"generate", "-input=model.go"}, 2, "flag provided but not defined")
	assertExit([]string{"generate"}, 0, "generated:")
	assertExit([]string{"check"}, 0, "")
	path := filepath.Join(dir, "model_gob.go")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "model.go")); err != nil {
		t.Fatal(err)
	}
	assertExit([]string{"check"}, 1, "obsolete owned output")
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	afterInfo, err := os.Stat(path)
	if err != nil || !bytes.Equal(before, after) || !info.ModTime().Equal(afterInfo.ModTime()) {
		t.Fatalf("check changed orphan bytes/mtime: %v", err)
	}
	assertExit([]string{"generate"}, 0, "removed:")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("orphan output remains: %v", err)
	}
}
