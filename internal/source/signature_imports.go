// Copyright 2025 Toly Pochkin
// SPDX-License-Identifier: Apache-2.0

package source

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"strconv"

	"golang.org/x/tools/go/packages"
)

// goimports preserves dot imports because their references are unqualified.
// Bootstrap signatures may omit the authored uses of such imports. Resolve
// import usage against package declaration inventories before normal formatting;
// incomplete dependency inventories conservatively retain the binding.
func signatureImports(pkg *Package, code []byte, effectiveExports map[string]dependencyInventory) ([]byte, error) {
	file, err := parser.ParseFile(token.NewFileSet(), "", code, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	known := make(map[string]bool)
	for _, source := range pkg.Files {
		for _, name := range declaredSymbols(source.Parsed.AST) {
			known[name] = true
		}
	}
	for _, spec := range file.Imports {
		path, _ := strconv.Unquote(spec.Path.Value)
		if spec.Name != nil {
			if spec.Name.Name != "." {
				known[spec.Name.Name] = true
			}
		} else if name := dependencyPackageName(pkg.metadata, path, effectiveExports); name != "" {
			known[name] = true
		}
	}
	free := make(map[string]bool)
	for _, id := range file.Unresolved {
		if !known[id.Name] && types.Universe.Lookup(id.Name) == nil {
			free[id.Name] = true
		}
	}
	remove := make(map[*ast.ImportSpec]bool)
	for _, spec := range file.Imports {
		if spec.Name == nil || spec.Name.Name != "." {
			continue
		}
		path, _ := strconv.Unquote(spec.Path.Value)
		names, complete, err := effectiveDependencyNames(pkg.metadata, path, effectiveExports)
		if err != nil {
			return nil, err
		}
		used := !complete
		for name := range free {
			if names[name] {
				used = true
			}
		}
		if !used {
			remove[spec] = true
		}
	}
	if len(remove) == 0 {
		return code, nil
	}
	var declarations []ast.Decl
	for _, decl := range file.Decls {
		if group, ok := decl.(*ast.GenDecl); ok && group.Tok == token.IMPORT {
			var specs []ast.Spec
			for _, spec := range group.Specs {
				if !remove[spec.(*ast.ImportSpec)] {
					specs = append(specs, spec)
				}
			}
			if len(specs) == 0 {
				continue
			}
			group.Specs = specs
		}
		declarations = append(declarations, decl)
	}
	file.Decls = declarations
	var out bytes.Buffer
	err = format.Node(&out, token.NewFileSet(), file)
	return out.Bytes(), err
}

// Selected dependencies use the same declarations as the combined overlay:
// authored/retained exports plus planned replacements, with masked exports gone.
// Unselected dependencies keep their on-disk inventory and completeness signal.
type dependencyInventory struct {
	name    string
	exports map[string]bool // nil means selected but not allocated yet.
}

func dependencyPackageName(metadata *packages.Package, path string, effectiveExports map[string]dependencyInventory) string {
	if inventory, selected := effectiveExports[path]; selected {
		return inventory.name
	}
	if dependency := metadata.Imports[path]; dependency != nil {
		return dependency.Name
	}
	return ""
}

func effectiveDependencyNames(metadata *packages.Package, path string, effectiveExports map[string]dependencyInventory) (map[string]bool, bool, error) {
	if inventory, selected := effectiveExports[path]; selected {
		if inventory.exports == nil {
			return nil, false, fmt.Errorf("selected dependency %s export inventory is not allocated", path)
		}
		return inventory.exports, true, nil
	}
	names, complete := dependencyNames(metadata.Imports[path])
	return names, complete, nil
}

func dependencyNames(pkg *packages.Package) (map[string]bool, bool) {
	names := make(map[string]bool)
	if pkg == nil || len(pkg.GoFiles) == 0 {
		return names, false
	}
	complete := true
	for _, filename := range pkg.GoFiles {
		data, err := os.ReadFile(filename)
		if err != nil {
			complete = false
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), filename, data, parser.SkipObjectResolution)
		if err != nil {
			complete = false
			continue
		}
		for _, name := range declaredSymbols(file) {
			if ast.IsExported(name) {
				names[name] = true
			}
		}
	}
	return names, complete
}
