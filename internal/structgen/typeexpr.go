// Copyright 2025 Toly Pochkin
// SPDX-License-Identifier: Apache-2.0

package structgen

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"

	"golang.org/x/tools/go/ast/astutil"
)

// RewriteType copies type syntax through the Go formatter/parser, then applies
// substitutions by original node identity. Source nodes, member names, tags,
// and selector members remain untouched. Traversal correspondence is checked
// before any replacement, rather than treating identifier spelling as binding.
func RewriteType(expr ast.Expr, bindings map[*ast.Ident]string, aliases map[*ast.StructType]ast.Expr) (ast.Expr, error) {
	if expr == nil {
		return nil, fmt.Errorf("missing type expression")
	}
	bad := false
	ast.Inspect(expr, func(node ast.Node) bool {
		if _, ok := node.(*ast.BadExpr); ok {
			bad = true
		}
		return true
	})
	if bad {
		return nil, fmt.Errorf("cannot format invalid type syntax")
	}
	var text bytes.Buffer
	if err := format.Node(&text, token.NewFileSet(), expr); err != nil {
		return nil, err
	}
	cloned, err := parser.ParseExprFrom(token.NewFileSet(), "", text.Bytes(), parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	originalIDs, originalStructs := typeNodes(expr)
	copyIDs, copyStructs := typeNodes(cloned)
	if len(originalIDs) != len(copyIDs) || len(originalStructs) != len(copyStructs) {
		return nil, fmt.Errorf("type syntax changed shape while copying")
	}
	renames := make(map[*ast.Ident]string)
	for i, original := range originalIDs {
		if original.Name != copyIDs[i].Name {
			return nil, fmt.Errorf("type identifier changed while copying")
		}
		if name, ok := bindings[original]; ok {
			renames[copyIDs[i]] = name
		}
	}
	replacements := make(map[*ast.StructType]ast.Expr)
	for i, original := range originalStructs {
		if alias, ok := aliases[original]; ok {
			replacements[copyStructs[i]] = alias
		}
	}
	result := astutil.Apply(cloned, func(cursor *astutil.Cursor) bool {
		switch node := cursor.Node().(type) {
		case *ast.Ident:
			if name, ok := renames[node]; ok {
				node.Name = name
			}
		case *ast.StructType:
			if alias, ok := replacements[node]; ok {
				cursor.Replace(alias)
				return false
			}
		}
		return true
	}, nil)
	return result.(ast.Expr), nil
}

func typeNodes(expr ast.Expr) ([]*ast.Ident, []*ast.StructType) {
	var ids []*ast.Ident
	var structs []*ast.StructType
	ast.Inspect(expr, func(node ast.Node) bool {
		switch node := node.(type) {
		case *ast.Ident:
			ids = append(ids, node)
		case *ast.StructType:
			structs = append(structs, node)
		}
		return true
	})
	return ids, structs
}

// FormatType rewrites and formats type syntax without modifying the source expression.
func FormatType(expr ast.Expr, bindings map[*ast.Ident]string, aliases map[*ast.StructType]ast.Expr) (string, error) {
	rewritten, err := RewriteType(expr, bindings, aliases)
	if err != nil {
		return "", err
	}
	var out bytes.Buffer
	err = format.Node(&out, token.NewFileSet(), rewritten)
	return out.String(), err
}
