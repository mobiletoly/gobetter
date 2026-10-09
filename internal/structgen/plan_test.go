package structgen

import (
	"go/ast"
	"go/token"
	"reflect"
	"testing"
)

func names(fields []Field) []string {
	var names []string
	for _, field := range fields {
		names = append(names, field.Name)
	}
	return names
}
func hasCode(t *testing.T, err error, code Code) {
	t.Helper()
	ds, ok := err.(Diagnostics)
	if !ok {
		t.Fatalf("expected diagnostics, got %T: %v", err, err)
	}
	for _, d := range ds {
		if d.Code == code {
			return
		}
	}
	t.Fatalf("missing %s in %v", code, ds)
}
func model() Target {
	return Target{PackageDir: "/models", Path: "Record", Name: "Record", Kind: DefinedStruct, Exported: true,
		Position: token.Position{Filename: "models.go", Line: 3, Column: 6}, Fields: []Field{
			{Name: "z", TypeExpr: ast.NewIdent("int")},
			{Name: "A", TypeExpr: ast.NewIdent("string")},
			{Name: "a", TypeExpr: ast.NewIdent("bool"), Options: FieldOptions{Name: new("Other")}},
		}}
}

func TestPrecedenceAndIndependentViews(t *testing.T) {
	target := model()
	target.Annotated = true
	target.Options = TypeOptions{Derive: new(Builder | String), Constructor: new(ExportedConstructor)}
	target.Fields[0].Options = FieldOptions{Getter: new(true), Optional: new(true), SkipEqual: new(true), SkipString: new(true)}
	external := TypeOverride{PackageDir: target.PackageDir, Path: target.Path, Options: TypeOptions{Derive: new(Builder | Equal)},
		Fields: []FieldOverride{{Name: "z", Options: FieldOptions{Getter: new(false), Optional: new(false), SkipEqual: new(false), SkipString: new(false)}},
			{Name: "a", Options: FieldOptions{Name: new("Other"), Getter: new(true), Optional: new(true)}}}}
	plans, err := MakePlans([]Target{target}, []TypeOverride{external}, Invocation{Options: TypeOptions{Derive: new(Builder | Equal | String), Constructor: new(PackageConstructor)}})
	if err != nil {
		t.Fatal(err)
	}
	p := plans[0]
	if p.Derive != Builder|Equal|String || p.Constructor != PackageConstructor {
		t.Fatalf("effective type settings: %+v", p)
	}
	if p.Fields[0].Optional || p.Fields[0].Getter || p.Fields[0].SkipEqual || p.Fields[0].SkipString {
		t.Fatal("explicit false did not clear source settings")
	}
	if p.Fields[2].SetterName != "Other" || p.Fields[2].GetterName != "Other" {
		t.Fatal("override did not preserve independent setter/getter naming")
	}
	for _, output := range []Outputs{Builder, Equal, String} {
		if got := names(p.FieldsFor(output)); !reflect.DeepEqual(got, []string{"z", "A", "a"}) {
			t.Fatalf("source ordering for %v: %v", output, got)
		}
	}
	if got := names(p.FieldsFor(Getters)); !reflect.DeepEqual(got, []string{"a"}) {
		t.Fatalf("independent getters: %v", got)
	}
	view := p.FieldsFor(Builder)
	view[0].Name = "changed"
	if !reflect.DeepEqual(names(p.Fields), []string{"z", "A", "a"}) || target.Fields[0].Optional {
		t.Fatal("view/normalization mutated canonical source inventory")
	}
}

func TestSelectionUnionAndDefaults(t *testing.T) {
	public := model()
	private := Target{PackageDir: "/models", Path: "private", Name: "private", Kind: DefinedStruct}
	alias := Target{PackageDir: "/models", Path: "Alias", Name: "Alias", Kind: UserAlias, Exported: true}
	cases := []struct {
		name       string
		mode       Selection
		annotation bool
		config     bool
		want       []string
	}{
		{"annotated default", Annotated, false, false, nil},
		{"exported", Exported, false, false, []string{"Record"}},
		{"all", All, false, false, []string{"Record", "private"}},
		{"annotated private union", Exported, true, false, []string{"Record", "private"}},
		{"configured private union", Exported, false, true, []string{"Record", "private"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			private.Annotated = tc.annotation
			var overrides []TypeOverride
			if tc.config {
				overrides = []TypeOverride{{PackageDir: "/models", Path: "private"}}
			}
			plans, err := MakePlans([]Target{public, private, alias}, overrides, Invocation{Selection: tc.mode})
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, plan := range plans {
				got = append(got, plan.Target.Path)
				if plan.Derive != Builder {
					t.Fatal("wrong defaults")
				}
				if plan.Target.Path == "private" && plan.Constructor != PackageConstructor {
					t.Fatal("auto must follow private type visibility")
				}
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("selection %v, want %v", got, tc.want)
			}
		})
	}
}

