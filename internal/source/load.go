// Copyright 2025 Toly Pochkin
// SPDX-License-Identifier: Apache-2.0

package source

import (
	"bytes"
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/mobiletoly/gobetter/internal/config"
	"github.com/mobiletoly/gobetter/internal/structgen"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/imports"
)

// Result contains discovered packages, resolved plans, and signature overlays.
type Result struct {
	Packages    []*Package
	Plans       []structgen.Plan
	Bootstrap   map[string][]byte // Logical signature overlay; never publish these bytes.
	resolved    []*packages.Package
	discovery   *discovery
	baseOverlay map[string][]byte
}

type packagePlan struct {
	pkg     *Package
	plans   []structgen.Plan
	symbols []string
	files   []methodFile // Ordinary active and retained artifact ASTs only.
	uses    map[*ast.Ident]*ast.Ident
	exports map[string]bool
}

// Load discovers all selected packages, builds one combined signature overlay,
// and resolves them together. It writes no source, output, or module files.
func Load(ctx context.Context, request Request) (*Result, error) {
	d, err := discover(ctx, request)
	if err != nil {
		return nil, err
	}
	result := &Result{Packages: d.Packages, discovery: d, Bootstrap: make(map[string][]byte), baseOverlay: make(map[string][]byte)}
	planned := make(map[string][]structgen.Plan)
	effectiveExports := make(map[string]dependencyInventory)
	var inputs []*packagePlan
	for _, pkg := range d.Packages {
		// Register every selected package before collecting or allocating imports.
		effectiveExports[pkg.Path] = dependencyInventory{name: pkg.Name}
		var targets []structgen.Target
		var symbols []string
		var methodFiles []methodFile
		uses := make(map[*ast.Ident]*ast.Ident)
		exports := make(map[string]bool)
		addDeclarations := func(file *ast.File) {
			declared := declaredSymbols(file)
			symbols = append(symbols, declared...)
			for _, name := range declared {
				if ast.IsExported(name) {
					exports[name] = true
				}
			}
		}
		for _, file := range pkg.Files {
			addDeclarations(file.Parsed.AST)
			methodFiles = append(methodFiles, methodFile{file.Parsed.AST, d.Fset})
			if file.ContextOnly || d.Request.File != "" && file.Path != d.Request.File {
				continue
			}
			targets = append(targets, file.Parsed.Targets...)
			bindings, err := lexicalBindings(file)
			if err != nil {
				return nil, err
			}
			maps.Copy(uses, bindings)
		}
		for _, artifact := range pkg.Artifacts {
			if !pkg.activeFiles[artifact.Path] {
				continue
			}
			sourceActive := false
			for _, file := range pkg.Files {
				if file.Path == artifact.Source {
					sourceActive = true
				}
			}
			_, sourceErr := os.Stat(artifact.Source)
			inactiveSource := sourceErr == nil && !sourceActive
			if inactiveSource || d.Request.File != "" && artifact.Source != d.Request.File {
				data, err := os.ReadFile(artifact.Path)
				if err != nil {
					return nil, err
				}
				fset := token.NewFileSet()
				file, err := parser.ParseFile(fset, artifact.Path, data, parser.SkipObjectResolution)
				if err != nil {
					return nil, err
				}
				addDeclarations(file)
				methodFiles = append(methodFiles, methodFile{file, fset})
				continue
			}
			result.baseOverlay[artifact.Path] = []byte("package " + pkg.Name + "\n")
		}
		declaredMethods := methods(methodFiles...)
		for i := range targets {
			targets[i].ExistingMethods = declaredMethods[targets[i].Name]
		}
		plans, err := structgen.MakePlans(targets, d.Overrides, d.Request.Invocation)
		if err != nil {
			return nil, err
		}
		inputs = append(inputs, &packagePlan{pkg, plans, symbols, methodFiles, uses, exports})
	}
	ordered, err := selectedDependencyOrder(inputs)
	if err != nil {
		return nil, err
	}
	for _, input := range ordered {
		pkg := input.pkg
		symbols := input.symbols
		for _, file := range input.files {
			bindings, err := importSymbols(file, pkg.metadata, effectiveExports)
			if err != nil {
				return nil, err
			}
			symbols = append(symbols, bindings...)
		}
		plans, err := structgen.AllocateNames(input.plans, symbols, input.uses)
		if err != nil {
			return nil, err
		}
		if err := structgen.ValidateMembers(plans); err != nil {
			return nil, err
		}
		planned[pkg.Path] = plans
		exports := input.exports
		for _, plan := range plans {
			if ast.IsExported(plan.Target.Name) {
				exports[plan.Target.Name] = true
			}
			if ast.IsExported(plan.Names.Constructor) {
				exports[plan.Names.Constructor] = true
			}
		}
		effectiveExports[pkg.Path] = dependencyInventory{name: pkg.Name, exports: exports}
	}
	// Preserve result plan ordering while allocating dependency names first.
	for _, pkg := range d.Packages {
		result.Plans = append(result.Plans, planned[pkg.Path]...)
	}
	for _, pkg := range d.Packages {
		plans := planned[pkg.Path]
		if err := validateCSignatures(plans); err != nil {
			return nil, err
		}
		for _, file := range pkg.Files {
			var selected []structgen.Plan
			for _, plan := range plans {
				if plan.Target.SourceFile == file.Path {
					selected = append(selected, plan)
				}
			}
			if len(selected) == 0 {
				continue
			}
			declarations, err := structgen.Signatures(selected)
			if err != nil {
				return nil, err
			}
			var code strings.Builder
			if file.Constraint != nil {
				fmt.Fprintf(&code, "//go:build %s\n\n", file.Constraint)
			}
			fmt.Fprintf(&code, "%s\n%sv0.20.0\n%s%s\n\npackage %s\n", GeneratedHeader, VersionMetadata, SourceMetadata, filepath.Base(file.Path), pkg.Name)
			if len(file.Parsed.AST.Imports) > 0 {
				code.WriteString("import (\n")
				for _, spec := range file.Parsed.AST.Imports {
					if spec.Path.Value == `"C"` {
						continue
					}
					if spec.Name != nil {
						code.WriteString(spec.Name.Name + " ")
					} else {
						path, _ := strconv.Unquote(spec.Path.Value)
						if name := dependencyPackageName(pkg.metadata, path, effectiveExports); name != "" {
							code.WriteString(name + " ")
						}
					}
					code.WriteString(spec.Path.Value + "\n")
				}
				code.WriteString(")\n")
			}
			code.WriteString(declarations)
			path := OutputFilename(file.Path)
			if info, err := os.Lstat(path); err == nil {
				if !info.Mode().IsRegular() {
					return nil, sourceError(path, structgen.OutputOwnership, "refusing non-regular output or symlink")
				}
			} else if !os.IsNotExist(err) {
				return nil, err
			}
			if data, err := os.ReadFile(path); err == nil {
				_, owned, ownershipErr := inspectArtifact(path, data)
				if ownershipErr != nil {
					return nil, ownershipErr
				}
				if !owned {
					return nil, sourceError(path, structgen.OutputOwnership, "refusing an existing non-owned output path")
				}
			} else if !os.IsNotExist(err) {
				return nil, err
			}
			withBindings, err := signatureImports(pkg, []byte(code.String()), effectiveExports)
			if err != nil {
				return nil, err
			}
			processed, err := imports.Process(path, withBindings, &imports.Options{Comments: true})
			if err != nil {
				return nil, sourceError(file.Path, structgen.PackageLoad, "cannot format bootstrap: "+err.Error())
			}
			result.Bootstrap[path] = processed
		}
	}
	overlay := cloneOverlay(result.baseOverlay)
	maps.Copy(overlay, result.Bootstrap)
	loaded, err := result.load(ctx, overlay)
	if err != nil {
		return nil, err
	}
	result.resolved = loaded
	if err := result.resolvePlans(); err != nil {
		return nil, err
	}
	if err := result.validateSynthesizedSignatures(); err != nil {
		return nil, err
	}
	return result, nil
}

