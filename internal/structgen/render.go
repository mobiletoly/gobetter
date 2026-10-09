// Copyright 2025 Toly Pochkin
// SPDX-License-Identifier: Apache-2.0

package structgen

import (
	"fmt"
	"go/ast"
	"go/format"
	"strings"
)

// Render emits runtime declarations and deliberate standard-library bindings
// for resolved plans from one package. Headers/source imports/finalization/I/O
// belong to the caller. Signatures share the same declaration graph.
func Render(plans []Plan) (Rendered, error) {
	var ds Diagnostics
	for _, p := range plans {
		add := func(code Code, message string) {
			ds = append(ds, Diagnostic{Package: p.Target.PackageDir, Position: p.Target.Position, Code: code, Message: message})
		}
		if p.Target.Object == nil || p.Target.Object.Name() != p.Target.Name || p.Target.StructType == nil || p.Names.Receiver == "" || p.Names.Result == "" {
			add(UnresolvedPlan, "runtime rendering requires a resolved, allocated target plan")
		}
		if p.Target.PackageDir != plans[0].Target.PackageDir {
			add(InvalidOption, "render declarations for one package at a time")
		}
		if p.Derive.Has(Equal) && p.Names.Other == "" || p.Derive.Has(String) && len(p.FieldsFor(String)) > 0 && p.Names.FmtImport == "" {
			add(UnresolvedPlan, "output locals/import bindings were not allocated")
		}
		for _, parameter := range p.Target.TypeParameters {
			if parameter.Object == nil || parameter.Binder == "" {
				add(UnresolvedPlan, "generic parameter has no resolved identity or allocated binder")
			}
		}
		required := 0
		for _, field := range p.Fields {
			if field.Type == nil {
				ds = append(ds, Diagnostic{Package: p.Target.PackageDir, Position: field.Position, Code: UnresolvedPlan, Message: "field type did not resolve: " + field.Name})
			}
			if p.Derive.Has(Builder) && !field.SkipBuilder && !field.Optional {
				required++
			}
		}
		if p.Derive.Has(Builder) && (p.Names.Constructor == "" || p.Names.Done == "" || p.Names.Argument == "" || len(p.Names.Steps) != required) {
			add(UnresolvedPlan, "builder stage names do not match the required fields")
		}
	}
	if len(ds) > 0 {
		return Rendered{}, ds.Sorted()
	}
	if err := ValidateMembers(plans); err != nil {
		return Rendered{}, err
	}
	if err := validateEquality(plans); err != nil {
		return Rendered{}, err
	}
	for _, binding := range runtimeImports(plans) {
		if binding.Name == "" {
			return Rendered{}, Diagnostics{{Code: UnresolvedPlan, Message: "standard-library import binding was not allocated"}}
		}
	}
	code, err := declarations(plans, true)
	if err != nil {
		return Rendered{}, err
	}
	return Rendered{Code: code, Imports: runtimeImports(plans)}, nil
}

