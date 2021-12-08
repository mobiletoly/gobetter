package example

import (
	"go/ast"
	"strings"
)

type Person struct { //+gob:constructor
	firstName, lastName *string                                  //+gob:getter
	Age                 int                                      `json:"age"`
	Description         *string                                  `json:"description"` //+gob:_
	Tags                []int                                    `json:"tags"`
	zz                  func(a1, a2 int, a3 *string) interface{} //+gob:getter
	test                strings.Builder                          //+gob:getter
	test2               *ast.Scope                               //+gob:getter
	test3               *map[string]interface{}
	xml                 *string //+gob:getter +gob:acronym
	anotherPerson       *anotherPerson
}

type fixedPerson struct {
	test string
	ap   *anotherPerson
}

// AnotherPerson is not marked with constructor flag and will not be processed
type anotherPerson struct { //+gob:constructor
	FirstName, LastName string
	Age                 int `json:"age"`
	result              int //+gob:getter
	dval                dummyValue
}

type dummyValue struct {
	str1 string
	str2 string
}

func a() {
	z := newFixedPerson(
		fixedPerson_Test(""),
		fixedPerson_Ap(newAnotherPerson_Ptr(
			anotherPerson_FirstName("fn"),
			anotherPerson_LastName("ln"),
			anotherPerson_Age(30),
			anotherPerson_Result(20),
			anotherPerson_Dval(newDummyValue(
				dummyValue_Str1("dummy1"),
				dummyValue_Str2("dummy2"),
			)),
		)),
	)
	println(z.test)
}