// Naming needs the effective exports of selected imports, before any combined
// type load. Read actual retained AST imports: metadata can prune disk cycles
// caused by stale files that the effective overlay masks. Sort roots/edges.
func selectedDependencyOrder(selected []*packagePlan) ([]*packagePlan, error) {
	byPath := make(map[string]*packagePlan)
	var paths []string
	for _, input := range selected {
		byPath[input.pkg.Path] = input
		paths = append(paths, input.pkg.Path)
	}
	slices.Sort(paths)
	state := make(map[string]uint8)
	var ordered []*packagePlan
	var stack []string
	var visit func(string) error
	visit = func(path string) error {
		input := byPath[path]
		if state[path] == 2 {
			return nil
		}
		state[path] = 1
		stack = append(stack, path)
		edges := make(map[string]token.Position)
		for _, file := range input.files {
			for _, spec := range file.file.Imports {
				imported, _ := strconv.Unquote(spec.Path.Value)
				if byPath[imported] != nil {
					if _, exists := edges[imported]; !exists {
						edges[imported] = file.fset.PositionFor(spec.Pos(), false)
					}
				}
			}
		}
		var dependencies []string
		for imported := range edges {
			dependencies = append(dependencies, imported)
		}
		slices.Sort(dependencies)
		for _, imported := range dependencies {
			if state[imported] == 1 {
				cycle := append(slices.Clone(stack[slices.Index(stack, imported):]), imported)
				return structgen.Diagnostics{{Package: input.pkg.Dir, Position: edges[imported], Code: structgen.PackageLoad, Message: "import cycle not allowed: " + strings.Join(cycle, " -> ")}}
			}
			if err := visit(imported); err != nil {
				return err
			}
		}
		state[path] = 2
		stack = stack[:len(stack)-1]
		ordered = append(ordered, input)
		return nil
	}
	for _, path := range paths {
		if err := visit(path); err != nil {
			return nil, err
		}
	}
	return ordered, nil
}

