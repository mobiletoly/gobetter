// Copyright 2025 Toly Pochkin
// SPDX-License-Identifier: Apache-2.0

package structgen

import (
	"go/token"
	"unicode"
	"unicode/utf8"
)

// ValidateTypeOptions checks output flags and constructor visibility settings.
func ValidateTypeOptions(o TypeOptions, pos token.Position) Diagnostics {
	var ds Diagnostics
	add := func(message string) {
		ds = append(ds, Diagnostic{Position: pos, Code: InvalidOption, Message: message})
	}
	if o.Derive != nil && (*o.Derive == 0 || *o.Derive & ^AllOutputs != 0) {
		add("derive must be a nonempty subset of builder,equal,string,getters")
	}
	if o.Constructor != nil && *o.Constructor != AutoConstructor && *o.Constructor != ExportedConstructor && *o.Constructor != PackageConstructor {
		add("constructor must be auto, exported, or package")
	}
	return ds
}

// ValidateFieldOptions checks explicit field method names.
func ValidateFieldOptions(o FieldOptions, pos token.Position) Diagnostics {
	if o.Name != nil && (!token.IsIdentifier(*o.Name) || *o.Name == "_") {
		return Diagnostics{{Position: pos, Code: InvalidOption, Message: "name must be a Go identifier other than _ or a keyword"}}
	}
	return nil
}

func mergeType(a, b TypeOptions) TypeOptions {
	if b.Derive != nil {
		a.Derive = b.Derive
	}
	if b.Constructor != nil {
		a.Constructor = b.Constructor
	}
	return a
}
func mergeField(a, b FieldOptions) FieldOptions {
	if b.Name != nil {
		a.Name = b.Name
	}
	if b.Getter != nil {
		a.Getter = b.Getter
	}
	if b.Optional != nil {
		a.Optional = b.Optional
	}
	if b.SkipBuilder != nil {
		a.SkipBuilder = b.SkipBuilder
	}
	if b.SkipEqual != nil {
		a.SkipEqual = b.SkipEqual
	}
	if b.SkipString != nil {
		a.SkipString = b.SkipString
	}
	return a
}
func enabled(v *bool) bool { return v != nil && *v }
func methodName(name string) string {
	r, size := utf8.DecodeRuneInString(name)
	return string(unicode.ToUpper(r)) + name[size:]
}
