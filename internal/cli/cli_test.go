package cli

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("output unavailable")
}

func TestOutputFailureReturnsErrorStatus(t *testing.T) {
	for _, command := range []string{"help", "version"} {
		t.Run(command, func(t *testing.T) {
			if code := Run(t.Context(), []string{command}, t.TempDir(), failingWriter{}, io.Discard); code != 2 {
				t.Fatalf("output failure returned exit status %d, want 2", code)
			}
		})
	}
}

func input(t *testing.T, dir, name, data string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestFieldGetterRequestsSurviveDeriveReplacement(t *testing.T) {
	for _, tc := range []struct {
		name, outputs, config string
		args                  []string
		getter, builder, str  bool
	}{
		{"builder_annotation", "builder", "", nil, true, true, false},
		{"string_annotation", "string", "", nil, true, false, true},
		{"string_config", "builder", `{"derive":["string"]}`, nil, true, false, true},
		{"string_cli", "builder", "", []string{"-derive=string"}, true, false, true},
		{"getter_config", "builder", `{"derive":["string"],"fields":{"value":{"getter":true,"name":"Value"}}}`, nil, true, false, true},
		{"clear_config", "builder", `{"derive":["string"],"fields":{"value":{"getter":false}}}`, []string{"-derive=string"}, false, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			input(t, dir, "go.mod", "module fixture\n\ngo 1.26.0\n")
			field := "value int //gobetter:field getter,name=Value"
			if tc.name == "getter_config" {
				field = "value int"
			}
			input(t, dir, "model.go", "package models\n//gobetter:struct "+tc.outputs+"\ntype Secret struct {\n"+field+"\n}\n")
			if tc.config != "" {
				input(t, dir, "gobetter.json", `{"schema_version":1,"types":[{"package":".","type":"Secret",`+strings.TrimPrefix(tc.config, "{")+`]}`)
			}
			args := append([]string{"generate"}, tc.args...)
			code, text := call(t, dir, args...)
			if code != 0 {
				t.Fatal(code, text)
			}
			data, err := os.ReadFile(filepath.Join(dir, "model_gob.go"))
			if err != nil {
				t.Fatal(err)
			}
			generated := string(data)
			if strings.Contains(generated, "*Secret) Value()") != tc.getter || strings.Contains(generated, "func NewSecretBuilder(") != tc.builder || strings.Contains(generated, ") String() string") != tc.str {
				t.Fatalf("effective getter/derive contract: %s", generated)
			}
			check := `package models
import "testing"
func TestValue(t *testing.T) { v:=Secret{value:7}; _=v;`
			if tc.getter {
				check += `if v.Value()!=7 {t.Fatal("getter")};`
			}
			if tc.builder {
				check += `b:=NewSecretBuilder().Value(8).Build();if b.Value()!=8 {t.Fatal("builder")};`
			}
			if tc.str {
				check += `if v.String()!="Secret{value:7}" {t.Fatal(v.String())};`
			}
			input(t, dir, "model_test.go", check+"}\n")
			command(t, dir, "test", "-mod=readonly", "./...")
			code, text = call(t, dir, append([]string{"check"}, tc.args...)...)
			if code != 0 {
				t.Fatal(code, text)
			}
		})
	}
}
func command(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "go", args...)
	cmd.Dir = dir
	data, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go %v: %v\n%s", args, err, data)
	}
	return string(data)
}
func call(t *testing.T, dir string, args ...string) (int, string) {
	t.Helper()
	var out, errs bytes.Buffer
	code := Run(t.Context(), args, dir, &out, &errs)
	return code, out.String() + errs.String()
}

func TestCLIFlagsExitsAndPrecedence(t *testing.T) {
	dir := t.TempDir()
	input(t, dir, "go.mod", "module fixture\n\ngo 1.26.0\n")
	input(t, dir, "model.go", "package models\n//gobetter:struct builder\ntype Item struct{Z int; A int}\n")
	for _, tc := range []struct {
		args    []string
		code    int
		message string
	}{
		{nil, 2, "usage:"}, {[]string{"version"}, 0, "gobetter version 0.20.0"}, {[]string{"version", "x"}, 2, "no arguments"},
		{[]string{"generate", "-input=x"}, 2, "flag provided but not defined"},
		{[]string{"generate", "-derive="}, 2, "nonempty"}, {[]string{"generate", "-derive=builder,builder"}, 2, "duplicate-free"},
		{[]string{"generate", "-constructor=none"}, 2, "constructor must"},
		{[]string{"generate", "-order=source"}, 2, "flag provided but not defined: -order"},
		{[]string{"generate", "-order=name"}, 2, "flag provided but not defined: -order"},
		{[]string{"check", "-order=source"}, 2, "flag provided but not defined: -order"},
		{[]string{"check", "-order=name"}, 2, "flag provided but not defined: -order"},
		{[]string{"generate", ".", "-derive=string"}, 2, "flags must precede"}, {[]string{"generate", "-file=model.go", "."}, 2, "cannot be combined"},
		{[]string{"check"}, 1, "missing owned output"},
	} {
		code, text := call(t, dir, tc.args...)
		if code != tc.code || !strings.Contains(text, tc.message) {
			t.Fatalf("%v: %d %s", tc.args, code, text)
		}
	}
	code, text := call(t, dir, "generate", "-derive=builder,string")
	if code != 0 {
		t.Fatal(code, text)
	}
	data, err := os.ReadFile(filepath.Join(dir, "model_gob.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "gobetter_Item_step0) Z(") || !strings.Contains(string(data), ") String() string") {
		t.Fatalf("explicit flag precedence: %s", data)
	}
	code, text = call(t, dir, "check", "-derive=builder,string")
	if code != 0 {
		t.Fatal(code, text)
	}
	code, text = call(t, dir, "check")
	if code != 1 || !strings.Contains(text, "changed owned output") {
		t.Fatal(code, text)
	}
}

