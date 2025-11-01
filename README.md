# gobetter - Go Builder Pattern Generator

**gobetter** is a code generator that creates type-safe builder patterns for Go structs, enforcing
mandatory fields at compile time through a fluent API similar to named arguments.

```go
type Config struct { //+gob:Constructor
    Env string
    ListenPort int

    Database struct { //+gob:Constructor
        Driver string
        Host   string
        Port   int
    }
}
```

```shell
go generate ./...
```

**Generated builders:**
```go
// Clean naming without underscores
db := NewConfigDatabaseBuilder().
    Driver("postgres").
    Host("db.example.com").
    Port(5432).
    Build()

c := NewConfigBuilder().
    Database(*db).
	Env("dev").
	ListenPort(8080).
    Build()
```


## Features

- **Compile-time safety** - Missing mandatory fields cause compilation errors
- **IDE-friendly** - Excellent autocomplete support showing only the next required field
- **Builder pattern** - Fluent API with method chaining
- **Nested struct support** - Generate builders for inner structs with clean naming
- **Generics-ready** - Works with Go type parameters (Go 1.18+) without additional setup
- **Struct tag preservation** - Maintains JSON, validation, and other struct tags
- **Flexible configuration** - Control visibility, optional fields, and generation scope

**IDE Autocomplete (no plugin needed)** - Only shows the next mandatory field:

![Autocomplete](autocomplete.png)

**Compile-time Validation** - Missing fields cause compilation errors:

![Missing Field error](error_sample.png)

## The Problem

Go structs don't enforce required fields. Consider this example:

```go
type Person struct {
    FirstName   string
    LastName    string
    Age         int
    Description string
}

// Traditional struct initialization
person := Person{
    FirstName: "Joe",
    LastName:  "Doe",
    Age:       40,
    // Easy to forget required fields!
}
```

**Where this breaks down:**
- Missing required fields still compile (zero values sneak in).
- Refactors are risky: adding a required field means hunting every construction site.
- Constructors are order‑sensitive and error‑prone; many code generators don’t emit them at all.
- IDEs can’t guide the next required field.

## The Solution

```go
p := NewPersonBuilder().
    DOB(time.Date(1990, 1, 1, 0, 0, 0, 0, time.UTC)).
    FirstName("John").
    LastName("Doe").
    Build()
```

**gobetter** generates type-safe builder patterns that:
- ✅ **Enforce required fields** at compile time
- ✅ **Prevent field order mistakes** through method chaining
- ✅ **Auto-update** when you add/remove fields
- ✅ **Provide excellent IDE support** with autocomplete

## Why builders instead of struct literals or `New...()` constructors?

### Plain struct literals / direct assignment

- **Silent omissions.** With keyed struct literals, leaving out a field is legal and compiles; the
  field is just the zero value. If that field is *logically required*, you won’t find out until
  runtime.
- **Refactor pain.** When you add a new required field, you must manually audit every construction
  site. Miss one, and you ship a subtle bug. The step-builder makes this a **compile error** until
  the new step is provided.
- **No guidance in IDEs.** Autocomplete can’t tell you what’s required next; the step-chain exposes
  exactly one valid next method.

### Hand-written `NewX(...)` constructors

- **Argument soup.** Go has no named parameters; long `NewX(a, b, c, d)` calls are order-sensitive
  and easy to mix up—especially when types repeat (`string, string, time.Time`). The compiler won’t
  catch swapped arguments of the same type.
- **Generated code rarely ships constructors.** Tools like Swagger/OpenAPI or ORM generators
  typically emit structs without `New...` helpers. **gobetter** can be applied to those externally
  generated files (e.g., `-generate-for=exported`) to produce builders **without modifying the
  original code**.

### What you get with gobetter

- **Compile-time guarantees**: can’t build until all mandatory fields are provided.
- **Refactor-friendly**: adding/removing required fields updates the chain; callers won’t compile
  until fixed.