// Validate replaces all signatures with caller-rendered final bytes in one
// combined overlay, leaving out-of-scope artifacts intact. It does not publish.
func (r *Result) Validate(ctx context.Context, final map[string][]byte) error {
	if len(final) != len(r.Bootstrap) {
		return fmt.Errorf("final artifact set must exactly replace the bootstrap set")
	}
	overlay := cloneOverlay(r.baseOverlay)
	for path := range r.Bootstrap {
		data, ok := final[path]
		if !ok {
			return fmt.Errorf("missing final artifact %s", path)
		}
		overlay[path] = data
	}
	_, err := r.load(ctx, overlay)
	return err
}

func (d *discovery) loaderConfig(ctx context.Context, mode packages.LoadMode, overlay map[string][]byte) *packages.Config {
	env := os.Environ()
	replace := func(key, value string) {
		prefix := key + "="
		env = slices.DeleteFunc(env, func(item string) bool { return strings.HasPrefix(item, prefix) })
		env = append(env, prefix+value)
	}
	replace("GOOS", d.Context.GOOS)
	replace("GOARCH", d.Context.GOARCH)
	if d.Context.CgoEnabled {
		replace("CGO_ENABLED", "1")
	} else {
		replace("CGO_ENABLED", "0")
	}
	flags := []string{"-mod=readonly"}
	if len(d.Request.Tags) > 0 {
		flags = append(flags, "-tags="+strings.Join(d.Request.Tags, ","))
	}
	cfg := &packages.Config{Context: ctx, Dir: d.Request.Dir, Mode: mode, Env: env, BuildFlags: flags, Fset: d.Fset, Overlay: overlay}
	cfg.ParseFile = func(fset *token.FileSet, filename string, data []byte) (*ast.File, error) {
		for _, pkg := range d.Packages {
			for _, file := range pkg.Files {
				if filename == file.Path && bytes.Equal(data, file.Content) {
					return file.Parsed.AST, nil
				}
			}
		}
		return parser.ParseFile(fset, filename, data, parser.ParseComments|parser.SkipObjectResolution)
	}
	return cfg
}