func TestPackageDirectiveToolAndStandaloneConsumer(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	input(t, dir, "go.mod", "module consumer\n\ngo 1.26.0\n\nrequire github.com/mobiletoly/gobetter v0.0.0\nreplace github.com/mobiletoly/gobetter => "+strconv.Quote(root)+"\n")
	// Register the checked-out command through Go's actual tool dependency flow.
	command(t, dir, "get", "-tool", "github.com/mobiletoly/gobetter/cmd/gobetter")
	input(t, dir, "models/generate.go", "package models\n//go:generate go run ./testdata/upstream\n//go:generate go tool gobetter generate -config=gobetter.json .\n")
	input(t, dir, "models/testdata/upstream/main.go", `package main
import "os"
func main(){if err:=os.WriteFile("upstream.go",[]byte("// Code generated by upstream; DO NOT EDIT.\npackage models\ntype Upstream struct{Value int}\n"),0644);err!=nil{panic(err)}}
`)
	input(t, dir, "models/a.go", "package models\n//gobetter:struct builder\ntype A struct{Value int}\n")
	input(t, dir, "models/b.go", "package models\n//gobetter:struct getters\ntype B struct{\n//gobetter:field getter,name=ID\n_id string}\n")
	input(t, dir, "models/gobetter.json", `{"schema_version":1,"types":[{"package":".","type":"Upstream","derive":["builder","equal","string"]}]}`)
	input(t, dir, "models/sub/generate.go", "package sub\n//go:generate go tool gobetter generate .\n")
	input(t, dir, "models/sub/model.go", "package sub\n//gobetter:struct builder\ntype Sub struct{Value int}\n")
	input(t, dir, "app/app_test.go", `package app
import("testing";"consumer/models")
func TestConsumer(t *testing.T){a:=models.NewABuilder().Value(1).Build();u:=models.NewUpstreamBuilder().Value(2).Build();b:=models.B{};if a.Value!=1||!u.Equal(u)||u.String()!="Upstream{Value:2}"||b.ID()!=""{t.Fatal(a,u,b)}}
`)
	command(t, dir, "generate", "./models")
	if _, err := os.Stat(filepath.Join(dir, "models/sub/model_gob.go")); !os.IsNotExist(err) {
		t.Fatal("package directive recursed")
	}
	upstream, err := os.ReadFile(filepath.Join(dir, "models/upstream.go"))
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, "models/upstream.go"))
	if err != nil {
		t.Fatal(err)
	}
	command(t, filepath.Join(dir, "models"), "tool", "gobetter", "check", ".")
	after, err := os.Stat(filepath.Join(dir, "models/upstream.go"))
	if err != nil {
		t.Fatal(err)
	}
	current, err := os.ReadFile(filepath.Join(dir, "models/upstream.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(upstream, current) || !info.ModTime().Equal(after.ModTime()) {
		t.Fatal("annotation-free input changed")
	}
	input(t, dir, "models/c.go", "package models\n//gobetter:struct builder\ntype C struct{Value int}\n")
	command(t, dir, "generate", "./...")
	for _, name := range []string{"models/a_gob.go", "models/b_gob.go", "models/c_gob.go", "models/upstream_gob.go", "models/sub/model_gob.go"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
	command(t, dir, "test", "-mod=readonly", "-count=1", "./...")
	command(t, dir, "tool", "gobetter", "check", "-config=models/gobetter.json", "./models")
	command(t, dir, "tool", "gobetter", "check", "./models/sub")
	mod, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mod), "tool github.com/mobiletoly/gobetter/cmd/gobetter") {
		t.Fatalf("tool registration absent: %s", mod)
	}
}
