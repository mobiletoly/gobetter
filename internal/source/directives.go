// Copyright 2025 Toly Pochkin
// SPDX-License-Identifier: Apache-2.0

package source

import (
	"bytes"
	"go/ast"
	"go/token"
	"strings"

	"github.com/mobiletoly/gobetter/internal/structgen"
)

// DirectiveTool is the tool name used in source directives.
const DirectiveTool = "gobetter"

// StructDirective identifies type-level generation directives.
const StructDirective = "struct"

// FieldDirective identifies field-level generation directives.
const FieldDirective = "field"

type annotations struct {
	fset       *token.FileSet
	directives map[*ast.Comment]ast.Directive
	used       map[*ast.Comment]bool
	errors     structgen.Diagnostics
}

func newAnnotations(fset *token.FileSet, file *ast.File, data []byte) *annotations {
	p := &annotations{fset: fset, directives: make(map[*ast.Comment]ast.Directive), used: make(map[*ast.Comment]bool)}
	for _, group := range file.Comments {
		for _, comment := range group.List {
			if strings.Contains(comment.Text, "+gob:") {
				p.add(comment.Pos(), structgen.LegacyDirective, "legacy +gob: annotations are unsupported; use //gobetter:struct and //gobetter:field directives")
			}
			if strings.HasPrefix(comment.Text, "/*") {
				if blockDirective(comment.Text) {
					p.add(comment.Pos(), structgen.DirectivePlacement, "gobetter directives must use // comments, not block comments")
				}
				continue
			}
			directive, ok := ast.ParseDirective(comment.Pos(), comment.Text)
			if !ok || directive.Tool != DirectiveTool {
				if strings.HasPrefix(strings.TrimSpace(strings.TrimPrefix(comment.Text, "//")), "gobetter:") {
					p.add(comment.Pos(), structgen.DirectiveSyntax, "use //gobetter:struct or //gobetter:field with no space after // or :")
				}
				continue
			}
			if directive.Name != StructDirective && directive.Name != FieldDirective {
				p.add(comment.Pos(), structgen.DirectiveSyntax, "unknown gobetter directive; use struct or field")
				continue
			}
			offset := fset.File(comment.Pos()).Offset(comment.Pos())
			lineStart := bytes.LastIndexByte(data[:offset], '\n') + 1
			if len(bytes.TrimSpace(data[lineStart:offset])) != 0 && directive.Name != FieldDirective {
				p.add(comment.Pos(), structgen.DirectivePlacement, "move the gobetter directive to a leading comment-only line")
				continue
			}
			p.directives[comment] = directive
		}
	}
	return p
}

func blockDirective(text string) bool {
	body := strings.TrimSuffix(strings.TrimPrefix(text, "/*"), "*/")
	for line := range strings.SplitSeq(body, "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "*"))
		if strings.HasPrefix(line, "gobetter:") || strings.HasPrefix(line, "//gobetter:") {
			return true
		}
	}
	return false
}
func (p *annotations) position(pos token.Pos) token.Position { return p.fset.PositionFor(pos, false) }
func (p *annotations) add(pos token.Pos, code structgen.Code, message string) {
	p.errors = append(p.errors, structgen.Diagnostic{Position: p.position(pos), Code: code, Message: message})
}
func (p *annotations) finish() {
	for comment, directive := range p.directives {
		if !p.used[comment] {
			message := "unattached directive; place it immediately above a supported declaration or struct field without a blank line"
			if directive.Name == FieldDirective {
				message += ", or inline after a struct field"
			}
			p.add(comment.Pos(), structgen.DirectivePlacement, message)
		}
	}
}

// tokens processes the gobetter CSV grammar, not Directive.ParseArgs' shell-like
// argument grammar. Duplicate keys/output tokens are rejected across the stack.
func (p *annotations) tokens(kind string, visit func(string, string, bool, token.Pos), groups ...*ast.CommentGroup) bool {
	seen := make(map[string]bool)
	found := false
	for _, group := range groups {
		if group == nil {
			continue
		}
		for _, comment := range group.List {
			directive, ok := p.directives[comment]
			if !ok || directive.Name != kind {
				continue
			}
			p.used[comment] = true
			found = true
			if directive.Args == "" {
				continue
			}
			offset := 0
			for raw := range strings.SplitSeq(directive.Args, ",") {
				text := strings.TrimSpace(raw)
				pos := directive.ArgsPos + token.Pos(offset+len(raw)-len(strings.TrimLeft(raw, " \t\r\n")))
				offset += len(raw) + 1
				if text == "" {
					p.add(pos, structgen.DirectiveSyntax, "empty option in comma-separated directive")
					continue
				}
				key, value, assigned := strings.Cut(text, "=")
				key, value = strings.TrimSpace(key), strings.TrimSpace(value)
				if seen[key] {
					p.add(pos, structgen.DirectiveDuplicate, "duplicate directive option "+key)
					continue
				}
				seen[key] = true
				visit(key, value, assigned, pos)
			}
		}
	}
	return found
}

func (p *annotations) typeOptions(group *ast.CommentGroup) (structgen.TypeOptions, bool) {
	var options structgen.TypeOptions
	found := p.tokens(StructDirective, func(key, value string, assigned bool, pos token.Pos) {
		if output, ok := structgen.ParseOutput(key); ok && !assigned {
			if options.Derive == nil {
				options.Derive = new(structgen.Outputs)
			}
			*options.Derive |= output
			return
		}
		switch key {
		case "constructor":
			if assigned && (value == string(structgen.AutoConstructor) || value == string(structgen.ExportedConstructor) || value == string(structgen.PackageConstructor)) {
				options.Constructor = new(structgen.Constructor(value))
				return
			}
		}
		p.add(pos, structgen.InvalidOption, "unknown or invalid struct option "+key+"; use builder,equal,string,getters,constructor=auto|exported|package")
	}, group)
	return options, found
}

func (p *annotations) fieldOptions(groups ...*ast.CommentGroup) (structgen.FieldOptions, bool) {
	var options structgen.FieldOptions
	found := p.tokens(FieldDirective, func(key, value string, assigned bool, pos token.Pos) {
		if key == "name" && assigned {
			options.Name = new(value)
			p.errors = append(p.errors, structgen.ValidateFieldOptions(options, p.position(pos))...)
			return
		}
		if !assigned {
			switch key {
			case "getter":
				options.Getter = new(true)
				return
			case "optional":
				options.Optional = new(true)
				return
			case "skip-builder":
				options.SkipBuilder = new(true)
				return
			case "skip-equal":
				options.SkipEqual = new(true)
				return
			case "skip-string":
				options.SkipString = new(true)
				return
			}
		}
		p.add(pos, structgen.InvalidOption, "unknown or invalid field option "+key+"; use name=<identifier>,getter,optional,skip-builder,skip-equal,skip-string")
	}, groups...)
	return options, found
}