func declarations(plans []Plan, bodies bool) (string, error) {
	if len(plans) == 0 {
		return "", nil
	}
	var out strings.Builder
	for _, p := range plans {
		aliases := AliasExpressions(plans, p)
		declaration, err := p.TypeDeclaration()
		if err != nil {
			return "", err
		}
		args := p.TypeArguments()
		target := p.Target.Name + args
		if p.Target.Kind == NestedAlias {
			ownAliases := make(map[*ast.StructType]ast.Expr)
			for node, alias := range aliases {
				if node != p.Target.StructType {
					ownAliases[node] = alias
				}
			}
			text, err := FormatType(p.Target.StructType, p.Bindings, ownAliases)
			if err != nil {
				return "", err
			}
			fmt.Fprintf(&out, "type %s%s = %s\n", p.Target.Name, declaration, text)
		}
		method := func(receiver, name, parameters, result, body string) {
			if !bodies {
				result = "(" + p.Names.Result + " " + result + ")"
				body = "return"
			}
			fmt.Fprintf(&out, "func (%s %s) %s(%s) %s { %s }\n", p.Names.Receiver, receiver, name, parameters, result, body)
		}
		if p.Derive.Has(Builder) {
			for _, name := range append(append([]string(nil), p.Names.Steps...), p.Names.Done) {
				fields := ""
				if bodies {
					fields = "root *" + target
				}
				fmt.Fprintf(&out, "type %s%s struct {%s}\n", name, declaration, fields)
			}
			first := p.Names.Done
			if len(p.Names.Steps) > 0 {
				first = p.Names.Steps[0]
			}
			result, body := first+args, "return"
			if bodies {
				body = fmt.Sprintf("var %s %s; return %s{root: &%s}", p.Names.Result, target, result, p.Names.Result)
			} else {
				result = "(" + p.Names.Result + " " + result + ")"
			}
			fmt.Fprintf(&out, "func %s%s() %s { %s }\n", p.Names.Constructor, declaration, result, body)
			builderFields := p.FieldsFor(Builder)
			var requiredNames []string
			for _, field := range builderFields {
				if !field.Optional {
					requiredNames = append(requiredNames, field.SetterName)
				}
			}
			step := 0
			for _, field := range builderFields {
				typeText, err := FormatType(field.TypeExpr, p.Bindings, aliases)
				if err != nil {
					return "", err
				}
				from := p.Names.Done
				if step < len(p.Names.Steps) {
					from = p.Names.Steps[step]
				}
				to := from
				if !field.Optional {
					step++
					to = p.Names.Done
					if step < len(p.Names.Steps) {
						to = p.Names.Steps[step]
					}
				}
				body := fmt.Sprintf("%s.root.%s = %s; return %s{root: %s.root}", p.Names.Receiver, field.Name, p.Names.Argument, to+args, p.Names.Receiver)
				if bodies {
					copied := writeFieldComments(&out, field)
					if !field.Optional && step < len(requiredNames) {
						if copied {
							out.WriteString("//\n")
						}
						fmt.Fprintf(&out, "// Next required setter: %s\n", requiredNames[step])
					}
				}
				method(from+args, field.SetterName, p.Names.Argument+" "+typeText, to+args, body)
			}
			method(p.Names.Done+args, BuildMethod, "", target, "return *"+p.Names.Receiver+".root")
			body = fmt.Sprintf("%s := *%s.root; return &%s", p.Names.Result, p.Names.Receiver, p.Names.Result)
			method(p.Names.Done+args, BuildPtrMethod, "", "*"+target, body)
		}
		for _, field := range p.FieldsFor(Getters) {
			typeText, err := FormatType(field.TypeExpr, p.Bindings, aliases)
			if err != nil {
				return "", err
			}
			if bodies {
				writeFieldComments(&out, field)
			}
			method("*"+target, field.GetterName, "", typeText, "return "+p.Names.Receiver+"."+field.Name)
		}
		if p.Derive.Has(Equal) {
			method(target, "Equal", p.Names.Other+" "+target, "bool", equalityBody(p))
		}
		if p.Derive.Has(String) {
			method(target, "String", "", "string", stringBody(p))
		}
	}
	formatted, err := format.Source([]byte("package generated\n" + out.String()))
	if err != nil {
		return "", err
	}
	return strings.TrimPrefix(string(formatted), "package generated\n\n"), nil
}

func writeFieldComments(out *strings.Builder, field Field) bool {
	if field.AST == nil {
		return false
	}
	copied := false
	for _, group := range []*ast.CommentGroup{field.AST.Doc, field.AST.Comment} {
		if group == nil {
			continue
		}
		for _, comment := range group.List {
			if strings.HasPrefix(comment.Text, "//gobetter:") {
				continue
			}
			out.WriteString(comment.Text)
			out.WriteByte('\n')
			copied = true
		}
	}
	return copied
}
