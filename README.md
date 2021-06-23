# GO Better - code generator for struct required fields

This project is an attempt to address lack of required fields in Go's struct types. As you are aware, when you create a
structure in Go - you cannot specify required fields. For example, if we have a structure for Person such as

```
type Person struct {
	FirstName   string
	LastName    string
	Age         int
	Description string // optional field
}
```

then normally this is how you will create the instance of this structure:

```
var person = Person{
    FirsName: "Joe",
    LastName: "Doe",
    Age: 40,
}
```

This is all good unless you have a different places where you have to create a `Person` structure, then when you
add new fields, it will become a challenge to scan through code to find all occurrences of creating Person. One of
the suggestions you can find is to create a construction function with arguments representing required fields.
In this case you can create and fill `Person` structure with `NewPerson()` constructor. Here is an example:

```
func NewPerson(firstName string, lastName string, age int) Person {
    return Person{
        FirstName = firstName,
        LastName = lastName,
        Age = age,
    }
}
```

The typical call will be `person := NewPerson("Joe", "Doe", 40)`.
This is actually not a bad solution, but unfortunately it means that you have to manually update your `NewPerson`
function every time when you add or remove fields. Moreover, because Go does not have named parameters, you
need to be very careful when you move fields within the structure or add a new one, because you might start
passing wrong values. E.g. if you swap FirstName and LastName in Person structure then suddenly your call to `NewPerson` 
will be resulting in FirstName being "Doe" and LastName being "Joe". Compiler does not help us here.

The approach I would like to use is to create a simple struct wrapper for every required field, such as

```
// structures for arguments (you don't create them directly)

type PersonFirstNameArg struct {
    Arg string
}
type PersonLastNameArgArg struct {
    Arg string
}
type PersonAgeArgArg struct {
    Arg int
}

// single-argument constructor for every argument structure (you pass it to main constructor)

func PersonFirstName(arg string) PersonFirstNameArg {
    return PersonFirstNameArg{Arg: arg}
}
func PersonLastName(arg string) PersonLastNameArg {
    return PersonLastNameArg{Arg: arg}
}
func PersonAge(arg int) PersonAgeArg {
    return PersonAgeArg{Arg: arg}
}

```

then constructor function is going to look like

```
func NewPerson(
    argFirstName PersonFirstNameArg,
    argLastName PersonLastNameArg,
    argAge PersonAgeArg,
) Person {
    return Person{
        FirstName: argFirstName.Arg,
        LastName: argLastName.Arg,
        Age: argAge.Arg,
    }
}
```

Here is typical call to create a new instance of Person struct

```
person := NewPerson(
    PersonFirstName("Joe"),
    PersonLastName("Doe"),
    PersonAge(40),
)
```

This is it! Now we have required fields and compiler will guarantee (with compiler-time errors) that we pass
parameters in correct order.

But you don't want to do this work manually, especially if you have to deal with many large structures. That is why
we have depeveloper a tool called **gobetter** to generate all this structs and constructors for you.

### Pre-requisites

You have to install two tools:

First one is **goimports** (if you don't have it already installed). It will be used by **gobetter** to optimize
imports and perform proper formatting.

```shell
go get -u golang.org/x/tools/cmd/goimports
```

Then you must install **gobetter** itself:

```shell
go get -u github.com/mobiletoly/gobetter
```

### Usage

Tool is very easy to use. First you have to add `go:generate` comment into a file with a structures you want to create
required parameters for, after that you can mark required fields with a special comment. E.g. this is how your 
data structure that you use to serialize/deserialize JSON is going to look like

```
//go:generate gobetter $GOFILE
package main

...

type Person struct { //+gob:constructor
	firstName   string `json:"first_name"` //+gob:required, +gob:getter
	lastName    string `json:"last_name"` //+gob:required, +gob:getter
	Age         int    `json:"age"` //+gob:required
	Description string `json:"description"`
}
```

`+gob:constructor` comment serves as a flag and must be on the same line as struct (you can add more text to this comment
but flag needs to be a separate word). It instructs gobetter to generate argument structures and constructor for this
structure.

`+gob:required` flag in comment hints gobetter that structure field is required and must be added to constructor.

`+gob:getter` is to generate a getter for field, should be applied only for fields that start in lowercase (fields
that are not accessible outside of a package). It will effectively make these fields read-only for callers outside
of a package.

All you have to do now is to run `go generate` tool to generate go files that will be containing argument structures
as well as constructors for your structures.

```shell
go generate ./...
```

For example if you have a file in example package `example/main.go` then new file `example/main_gob.go` will be
generated, and it will contain argument structures and constructors for structures from `main.go` file.
