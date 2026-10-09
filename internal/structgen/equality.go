// Copyright 2025 Toly Pochkin
// SPDX-License-Identifier: Apache-2.0

package structgen

import (
	"fmt"
	"go/types"
	"strconv"
	"strings"
)

type comparison uint8

const (
	directComparison comparison = iota
	sliceComparison
	mapComparison
)

func equalityComparison(t types.Type) (comparison, bool) {
	if t == nil {
		return 0, false
	}
	t = types.Unalias(t)
	if parameter, ok := t.(*types.TypeParam); ok {
		set, ok := constraintTerms(parameter.Constraint(), make(map[types.Type]bool))
		if !ok || !set.all && len(set.terms) == 0 {
			return 0, false
		}
		if types.Comparable(parameter) {
			return directComparison, true
		}
		t = collectionConstraint(set)
		if t == nil {
			return 0, false
		}
		// Method filtering can narrow the semantic set beyond the structural
		// terms Go uses for generic calls. Require the library's ~collection
		// constraint too, so accepted output still type-checks on the Go pin.
		libraryConstraint := types.NewInterfaceType(nil, []types.Type{types.NewUnion([]*types.Term{types.NewTerm(true, t)})}).Complete()
		if !types.Satisfies(parameter, libraryConstraint) {
			return 0, false
		}
	}
	switch underlying := t.Underlying().(type) {
	case *types.Slice:
		return sliceComparison, safeComparable(underlying.Elem())
	case *types.Map:
		return mapComparison, safeComparable(underlying.Elem())
	default:
		return directComparison, safeComparable(t)
	}
}

// Concrete interface values, including those within value aggregates, are
// excluded. Type parameters follow normal Go comparison semantics, which can
// panic for interface instantiations. Pointers/channels compare by identity.
func safeComparable(t types.Type) bool {
	t = types.Unalias(t)
	if parameter, ok := t.(*types.TypeParam); ok {
		return types.Comparable(parameter)
	}
	if !types.Comparable(t) {
		return false
	}
	switch value := t.Underlying().(type) {
	case *types.Interface:
		return false
	case *types.Array:
		return safeComparable(value.Elem())
	case *types.Struct:
		for field := range value.Fields() {
			if !safeComparable(field.Type()) {
				return false
			}
		}
	}
	return true
}

type constraintTerm struct {
	typeOf types.Type
	tilde  bool
}

// all distinguishes an unrestricted set from a finite (possibly empty) union.
// Required methods filter exact terms; tilde terms may have named implementations.
type constraintSet struct {
	all        bool
	comparable bool
	terms      []constraintTerm
}

// Expand unions and intersect interface elements before selecting an operation.
// Keep exact named types distinct from terms describing an underlying type.
func constraintTerms(t types.Type, visiting map[types.Type]bool) (constraintSet, bool) {
	t = types.Unalias(t)
	if visiting[t] {
		return constraintSet{}, false
	}
	visiting[t] = true
	defer delete(visiting, t)
	switch value := t.Underlying().(type) {
	case *types.Interface:
		set := constraintSet{all: true, comparable: value.IsComparable()}
		for embedded := range value.EmbeddedTypes() {
			part, ok := constraintTerms(embedded, visiting)
			if !ok {
				return constraintSet{}, false
			}
			set = intersectConstraints(set, part)
		}
		if !set.all {
			var eligible []constraintTerm
			for _, term := range set.terms {
				if set.comparable && !types.Comparable(term.typeOf) {
					continue
				}
				if !term.tilde && value.NumMethods() > 0 && !types.Implements(term.typeOf, value) {
					continue
				}
				eligible = append(eligible, term)
			}
			set.terms = eligible
		}
		return set, true
	case *types.Union:
		var set constraintSet
		for term := range value.Terms() {
			if term.Tilde() {
				set.terms = appendConstraintTerm(set.terms, constraintTerm{types.Unalias(term.Type()), true})
				continue
			}
			part, ok := constraintTerms(term.Type(), visiting)
			if !ok {
				return constraintSet{}, false
			}
			if part.all {
				return part, true
			}
			for _, item := range part.terms {
				set.terms = appendConstraintTerm(set.terms, item)
			}
		}
		return set, true
	default:
		return constraintSet{terms: []constraintTerm{{typeOf: t}}}, true
	}
}

