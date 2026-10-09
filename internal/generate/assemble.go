// Copyright 2025 Toly Pochkin
// SPDX-License-Identifier: Apache-2.0

package generate

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"

	"github.com/mobiletoly/gobetter/internal/source"
	"github.com/mobiletoly/gobetter/internal/structgen"
	"golang.org/x/tools/imports"
)

func assemble(r *source.Result) (map[string][]byte, error) {
	final := make(map[string][]byte)
	for _, pkg := range r.Packages {
		for _, file := range pkg.Files {
			path := source.OutputFilename(file.Path)
			bootstrap, selected := r.Bootstrap[path]
			if !selected {
				continue
			}
			var plans []structgen.Plan
			for _, p := range r.Plans {
				if p.Target.SourceFile == file.Path {
					plans = append(plans, p)
				}
			}
			rendered, err := structgen.Render(plans)
			if err != nil {
				return nil, err
			}
			header, err := file.ConstraintHeader()
			if err != nil {
				return nil, err
			}
			var code bytes.Buffer
			fmt.Fprintf(&code, "%s%s\n%s%s\n%s%s\n\npackage %s\n\n", header, source.GeneratedHeader, source.VersionMetadata, Version, source.SourceMetadata, filepath.Base(file.Path), pkg.Name)
			fset := token.NewFileSet()
			envelope, err := parser.ParseFile(fset, path, bootstrap, parser.ParseComments)
			if err != nil {
				return nil, err
			}
			for _, decl := range envelope.Decls {
				if group, ok := decl.(*ast.GenDecl); ok && group.Tok == token.IMPORT {
					if err := format.Node(&code, fset, group); err != nil {
						return nil, err
					}
					code.WriteString("\n")
				}
			}
			for _, binding := range rendered.Imports {
				fmt.Fprintf(&code, "import %s %s\n", binding.Name, strconv.Quote(binding.Path))
			}
			code.WriteString("\n" + rendered.Code)
			data, err := imports.Process(path, code.Bytes(), &imports.Options{Comments: true})
			if err != nil {
				return nil, artifactError(path, "import finalization: "+err.Error())
			}
			if _, exists := final[path]; exists {
				return nil, artifactError(path, "output path collision")
			}
			final[path] = data
		}
	}
	return final, nil
}
