package example

type Person struct { //+gob:Constructor
	firstName, lastName string  //+gob:getter
	Age                 int     `json:"age"`
	Description         *string `json:"description"`
	Tags                []int   `json:"tags"`
}