- **Great DX**: fluent steps + precise autocomplete; optionals can be skipped or added later.
- **Inner struct support**: generates builders for inner structs with clean naming.

## How it works (in one line)

gobetter generates a chain of tiny step types (each `struct{ root *T }`) that expose only the next
valid setter. Setters are trivial assignments that Go inlines, so no performance of memory penalty;
the step values stay on the stack, and `Build()` returns the single `*T` you’re constructing. Net
result: compile‑time required with essentially zero runtime overhead.

## Installation

Install **gobetter** as standalone utility:

```bash
go install github.com/mobiletoly/gobetter@latest
```

or if you use Go 1.24+ then you have a better alternative to use **gobetter** as
a tool, instead of installing it system-wide:

```bash
go get -tool github.com/mobiletoly/gobetter@latest
```


## Quick Start

### 1. Annotate Your Structs

Add annotations to your Go structs:

```go
package main

/* Put this line on top of the file if you installed gobetter as standalone utility */
//go:generate gobetter -input $GOFILE

/* OR put this line on top of the file if you installed gobetter as tool in your go.mod */
//go:generate go tool gobetter -input $GOFILE

type Person struct { //+gob:Constructor
	FirstName   string
	LastName    string
	email       string //+gob:getter
	dob         string //+gob:getter +gob:acronym
	Score       int
	Description string //+gob:_
}
```

IMPORTANT: +gob:getter annotation must be used for private fields (starting with lowercase letter)
only.

### 2. Generate Code

Run the generator to generate all annotated structs:

```bash
go generate ./...
```

it will result in creating files with suffix `_gob.go` for each of your file that contains 
annotated structs.

### 3. Use the Generated Builder

```go
person := NewPersonBuilder().
    DOB("01/01/1990").
    Email("john.doe@example.com").
    FirstName("John").
    LastName("Doe").
    Score(85).
    Build()

// Set optional fields after building
person.Description = "Software engineer"

fmt.Println(person.FirstName)   // "John"
fmt.Println(person.LastName)    // "Doe"
fmt.Println(person.Email())     // "john.doe@example.com" (getter function to call from outside)
fmt.Println(person.DOB())       // "01/01/1990" (getter function, acronym is DOB instead of dob)
fmt.Println(person.Score)       // 85 (public field, no function needed)
fmt.Println(person.Description) // "Software engineer"
```

## Annotations Reference

| Annotation           | Description                              | Example                                   |
|----------------------|------------------------------------------|-------------------------------------------|
| `//+gob:Constructor` | Generate builder for struct              | `type Person struct { //+gob:Constructor` |
| `//+gob:constructor` | Generate package-level builder           | `type person struct { //+gob:constructor` |
| `//+gob:getter`      | Generate getter for private field        | `name string //+gob:getter`               |
| `//+gob:acronym`     | Treat field as acronym (DOB vs Dob)      | `dob string //+gob:acronym`               |
| `//+gob:_`           | Mark field as optional (skip in builder) | `description string //+gob:_`             |

## Configuration Options

### Command Line Flags

| Flag            | Values                     | Description                        |
|-----------------|----------------------------|------------------------------------|
| `-input`        | `<path>`                   | Input file or directory            |
| `-generate-for` | `all\|exported\|annotated` | Which structs to process           |
| `-constructor`  | `exported\|package\|none`  | Constructor visibility level       |
| `-sort`         | `seq\|abc`                 | Builder step order (`abc` default) |

### Examples

```bash
# Process only annotated structs (default)
gobetter -input=models.go

# Process all exported structs without annotations
gobetter -input=models.go -generate-for=exported

# Generate package-level constructors for all structs
gobetter -input=models.go -generate-for=all -constructor=package
```

### Sorting

With command-line flag `-sort=seq`, the builder steps maintain the order of struct fields as
declared. With `-sort=abc` (the default), fields are sorted alphabetically. 

