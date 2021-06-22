//go:generate gobetter $GOFILE
package example

import (
	"go/ast"
	"strings"
)

// Person is not marked with +constructor flag
type Person struct { //+constructor
	FirstName, LastName string                                   //+required
	Age                 int                                      `json:"age"` //+required
	Description         *string                                  `json:"description"`
	Tags                []int                                    `json:"tags"`
	ZZ                  func(a1, a2 int, a3 *string) interface{} //+required
	Test                strings.Builder                          //+required
	test2               *ast.Scope
}

// AnotherPerson is not marked with +constructor flag
type AnotherPerson struct {
	FirstName, LastName string //+required
	Age                 int    `json:"age"` //+required
}