func TestGetterOnlyRoutes(t *testing.T) {
	for _, route := range []string{"annotation", "config", "cli"} {
		t.Run(route, func(t *testing.T) {
			target := Target{PackageDir: "/models", Path: "Secret", Name: "Secret", Kind: DefinedStruct, Exported: true, Fields: []Field{{Name: "value", Options: FieldOptions{Getter: new(true), SkipBuilder: new(true)}}}}
			var overrides []TypeOverride
			invocation := Invocation{}
			switch route {
			case "annotation":
				target.Annotated = true
				target.Options.Derive = new(Getters)
			case "config":
				target.Fields[0].Options = FieldOptions{}
				overrides = []TypeOverride{{PackageDir: "/models", Path: "Secret", Options: TypeOptions{Derive: new(Getters)}, Fields: []FieldOverride{{Name: "value", Options: FieldOptions{Getter: new(true), SkipBuilder: new(true)}}}}}
			case "cli":
				invocation.Selection = Exported
				invocation.Options.Derive = new(Getters)
			}
			plans, err := MakePlans([]Target{target}, overrides, invocation)
			if err != nil {
				t.Fatal(err)
			}
			p := plans[0]
			if p.Derive != Getters || p.Constructor != "" || len(p.FieldsFor(Builder)) != 0 || len(p.FieldsFor(Equal)) != 0 || len(p.FieldsFor(String)) != 0 {
				t.Fatal("getter-only plan contains unrelated output or a constructor")
			}
			if fields := p.FieldsFor(Getters); len(fields) != 1 || fields[0].GetterName != "Value" {
				t.Fatalf("getter inventory: %+v", fields)
			}
		})
	}
}

func TestPlanRejections(t *testing.T) {
	cases := []struct {
		name   string
		field  Field
		derive Outputs
		kind   DeclarationKind
		code   Code
	}{
		{"optional skip conflict", Field{Name: "value", Options: FieldOptions{Optional: new(true), SkipBuilder: new(true)}}, Builder, DefinedStruct, InvalidOption},
		{"exported getter", Field{Name: "Value", Options: FieldOptions{Getter: new(true)}}, Getters, DefinedStruct, InvalidOption},
		{"empty getter-only", Field{Name: "value"}, Getters, DefinedStruct, EmptyGetters},
		{"underscore initial", Field{Name: "_id"}, Builder, DefinedStruct, InvalidMethodName},
		{"uncased initial", Field{Name: "值"}, Builder, DefinedStruct, InvalidMethodName},
		{"lowercase override", Field{Name: "value", Options: FieldOptions{Name: new("lower")}}, Builder, DefinedStruct, InvalidMethodName},
		{"alias output", Field{Name: "value"}, Builder, UserAlias, UnsupportedTarget},
		{"nested method output", Field{Name: "value"}, String, NestedAlias, UnsupportedTarget},
		{"nested field getter", Field{Name: "value", Options: FieldOptions{Getter: new(true)}}, Builder, NestedAlias, UnsupportedTarget},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			target := model()
			target.Kind = tc.kind
			target.Annotated = true
			target.Fields = []Field{tc.field}
			target.Options.Derive = new(tc.derive)
			_, err := MakePlans([]Target{target}, nil, Invocation{})
			hasCode(t, err, tc.code)
		})
	}
	for _, name := range []string{"_id", "值"} {
		target := model()
		target.Annotated = true
		target.Fields = []Field{{Name: name, Options: FieldOptions{Name: new("ID")}}}
		plans, err := MakePlans([]Target{target}, nil, Invocation{})
		if err != nil || plans[0].Fields[0].SetterName != "ID" || plans[0].Fields[0].Name != name {
			t.Fatalf("explicit name override: %v", err)
		}
		target.Options.Derive = new(Equal)
		target.Fields[0].Options = FieldOptions{}
		if _, err := MakePlans([]Target{target}, nil, Invocation{}); err != nil {
			t.Fatalf("unrequested method name rejected: %v", err)
		}
	}
}

func TestNestedSelectionAndFieldExclusions(t *testing.T) {
	outer := Target{PackageDir: "/models", Path: "Outer", Name: "Outer", Kind: DefinedStruct, Annotated: true, Fields: []Field{{Name: "Config"}}}
	inner := Target{PackageDir: "/models", Path: "Outer.Config", ParentPath: "Outer", ParentField: "Config", Kind: NestedAlias, Fields: []Field{{Name: "Value"}}}
	plans, err := MakePlans([]Target{outer, inner}, nil, Invocation{})
	if err != nil || len(plans) != 2 || plans[1].Derive != Builder {
		t.Fatalf("nested default selection: %v %v", plans, err)
	}
	outer.Fields[0].Options.SkipBuilder = new(true)
	plans, err = MakePlans([]Target{outer, inner}, nil, Invocation{})
	if err != nil || len(plans) != 1 {
		t.Fatalf("skip-builder should suppress automatic nested selection: %v", err)
	}
	inner.Annotated = true
	_, err = MakePlans([]Target{outer, inner}, nil, Invocation{Options: TypeOptions{Derive: new(Getters)}})
	hasCode(t, err, EmptyGetters)
	// A global output list must not attach methods to independently selected aliases.
	outer.Options.Derive = new(Equal)
	plans, err = MakePlans([]Target{outer, inner}, nil, Invocation{Options: TypeOptions{Derive: new(Equal)}})
	if err != nil || plans[1].Derive != Builder {
		t.Fatalf("nested derive override leaked: %v", err)
	}
}

func TestUnusedFieldAndMalformedLowerLayer(t *testing.T) {
	target := model()
	target.Fields[0].Annotated = true
	_, err := MakePlans([]Target{target}, nil, Invocation{})
	hasCode(t, err, UnusedFieldDirective)
	target.Annotated = true
	target.Options.Constructor = new(Constructor("unknown"))
	_, err = MakePlans([]Target{target}, nil, Invocation{Options: TypeOptions{Constructor: new(AutoConstructor)}})
	hasCode(t, err, InvalidOption)
	_, err = MakePlans(nil, nil, Invocation{Options: TypeOptions{Derive: new(Outputs(0))}})
	hasCode(t, err, InvalidOption)
}
