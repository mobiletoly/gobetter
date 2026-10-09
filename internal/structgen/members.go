// Copyright 2025 Toly Pochkin
// SPDX-License-Identifier: Apache-2.0

package structgen

import (
	"fmt"
	"go/token"
)

// ValidateMembers checks declaration-level conflicts before bootstrap emission.
// Promoted members may be shadowed under normal Go rules; direct members may not.
func ValidateMembers(plans []Plan) error {
	var ds Diagnostics
	for _, p := range plans {
		members := make(map[string]token.Position)
		for _, field := range p.Target.Fields {
			members[field.Name] = field.Position
		}
		for _, method := range p.Target.ExistingMethods {
			members[method.Name] = method.Position
		}
		claim := func(name string, pos token.Position) {
			if conflict, exists := members[name]; exists {
				ds = append(ds, Diagnostic{Package: p.Target.PackageDir, Position: pos, Code: SymbolCollision, Message: "generated method " + name + " conflicts with member at " + conflict.String()})
			} else {
				members[name] = pos
			}
		}
		for _, field := range p.FieldsFor(Getters) {
			claim(field.GetterName, field.Position)
		}
		if p.Derive.Has(Equal) {
			claim("Equal", p.Target.Position)
		}
		if p.Derive.Has(String) {
			claim("String", p.Target.Position)
		}
		ds = append(ds, validateBuilderMethods(p)...)
	}
	if len(ds) > 0 {
		return ds.Sorted()
	}
	return nil
}

// Setter names identify one source field throughout a builder, even across
// stages. Optional setters reserve build-method names in every window.
func validateBuilderMethods(p Plan) Diagnostics {
	var ds Diagnostics
	setters := make(map[string]Field)
	for _, field := range p.FieldsFor(Builder) {
		name := field.SetterName
		if name == "" {
			continue // A missing/invalid name is diagnosed during normalization.
		}
		if field.Optional && (name == BuildMethod || name == BuildPtrMethod) {
			ds = append(ds, Diagnostic{Package: p.Target.PackageDir, Position: field.Position, Code: SymbolCollision, Message: "optional setter " + name + " for field " + field.Name + " uses a reserved build-method name; supply an explicit name"})
		}
		if first, exists := setters[name]; exists {
			ds = append(ds, Diagnostic{Package: p.Target.PackageDir, Position: field.Position, Code: SymbolCollision, Message: fmt.Sprintf("builder setter %s for field %s conflicts with field %s at %s; effective setter names must be unique", name, field.Name, first.Name, first.Position)})
		} else {
			setters[name] = field
		}
	}
	return ds
}
