// Copyright 2025 Toly Pochkin
// SPDX-License-Identifier: Apache-2.0

package source

import (
	"context"
	"fmt"
	"go/ast"
	"go/build"
	"go/build/constraint"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/mobiletoly/gobetter/internal/config"
	"github.com/mobiletoly/gobetter/internal/structgen"
	"golang.org/x/mod/modfile"
	"golang.org/x/tools/go/packages"
)

// Request selects source packages, build context, and generation settings.
type Request struct {
	Dir        string
	Patterns   []string
	File       string
	Tags       []string
	GOOS       string
	GOARCH     string
	CgoEnabled *bool
	Config     string // Empty discovers gobetter.json in Dir only.
	Invocation structgen.Invocation
}

// SourceFile holds a discovered file, its parsed declarations, and build constraint.
type SourceFile struct {
	Path        string
	Content     []byte
	Parsed      *File
	Constraint  constraint.Expr
	ContextOnly bool // Retained Go declarations, never generation targets.
}

// Package inventories active sources, inactive files, and owned artifacts.
type Package struct {
	Dir            string
	Name           string
	Path           string
	Files          []*SourceFile // Active inputs/context, excluding owned artifacts/tests.
	Inactive       []string
	InactiveErrors structgen.Diagnostics
	Artifacts      []Artifact
	metadata       *packages.Package
	activeFiles    map[string]bool // Go loader's original-source inventory, including cgo.
}
type discovery struct {
	Request   Request
	Packages  []*Package
	Fset      *token.FileSet
	ModuleDir string
	Context   build.Context
	Overrides []structgen.TypeOverride
}