func (t constraintTerm) contains(other constraintTerm) bool {
	if t.tilde {
		return types.Identical(t.typeOf, other.typeOf.Underlying())
	}
	return !other.tilde && types.Identical(t.typeOf, other.typeOf)
}

func appendConstraintTerm(terms []constraintTerm, term constraintTerm) []constraintTerm {
	for _, existing := range terms {
		if existing.contains(term) {
			return terms
		}
	}
	kept := terms[:0]
	for _, existing := range terms {
		if !term.contains(existing) {
			kept = append(kept, existing)
		}
	}
	return append(kept, term)
}

func intersectConstraints(left, right constraintSet) constraintSet {
	if left.all {
		right.comparable = right.comparable || left.comparable
		return right
	}
	if right.all {
		left.comparable = left.comparable || right.comparable
		return left
	}
	result := constraintSet{comparable: left.comparable || right.comparable}
	for _, a := range left.terms {
		for _, b := range right.terms {
			switch {
			case a.contains(b):
				result.terms = appendConstraintTerm(result.terms, b)
			case b.contains(a):
				result.terms = appendConstraintTerm(result.terms, a)
			}
		}
	}
	return result
}

func collectionConstraint(set constraintSet) types.Type {
	if set.all || len(set.terms) == 0 {
		return nil
	}
	var collection types.Type
	for _, term := range set.terms {
		candidate := term.typeOf.Underlying()
		switch candidate.(type) {
		case *types.Slice, *types.Map:
		default:
			return nil
		}
		if collection != nil && !types.Identical(collection, candidate) {
			return nil
		}
		collection = candidate
	}
	return collection
}

func validateEquality(plans []Plan) error {
	var ds Diagnostics
	for _, p := range plans {
		for _, field := range p.FieldsFor(Equal) {
			if _, ok := equalityComparison(field.Type); !ok {
				ds = append(ds, Diagnostic{Package: p.Target.PackageDir, Position: field.Position, Code: UnsupportedEquality, Message: "field " + field.Name + " does not support shallow equality; use skip-equal or a supported comparable/collection type"})
			}
		}
	}
	if len(ds) > 0 {
		return ds.Sorted()
	}
	return nil
}

func equalityBody(p Plan) string {
	var parts []string
	for _, field := range p.FieldsFor(Equal) {
		left, right := p.Names.Receiver+"."+field.Name, p.Names.Other+"."+field.Name
		kind, _ := equalityComparison(field.Type)
		switch kind {
		case sliceComparison:
			parts = append(parts, fmt.Sprintf("%s.Equal(%s, %s)", p.Names.SlicesImport, left, right))
		case mapComparison:
			parts = append(parts, fmt.Sprintf("%s.Equal(%s, %s)", p.Names.MapsImport, left, right))
		default:
			parts = append(parts, left+" == "+right)
		}
	}
	if len(parts) == 0 {
		return "return 0 == 0"
	}
	return "return " + strings.Join(parts, " && ")
}

func stringBody(p Plan) string {
	fields := p.FieldsFor(String)
	if len(fields) == 0 {
		return "return " + strconv.Quote(p.Target.Name+"{}")
	}
	var formats, args []string
	for _, field := range fields {
		formats = append(formats, field.Name+":%#v")
		args = append(args, p.Names.Receiver+"."+field.Name)
	}
	return fmt.Sprintf("return %s.Sprintf(%s, %s)", p.Names.FmtImport, strconv.Quote(p.Target.Name+"{"+strings.Join(formats, ", ")+"}"), strings.Join(args, ", "))
}

func runtimeImports(plans []Plan) []Import {
	used := make(map[string]string)
	for _, p := range plans {
		if p.Derive.Has(String) && len(p.FieldsFor(String)) > 0 {
			used["fmt"] = p.Names.FmtImport
		}
		for _, field := range p.FieldsFor(Equal) {
			kind, _ := equalityComparison(field.Type)
			if kind == sliceComparison {
				used["slices"] = p.Names.SlicesImport
			}
			if kind == mapComparison {
				used["maps"] = p.Names.MapsImport
			}
		}
	}
	var imports []Import
	for _, path := range []string{"fmt", "maps", "slices"} {
		if name, ok := used[path]; ok {
			imports = append(imports, Import{Path: path, Name: name})
		}
	}
	return imports
}
