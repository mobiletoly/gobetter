// Copyright 2025 Toly Pochkin
// SPDX-License-Identifier: Apache-2.0

package structgen

import (
	"go/ast"
	"go/token"
	"slices"
)

// MakePlans consumes an ordered declaration inventory and already-bound config
// records. Nested declarations follow their parents. It resolves annotation,
// configuration, and explicit invocation settings, with no rendering or I/O.
func MakePlans(targets []Target, overrides []TypeOverride, invocation Invocation) ([]Plan, error) {
	var ds Diagnostics
	if invocation.Selection == "" {
		invocation.Selection = Annotated
	}
	if invocation.Selection != Annotated && invocation.Selection != Exported && invocation.Selection != All {
		ds = append(ds, Diagnostic{Code: InvalidOption, Message: "generate-for must be annotated, exported, or all"})
	}
	ds = append(ds, ValidateTypeOptions(invocation.Options, token.Position{})...)
	var plans []Plan
	for _, target := range targets {
		var external *TypeOverride
		for i := range overrides {
			if overrides[i].PackageDir == target.PackageDir && overrides[i].Path == target.Path {
				if external != nil {
					ds = append(ds, Diagnostic{Position: overrides[i].Position, Code: ConfigDuplicate, Message: "duplicate type configuration for " + target.Path})
				}
				external = &overrides[i]
			}
		}
		byParent := false
		if target.Kind == NestedAlias {
			for _, parent := range plans {
				if parent.Target.PackageDir == target.PackageDir && parent.Target.Path == target.ParentPath && parent.Derive.Has(Builder) {
					for _, field := range parent.Fields {
						if field.Name == target.ParentField && !field.SkipBuilder {
							byParent = true
						}
					}
				}
			}
		}
		plan, selected, errors := normalize(target, external, invocation, byParent)
		for i := range errors {
			errors[i].Package = target.PackageDir
		}
		ds = append(ds, errors...)
		if selected {
			plans = append(plans, plan)
		}
	}
	if len(ds) != 0 {
		return nil, ds.Sorted()
	}
	return plans, nil
}

func normalize(target Target, external *TypeOverride, invocation Invocation, byParent bool) (Plan, bool, Diagnostics) {
	var ds Diagnostics
	add := func(pos token.Position, code Code, message string) {
		ds = append(ds, Diagnostic{Package: target.PackageDir, Position: pos, Code: code, Message: message})
	}
	ds = append(ds, ValidateTypeOptions(target.Options, target.Position)...)
	for _, field := range target.Fields {
		ds = append(ds, ValidateFieldOptions(field.Options, field.Position)...)
	}
	selected := target.Annotated || external != nil || byParent || target.Kind == DefinedStruct && (invocation.Selection == All || invocation.Selection == Exported && target.Exported)
	if !selected {
		for _, field := range target.Fields {
			if field.Annotated {
				add(field.Position, UnusedFieldDirective, "field directive requires a selected containing struct; add a struct directive or select it through configuration/generate-for")
			}
		}
		return Plan{}, false, ds
	}
	if target.Kind != DefinedStruct && target.Kind != NestedAlias {
		add(target.Position, UnsupportedTarget, "generation targets defined structs or gobetter nested aliases; use a named defined struct")
		return Plan{}, true, ds
	}
	defaults := TypeOptions{Derive: new(Builder), Constructor: new(AutoConstructor)}
	options := mergeType(defaults, target.Options)
	if external != nil {
		ds = append(ds, ValidateTypeOptions(external.Options, external.Position)...)
		options = mergeType(options, external.Options)
	}
	if target.Kind == DefinedStruct {
		options = mergeType(options, invocation.Options)
	} else {
		// Global output overrides apply to defined structs, not unnamed aliases.
		options = mergeType(options, TypeOptions{Constructor: invocation.Options.Constructor})
		if *options.Derive != Builder {
			add(target.Position, UnsupportedTarget, "nested aliases support builders only; use a named defined struct for getters, Equal, or String")
		}
	}
	plan := Plan{Target: target, Derive: *options.Derive, Fields: slices.Clone(target.Fields)}
	if plan.Derive.Has(Builder) {
		plan.Constructor = *options.Constructor
		if plan.Constructor == AutoConstructor {
			if target.Exported {
				plan.Constructor = ExportedConstructor
			} else {
				plan.Constructor = PackageConstructor
			}
		}
	}
	if external != nil {
		for _, override := range external.Fields {
			ds = append(ds, ValidateFieldOptions(override.Options, override.Position)...)
			found := false
			for i := range plan.Fields {
				if plan.Fields[i].Name == override.Name {
					plan.Fields[i].Options = mergeField(plan.Fields[i].Options, override.Options)
					found = true
				}
			}
			if !found {
				add(override.Position, ConfigTarget, "unknown field "+override.Name+" of "+target.Path)
			}
		}
	}
	getters := 0
	for i := range plan.Fields {
		field := &plan.Fields[i]
		o := field.Options
		field.Optional, field.Getter = enabled(o.Optional), enabled(o.Getter)
		field.SkipBuilder, field.SkipEqual, field.SkipString = enabled(o.SkipBuilder), enabled(o.SkipEqual), enabled(o.SkipString)
		if field.Optional && field.SkipBuilder {
			add(field.Position, InvalidOption, "optional and skip-builder conflict; clear one setting")
		}
		if field.Getter {
			getters++
			if target.Kind == NestedAlias {
				add(field.Position, UnsupportedTarget, "getters cannot be attached to a nested alias; use a named defined struct")
			}
			if ast.IsExported(field.Name) {
				add(field.Position, InvalidOption, "getters are supported only for private fields")
			}
		}
		setter := plan.Derive.Has(Builder) && !field.SkipBuilder
		if !setter && !field.Getter {
			continue
		}
		name := methodName(field.Name)
		if o.Name != nil {
			name = *o.Name
		}
		if !token.IsIdentifier(name) || !ast.IsExported(name) {
			add(field.Position, InvalidMethodName, "requested setter/getter for "+field.Name+" requires an exported Go identifier; supply an explicit name")
		}
		if setter {
			field.SetterName = name
		}
		if field.Getter {
			field.GetterName = name
		}
	}
	if plan.Derive == Getters && getters == 0 {
		add(target.Position, EmptyGetters, "getters-only target requires at least one explicit field getter")
	}
	ds = append(ds, validateBuilderMethods(plan)...)
	return plan, true, ds
}

// FieldsFor derives ordered views from the canonical inventory. Getters remain
// independent of other output lists; optional fields stay in the builder view.
func (p Plan) FieldsFor(output Outputs) []Field {
	var fields []Field
	for _, field := range p.Fields {
		include := false
		switch output {
		case Builder:
			include = p.Derive.Has(Builder) && !field.SkipBuilder
		case Equal:
			include = p.Derive.Has(Equal) && !field.SkipEqual
		case String:
			include = p.Derive.Has(String) && !field.SkipString
		case Getters:
			include = field.Getter
		}
		if include {
			fields = append(fields, field)
		}
	}
	return fields
}