func discover(ctx context.Context, request Request) (*discovery, error) {
	dir, err := filepath.Abs(request.Dir)
	if err != nil {
		return nil, err
	}
	request.Dir = dir
	if request.File != "" && len(request.Patterns) != 0 {
		return nil, fmt.Errorf("file mode cannot be combined with package patterns")
	}
	if request.File != "" {
		if !filepath.IsAbs(request.File) {
			request.File = filepath.Join(dir, request.File)
		}
		request.File = filepath.Clean(request.File)
		if strings.HasSuffix(request.File, "_test.go") {
			return nil, sourceError(request.File, structgen.UnsupportedTarget, "test-file input is unsupported")
		}
		if _, err := os.Stat(request.File); err != nil {
			return nil, err
		}
		request.Patterns = []string{filepath.Dir(request.File)}
	} else if len(request.Patterns) == 0 {
		request.Patterns = []string{"."}
	}
	moduleDir, err := moduleRoot(dir)
	if err != nil {
		return nil, err
	}
	d := &discovery{Request: request, Fset: token.NewFileSet(), Context: build.Default, ModuleDir: moduleDir}
	if request.GOOS != "" {
		d.Context.GOOS = request.GOOS
	}
	if request.GOARCH != "" {
		d.Context.GOARCH = request.GOARCH
	}
	if request.CgoEnabled != nil {
		d.Context.CgoEnabled = *request.CgoEnabled
	}
	d.Context.BuildTags = slices.Clone(request.Tags)
	if err := preflightSourceKinds(ctx, request, moduleDir); err != nil {
		return nil, err
	}
	metadata, err := packages.Load(d.loaderConfig(ctx, packages.NeedName|packages.NeedFiles|packages.NeedCompiledGoFiles|packages.NeedImports|packages.NeedDeps|packages.NeedModule, nil), request.Patterns...)
	if err != nil {
		return nil, err
	}
	if len(metadata) == 0 {
		return nil, fmt.Errorf("no packages matched the requested patterns")
	}
	seen := make(map[string]bool)
	slices.SortFunc(metadata, func(a, b *packages.Package) int { return strings.Compare(a.Dir, b.Dir) })
	for _, meta := range metadata {
		if meta.Module == nil || meta.Dir == "" {
			return nil, fmt.Errorf("target must belong to the input module: %s", meta.ID)
		}
		if d.ModuleDir != meta.Module.Dir {
			return nil, fmt.Errorf("target must belong to the invocation module: %s", meta.ID)
		}
		if seen[meta.Dir] {
			continue
		}
		seen[meta.Dir] = true
		pkg := &Package{Dir: meta.Dir, Path: meta.PkgPath, metadata: meta, activeFiles: make(map[string]bool)}
		for _, filename := range meta.GoFiles {
			pkg.activeFiles[filepath.Clean(filename)] = true
		}
		entries, err := os.ReadDir(meta.Dir)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			name := entry.Name()
			if !goSourceName(name) {
				continue
			}
			full := filepath.Join(meta.Dir, name)
			if directoryEntry(full, entry) {
				continue
			}
			if err := requireRegularSource(full); err != nil {
				return nil, err
			}
			data, err := os.ReadFile(full)
			if err != nil {
				return nil, err
			}
			artifact, owned, err := inspectArtifact(full, data)
			if err != nil {
				return nil, err
			}
			if owned {
				pkg.Artifacts = append(pkg.Artifacts, artifact)
				continue
			}
			active := pkg.activeFiles[full]
			var parsed *File
			contextOnly := false
			if legacySource, legacy := legacyArtifactSource(full, data); legacy {
				if !active {
					continue
				}
				_, sourceErr := os.Stat(legacySource)
				inactiveSource := sourceErr == nil && !pkg.activeFiles[legacySource]
				if !inactiveSource && (request.File == "" || legacySource == request.File) {
					return nil, sourceError(full, structgen.OutputOwnership, "legacy output format requires explicit removal before v0.20.0 generation")
				}
				// Keep active out-of-scope bytes in Go loading and declaration
				// inventories without interpreting old generator comments or
				// granting publication/cleanup ownership.
				file, err := parseGoFile(d.Fset, full, data)
				if err != nil {
					return nil, err
				}
				parsed = &File{AST: file, FileSet: d.Fset}
				contextOnly = true
			} else {
				if !active {
					ids, errors := inactiveIdentities(full, data)
					pkg.Inactive = append(pkg.Inactive, ids...)
					pkg.InactiveErrors = append(pkg.InactiveErrors, errors...)
					continue
				}
				parsed, err = parseWithSet(d.Fset, full, data)
				if err != nil {
					return nil, err
				}
			}
			if pkg.Name == "" {
				pkg.Name = parsed.AST.Name.Name
			}
			if pkg.Name != parsed.AST.Name.Name {
				return nil, sourceError(full, structgen.PackageLoad, "multiple active package names")
			}
			if contextOnly {
				pkg.Files = append(pkg.Files, &SourceFile{Path: full, Content: data, Parsed: parsed, ContextOnly: true})
				continue
			}
			header, err := effectiveConstraint(parsed.AST, data)
			if err != nil {
				return nil, sourceError(full, structgen.BuildContext, err.Error())
			}
			file := &SourceFile{Path: full, Content: data, Parsed: parsed, Constraint: header}
			for i := range parsed.Targets {
				parsed.Targets[i].Imports = parsed.AST.Imports
				parsed.Targets[i].BuildConstraint = header
			}
			pkg.Files = append(pkg.Files, file)
		}
		if request.File != "" {
			found := false
			for _, file := range pkg.Files {
				if !file.ContextOnly && file.Path == request.File {
					found = true
				}
			}
			if !found {
				return nil, sourceError(request.File, structgen.BuildContext, fmt.Sprintf("file is not active for GOOS=%s GOARCH=%s cgo=%t tags=%s", d.Context.GOOS, d.Context.GOARCH, d.Context.CgoEnabled, strings.Join(request.Tags, ",")))
			}
		}
		if len(pkg.Files) == 0 {
			// An active owned sidecar can be the package's last file after its
			// source is deleted. Keep Go's package identity so normal overlay
			// validation and scoped obsolete comparison/removal still apply.
			for _, artifact := range pkg.Artifacts {
				if pkg.activeFiles[artifact.Path] {
					pkg.Name = meta.Name
					break
				}
			}
			if pkg.Name == "" {
				return nil, fmt.Errorf("no active source in selected package %s", pkg.Dir)
			}
		}
		d.Packages = append(d.Packages, pkg)
	}
	configFile := request.Config
	if configFile == "" {
		configFile = filepath.Join(dir, "gobetter.json")
		if _, err := os.Stat(configFile); os.IsNotExist(err) {
			configFile = ""
		} else if err != nil {
			return nil, err
		}
	} else if !filepath.IsAbs(configFile) {
		configFile = filepath.Join(dir, configFile)
	}
	if configFile != "" {
		data, err := os.ReadFile(configFile)
		if err != nil {
			return nil, err
		}
		doc, err := config.Decode(configFile, data)
		if err != nil {
			return nil, err
		}
		var inventory []config.PackageInventory
		for _, pkg := range d.Packages {
			item := config.PackageInventory{Dir: pkg.Dir, Inactive: pkg.Inactive}
			for _, file := range pkg.Files {
				item.Active = append(item.Active, file.Parsed.Targets...)
			}
			inventory = append(inventory, item)
		}
		d.Overrides, err = config.Bind(doc, configFile, d.ModuleDir, inventory)
		if err != nil {
			for _, pkg := range d.Packages {
				if len(pkg.InactiveErrors) > 0 {
					return nil, fmt.Errorf("%w\ninactive declaration inventory is incomplete:\n%v", err, pkg.InactiveErrors.Sorted())
				}
			}
			return nil, err
		}
	}
	return d, nil
}

