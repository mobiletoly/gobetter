// Copyright 2025 Toly Pochkin
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/token"
	"io"
	"path/filepath"
	"slices"
	"strings"

	"github.com/mobiletoly/gobetter/internal/structgen"
)

// SchemaVersion is the supported configuration schema version.
const SchemaVersion = 1

// Document contains validated external type configuration.
type Document struct{ Types []Type }

// Type associates configuration overrides with a package and source type path.
type Type struct {
	Package  string
	Path     string
	Position token.Position
	Options  structgen.TypeOptions
	Fields   []structgen.FieldOverride
}

// Decode rejects duplicate keys at every depth, unknown/case-variant properties,
// nulls, wrong JSON types, invalid options, and extra top-level values. It retains
// option presence so false does not collapse into an omitted setting.
func Decode(filename string, data []byte) (Document, error) {
	positions := make(map[string]token.Position)
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := scan(decoder, data, filename, "", positions); err != nil {
		return Document{}, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return Document{}, diagnostic(position(filename, data, int(decoder.InputOffset())), structgen.ConfigSyntax, "configuration must contain exactly one JSON object")
	}
	pos := func(path string) token.Position {
		if p, ok := positions[path]; ok {
			return p
		}
		return position(filename, data, 0)
	}
	root, err := object(data, []string{"schema_version", "types"}, "", pos)
	if err != nil {
		return Document{}, err
	}
	var version int
	if err := json.Unmarshal(root["schema_version"], &version); err != nil || version != SchemaVersion {
		return Document{}, diagnostic(pos("/schema_version"), structgen.ConfigSchema, "schema_version must be 1")
	}
	var records []json.RawMessage
	if value, ok := root["types"]; ok {
		if err := json.Unmarshal(value, &records); err != nil {
			return Document{}, diagnostic(pos("/types"), structgen.ConfigSchema, "types must be an array")
		}
	}
	var document Document
	for i, record := range records {
		path := fmt.Sprintf("/types/%d", i)
		values, err := object(record, []string{"package", "type", "derive", "constructor", "fields"}, path, pos)
		if err != nil {
			return Document{}, err
		}
		for _, required := range []string{"package", "type"} {
			if _, ok := values[required]; !ok {
				return Document{}, diagnostic(pos(path), structgen.ConfigTarget, required+" is required")
			}
		}
		var item struct {
			Package     string                     `json:"package"`
			Type        string                     `json:"type"`
			Derive      *[]string                  `json:"derive"`
			Constructor *structgen.Constructor     `json:"constructor"`
			Fields      map[string]json.RawMessage `json:"fields"`
		}
		if err := json.Unmarshal(record, &item); err != nil {
			return Document{}, diagnostic(pos(path), structgen.ConfigSchema, err.Error())
		}
		if item.Package == "" || strings.ContainsAny(item.Package, "*?[]") || slices.Contains(strings.Split(filepath.ToSlash(item.Package), "/"), "...") {
			return Document{}, diagnostic(pos(path+"/package"), structgen.ConfigTarget, "package must be an exact directory path without wildcards")
		}
		for part := range strings.SplitSeq(item.Type, ".") {
			if !token.IsIdentifier(part) || part == "_" {
				return Document{}, diagnostic(pos(path+"/type"), structgen.ConfigTarget, "type must be a declaration name or dotted nested field path without type arguments")
			}
		}
		options := structgen.TypeOptions{Constructor: item.Constructor}
		if item.Derive != nil {
			var outputs structgen.Outputs
			for _, name := range *item.Derive {
				output, ok := structgen.ParseOutput(name)
				if !ok || outputs.Has(output) {
					return Document{}, diagnostic(pos(path+"/derive"), structgen.InvalidOption, "derive must contain unique builder,equal,string,getters values")
				}
				outputs |= output
			}
			options.Derive = &outputs
		}
		if errors := structgen.ValidateTypeOptions(options, pos(path)); len(errors) != 0 {
			return Document{}, errors.Sorted()
		}
		result := Type{Package: item.Package, Path: item.Type, Position: pos(path + "/type"), Options: options}
		fieldNames := make([]string, 0, len(item.Fields))
		for name := range item.Fields {
			fieldNames = append(fieldNames, name)
		}
		slices.Sort(fieldNames)
		for _, name := range fieldNames {
			fieldPath := path + "/fields/" + escape(name)
			if !token.IsIdentifier(name) || name == "_" {
				return Document{}, diagnostic(pos(fieldPath), structgen.ConfigTarget, "field selector must name a nonblank Go field")
			}
			if _, err := object(item.Fields[name], []string{"name", "getter", "optional", "skip_builder", "skip_equal", "skip_string"}, fieldPath, pos); err != nil {
				return Document{}, err
			}
			var field struct {
				Name        *string `json:"name"`
				Getter      *bool   `json:"getter"`
				Optional    *bool   `json:"optional"`
				SkipBuilder *bool   `json:"skip_builder"`
				SkipEqual   *bool   `json:"skip_equal"`
				SkipString  *bool   `json:"skip_string"`
			}
			if err := json.Unmarshal(item.Fields[name], &field); err != nil {
				return Document{}, diagnostic(pos(fieldPath), structgen.ConfigSchema, err.Error())
			}
			settings := structgen.FieldOptions{Name: field.Name, Getter: field.Getter, Optional: field.Optional, SkipBuilder: field.SkipBuilder, SkipEqual: field.SkipEqual, SkipString: field.SkipString}
			if errors := structgen.ValidateFieldOptions(settings, pos(fieldPath)); len(errors) != 0 {
				return Document{}, errors.Sorted()
			}
			result.Fields = append(result.Fields, structgen.FieldOverride{Name: name, Position: pos(fieldPath), Options: settings})
		}
		document.Types = append(document.Types, result)
	}
	return document, nil
}

