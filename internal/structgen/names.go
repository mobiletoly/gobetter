// Copyright 2025 Toly Pochkin
// SPDX-License-Identifier: Apache-2.0

package structgen

import (
	"fmt"
	"go/ast"
	"maps"
	"slices"
	"strings"
)

// AllocateNames is package-local and deterministic. aliases may use suffixes;
// fixed generated step/constructor symbols diagnose collisions. Binding maps
// come from the source owner's lexical analysis and are verified after loading.
func AllocateNames(plans []Plan, symbols []string, uses map[*ast.Ident]*ast.Ident) ([]Plan, error) {
	plans = slices.Clone(plans)
	reserved := make(map[string]bool)
	for _, name := range symbols {
		reserved[name] = true
	}
	var ds Diagnostics
	for i := range plans {
		p := &plans[i]
		p.Target.TypeParameters = slices.Clone(p.Target.TypeParameters)
		if p.Target.Kind == NestedAlias {
			base := strings.ReplaceAll(p.Target.Path, ".", "")
			name := base
			for suffix := 2; reserved[name]; suffix++ {
				name = fmt.Sprintf("%s_%d", base, suffix)
			}
			p.Target.Name = name
			reserved[name] = true
		}
	}
	for i := range plans {
		p := &plans[i]
		claim := func(name string) string {
			if reserved[name] {
				ds = append(ds, Diagnostic{Package: p.Target.PackageDir, Position: p.Target.Position, Code: SymbolCollision, Message: "generated symbol " + name + " conflicts with package scope"})
			}
			reserved[name] = true
			return name
		}
		if p.Derive.Has(Builder) {
			prefix := "New"
			if p.Constructor == PackageConstructor {
				prefix = "new"
			}
			p.Names.Constructor = claim(prefix + methodName(p.Target.Name) + BuilderSuffix)
			p.Names.Done = claim(StatePrefix + p.Target.Name + "_done")
			for _, field := range p.FieldsFor(Builder) {
				if !field.Optional {
					p.Names.Steps = append(p.Names.Steps, claim(fmt.Sprintf("%s%s_step%d", StatePrefix, p.Target.Name, len(p.Names.Steps))))
				}
			}
		}
	}
	imports := maps.Clone(reserved)
	for _, p := range plans {
		reserve := func(expr ast.Expr) {
			ast.Inspect(expr, func(n ast.Node) bool {
				if id, ok := n.(*ast.Ident); ok {
					imports[id.Name] = true
				}
				return true
			})
		}
		for _, field := range p.Fields {
			reserve(field.TypeExpr)
		}
		for _, parameter := range p.Target.TypeParameters {
			reserve(parameter.Constraint)
		}
	}
	importName := func(base string) string {
		name := base
		for n := 2; imports[name]; n++ {
			name = fmt.Sprintf("%s%d", base, n)
		}
		imports[name] = true
		reserved[name] = true
		return name
	}
	fmtName, slicesName, mapsName := importName("gobetterFmt"), importName("gobetterSlices"), importName("gobetterMaps")
	for i := range plans {
		p := &plans[i]
		p.Names.FmtImport, p.Names.SlicesImport, p.Names.MapsImport = fmtName, slicesName, mapsName
		local := make(map[string]bool)
		for name := range reserved {
			local[name] = true
		}
		// Free identifiers and import qualifiers also reserve generated binders.
		for _, field := range p.Fields {
			ast.Inspect(field.TypeExpr, func(n ast.Node) bool {
				if id, ok := n.(*ast.Ident); ok {
					local[id.Name] = true
				}
				return true
			})
		}
		for _, parameter := range p.Target.TypeParameters {
			ast.Inspect(parameter.Constraint, func(n ast.Node) bool {
				if id, ok := n.(*ast.Ident); ok {
					local[id.Name] = true
				}
				return true
			})
		}
		p.Bindings = make(map[*ast.Ident]string)
		parameterNames := make(map[*ast.Ident]string)
		next := 0
		for j := range p.Target.TypeParameters {
			parameter := &p.Target.TypeParameters[j]
			name := fmt.Sprintf("%s%d", TypeParameterPrefix, next)
			for local[name] {
				next++
				name = fmt.Sprintf("%s%d", TypeParameterPrefix, next)
			}
			next++
			local[name] = true
			parameter.Binder = name
			parameterNames[parameter.Ident] = name
		}
		for use, parameter := range uses {
			if name, ok := parameterNames[parameter]; ok {
				p.Bindings[use] = name
			}
		}
		allocate := func(base string) string {
			name := base
			for i := 2; local[name]; i++ {
				name = fmt.Sprintf("%s%d", base, i)
			}
			local[name] = true
			return name
		}
		p.Names.Receiver = allocate("gobetterReceiver")
		p.Names.Argument = allocate("gobetterArg")
		p.Names.Result = allocate("gobetterResult")
		p.Names.Other = allocate("gobetterOther")
	}
	if len(ds) != 0 {
		return nil, ds.Sorted()
	}
	return plans, nil
}

// TypeArguments formats the allocated type parameter names for a type use.
func (p Plan) TypeArguments() string {
	if len(p.Target.TypeParameters) == 0 {
		return ""
	}
	names := make([]string, len(p.Target.TypeParameters))
	for i, parameter := range p.Target.TypeParameters {
		names[i] = parameter.Binder
	}
	return "[" + strings.Join(names, ", ") + "]"
}

// TypeDeclaration formats allocated type parameters with their rewritten constraints.
func (p Plan) TypeDeclaration() (string, error) {
	if len(p.Target.TypeParameters) == 0 {
		return "", nil
	}
	var parameters []string
	for _, parameter := range p.Target.TypeParameters {
		constraint, err := FormatType(parameter.Constraint, p.Bindings, nil)
		if err != nil {
			return "", err
		}
		parameters = append(parameters, parameter.Binder+" "+constraint)
	}
	return "[" + strings.Join(parameters, ", ") + "]", nil
}

// AliasExpressions maps nested struct syntax to aliases using the owner's type parameters.
func AliasExpressions(plans []Plan, owner Plan) map[*ast.StructType]ast.Expr {
	aliases := make(map[*ast.StructType]ast.Expr)
	for _, p := range plans {
		if p.Target.Kind != NestedAlias {
			continue
		}
		var expr ast.Expr = ast.NewIdent(p.Target.Name)
		if len(p.Target.TypeParameters) > 0 {
			args := make([]ast.Expr, len(p.Target.TypeParameters))
			for i, parameter := range p.Target.TypeParameters {
				name := parameter.Binder
				for _, inScope := range owner.Target.TypeParameters {
					if inScope.Ident == parameter.Ident {
						name = inScope.Binder
					}
				}
				args[i] = ast.NewIdent(name)
			}
			expr = &ast.IndexListExpr{X: expr, Indices: args}
		}
		aliases[p.Target.StructType] = expr
	}
	return aliases
}
