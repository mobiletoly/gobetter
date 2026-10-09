package config

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/mobiletoly/gobetter/internal/structgen"
)

func requireCode(t *testing.T, err error, want structgen.Code) {
	t.Helper()
	ds, ok := err.(structgen.Diagnostics)
	if !ok {
		t.Fatalf("expected diagnostic %s, got %T: %v", want, err, err)
	}
	for _, d := range ds {
		if d.Code == want {
			if d.Position.Filename == "" || d.Position.Line < 1 || d.Position.Column < 1 {
				t.Fatalf("missing positioned diagnostic: %+v", d)
			}
			return
		}
	}
	t.Fatalf("missing %s in %v", want, ds)
}

func TestRemovedOrderPropertyDiagnostics(t *testing.T) {
	for _, value := range []string{"source", "name"} {
		data := `{"schema_version":1,"types":[{"package":".","type":"T","order":"` + value + `"}]}`
		_, err := Decode("gobetter.json", []byte(data))
		ds, ok := err.(structgen.Diagnostics)
		if !ok || len(ds) != 1 || ds[0].Code != structgen.ConfigSchema || ds[0].Message != "unknown property order" || ds[0].Position.Filename != "gobetter.json" || ds[0].Position.Line != 1 || ds[0].Position.Offset != strings.Index(data, `"`+value+`"`) {
			t.Fatalf("removed order input must fail at strict JSON boundary: %v", err)
		}
	}
}

func TestStrictDecode(t *testing.T) {
	cases := []struct {
		name, data string
		code       structgen.Code
	}{
		{"malformed", `{"schema_version":1,`, structgen.ConfigSyntax},
		{"root array", `[]`, structgen.ConfigSchema},
		{"extra document", `{"schema_version":1} {}`, structgen.ConfigSyntax},
		{"missing version", `{}`, structgen.ConfigSchema},
		{"wrong version", `{"schema_version":2}`, structgen.ConfigSchema},
		{"fractional version", `{"schema_version":1.0}`, structgen.ConfigSchema},
		{"root key", `{"schema_version":1,"extra":true}`, structgen.ConfigSchema},
		{"case variant", `{"schema_version":1,"Types":[]}`, structgen.ConfigSchema},
		{"types object", `{"schema_version":1,"types":{}}`, structgen.ConfigSchema},
		{"type not object", `{"schema_version":1,"types":[true]}`, structgen.ConfigSchema},
		{"type unknown", `{"schema_version":1,"types":[{"package":".","type":"T","wat":1}]}`, structgen.ConfigSchema},
		{"type case variant", `{"schema_version":1,"types":[{"package":".","Type":"T"}]}`, structgen.ConfigSchema},
		{"root duplicate", `{"schema_version":1,"schema_version":1}`, structgen.ConfigDuplicate},
		{"escaped duplicate", `{"schema_version":1,"schema_\u0076ersion":1}`, structgen.ConfigDuplicate},
		{"record duplicate key", `{"schema_version":1,"types":[{"package":".","type":"T","type":"T"}]}`, structgen.ConfigDuplicate},
		{"field duplicate", `{"schema_version":1,"types":[{"package":".","type":"T","fields":{"x":{"getter":true,"getter":false}}}]}`, structgen.ConfigDuplicate},
		{"field name duplicate", `{"schema_version":1,"types":[{"package":".","type":"T","fields":{"x":{},"x":{}}}]}`, structgen.ConfigDuplicate},
		{"null root", `null`, structgen.ConfigSchema},
		{"null derive", `{"schema_version":1,"types":[{"package":".","type":"T","derive":null}]}`, structgen.ConfigSchema},
		{"null bool", `{"schema_version":1,"types":[{"package":".","type":"T","fields":{"x":{"getter":null}}}]}`, structgen.ConfigSchema},
		{"bool as string", `{"schema_version":1,"types":[{"package":".","type":"T","fields":{"x":{"getter":"false"}}}]}`, structgen.ConfigSchema},
		{"field unknown key", `{"schema_version":1,"types":[{"package":".","type":"T","fields":{"x":{"skip-builder":true}}}]}`, structgen.ConfigSchema},
		{"empty derive", `{"schema_version":1,"types":[{"package":".","type":"T","derive":[]}]}`, structgen.InvalidOption},
		{"unknown derive", `{"schema_version":1,"types":[{"package":".","type":"T","derive":["wat"]}]}`, structgen.InvalidOption},
		{"duplicate derive", `{"schema_version":1,"types":[{"package":".","type":"T","derive":["builder","builder"]}]}`, structgen.InvalidOption},
		{"order", `{"schema_version":1,"types":[{"package":".","type":"T","order":"abc"}]}`, structgen.ConfigSchema},
		{"constructor", `{"schema_version":1,"types":[{"package":".","type":"T","constructor":"none"}]}`, structgen.InvalidOption},
		{"keyword name", `{"schema_version":1,"types":[{"package":".","type":"T","fields":{"x":{"name":"func"}}}]}`, structgen.InvalidOption},
		{"missing package", `{"schema_version":1,"types":[{"type":"T"}]}`, structgen.ConfigTarget},
		{"missing type", `{"schema_version":1,"types":[{"package":"."}]}`, structgen.ConfigTarget},
		{"package glob", `{"schema_version":1,"types":[{"package":"./...","type":"T"}]}`, structgen.ConfigTarget},
		{"generic selector", `{"schema_version":1,"types":[{"package":".","type":"T[int]"}]}`, structgen.ConfigTarget},
		{"blank field", `{"schema_version":1,"types":[{"package":".","type":"T","fields":{"_":{}}}]}`, structgen.ConfigTarget},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Decode("gobetter.json", []byte(tc.data))
			requireCode(t, err, tc.code)
		})
	}
}