func (r *Result) load(ctx context.Context, overlay map[string][]byte) ([]*packages.Package, error) {
	mode := packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedImports | packages.NeedDeps | packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo | packages.NeedModule
	patterns := make([]string, len(r.Packages))
	for i, pkg := range r.Packages {
		patterns[i] = pkg.Dir
	}
	loaded, err := packages.Load(r.discovery.loaderConfig(ctx, mode, overlay), patterns...)
	if err != nil {
		return nil, err
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	var errors []string
	packages.Visit(loaded, nil, func(pkg *packages.Package) {
		// TypeErrors retain actual token positions. Displayed //line positions
		// are not identities: distinct files can report the same position/message.
		type origin struct {
			fset    *token.FileSet
			pos     token.Pos
			message string
		}
		seen := make(map[origin]bool)
		summaries := make(map[packages.Error]int)
		for _, typeError := range pkg.TypeErrors {
			err := packages.Error{Pos: typeError.Fset.Position(typeError.Pos).String(), Msg: typeError.Msg, Kind: packages.TypeError}
			summaries[err]++
			key := origin{typeError.Fset, typeError.Pos, typeError.Msg}
			if typeError.Pos.IsValid() && seen[key] {
				continue
			}
			seen[key] = true
			message := err.Error()
			physical := typeError.Fset.PositionFor(typeError.Pos, false)
			if physical.IsValid() && physical.String() != err.Pos {
				message += "\nphysical source: " + physical.String()
			}
			dependency := ""
			if r.generatedDependencyError(pkg, err) {
				dependency = pkg.PkgPath
			} else {
				dependency = r.importedGeneratedDependency(pkg, typeError)
			}
			if dependency != "" {
				message += "\nmissing/stale generated dependency " + dependency + ": run the affected dependency's own go:generate directive first, preserving its local configuration/flags, then rerun the dependent package; or select both packages under one equivalent configuration"
			}
			errors = append(errors, message)
		}
		// Consume only the counted summaries already represented above. Other
		// loader diagnostics retain their original text, including repetitions.
		for _, err := range pkg.Errors {
			if summaries[err] > 0 {
				summaries[err]--
				continue
			}
			errors = append(errors, err.Error())
		}
	})
	slices.Sort(errors)
	if len(errors) > 0 {
		return nil, fmt.Errorf("%s", strings.Join(errors, "\n"))
	}
	return loaded, nil
}

// Error ownership belongs to the caller, while API ownership belongs to the
// imported package. Match an actual selector at the typed-error position and
// use its resolved import object; spelling alone does not identify a dependency.
func (r *Result) importedGeneratedDependency(pkg *packages.Package, err types.Error) string {
	if !strings.HasPrefix(err.Msg, "undefined: ") || pkg.TypesInfo == nil {
		return ""
	}
	var dependency string
	for _, file := range pkg.Syntax {
		ast.Inspect(file, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok || err.Pos < selector.Pos() || err.Pos >= selector.End() {
				return true
			}
			qualifier, ok := selector.X.(*ast.Ident)
			if !ok {
				return true
			}
			binding, ok := pkg.TypesInfo.Uses[qualifier].(*types.PkgName)
			if !ok {
				return true
			}
			path := binding.Imported().Path()
			for _, selected := range r.Packages {
				if selected.Path == path {
					return true
				}
			}
			imported := pkg.Imports[path]
			if imported == nil {
				return true
			}
			// Regeneration cannot make a package-private constructor callable
			// from an importing package. Keep its original diagnostic instead.
			if ast.IsExported(selector.Sel.Name) && dependencyBuilders(imported)[selector.Sel.Name] != "" {
				dependency = path
			}
			return true
		})
	}
	return dependency
}

