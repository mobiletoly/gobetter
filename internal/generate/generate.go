// Copyright 2025 Toly Pochkin
// SPDX-License-Identifier: Apache-2.0

// Package generate owns final assembly, validation, comparison and publication.
package generate

import (
	"bytes"
	"context"
	"fmt"
	"go/token"
	"go/version"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/mobiletoly/gobetter/internal/source"
	"github.com/mobiletoly/gobetter/internal/structgen"
	"golang.org/x/mod/modfile"
)

// Version identifies the CLI and owned runtime artifacts.
const Version = "0.20.0"

// Mode selects artifact publication or comparison only.
type Mode uint8

// Generate publishes validated artifacts; Check reports drift without publication.
const (
	Generate Mode = iota
	Check
)

// Change identifies a missing, changed, or obsolete owned artifact.
type Change struct{ Path, Kind string }

// Report records artifact differences and successfully published changes.
type Report struct {
	Changes          []Change
	Written, Removed []string
}

// Run validates the complete final set before comparison or any publication.
func Run(ctx context.Context, request source.Request, mode Mode) (Report, error) {
	return run(ctx, request, mode, atomicReplace)
}

func run(ctx context.Context, request source.Request, mode Mode, publish func(string, []byte) error) (Report, error) {
	var report Report
	if mode != Generate && mode != Check {
		return report, fmt.Errorf("invalid generation mode")
	}
	if err := consumerFloor(request.Dir); err != nil {
		return report, err
	}
	r, err := source.Load(ctx, request)
	if err != nil {
		return report, err
	}
	final, err := assemble(r)
	if err != nil {
		return report, err
	}
	if err := r.Validate(ctx, final); err != nil {
		return report, err
	}
	var paths []string
	for path := range final {
		paths = append(paths, path)
	}
	slices.Sort(paths)
	before := make(map[string][]byte)
	for _, path := range paths {
		data, exists, err := readOwned(path)
		if err != nil {
			return report, err
		}
		before[path] = data
		if !exists {
			report.Changes = append(report.Changes, Change{path, "missing"})
		} else if !bytes.Equal(data, final[path]) {
			report.Changes = append(report.Changes, Change{path, "changed"})
		}
	}
	for _, artifact := range r.ArtifactsInScope() {
		if _, expected := final[artifact.Path]; expected {
			continue
		}
		data, exists, err := readOwned(artifact.Path)
		if err != nil {
			return report, err
		}
		if exists {
			before[artifact.Path] = data
			report.Changes = append(report.Changes, Change{artifact.Path, "obsolete"})
		}
	}
	slices.SortFunc(report.Changes, func(a, b Change) int { return bytes.Compare([]byte(a.Path), []byte(b.Path)) })
	if mode == Check {
		return report, nil
	}
	// Verify all original sources and compared output states before publication.
	for _, pkg := range r.Packages {
		for _, file := range pkg.Files {
			data, err := os.ReadFile(file.Path)
			if err != nil || !bytes.Equal(data, file.Content) {
				return report, artifactError(file.Path, "source changed during planning; retry generation")
			}
		}
	}
	for _, change := range report.Changes {
		data, exists, err := readOwned(change.Path)
		if err != nil {
			return report, err
		}
		if (change.Kind == "missing" && exists) || !bytes.Equal(data, before[change.Path]) {
			return report, artifactError(change.Path, "output changed during planning; retry generation")
		}
	}
	for _, change := range report.Changes {
		if change.Kind == "obsolete" {
			continue
		}
		if err := ctx.Err(); err != nil {
			return report, err
		}
		if err := publish(change.Path, final[change.Path]); err != nil {
			return report, artifactError(change.Path, fmt.Sprintf("replacement failed after %d writes: %v; rerun to converge", len(report.Written), err))
		}
		report.Written = append(report.Written, change.Path)
	}
	for _, change := range report.Changes {
		if change.Kind != "obsolete" {
			continue
		}
		if err := ctx.Err(); err != nil {
			return report, err
		}
		data, exists, err := readOwned(change.Path)
		if err != nil {
			return report, err
		}
		if !exists || !bytes.Equal(data, before[change.Path]) {
			return report, artifactError(change.Path, "obsolete output changed during publication; rerun to converge")
		}
		if err := os.Remove(change.Path); err != nil {
			return report, artifactError(change.Path, fmt.Sprintf("obsolete removal failed after %d writes and %d removals: %v; rerun to converge", len(report.Written), len(report.Removed), err))
		}
		report.Removed = append(report.Removed, change.Path)
	}
	return report, nil
}

func consumerFloor(dir string) error {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	for {
		path := filepath.Join(dir, "go.mod")
		info, err := os.Stat(path)
		if err == nil && !info.IsDir() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			file, err := modfile.Parse(path, data, nil)
			if err != nil {
				return err
			}
			language := ""
			if file.Go != nil {
				language = file.Go.Version
				if strings.Count(language, ".") == 1 {
					language += ".0"
				}
			}
			if file.Go == nil || version.Compare("go"+language, "go1.26.0") < 0 {
				return structgen.Diagnostics{{Position: token.Position{Filename: path, Line: 1, Column: 1}, Code: structgen.ConsumerGoVersion, Message: "target module requires go 1.26.0 or newer; update its language version before generation"}}
			}
			return nil
		}
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return fmt.Errorf("invocation requires an existing Go module")
		}
		dir = parent
	}
}

func artifactError(path, message string) error {
	return structgen.Diagnostics{{Position: token.Position{Filename: path, Line: 1, Column: 1}, Code: structgen.ArtifactIO, Message: message}}
}

func readOwned(path string) ([]byte, bool, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, artifactError(path, err.Error())
	}
	if !info.Mode().IsRegular() {
		return nil, false, artifactError(path, "refusing non-regular output or symlink")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false, artifactError(path, err.Error())
	}
	_, owned, err := source.InspectArtifact(path, data)
	if err != nil || !owned {
		return nil, false, artifactError(path, "refusing non-owned output")
	}
	return data, true, nil
}

func atomicReplace(path string, data []byte) error {
	mode := os.FileMode(0644)
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("refusing non-regular output or symlink")
		}
		if _, _, err := readOwned(path); err != nil {
			return err
		}
		mode = info.Mode().Perm()
	} else if !os.IsNotExist(err) {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".gobetter-*")
	if err != nil {
		return err
	}
	name := file.Name()
	// Best-effort cleanup also runs after an earlier write or publication error.
	defer func() { _ = os.Remove(name) }()
	defer func() { _ = file.Close() }()
	if _, err = file.Write(data); err != nil {
		return err
	}
	if err = file.Chmod(mode); err != nil {
		return err
	}
	if err = file.Sync(); err != nil {
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	if _, _, err = readOwned(path); err != nil {
		return err
	}
	return os.Rename(name, path)
}
