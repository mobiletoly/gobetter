// Copyright 2025 Toly Pochkin
// SPDX-License-Identifier: Apache-2.0

package structgen

import (
	"cmp"
	"fmt"
	"go/token"
	"slices"
	"strings"
)

// Code is a stable diagnostic identity independent of explanatory wording.
type Code string

// Diagnostic codes identify source, configuration, binding, and artifact failures.
const (
	SourceSyntax          Code = "source_syntax"
	DirectiveSyntax       Code = "directive_syntax"
	DirectivePlacement    Code = "directive_placement"
	DirectiveDuplicate    Code = "directive_duplicate"
	LegacyDirective       Code = "legacy_directive"
	UnsupportedTarget     Code = "unsupported_target"
	UnusedFieldDirective  Code = "unused_field_directive"
	InvalidOption         Code = "invalid_option"
	InvalidMethodName     Code = "invalid_method_name"
	EmptyGetters          Code = "empty_getters"
	ConfigSyntax          Code = "config_syntax"
	ConfigDuplicate       Code = "config_duplicate"
	ConfigSchema          Code = "config_schema"
	ConfigTarget          Code = "config_target"
	PackageLoad           Code = "package_load"
	BuildContext          Code = "build_context"
	OutputOwnership       Code = "output_ownership"
	SymbolCollision       Code = "symbol_collision"
	BindingMismatch       Code = "binding_mismatch"
	UnresolvedPlan        Code = "unresolved_plan"
	UnsupportedOutput     Code = "unsupported_output"
	UnsupportedEquality   Code = "unsupported_equality"
	UnsupportedCSignature Code = "unsupported_c_signature"
	ConsumerGoVersion     Code = "consumer_go_version"
	ArtifactDrift         Code = "artifact_drift"
	ArtifactIO            Code = "artifact_io"
)

// Diagnostic describes a failure with a stable code and physical source position.
type Diagnostic struct {
	Package  string
	Position token.Position
	Code     Code
	Message  string
}

func (d Diagnostic) Error() string {
	return fmt.Sprintf("%s: %s: %s", d.Position, d.Code, d.Message)
}

// Diagnostics is sorted by package, physical source position, code, and message.
type Diagnostics []Diagnostic

func (ds Diagnostics) Error() string {
	lines := make([]string, len(ds))
	for i, d := range ds {
		lines[i] = d.Error()
	}
	return strings.Join(lines, "\n")
}

// Sorted returns a sorted copy without modifying the original diagnostics.
func (ds Diagnostics) Sorted() Diagnostics {
	sorted := slices.Clone(ds)
	slices.SortFunc(sorted, func(a, b Diagnostic) int {
		return cmp.Or(cmp.Compare(a.Package, b.Package), cmp.Compare(a.Position.Filename, b.Position.Filename),
			cmp.Compare(a.Position.Line, b.Position.Line), cmp.Compare(a.Position.Column, b.Position.Column),
			cmp.Compare(a.Code, b.Code), cmp.Compare(a.Message, b.Message))
	})
	return sorted
}