func TestPresenceAndLocations(t *testing.T) {
	data := []byte("{\n\"schema_version\":1,\n\"types\":[{\"package\":\".\",\"type\":\"T\",\"fields\":{\"x\":{\"getter\":false,\"optional\":false}}}]\n}")
	document, err := Decode("gobetter.json", data)
	if err != nil {
		t.Fatal(err)
	}
	item := document.Types[0]
	if item.Options.Derive != nil || item.Options.Constructor != nil {
		t.Fatal("omitted type settings became explicit")
	}
	field := item.Fields[0]
	if field.Options.Getter == nil || *field.Options.Getter || field.Options.Optional == nil || *field.Options.Optional || field.Options.SkipBuilder != nil {
		t.Fatal("false/omission distinction lost")
	}
	if item.Position.Line != 3 || field.Position.Line != 3 {
		t.Fatalf("wrong JSON source positions: %+v %+v", item.Position, field.Position)
	}
}

func TestBindScopeAndSelectors(t *testing.T) {
	module := t.TempDir()
	dir := filepath.Join(module, "models")
	configFile := filepath.Join(dir, "gobetter.json")
	target := structgen.Target{PackageDir: dir, Path: "T", Name: "T", Kind: structgen.DefinedStruct, Fields: []structgen.Field{{Name: "value"}}}
	inventory := []PackageInventory{{Dir: dir, Active: []structgen.Target{target}, Inactive: []string{"Inactive"}}}
	cases := []struct {
		name, data string
		code       structgen.Code
		want       int
	}{
		{"relative to config", `{"schema_version":1,"types":[{"package":".","type":"T"}]}`, "", 1},
		{"nested unknown", `{"schema_version":1,"types":[{"package":".","type":"T.Unknown"}]}`, structgen.ConfigTarget, 0},
		{"unknown type", `{"schema_version":1,"types":[{"package":".","type":"Missing"}]}`, structgen.ConfigTarget, 0},
		{"unknown active field", `{"schema_version":1,"types":[{"package":".","type":"T","fields":{"missing":{}}}]}`, structgen.ConfigTarget, 0},
		{"known inactive", `{"schema_version":1,"types":[{"package":".","type":"Inactive","fields":{"missing":{}}}]}`, "", 0},
		{"outside scope", `{"schema_version":1,"types":[{"package":"../other","type":"Missing","fields":{"missing":{}}}]}`, "", 0},
		{"outside scope duplicate records", `{"schema_version":1,"types":[{"package":"../other","type":"Missing"},{"package":"../other","type":"Missing"}]}`, "", 0},
		{"outside module", `{"schema_version":1,"types":[{"package":"../..","type":"T"}]}`, structgen.ConfigTarget, 0},
		{"duplicate normalized record", `{"schema_version":1,"types":[{"package":".","type":"T"},{"package":"./","type":"T"}]}`, structgen.ConfigDuplicate, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			document, err := Decode(configFile, []byte(tc.data))
			if err != nil {
				t.Fatal(err)
			}
			overrides, err := Bind(document, configFile, module, inventory)
			if tc.code != "" {
				requireCode(t, err, tc.code)
				return
			}
			if err != nil || len(overrides) != tc.want {
				t.Fatalf("binding: %+v %v", overrides, err)
			}
			if tc.want == 1 && overrides[0].PackageDir != dir {
				t.Fatal("package did not resolve relative to config")
			}
		})
	}
	document, err := Decode(configFile, []byte(`{"schema_version":1,"types":[{"package":".","type":"T"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	inventory[0].Active = append(inventory[0].Active, target)
	_, err = Bind(document, configFile, module, inventory)
	requireCode(t, err, structgen.ConfigTarget)
}