// This is read-only diagnostic evidence, not an additional generation scope.
// Honor the dependency's discoverable local config; if its source/settings
// cannot be established, leave its errors unclassified.
func dependencyBuilders(pkg *packages.Package) map[string]string {
	if pkg.Module == nil || pkg.Dir == "" {
		return nil
	}
	var targets []structgen.Target
	var symbols []string
	files := slices.Clone(pkg.GoFiles)
	slices.Sort(files)
	for _, filename := range files {
		if filepath.Dir(filename) != pkg.Dir || strings.HasSuffix(filename, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(filename)
		if err != nil {
			return nil
		}
		_, owned, err := inspectArtifact(filename, data)
		if err != nil {
			return nil
		}
		if owned {
			continue
		}
		parsed, err := Parse(filename, data)
		if err != nil {
			return nil
		}
		if unknownDependencySettings(parsed.AST) {
			return nil
		}
		targets = append(targets, parsed.Targets...)
		symbols = append(symbols, declaredSymbols(parsed.AST)...)
		for _, spec := range parsed.AST.Imports {
			if spec.Name != nil {
				symbols = append(symbols, spec.Name.Name)
			}
		}
	}
	var overrides []structgen.TypeOverride
	configFile := filepath.Join(pkg.Dir, "gobetter.json")
	if data, err := os.ReadFile(configFile); err == nil {
		document, err := config.Decode(configFile, data)
		if err != nil {
			return nil
		}
		inventory := config.PackageInventory{Dir: pkg.Dir, Active: targets}
		for _, filename := range pkg.IgnoredFiles {
			if !strings.HasSuffix(filename, ".go") || strings.HasSuffix(filename, "_test.go") {
				continue
			}
			data, err := os.ReadFile(filename)
			if err != nil {
				return nil
			}
			_, owned, err := inspectArtifact(filename, data)
			if err != nil {
				return nil
			}
			if owned {
				continue
			}
			identities, errors := inactiveIdentities(filename, data)
			if len(errors) > 0 {
				return nil
			}
			inventory.Inactive = append(inventory.Inactive, identities...)
		}
		overrides, err = config.Bind(document, configFile, pkg.Module.Dir, []config.PackageInventory{inventory})
		if err != nil {
			return nil
		}
	} else if !os.IsNotExist(err) {
		return nil
	}
	plans, err := structgen.MakePlans(targets, overrides, structgen.Invocation{})
	if err != nil {
		return nil
	}
	plans, err = structgen.AllocateNames(plans, symbols, nil)
	if err != nil {
		return nil
	}
	builders := make(map[string]string)
	for _, plan := range plans {
		if !plan.Derive.Has(structgen.Builder) {
			continue
		}
		builders[plan.Names.Constructor] = plan.Target.SourceFile
	}
	return builders
}

// Only the standard default invocation establishes the settings inspected
// above. Explicit flags, substitutions, and wrappers need their own context;
// do not infer them or inherit the importing invocation's options.
func unknownDependencySettings(file *ast.File) bool {
	for _, group := range file.Comments {
		for _, comment := range group.List {
			arguments, ok := strings.CutPrefix(comment.Text, "//go:generate")
			if !ok || len(arguments) == 0 || arguments[0] != ' ' && arguments[0] != '\t' || !strings.Contains(arguments, "gobetter") {
				continue
			}
			words := strings.Fields(arguments)
			if slices.Equal(words, []string{"gobetter", "generate", "."}) || slices.Equal(words, []string{"go", "tool", "gobetter", "generate", "."}) {
				continue
			}
			if len(words) == 5 && words[0] == "go" && words[1] == "run" && strings.HasSuffix(filepath.ToSlash(words[2]), "/cmd/gobetter") && words[3] == "generate" && words[4] == "." {
				continue
			}
			return true
		}
	}
	return false
}

func (r *Result) generatedDependencyError(pkg *packages.Package, err packages.Error) bool {
	for _, selected := range r.Packages {
		if selected.Path == pkg.PkgPath {
			return false
		}
	}
	name := strings.TrimPrefix(err.Msg, "undefined: ")
	for constructor, filename := range dependencyBuilders(pkg) {
		if strings.HasPrefix(err.Msg, "undefined: ") && name == constructor {
			return true
		}
		if strings.Contains(err.Msg, constructor+"()") {
			output := OutputFilename(filename)
			data, readErr := os.ReadFile(output)
			if readErr != nil {
				continue
			}
			_, owned, ownershipErr := inspectArtifact(output, data)
			if !owned || ownershipErr != nil {
				continue
			}
			if pkg.Types == nil {
				continue
			}
			object := pkg.Types.Scope().Lookup(constructor)
			if object != nil {
				if signature, ok := object.Type().(*types.Signature); ok && signature.Results().Len() == 1 {
					// A scalar result cannot be a gobetter builder stage. More
					// ambiguous mismatches retain only their original diagnostic.
					if _, scalar := signature.Results().At(0).Type().Underlying().(*types.Basic); scalar {
						return true
					}
				}
			}
		}
	}
	return false
}

func declaredSymbols(file *ast.File) []string {
	var symbols []string
	for _, decl := range file.Decls {
		switch decl := decl.(type) {
		case *ast.FuncDecl:
			if decl.Recv == nil {
				symbols = append(symbols, decl.Name.Name)
			}
		case *ast.GenDecl:
			for _, spec := range decl.Specs {
				switch spec := spec.(type) {
				case *ast.TypeSpec:
					symbols = append(symbols, spec.Name.Name)
				case *ast.ValueSpec:
					for _, name := range spec.Names {
						symbols = append(symbols, name.Name)
					}
				}
			}
		}
	}
	return symbols
}

// Import bindings in any retained file can conflict with package declarations.
// Use the declared package name for default imports; blank imports bind nothing.
func importSymbols(file methodFile, metadata *packages.Package, effectiveExports map[string]dependencyInventory) ([]string, error) {
	var symbols []string
	for _, spec := range file.file.Imports {
		path, _ := strconv.Unquote(spec.Path.Value)
		if spec.Name == nil {
			if name := dependencyPackageName(metadata, path, effectiveExports); name != "" {
				symbols = append(symbols, name)
			}
			continue
		}
		switch spec.Name.Name {
		case "_":
		case ".":
			names, _, err := effectiveDependencyNames(metadata, path, effectiveExports)
			if err != nil {
				return nil, structgen.Diagnostics{{Position: file.fset.PositionFor(spec.Pos(), false), Code: structgen.PackageLoad, Message: err.Error()}}
			}
			for name := range names {
				symbols = append(symbols, name)
			}
		default:
			symbols = append(symbols, spec.Name.Name)
		}
	}
	slices.Sort(symbols)
	return symbols, nil
}

func lexicalBindings(file *SourceFile) (map[*ast.Ident]*ast.Ident, error) {
	// The standard parser's lexical resolver handles local scopes and shadowing.
	// It is only bootstrap evidence; every chosen use is checked against go/types.
	fset := token.NewFileSet()
	resolved, err := parser.ParseFile(fset, file.Path, file.Content, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	objects := make(map[int]*ast.Object)
	ast.Inspect(resolved, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok {
			objects[fset.PositionFor(id.Pos(), false).Offset] = id.Obj
		}
		return true
	})
	parameters := make(map[*ast.Object]*ast.Ident)
	for _, target := range file.Parsed.Targets {
		for _, parameter := range target.TypeParameters {
			object := objects[file.Parsed.FileSet.PositionFor(parameter.Ident.Pos(), false).Offset]
			if object == nil {
				return nil, sourceError(file.Path, structgen.BindingMismatch, "cannot establish lexical generic parameter identity")
			}
			parameters[object] = parameter.Ident
		}
	}
	uses := make(map[*ast.Ident]*ast.Ident)
	ast.Inspect(file.Parsed.AST, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok {
			object := objects[file.Parsed.FileSet.PositionFor(id.Pos(), false).Offset]
			if parameter, ok := parameters[object]; ok {
				uses[id] = parameter
			}
		}
		return true
	})
	return uses, nil
}