func goSourceName(name string) bool {
	return goHeaderName(name) && !strings.HasSuffix(name, "_test.go")
}

// Go reads eligible test-file headers even when tests are not generation inputs.
func goHeaderName(name string) bool {
	return strings.HasSuffix(name, ".go") && !strings.HasPrefix(name, ".") && !strings.HasPrefix(name, "_")
}

// Go skips directory links before reading headers. Unconfirmed targets remain
// subject to the ordinary symlink/nonregular refusal; never open them here.
func directoryEntry(path string, entry os.DirEntry) bool {
	if entry.IsDir() {
		return true
	}
	if entry.Type()&os.ModeSymlink != 0 {
		info, err := os.Stat(path)
		return err == nil && info.IsDir()
	}
	return false
}

// Ignore paths are literal slash-bounded prefixes (./x) or sequences (x),
// matching Go's module-relative recursive selection without glob expansion.
func ignoredDirectory(relative string, ignores []*modfile.Ignore) bool {
	if relative == "" {
		return false
	}
	normalize := func(path string) string {
		path = filepath.ToSlash(path)
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		if !strings.HasSuffix(path, "/") {
			path += "/"
		}
		return path
	}
	relative = normalize(relative)
	for _, ignore := range ignores {
		path, rooted := strings.CutPrefix(ignore.Path, "./")
		path = normalize(path)
		if rooted && strings.HasPrefix(relative, path) || !rooted && strings.Contains(relative, path) {
			return true
		}
	}
	return false
}

func requireRegularSource(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return sourceError(path, structgen.OutputOwnership, "symlink source/output ownership is ambiguous")
	}
	if !info.Mode().IsRegular() {
		return sourceError(path, structgen.OutputOwnership, "refusing non-regular source/output")
	}
	return nil
}

