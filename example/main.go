package example

import (
	"go/ast"
	"strings"
)

type Person struct { //+gob:Constructor
	firstName, lastName *string                                  //+gob:getter
	Age                 int                                      `json:"age"`
	Description         *string                                  `json:"description"` //+gob:_
	Tags                []int                                    `json:"tags"`
	zz                  func(a1, a2 int, a3 *string) interface{} //+gob:getter
	test                strings.Builder                          //+gob:getter
	test2               *ast.Scope                               //+gob:getter
	test3               *map[string]interface{}
}

// AnotherPerson is not marked with constructor flag and will not be processed
type anotherPerson struct { //+gob:constructor
	FirstName, LastName string
	Age                 int `json:"age"`
	result              int //+gob:getter
}
