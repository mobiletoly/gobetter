// Copyright 2025 Toly Pochkin
// SPDX-License-Identifier: Apache-2.0

package source

import (
	"fmt"
	"go/ast"
	"go/build/constraint"
	"go/token"
	"os"
	"strconv"
	"strings"

	"github.com/mobiletoly/gobetter/internal/structgen"
)

// ConstraintHeader preserves genuine legacy headers using the composed context.
func (f *SourceFile) ConstraintHeader() (string, error) {
	if f.Constraint == nil {
		return "", nil
	}
	header := "//go:build " + f.Constraint.String() + "\n"
	end := legacyHeaderEnd(f.Parsed.AST, f.Content)
	legacy := false
	for _, group := range f.Parsed.AST.Comments {
		for _, comment := range group.List {
			if int(comment.Pos()-f.Parsed.AST.FileStart) < end && constraint.IsPlusBuild(comment.Text) {
				legacy = true
			}
		}
	}
	if legacy {
		lines, err := constraint.PlusBuildLines(f.Constraint)
		if err != nil {
			return "", err
		}
		header += strings.Join(lines, "\n") + "\n"
	}
	return header + "\n", nil
}

func legacyHeaderEnd(file *ast.File, data []byte) int {
	end, offset := 0, 0
	for line := range strings.SplitSeq(string(data[:int(file.Package-file.FileStart)]), "\n") {
		text := strings.TrimSpace(line)
		offset += len(line) + 1
		if text == "" {
			end = offset
			continue
		}
		if !strings.HasPrefix(text, "//") {
			break
		}
	}
	return end
}

// ArtifactsInScope excludes existing sources outside the selected context/file.
// Deleted-source artifacts remain in scope for obsolete comparison/removal.
func (r *Result) ArtifactsInScope() []Artifact {
	var result []Artifact
	for _, pkg := range r.Packages {
		active := make(map[string]bool)
		for _, file := range pkg.Files {
			active[file.Path] = true
		}
		for _, artifact := range pkg.Artifacts {
			if r.discovery.Request.File != "" && artifact.Source != r.discovery.Request.File {
				continue
			}
			if _, err := os.Stat(artifact.Source); err == nil && !active[artifact.Source] {
				continue
			}
			result = append(result, artifact)
		}
	}
	return result
}

// Check emitted syntax before either bootstrap or final import completion.
// Import-path identity and proven binder uses distinguish pseudo-C from names.
func validateCSignatures(plans []structgen.Plan) error {
	var ds structgen.Diagnostics
	for _, p := range plans {
		pseudo := make(map[string]bool)
		for _, spec := range p.Target.Imports {
			path, _ := strconv.Unquote(spec.Path.Value)
			if path == "C" {
				name := "C"
				if spec.Name != nil {
					name = spec.Name.Name
				}
				pseudo[name] = true
			}
		}
		check := func(expr ast.Expr, pos token.Position) {
			bad := false
			ast.Inspect(expr, func(n ast.Node) bool {
				if selector, ok := n.(*ast.SelectorExpr); ok {
					if id, ok := ast.Unparen(selector.X).(*ast.Ident); ok && pseudo[id.Name] && p.Bindings[id] == "" {
						bad = true
					}
				}
				return true
			})
			if bad {
				ds = append(ds, structgen.Diagnostic{Package: p.Target.PackageDir, Position: pos, Code: structgen.UnsupportedCSignature, Message: "emitted signature requires pseudo-package C; exclude this output/field or add an authored Go type boundary"})
			}
		}
		for _, parameter := range p.Target.TypeParameters {
			check(parameter.Constraint, p.Target.Position)
		}
		if p.Target.Kind == structgen.NestedAlias {
			check(p.Target.StructType, p.Target.Position)
		}
		for _, field := range append(p.FieldsFor(structgen.Builder), p.FieldsFor(structgen.Getters)...) {
			check(field.TypeExpr, field.Position)
		}
	}
	if len(ds) > 0 {
		return ds.Sorted()
	}
	return nil
}

func (r *Result) validateSynthesizedSignatures() error {
	for _, p := range r.Plans {
		declared := make(map[string]bool)
		for _, pkg := range r.Packages {
			if pkg.Dir == p.Target.PackageDir {
				for _, file := range pkg.Files {
					for _, name := range declaredSymbols(file.Parsed.AST) {
						declared[name] = true
					}
				}
			}
		}
		check := func(expr ast.Expr) bool {
			bad := false
			typeIdentifiers(expr, func(id *ast.Ident) {
				if declared[id.Name] || p.Bindings[id] != "" {
					return
				}
				if strings.HasPrefix(id.Name, "_Ctype_") || strings.HasPrefix(id.Name, "_Cgo_") {
					for _, pkg := range r.resolved {
						if pkg.PkgPath == p.Target.Object.Pkg().Path() && pkg.Types.Scope().Lookup(id.Name) != nil {
							bad = true
						}
					}
				}
			})
			return bad
		}
		for _, field := range append(p.FieldsFor(structgen.Builder), p.FieldsFor(structgen.Getters)...) {
			if check(field.TypeExpr) {
				return structgen.Diagnostics{{Package: p.Target.PackageDir, Position: field.Position, Code: structgen.UnsupportedCSignature, Message: "emitted signature requires a loader-synthesized cgo name; exclude the field or add an authored Go type boundary"}}
			}
		}
		for _, parameter := range p.Target.TypeParameters {
			if check(parameter.Constraint) {
				return fmt.Errorf("%s: %s: synthesized cgo constraint requires an authored Go type boundary", p.Target.Position, structgen.UnsupportedCSignature)
			}
		}
		if p.Target.Kind == structgen.NestedAlias && check(p.Target.StructType) {
			return fmt.Errorf("%s: %s: synthesized cgo alias requires an authored Go type boundary", p.Target.Position, structgen.UnsupportedCSignature)
		}
	}
	return nil
}

// Only type references participate; field/parameter/member names and array
// length values can legally share a synthesized-looking spelling.
func typeIdentifiers(expr ast.Expr, visit func(*ast.Ident)) {
	switch node := ast.Unparen(expr).(type) {
	case *ast.Ident:
		visit(node)
	case *ast.StarExpr:
		typeIdentifiers(node.X, visit)
	case *ast.ArrayType:
		typeIdentifiers(node.Elt, visit)
	case *ast.MapType:
		typeIdentifiers(node.Key, visit)
		typeIdentifiers(node.Value, visit)
	case *ast.ChanType:
		typeIdentifiers(node.Value, visit)
	case *ast.Ellipsis:
		typeIdentifiers(node.Elt, visit)
	case *ast.IndexExpr:
		typeIdentifiers(node.X, visit)
		typeIdentifiers(node.Index, visit)
	case *ast.IndexListExpr:
		typeIdentifiers(node.X, visit)
		for _, arg := range node.Indices {
			typeIdentifiers(arg, visit)
		}
	case *ast.UnaryExpr:
		typeIdentifiers(node.X, visit)
	case *ast.BinaryExpr:
		typeIdentifiers(node.X, visit)
		typeIdentifiers(node.Y, visit)
	case *ast.StructType:
		for _, field := range node.Fields.List {
			typeIdentifiers(field.Type, visit)
		}
	case *ast.InterfaceType:
		for _, field := range node.Methods.List {
			typeIdentifiers(field.Type, visit)
		}
	case *ast.FuncType:
		for _, list := range []*ast.FieldList{node.TypeParams, node.Params, node.Results} {
			if list != nil {
				for _, field := range list.List {
					typeIdentifiers(field.Type, visit)
				}
			}
		}
	}
}
