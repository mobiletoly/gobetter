// Copyright 2025 Toly Pochkin
// SPDX-License-Identifier: Apache-2.0

package source

import (
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"path/filepath"
	"strings"

	"github.com/mobiletoly/gobetter/internal/structgen"
)

// File contains an in-memory syntax tree and its generation target inventory.
type File struct {
	AST     *ast.File
	FileSet *token.FileSet
	Targets []structgen.Target
}

// Parse retains physical source positions and original AST identities. It does
// not load dependencies, resolve types, or read/write a source file itself.
func Parse(filename string, content []byte) (*File, error) {
	return parseWithSet(token.NewFileSet(), filename, content)
}

func parseWithSet(fset *token.FileSet, filename string, content []byte) (*File, error) {
	if strings.HasSuffix(filename, "_test.go") {
		return nil, structgen.Diagnostics{{Position: token.Position{Filename: filename, Line: 1, Column: 1}, Code: structgen.UnsupportedTarget, Message: "test-file generation is unsupported; select an ordinary Go source"}}
	}
	packageDir, err := filepath.Abs(filepath.Dir(filename))
	if err != nil {
		return nil, err
	}
	file, err := parseGoFile(fset, filename, content)
	if err != nil {
		return nil, err
	}
	p := newAnnotations(fset, file, content)
	result := &File{AST: file, FileSet: fset}
	for _, declaration := range file.Decls {
		group, ok := declaration.(*ast.GenDecl)
		if !ok || group.Tok != token.TYPE {
			continue
		}
		for _, spec := range group.Specs {
			typeSpec := spec.(*ast.TypeSpec)
			doc := typeSpec.Doc
			if doc == nil && len(group.Specs) == 1 {
				doc = group.Doc
			}
			options, annotated := p.typeOptions(doc)
			st, isStruct := ast.Unparen(typeSpec.Type).(*ast.StructType)
			kind := structgen.OtherDeclaration
			if isStruct {
				kind = structgen.DefinedStruct
			}
			if typeSpec.Assign.IsValid() {
				kind = structgen.UserAlias
			}
			target := structgen.Target{PackageDir: packageDir, SourceFile: filename, Path: typeSpec.Name.Name,
				Name: typeSpec.Name.Name, Position: p.position(typeSpec.Name.Pos()), Kind: kind, Exported: typeSpec.Name.IsExported(),
				TypeSpec: typeSpec, StructType: st, Annotated: annotated, Options: options}
			if typeSpec.TypeParams != nil {
				for _, field := range typeSpec.TypeParams.List {
					for _, name := range field.Names {
						target.TypeParameters = append(target.TypeParameters, structgen.TypeParameter{Ident: name, Constraint: field.Type})
					}
				}
			}
			if annotated && kind != structgen.DefinedStruct {
				p.add(typeSpec.Pos(), structgen.UnsupportedTarget, "struct directive requires a named defined struct, not an alias or another declaration kind")
			}
			if isStruct && kind == structgen.DefinedStruct {
				p.collectStruct(&target, &result.Targets)
			} else {
				result.Targets = append(result.Targets, target)
			}
		}
	}
	p.finish()
	if len(p.errors) != 0 {
		return nil, p.errors.Sorted()
	}
	declared := methods(methodFile{file, fset})
	for i := range result.Targets {
		result.Targets[i].ExistingMethods = declared[result.Targets[i].Name]
	}
	return result, nil
}

// Retained Go context uses the same physical syntax diagnostics as ordinary
// inputs, while directive extraction remains a separate operation.
func parseGoFile(fset *token.FileSet, filename string, content []byte) (*ast.File, error) {
	file, err := parser.ParseFile(fset, filename, content, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		var ds structgen.Diagnostics
		if errors, ok := err.(scanner.ErrorList); ok {
			// Syntax errors use adjusted //line positions, but their offsets
			// still identify bytes in the parser's original source file.
			sourceFile := fset.File(file.FileStart)
			for _, syntaxError := range errors {
				position := sourceFile.PositionFor(sourceFile.Pos(syntaxError.Pos.Offset), false)
				ds = append(ds, structgen.Diagnostic{Position: position, Code: structgen.SourceSyntax, Message: syntaxError.Msg})
			}
		} else {
			ds = append(ds, structgen.Diagnostic{Position: token.Position{Filename: filename, Line: 1, Column: 1}, Code: structgen.SourceSyntax, Message: err.Error()})
		}
		return nil, ds.Sorted()
	}
	return file, nil
}

func (p *annotations) collectStruct(target *structgen.Target, targets *[]structgen.Target) {
	var children []structgen.Target
	for _, field := range target.StructType.Fields.List {
		options, annotated := p.fieldOptions(field.Doc, field.Comment)
		nestedOptions, nestedAnnotated := p.typeOptions(field.Doc)
		st := anonymousStruct(field.Type)
		if (annotated || nestedAnnotated) && len(field.Names) > 1 {
			p.add(field.Pos(), structgen.UnsupportedTarget, "a field directive must attach to one field; split the grouped declaration")
		}
		if nestedAnnotated && st == nil {
			p.add(field.Pos(), structgen.UnsupportedTarget, "nested struct directive requires a direct or pointer anonymous-struct field")
		}
		names := field.Names
		if len(names) == 0 {
			name := embeddedName(field.Type)
			if name == "" {
				p.add(field.Pos(), structgen.UnsupportedTarget, "cannot identify the embedded field's declared name")
				continue
			}
			names = []*ast.Ident{{Name: name, NamePos: field.Pos()}}
		}
		for _, name := range names {
			if name.Name == "_" {
				if annotated || nestedAnnotated {
					p.add(field.Pos(), structgen.UnsupportedTarget, "blank fields cannot carry generation directives")
				}
				continue
			}
			target.Fields = append(target.Fields, structgen.Field{Name: name.Name, Position: p.position(name.Pos()), AST: field, TypeExpr: field.Type, Embedded: len(field.Names) == 0, Annotated: annotated, Options: options})
			if st != nil {
				children = append(children, structgen.Target{PackageDir: target.PackageDir, SourceFile: target.SourceFile, Path: target.Path + "." + name.Name,
					ParentPath: target.Path, ParentField: name.Name, Position: p.position(name.Pos()), Kind: structgen.NestedAlias, Exported: target.Exported,
					StructType: st, TypeParameters: target.TypeParameters, Annotated: nestedAnnotated, Options: nestedOptions})
			}
		}
	}
	*targets = append(*targets, *target)
	for _, child := range children {
		p.collectStruct(&child, targets)
	}
}

func anonymousStruct(expr ast.Expr) *ast.StructType {
	expr = ast.Unparen(expr)
	if pointer, ok := expr.(*ast.StarExpr); ok {
		expr = ast.Unparen(pointer.X)
	}
	st, _ := expr.(*ast.StructType)
	return st
}
func embeddedName(expr ast.Expr) string {
	switch value := ast.Unparen(expr).(type) {
	case *ast.Ident:
		return value.Name
	case *ast.StarExpr:
		return embeddedName(value.X)
	case *ast.SelectorExpr:
		return value.Sel.Name
	case *ast.IndexExpr:
		return embeddedName(value.X)
	case *ast.IndexListExpr:
		return embeddedName(value.X)
	default:
		return ""
	}
}
