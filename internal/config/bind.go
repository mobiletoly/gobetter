// Copyright 2025 Toly Pochkin
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/mobiletoly/gobetter/internal/structgen"
)

// PackageInventory describes a selected package. Inactive paths are identities
// only, not selected or type-checked targets. Discovery belongs to source loading.
type PackageInventory struct {
	Dir      string
	Active   []structgen.Target
	Inactive []string
}

// Bind validates records within the caller's selected package scope. Package
// paths are resolved relative to the config file, not the invocation directory.
// It checks lexical module containment; source loading checks actual module and
// build-context membership. It does not access the filesystem.
func Bind(document Document, configFile, moduleDir string, packages []PackageInventory) ([]structgen.TypeOverride, error) {
	moduleDir, err := filepath.Abs(moduleDir)
	if err != nil {
		return nil, err
	}
	configDir, err := filepath.Abs(filepath.Dir(configFile))
	if err != nil {
		return nil, err
	}
	packages = slices.Clone(packages)
	for i := range packages {
		packages[i].Dir, err = filepath.Abs(packages[i].Dir)
		if err != nil {
			return nil, err
		}
	}
	var overrides []structgen.TypeOverride
	var ds structgen.Diagnostics
	seen := make(map[string]bool)
	for _, item := range document.Types {
		dir := item.Package
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(configDir, dir)
		}
		dir = filepath.Clean(dir)
		rel, err := filepath.Rel(moduleDir, dir)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			ds = append(ds, structgen.Diagnostic{Position: item.Position, Code: structgen.ConfigTarget, Message: "configured package must be within the input module"})
			continue
		}
		key := dir + "\x00" + item.Path
		for _, pkg := range packages {
			if filepath.Clean(pkg.Dir) != dir {
				continue
			}
			if seen[key] {
				ds = append(ds, structgen.Diagnostic{Position: item.Position, Code: structgen.ConfigDuplicate, Message: "duplicate normalized type configuration for " + item.Path})
				break
			}
			seen[key] = true
			var matches []structgen.Target
			for _, target := range pkg.Active {
				if target.Path == item.Path {
					matches = append(matches, target)
				}
			}
			if len(matches) == 0 && slices.Contains(pkg.Inactive, item.Path) {
				break
			}
			if len(matches) != 1 {
				ds = append(ds, structgen.Diagnostic{Package: dir, Position: item.Position, Code: structgen.ConfigTarget, Message: fmt.Sprintf("type selector %s must identify one active or known inactive declaration (found %d active)", item.Path, len(matches))})
				break
			}
			target := matches[0]
			for _, field := range item.Fields {
				known := false
				for _, sourceField := range target.Fields {
					if field.Name == sourceField.Name {
						known = true
					}
				}
				if !known {
					ds = append(ds, structgen.Diagnostic{Package: dir, Position: field.Position, Code: structgen.ConfigTarget, Message: "unknown field " + field.Name + " of " + item.Path})
				}
			}
			overrides = append(overrides, structgen.TypeOverride{PackageDir: dir, Path: item.Path, Position: item.Position, Options: item.Options, Fields: item.Fields})
			break
		}
	}
	if len(ds) != 0 {
		return nil, ds.Sorted()
	}
	return overrides, nil
}