func diagnostic(pos token.Position, code structgen.Code, message string) error {
	return structgen.Diagnostics{{Position: pos, Code: code, Message: message}}
}

func object(data []byte, allowed []string, path string, pos func(string) token.Position) (map[string]json.RawMessage, error) {
	var values map[string]json.RawMessage
	if err := json.Unmarshal(data, &values); err != nil || values == nil {
		return nil, diagnostic(pos(path), structgen.ConfigSchema, "expected a JSON object")
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		if !slices.Contains(allowed, key) {
			return nil, diagnostic(pos(path+"/"+escape(key)), structgen.ConfigSchema, "unknown property "+key)
		}
	}
	return values, nil
}

func position(filename string, data []byte, offset int) token.Position {
	if offset > len(data) {
		offset = len(data)
	}
	p := token.Position{Filename: filename, Offset: offset, Line: 1, Column: 1}
	for _, b := range data[:offset] {
		if b == '\n' {
			p.Line++
			p.Column = 1
		} else {
			p.Column++
		}
	}
	return p
}
func escape(s string) string { return strings.ReplaceAll(strings.ReplaceAll(s, "~", "~0"), "/", "~1") }

func scan(decoder *json.Decoder, data []byte, filename, path string, positions map[string]token.Position) error {
	offset := int(decoder.InputOffset())
	for offset < len(data) && strings.ContainsRune(" \t\r\n,:", rune(data[offset])) {
		offset++
	}
	positions[path] = position(filename, data, offset)
	tok, err := decoder.Token()
	if err != nil {
		return diagnostic(positions[path], structgen.ConfigSyntax, err.Error())
	}
	if tok == nil {
		return diagnostic(positions[path], structgen.ConfigSchema, "null is not supported; omit the property instead")
	}
	delim, composite := tok.(json.Delim)
	if !composite {
		return nil
	}
	switch delim {
	case '{':
		seen := make(map[string]bool)
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return diagnostic(position(filename, data, int(decoder.InputOffset())), structgen.ConfigSyntax, err.Error())
			}
			key, ok := keyToken.(string)
			if !ok {
				return diagnostic(positions[path], structgen.ConfigSyntax, "object key must be a string")
			}
			if seen[key] {
				return diagnostic(position(filename, data, int(decoder.InputOffset())), structgen.ConfigDuplicate, "duplicate JSON key "+key)
			}
			seen[key] = true
			if err := scan(decoder, data, filename, path+"/"+escape(key), positions); err != nil {
				return err
			}
		}
	case '[':
		for i := 0; decoder.More(); i++ {
			if err := scan(decoder, data, filename, fmt.Sprintf("%s/%d", path, i), positions); err != nil {
				return err
			}
		}
	default:
		return diagnostic(positions[path], structgen.ConfigSyntax, "unexpected JSON delimiter")
	}
	if _, err := decoder.Token(); err != nil {
		return diagnostic(positions[path], structgen.ConfigSyntax, err.Error())
	}
	return nil
}