For example for this structure:
```go
type Person struct {
    FirstName string
    LastName  string
    Age       int
}
```

the `-sort=seq` will generate:
```go
bld := NewPersonBuilder().
    FirstName("John").
    LastName("Doe").
    Age(40).
    Build()
```

and `-sort=abc` will generate:
```go
bld := NewPersonBuilder().
    Age(40).
    FirstName("John").
    LastName("Doe").
    Build()
```


## Optional IDE Integration

### IntelliJ IDEA / GoLand

Set up a File Watcher for automatic generation (no need to run `go generate`):

1. Go to **Preferences → Tools → File Watchers**
2. Add **Custom** watcher:
   - **Name**: `Go Generate`
   - **File type**: `Go files`
   - **Program**: `go`
   - **Arguments**: `generate`
   - **Scope**: Create scope with pattern `file:*.go&&!file:*_gob.go`

Now builders regenerate automatically when you save Go files


## Performance

**Summary:** For typical structs, gobetter’s step-builder is *as fast* as direct struct
initialization and has the *same* allocation profile.

- **CPU:** Direct literal ~19.8–20.8 ns/op; Builder chain ~20.1–24.1 ns/op in our latest run (
  averages ≈ **20.18 ns/op** vs **21.64 ns/op**, respectively). The single-digit ns delta is within
  typical microbenchmark variance and both approaches remain essentially equivalent for real
  workloads.
- **Allocations:** **1 alloc/op** (the single `*T` instance you ultimately build), **~96 B/op** for
  the `Person` example. Step structs are tiny (a single pointer to the root) and stay on the **stack
  **.
- **Why it’s fast:**
    - Setters are trivial field assignments that the compiler **inlines**.
    - Step structs are returned **by value** and typically do **not escape** (escape analysis keeps
      them on stack).
    - `Build()` returns the same `*T` allocated once at the start of the chain — identical to
      `&T{...}`.

### Reproduce the benchmark

```go
func BenchmarkDirectLiteral(b *testing.B) {
    b.ReportAllocs()
    for i := 0; i < b.N; i++ {
        p := &Person{firstName: "John", lastName: "Doe", dob: tDOB, Email: "john.doe@example.com"}
        sink = p
    }
}

func BenchmarkBuilderChain(b *testing.B) {
    b.ReportAllocs()
    for i := 0; i < b.N; i++ {
        p := NewPersonBuilder().
            FirstName("John").
            LastName("Doe").
            DOB(tDOB).
            Email("john.doe@example.com").
            GobFinalizer().
            Build()
        sink = p
    }
}
```

**Sample results (one machine):**

```
BenchmarkDirectLiteral-12    	57792015        	20.02 ns/op     	96 B/op  	1 allocs/op
BenchmarkDirectLiteral-12    	59628193        	19.77 ns/op     	96 B/op  	1 allocs/op
BenchmarkDirectLiteral-12    	59373484        	20.78 ns/op     	96 B/op  	1 allocs/op
BenchmarkDirectLiteral-12    	57435504        	19.94 ns/op     	96 B/op  	1 allocs/op
BenchmarkDirectLiteral-12    	55259538        	20.38 ns/op     	96 B/op  	1 allocs/op
BenchmarkBuilderChain-12     	58040876        	20.08 ns/op     	96 B/op  	1 allocs/op
BenchmarkBuilderChain-12     	57155670        	21.04 ns/op     	96 B/op  	1 allocs/op
BenchmarkBuilderChain-12     	49360202        	22.16 ns/op     	96 B/op  	1 allocs/op
BenchmarkBuilderChain-12     	54273560        	24.14 ns/op     	96 B/op  	1 allocs/op
BenchmarkBuilderChain-12     	55646794        	20.80 ns/op     	96 B/op  	1 allocs/op
```

(Results vary by CPU/Go version and flags; use multiple runs with `-count` for stability.)
