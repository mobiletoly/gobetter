package source_test

import (
	"context"
	"strings"
	"testing"

	"github.com/mobiletoly/gobetter/internal/source"
)

func TestGenericBindingAndAliasNameCollisions(t *testing.T) {
	dir := module(t, map[string]string{"models.go": `package models
type gobetterT0 int
type gobetterArg int
type OuterConfig int
type OuterConfig_2 int
type Base[T any] struct { Value T }
//gobetter:struct builder
type Outer[Outer ~int | ~int64] struct {
    Base[Outer]
    Config struct { Outer Outer ` + "`json:\"Outer\"`" + ` }
    Call func(Outer int) Outer
    Value gobetterArg
    Array [len([2]func(){func(){ Outer:=3;_ = Outer }})]Outer
}
var _=NewOuterBuilder[int]().Base(Base[int]{}).Config(struct { Outer int ` + "`json:\"Outer\"`" + ` }{}).Call(nil).Value(1).Array([2]int{}).Build()
`})
	r, err := source.Load(context.Background(), source.Request{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Plans) != 2 || r.Plans[1].Target.Name != "OuterConfig_3" {
		t.Fatalf("alias allocation: %+v", r.Plans)
	}
	if r.Plans[0].Target.TypeParameters[0].Binder == "gobetterT0" || r.Plans[0].Names.Argument == "gobetterArg" {
		t.Fatal("generated binder/local collided with source")
	}
	for _, data := range r.Bootstrap {
		code := string(data)
		if !strings.Contains(code, `json:"Outer"`) || !strings.Contains(code, "Outer int") || !strings.Contains(code, "Outer := 3") {
			t.Fatalf("same-spelled member/parameter/local/tag was rewritten:\n%s", code)
		}
	}
	compileOverlay(t, dir, r.Bootstrap)
}
