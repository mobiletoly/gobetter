//go:generate gobetter $GOFILE
package example

import (
	"go/ast"
	"strings"
)

type Person struct { //+gob:constructor
	firstName, lastName string                                   //+gob:required +gob:getter
	Age                 int                                      `json:"age"` //+gob:required
	Description         *string                                  `json:"description"`
	Tags                []int                                    `json:"tags"`
	zz                  func(a1, a2 int, a3 *string) interface{} //+gob:required +gob:getter
	test                strings.Builder                          //+gob:required +gob:getter
	test2               *ast.Scope
	test3               *map[string]interface{} //+gob:required +gob:getter
}

// AnotherPerson is not marked with constructor flag and will not be processed
type AnotherPerson struct {
	FirstName, LastName string //+gob:required
	Age                 int    `json:"age"` //+gob:required
}