// Go's initial go list may open a FIFO while reading package headers. Check
// entry kinds in the requested module directories first, without opening Go
// files. This does not select active sources: go/packages still owns selection.
func preflightSourceKinds(ctx context.Context, request Request, moduleDir string) error {
	data, err := os.ReadFile(filepath.Join(moduleDir, "go.mod"))
	if err != nil {
		return err
	}
	module, err := modfile.Parse(filepath.Join(moduleDir, "go.mod"), data, nil)
	if err != nil {
		return err
	}
	modulePath := ""
	if module.Module != nil {
		modulePath = module.Module.Mod.Path
	}
	checked := make(map[string]bool)
	for _, pattern := range request.Patterns {
		localPattern := filepath.IsAbs(pattern) || pattern == "." || pattern == ".." || strings.HasPrefix(pattern, "./") || strings.HasPrefix(pattern, "../")
		anchor := moduleDir
		switch {
		case pattern == "all":
			pattern = "..."
		case pattern == "." || pattern == ".." || strings.HasPrefix(pattern, "./") || strings.HasPrefix(pattern, "../"):
			anchor = request.Dir
		case filepath.IsAbs(pattern):
			pattern, err = filepath.Rel(moduleDir, pattern)
			if err != nil {
				return err
			}
		case pattern == modulePath:
			pattern = "."
		case modulePath != "" && strings.HasPrefix(pattern, modulePath+"/"):
			pattern = strings.TrimPrefix(pattern, modulePath+"/")
		default:
			continue // Other modules and unmatched patterns are diagnosed by Go.
		}
		pattern = filepath.Clean(pattern)
		base := pattern
		recursive := strings.Contains(pattern, "...")
		if recursive {
			base = filepath.Dir(pattern[:strings.Index(pattern, "...")] + "placeholder")
		}
		base = filepath.Join(anchor, base)
		rel, err := filepath.Rel(moduleDir, base)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("target must belong to the invocation module: %s", base)
		}
		if owner, err := moduleRoot(base); err == nil && owner != moduleDir {
			return fmt.Errorf("target must belong to the invocation module: %s", base)
		}
		expression := regexp.QuoteMeta(filepath.ToSlash(pattern))
		if before, ok := strings.CutSuffix(expression, `/\.\.\.`); ok {
			expression = before + `(/\.\.\.)?`
		}
		match := regexp.MustCompile("^" + strings.ReplaceAll(expression, `\.\.\.`, ".*") + "$")
		// Match Go's recursive matcher: a trailing separator follows a
		// symlinked root while WalkDir still skips descendant symlinks.
		err = filepath.WalkDir(base+string(filepath.Separator), func(path string, entry os.DirEntry, err error) error {
			if os.IsNotExist(err) {
				return nil // Let Go retain its unmatched-pattern diagnostic.
			}
			if err != nil {
				return err
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			path = filepath.Clean(path)
			if !entry.IsDir() {
				return nil
			}
			if recursive {
				relative, err := filepath.Rel(moduleDir, path)
				if err != nil {
					return err
				}
				// Local directory matching checks the module root as ".".
				// Go's import-path matcher exempts the module root itself.
				if (relative != "." || localPattern) && ignoredDirectory(relative, module.Ignore) {
					return filepath.SkipDir
				}
			}
			prune := false
			if path != base {
				name := entry.Name()
				if !recursive || name == "testdata" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
					return filepath.SkipDir
				}
				// Go may select code in vendor itself, but not its vendored
				// descendants through a wildcard. Inspect that directory only.
				prune = name == "vendor"
				if info, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
					if !info.IsDir() {
						return filepath.SkipDir
					}
				} else if !os.IsNotExist(err) {
					return err
				}
			}
			relative, err := filepath.Rel(anchor, path)
			if err != nil {
				return err
			}
			if !match.MatchString(filepath.ToSlash(relative)) || checked[path] {
				if prune {
					return filepath.SkipDir
				}
				return nil
			}
			checked[path] = true
			entries, err := os.ReadDir(path)
			if err != nil {
				return err
			}
			for _, entry := range entries {
				full := filepath.Join(path, entry.Name())
				if goHeaderName(entry.Name()) && !directoryEntry(full, entry) {
					if err := requireRegularSource(full); err != nil {
						return err
					}
				}
			}
			if prune {
				return filepath.SkipDir
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func sourceError(filename string, code structgen.Code, message string) error {
	return structgen.Diagnostics{{Position: token.Position{Filename: filename, Line: 1, Column: 1}, Code: code, Message: message}}
}

func moduleRoot(dir string) (string, error) {
	for {
		if info, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil && !info.IsDir() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("invocation requires an existing Go module")
		}
		dir = parent
	}
}
func importsC(file *ast.File) bool {
	if file == nil {
		return false
	}
	for _, spec := range file.Imports {
		path, _ := strconv.Unquote(spec.Path.Value)
		if path == "C" {
			return true
		}
	}
	return false
}

func effectiveConstraint(file *ast.File, data []byte) (constraint.Expr, error) {
	var expression constraint.Expr
	lines := leadingLineComments(file, data)
	for _, line := range lines {
		if constraint.IsGoBuild(line) {
			parsed, err := constraint.Parse(line)
			if err != nil {
				return nil, err
			}
			expression = parsed
			break
		}
	}
	if expression == nil {
		// Legacy +build directives are restricted to the initial // header
		// ending before a blank line. A block comment ends that legacy region.
		end := legacyHeaderEnd(file, data)
		for _, group := range file.Comments {
			for _, comment := range group.List {
				if int(comment.Pos()-file.FileStart) >= end || !constraint.IsPlusBuild(comment.Text) {
					continue
				}
				parsed, err := constraint.Parse(comment.Text)
				if err != nil {
					return nil, err
				}
				if expression == nil {
					expression = parsed
				} else {
					expression = &constraint.AndExpr{X: expression, Y: parsed}
				}
			}
		}
	}
	if importsC(file) {
		cgo := &constraint.TagExpr{Tag: "cgo"}
		if expression == nil {
			expression = cgo
		} else {
			expression = &constraint.AndExpr{X: expression, Y: cgo}
		}
	}
	return expression, nil
}

func inactiveIdentities(filename string, data []byte) ([]string, structgen.Diagnostics) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, data, parser.SkipObjectResolution)
	var ids []string
	var visit func(string, *ast.StructType)
	visit = func(path string, st *ast.StructType) {
		for _, field := range st.Fields.List {
			if child := anonymousStruct(field.Type); child != nil {
				for _, name := range field.Names {
					if name.Name != "_" {
						nested := path + "." + name.Name
						ids = append(ids, nested)
						visit(nested, child)
					}
				}
			}
		}
	}
	if file != nil {
		for _, decl := range file.Decls {
			if group, ok := decl.(*ast.GenDecl); ok && group.Tok == token.TYPE {
				for _, spec := range group.Specs {
					if named, ok := spec.(*ast.TypeSpec); ok && token.IsIdentifier(named.Name.Name) {
						ids = append(ids, named.Name.Name)
						if st, ok := ast.Unparen(named.Type).(*ast.StructType); ok {
							visit(named.Name.Name, st)
						}
					}
				}
			}
		}
	}
	if err != nil {
		return ids, structgen.Diagnostics{{Position: token.Position{Filename: filename, Line: 1, Column: 1}, Code: structgen.SourceSyntax, Message: "cannot completely inventory inactive source: " + err.Error()}}
	}
	return ids, nil
}