func cloneOverlay(original map[string][]byte) map[string][]byte {
	cloned := make(map[string][]byte, len(original))
	maps.Copy(cloned, original)
	return cloned
}

func (r *Result) resolvePlans() error {
	for i := range r.Plans {
		plan := &r.Plans[i]
		var pkg *packages.Package
		for _, candidate := range r.resolved {
			if candidate.Dir == plan.Target.PackageDir {
				pkg = candidate
			}
		}
		if pkg == nil {
			return fmt.Errorf("resolved package is missing")
		}
		plan.Target.Object, _ = pkg.Types.Scope().Lookup(plan.Target.Name).(*types.TypeName)
		if plan.Target.Object == nil {
			return sourceError(plan.Target.SourceFile, structgen.BindingMismatch, "target has no resolved declaration object")
		}
		for j := range plan.Target.TypeParameters {
			parameter := &plan.Target.TypeParameters[j]
			object := r.resolvedObject(pkg, parameter.Ident)
			if object == nil {
				return sourceError(plan.Target.SourceFile, structgen.BindingMismatch, "generic parameter has no resolved source object")
			}
			parameter.Object, _ = object.Type().(*types.TypeParam)
			if parameter.Object == nil {
				return sourceError(plan.Target.SourceFile, structgen.BindingMismatch, "generic parameter did not resolve to a type parameter")
			}
		}
		for use := range plan.Bindings {
			object := r.resolvedObject(pkg, use)
			valid := false
			for _, parameter := range plan.Target.TypeParameters {
				if parameter.Ident.Name == use.Name && object == parameter.Object.Obj() {
					valid = true
				}
			}
			if !valid {
				return sourceError(plan.Target.SourceFile, structgen.BindingMismatch, "lexical generic substitution disagrees with resolved Go binding")
			}
		}
		for j := range plan.Fields {
			field := &plan.Fields[j]
			field.Type = pkg.TypesInfo.TypeOf(field.TypeExpr)
			if field.Type == nil && r.isCgoSource(plan.Target.SourceFile) {
				// cgo type-checks transformed syntax. The named declaration's
				// resolved structural fields retain actual Go type identities.
				st := resolvedStruct(pkg, plan.Target.Path)
				if st != nil {
					for resolvedField := range st.Fields() {
						if resolvedField.Name() == field.Name {
							field.Type = resolvedField.Type()
						}
					}
				}
			}
			if field.Type == nil {
				return sourceError(plan.Target.SourceFile, structgen.PackageLoad, "field type did not resolve: "+field.Name)
			}
		}
	}
	return nil
}

