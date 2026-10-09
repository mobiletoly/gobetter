// Copyright 2025 Toly Pochkin
// SPDX-License-Identifier: Apache-2.0

// Package cli implements the command contract without process-global flags.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"go/token"
	"io"
	"strings"

	"github.com/mobiletoly/gobetter/internal/generate"
	"github.com/mobiletoly/gobetter/internal/source"
	"github.com/mobiletoly/gobetter/internal/structgen"
)

const usage = "usage: gobetter generate|check [flags] [package patterns]\n       gobetter version\n"

// Run executes a command and returns its process exit status.
func Run(ctx context.Context, args []string, dir string, out, errs io.Writer) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(errs, usage)
		return 2
	}
	command := args[0]
	if command == "help" || command == "-h" || command == "--help" {
		if _, err := fmt.Fprint(out, usage); err != nil {
			return 2
		}
		return 0
	}
	if command == "version" {
		if len(args) != 1 {
			_, _ = fmt.Fprint(errs, "version takes no arguments\n"+usage)
			return 2
		}
		if _, err := fmt.Fprintf(out, "gobetter version %s\n", generate.Version); err != nil {
			return 2
		}
		return 0
	}
	if command != "generate" && command != "check" {
		_, _ = fmt.Fprintf(errs, "unknown command %q\n%s", command, usage)
		return 2
	}
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(errs)
	file := fs.String("file", "", "single active Go source; excludes package patterns")
	selection := fs.String("generate-for", "annotated", "annotated, exported, or all")
	derive := fs.String("derive", "", "builder,equal,string,getters subset")
	constructor := fs.String("constructor", "auto", "auto, exported, or package")
	config := fs.String("config", "", "explicit schema-v1 JSON configuration")
	tags := fs.String("tags", "", "comma-separated build tags")
	fs.Usage = func() { _, _ = fmt.Fprint(errs, usage); fs.PrintDefaults() }
	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	// Diagnostic writes are best-effort on paths that already return failure.
	bad := func(message string) int { _, _ = fmt.Fprintln(errs, message); return 2 }
	for _, arg := range fs.Args() {
		if strings.HasPrefix(arg, "-") {
			return bad("flags must precede package patterns")
		}
	}
	request := source.Request{Dir: dir, Patterns: fs.Args(), File: *file, Config: *config, Invocation: structgen.Invocation{Selection: structgen.Selection(*selection)}}
	if request.Invocation.Selection != structgen.Annotated && request.Invocation.Selection != structgen.Exported && request.Invocation.Selection != structgen.All {
		return bad("generate-for must be annotated, exported, or all")
	}
	explicit := make(map[string]bool)
	fs.Visit(func(f *flag.Flag) { explicit[f.Name] = true })
	if explicit["file"] && *file == "" {
		return bad("file must not be empty")
	}
	if explicit["config"] && *config == "" {
		return bad("config must not be empty")
	}
	if *file != "" && len(fs.Args()) != 0 {
		return bad("file mode cannot be combined with package patterns")
	}
	if explicit["derive"] {
		var outputs structgen.Outputs
		for part := range strings.SplitSeq(*derive, ",") {
			output, ok := structgen.ParseOutput(strings.TrimSpace(part))
			if !ok || outputs.Has(output) {
				return bad("derive must be a nonempty, duplicate-free subset of builder,equal,string,getters")
			}
			outputs |= output
		}
		request.Invocation.Options.Derive = &outputs
	}
	if explicit["constructor"] {
		value := structgen.Constructor(*constructor)
		request.Invocation.Options.Constructor = &value
	}
	if ds := structgen.ValidateTypeOptions(request.Invocation.Options, token.Position{}); len(ds) > 0 {
		return bad(ds.Error())
	}
	if explicit["tags"] {
		for tag := range strings.SplitSeq(*tags, ",") {
			tag = strings.TrimSpace(tag)
			if tag == "" {
				return bad("tags must be nonempty comma-separated build tags")
			}
			for _, r := range tag {
				if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '_' && r != '.' {
					return bad("invalid build tag " + tag)
				}
			}
			request.Tags = append(request.Tags, tag)
		}
	}
	mode := generate.Generate
	if command == "check" {
		mode = generate.Check
	}
	report, err := generate.Run(ctx, request, mode)
	if err != nil {
		_, _ = fmt.Fprintln(errs, err)
		for _, path := range report.Written {
			_, _ = fmt.Fprintln(errs, "already replaced:", path)
		}
		for _, path := range report.Removed {
			_, _ = fmt.Fprintln(errs, "already removed:", path)
		}
		return 2
	}
	if mode == generate.Check {
		for _, change := range report.Changes {
			if _, err := fmt.Fprintf(out, "%s:1:1: %s: %s owned output\n", change.Path, structgen.ArtifactDrift, change.Kind); err != nil {
				return 2
			}
		}
		if len(report.Changes) > 0 {
			return 1
		}
		return 0
	}
	for _, path := range report.Written {
		if _, err := fmt.Fprintln(out, "generated:", path); err != nil {
			return 2
		}
	}
	for _, path := range report.Removed {
		if _, err := fmt.Fprintln(out, "removed:", path); err != nil {
			return 2
		}
	}
	return 0
}
