// Copyright 2025 Toly Pochkin
// SPDX-License-Identifier: Apache-2.0

package source

import (
	"go/ast"
	"go/token"

	"github.com/mobiletoly/gobetter/internal/structgen"
)

type methodFile struct {
	file *ast.File
	fset *token.FileSet
}

func methods(files ...methodFile) map[string][]structgen.Member {
	declarations := make(map[string]*ast.TypeSpec)
	for _, source := range files {
		for _, decl := range source.file.Decls {
			group, ok := decl.(*ast.GenDecl)
			if !ok || group.Tok != token.TYPE {
				continue
			}
			for _, spec := range group.Specs {
				typeSpec := spec.(*ast.TypeSpec)
				declarations[typeSpec.Name.Name] = typeSpec
			}
		}
	}
	result := make(map[string][]structgen.Member)
	for _, source := range files {
		for _, decl := range source.file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || len(fn.Recv.List) != 1 {
				continue
			}
			name := receiverBase(fn.Recv.List[0].Type, declarations)
			if name != "" {
				result[name] = append(result[name], structgen.Member{Name: fn.Name.Name, Position: source.fset.PositionFor(fn.Name.Pos(), false)})
			}
		}
	}
	return result
}

// Resolve valid local, non-generic alias chains across the complete inventory
// before signatures exist. Invalid receiver forms remain loader/type errors.
func receiverBase(expr ast.Expr, declarations map[string]*ast.TypeSpec) string {
	name := receiverName(expr)
	decl := declarations[name]
	if decl == nil || !decl.Assign.IsValid() {
		return name
	}
	pointers := 0
	receiver := ast.Unparen(expr)
	if pointer, ok := receiver.(*ast.StarExpr); ok {
		pointers++
		receiver = ast.Unparen(pointer.X)
	}
	if _, ok := receiver.(*ast.Ident); !ok {
		return ""
	}
	seen := make(map[string]bool)
	for decl != nil && decl.Assign.IsValid() {
		if seen[name] || decl.TypeParams != nil && len(decl.TypeParams.List) > 0 {
			return ""
		}
		seen[name] = true
		target := ast.Unparen(decl.Type)
		if pointer, ok := target.(*ast.StarExpr); ok {
			pointers++
			target = ast.Unparen(pointer.X)
		}
		ident, ok := target.(*ast.Ident)
		if !ok || pointers > 1 {
			return ""
		}
		name, decl = ident.Name, declarations[ident.Name]
	}
	if decl == nil || decl.TypeParams != nil && len(decl.TypeParams.List) > 0 {
		return ""
	}
	return name
}

func receiverName(expr ast.Expr) string {
	switch t := ast.Unparen(expr).(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return receiverName(t.X)
	case *ast.IndexExpr:
		return receiverName(t.X)
	case *ast.IndexListExpr:
		return receiverName(t.X)
	default:
		return ""
	}
}
