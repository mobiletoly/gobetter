package example

//go:generate gobetter main.go

import (
	"go/ast"
	"strings"
)

// DummyInterface for interface
type DummyInterface interface{}

// HelloStruct comment
type HelloStruct struct { //+constructor
	FirstName, LastName string  //+required
	Age                 int     `json:"age"` //+required
	Description         *string `json:"description"`
	Tags                []int   `json:"tags"`
	ZZ                  func(a1, a2 int,
		a3 *string) interface{} //+required
	Test  strings.Builder //+required
	test2 *ast.Scope
}

type AnotherStruct struct { //+constructor
	FieldAlpha string // +required
	FieldBeta  string
	FieldGamma HelloStruct // +required
}

func test() {
	var z *ast.Scope = nil
	println(z)
}
