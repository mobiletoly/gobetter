package example

import (
	"strings"
)

type Person struct { //+gob:Constructor
	firstName, lastName string                                   //+gob:getter
	Age                 int                                      `json:"age"`
	Description         *string                                  `json:"description"` //+gob:_
	Tags                []int                                    `json:"tags"`
	zz                  func(a1, a2 int, a3 *string) interface{} //+gob:getter
	test                strings.Builder                          //+gob:getter
	test3               *map[string]interface{}
	xml                 *string //+gob:getter +gob:acronym
}
