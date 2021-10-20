# GO Better - code generator for struct required fields

This project is an attempt to address lack of required fields in Go's struct types and to create a constructor that
will actually enforce specifying mandatory fields in constructor with the approach similar to "named arguments".
Named arguments will allow you to specify multiple arguments in constructor without concerns that you have to be
very careful passing arguments in correct order.

As you are aware, when you create a structure in Go - you cannot specify required fields. For example, if we have
a structure for Person such as

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
the suggestions you can find is to create a constructor function with arguments representing required fields.
In this case you can create and fill `Person` structure with `NewPerson()` function. Here is an example:

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

**gobetter** addresses this issue by creating constructor function for you and by wrapping each parameter in its own
type. In this case compiler will raise an error if you missed a parameter or put it in a different order. 

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

Tool uses generation approach to create constructors with named arguments. First you have to add `go:generate` comment
into a file with a structures you want to create  required parameters for, after that you can mark required fields
with a special comment. E.g. this is how your data structure to serialize/deserialize JSON is going to look like:

```
//go:generate gobetter -input $GOFILE
package main

type Person struct { //+gob:Constructor
	firstName   string  //+gob:getter
	lastName    string  //+gob:getter
	Age         int     
	Description string  //+gob:_
}
```

(you can add field tags e.g. `json:"age"` into your struct if you need)

- `+gob:Constructor` comment serves as a flag and must be on the same line as struct (you can add more text to this
comment but flag needs to be a separate word). It instructs gobetter to generate argument structures and
constructor for this structure. Please read below to find out why "Constructor" starts with upper-cased "C".


- `//+gob:getter` is to generate a getter for field, should be applied only for fields that start in lowercase (fields
that are not accessible outside of a package). It will effectively make these fields read-only for callers outside
a package.


- `//+gob:_` flag in comment hints gobetter that structure field is option and should not be added to
constructor.


All you have to do now is to run `go generate` tool to generate go files that will be containing argument structures
as well as constructors for your structures.

```shell
go generate ./...
```

For example if you have a file in example package `example/main.go` then new file `example/main_gob.go` will be
generated, and it will contain argument structures and constructors for structures from `main.go` file.

Now you can build Person structure with a call:

```
person := NewPerson(
    Person_FirstName("Joe"),
    Person_LastName("Doe"),
    Person_Age(40),
)
// optional parameters
person.Description = "some description"
```

### Constructor options

Unless you specify otherwise with comnand-line flags - gobetter only processes structures marked with `//+gob:`
comment annotations, and you have few options to choose from:

- `//+gob:Constructor` - generate upper-cased exported constructor in form of **NewClassName**. This flag is
  honored only if class itself is exported (started with uppercase character), otherwise package-level lower-cased
  constructor **newClassName** will be generated;


- `//+gob:constructor` - generate package-level constructor in form of **newClassName** even for exported classes;


- `//+gob:_` - no constructor is generated. This flag is useful if you don't want to generate
  constructor but still want for gobetter to process another fields, such as marked with `gob:getter` to generate
  getters;


### Integration with IntelliJ

It can be annoying to run `go generate ./...` from a terminal every time. Moreover, call this command will be generating
required fields support for all your files every time, while most of the time you want to do it on per-file basis.
The easiest approach for IntelliJ is to set up a FileWatcher for .go files and run generate command every time you
change a file. Depending on your OS - instructions can be slightly different but in overall they remain the same.
For Mac OS in your IntelliJ select from main menu **IntelliJ IDEA / Preferences / Tools / File Watcher** and add
<custom> task. Name it `Go Generate files` and setup **Files type**: `Go files`, **Program**: `go`,
**Arguments**: `generate`.<br>
At this point it should work, but File Watcher will be monitoring your entire project directory and not only your own
files, but also generated _gob.go files as well. It means that gob files will be constantly re-generated, and it might
annoy you with a little status bar progress constantly flashing. We want to exclude generated gob files from being
watched by selecting **Scope** text field of **File Watcher** dialog. Click `...` button on the right of the **Scope**
text field, in new window create new **Local** scope, name it (e.g. `Go project files`) and add
`file:*.go&&!file:*_gob.go` to **Pattern** text field.

This will do it. Now when you save Go file - `go generate` will be automatically run for your file.

### Gobetter generator customization

**gobetter** generator has few switches allowing you a better control of generated output.

`-input <input-file-name>` - input file name where to read structures from

`-output <output-file-name>` - optional file name to save generated data into. if this switch is not specified
then gobetter will create a filename with suffix `_gob.go` in the same directory where the input file resides.

`-generate-for all|exported` - sometimes you don't want to annotate structures with *//+gob:* constructor
annotation, or you don't have this option, because files with a structures could be auto-generated for you by
some other tool. In this case you can invoke gobetter from some other file and pass `-generate-for` flag to
specify that you want to generate constructors for all struct types. `all` option will process all exported
and package-level structs while `exported` will process only exported (started with uppercase character)
structures.

Example:

```
package main

//go:generate gobetter -input=./internal/graph/model/models_gen.go -default-types=all

import (
    ...
)
```