func (r *Result) isCgoSource(filename string) bool {
	for _, pkg := range r.Packages {
		for _, file := range pkg.Files {
			if file.Path == filename {
				return importsC(file.Parsed.AST)
			}
		}
	}
	return false
}

func (r *Result) resolvedObject(pkg *packages.Package, id *ast.Ident) types.Object {
	if object := pkg.TypesInfo.Uses[id]; object != nil {
		return object
	}
	if object := pkg.TypesInfo.Defs[id]; object != nil {
		return object
	}
	position := r.discovery.Fset.PositionFor(id.Pos(), true)
	if !r.isCgoSource(r.discovery.Fset.PositionFor(id.Pos(), false).Filename) {
		return nil
	}
	// cgo's generated syntax carries source //line coordinates. Match those
	// coordinates and spelling; never infer a binding from spelling alone.
	for node, object := range pkg.TypesInfo.Uses {
		candidate := pkg.Fset.PositionFor(node.Pos(), true)
		if node.Name == id.Name && candidate.Filename == position.Filename && candidate.Line == position.Line && candidate.Column == position.Column {
			return object
		}
	}
	for node, object := range pkg.TypesInfo.Defs {
		candidate := pkg.Fset.PositionFor(node.Pos(), true)
		if node.Name == id.Name && candidate.Filename == position.Filename && candidate.Line == position.Line && candidate.Column == position.Column {
			return object
		}
	}
	return nil
}

func resolvedStruct(pkg *packages.Package, path string) *types.Struct {
	parts := strings.Split(path, ".")
	object := pkg.Types.Scope().Lookup(parts[0])
	if object == nil {
		return nil
	}
	typ := object.Type()
	for _, field := range parts[1:] {
		if pointer, ok := typ.(*types.Pointer); ok {
			typ = pointer.Elem()
		}
		st, ok := typ.Underlying().(*types.Struct)
		if !ok {
			return nil
		}
		found := false
		for resolvedField := range st.Fields() {
			if resolvedField.Name() == field {
				typ = resolvedField.Type()
				found = true
				break
			}
		}
		if !found {
			return nil
		}
	}
	if pointer, ok := typ.(*types.Pointer); ok {
		typ = pointer.Elem()
	}
	st, _ := typ.Underlying().(*types.Struct)
	return st
}
