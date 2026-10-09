// Copyright 2025 Toly Pochkin
// SPDX-License-Identifier: Apache-2.0

package structgen

import (
	"go/ast"
	"go/build/constraint"
	"go/token"
	"go/types"
)

// Outputs is a bit set of requested generated features.
type Outputs uint8

// Output flags select builders, equality, string formatting, and getters.
const (
	Builder Outputs = 1 << iota
	Equal
	String
	Getters
)

// AllOutputs contains every supported output flag.
const AllOutputs = Builder | Equal | String | Getters

// Has reports whether the requested output flags are set.
func (o Outputs) Has(output Outputs) bool { return o&output != 0 }

// ParseOutput recognizes a single output name.
func ParseOutput(s string) (Outputs, bool) {
	switch s {
	case "builder":
		return Builder, true
	case "equal":
		return Equal, true
	case "string":
		return String, true
	case "getters":
		return Getters, true
	default:
		return 0, false
	}
}

// Constructor selects the visibility of a generated builder constructor.
type Constructor string

// Constructor modes infer visibility or explicitly select it.
const (
	AutoConstructor     Constructor = "auto"
	ExportedConstructor Constructor = "exported"
	PackageConstructor  Constructor = "package"
)

// Selection determines which source declarations are generation targets.
type Selection string

// Selection modes choose annotated, exported, or all eligible declarations.
const (
	Annotated Selection = "annotated"
	Exported  Selection = "exported"
	All       Selection = "all"
)

// DeclarationKind distinguishes struct definitions, aliases, and other types.
type DeclarationKind uint8

// Declaration kinds identify the supported forms of source types.
const (
	DefinedStruct DeclarationKind = iota
	NestedAlias
	UserAlias
	OtherDeclaration
)

// TypeOptions holds output and constructor overrides; nil settings are omitted.
type TypeOptions struct {
	Derive      *Outputs
	Constructor *Constructor
}

// FieldOptions holds field overrides, distinguishing omission from explicit false.
type FieldOptions struct {
	Name        *string
	Getter      *bool
	Optional    *bool
	SkipBuilder *bool
	SkipEqual   *bool
	SkipString  *bool
}

// FieldOverride associates field settings with a source name and position.
type FieldOverride struct {
	Name     string
	Position token.Position
	Options  FieldOptions
}

// TypeOverride applies external settings to a source type and its fields.
type TypeOverride struct {
	PackageDir string
	Path       string // Source identity: Type or Outer.Config.Database; not an allocated alias.
	Position   token.Position
	Options    TypeOptions
	Fields     []FieldOverride
}

// Invocation holds declaration selection and explicit command-line overrides.
type Invocation struct {
	Selection Selection
	Options   TypeOptions // Explicit CLI overrides only; zero values are omitted.
}

// TypeParameter retains a source parameter and its constraint syntax. Loading
// resolves its Go object; name allocation assigns its generated binder.
type TypeParameter struct {
	Ident      *ast.Ident
	Constraint ast.Expr
	Object     *types.TypeParam
	Binder     string
}

// Field retains source syntax, resolved type information, and effective field settings.
type Field struct {
	Name        string
	Position    token.Position
	AST         *ast.Field
	TypeExpr    ast.Expr
	Type        types.Type
	Embedded    bool
	Annotated   bool
	Options     FieldOptions
	SetterName  string
	GetterName  string
	Optional    bool
	Getter      bool
	SkipBuilder bool
	SkipEqual   bool
	SkipString  bool
}

// Target describes a source declaration and its canonical field inventory.
type Target struct {
	PackageDir      string
	SourceFile      string
	Path            string
	Name            string // Empty for nested aliases until the alias allocator runs.
	ParentPath      string
	ParentField     string
	Position        token.Position
	Kind            DeclarationKind
	Exported        bool
	TypeSpec        *ast.TypeSpec
	StructType      *ast.StructType
	Object          *types.TypeName // Checked declaration/alias identity; nil before loading.
	ExistingMethods []Member        // Authored/out-of-scope methods, excluding masked output.
	TypeParameters  []TypeParameter
	Imports         []*ast.ImportSpec
	BuildConstraint constraint.Expr
	Annotated       bool
	Options         TypeOptions
	Fields          []Field // Canonical inventory in declaration order; excludes blanks.
}

// Plan holds effective output settings, allocated names, and resolved type bindings.
type Plan struct {
	Target      Target
	Derive      Outputs
	Constructor Constructor // Empty when no builder is requested.
	Fields      []Field     // Effective canonical inventory; views do not mutate it.
	Names       GeneratedNames
	Bindings    map[*ast.Ident]string // Only proven lexical type-parameter uses.
}

// GeneratedNames holds identifiers allocated for one target and its runtime helpers.
type GeneratedNames struct {
	Constructor  string
	Steps        []string
	Done         string
	Receiver     string
	Argument     string
	Result       string
	Other        string
	FmtImport    string
	SlicesImport string
	MapsImport   string
}

// Member identifies an existing method and its source position.
type Member struct {
	Name     string
	Position token.Position
}

// Rendered separates declarations from deliberate standard-library bindings.
// Package envelopes, source imports, finalization, and publication are caller-owned.
type Rendered struct {
	Code    string
	Imports []Import
}

// Import pairs an import path with its allocated local binding.
type Import struct{ Path, Name string }

// Generated identifier conventions define helper prefixes and builder method names.
const (
	StatePrefix         = "gobetter_"
	TypeParameterPrefix = "gobetterT"
	BuilderSuffix       = "Builder"
	BuildMethod         = "Build"
	BuildPtrMethod      = "BuildPtr"
)
