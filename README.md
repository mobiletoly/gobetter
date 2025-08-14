# gobetter - Go Builder Pattern Generator

**gobetter** is a code generator that creates type-safe builder patterns for Go structs, enforcing mandatory fields at compile time through a fluent API similar to named arguments.

## ✨ Features

- **Compile-time safety** - Missing mandatory fields cause compilation errors
- **IDE-friendly** - Excellent autocomplete support showing only the next required field
- **Builder pattern** - Fluent API with method chaining
- **Nested struct support** - Generate builders for inner structs with clean naming
- **Struct tag preservation** - Maintains JSON, validation, and other struct tags
- **Flexible configuration** - Control visibility, optional fields, and generation scope

## 🎬 Demo

**IDE Autocomplete** - Only shows the next mandatory field:
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
    Description string // optional field
}

// Traditional struct initialization
person := Person{
    FirstName: "Joe",
    LastName:  "Doe",
    Age:       40,
    // Easy to forget required fields!
}
```

**Problems with traditional approaches:**
- ❌ Easy to forget required fields
- ❌ No compile-time validation
- ❌ Manual constructor functions need constant maintenance
- ❌ Parameter order mistakes (no named parameters in Go)

## The Solution

**gobetter** generates type-safe builder patterns that:
- ✅ **Enforce required fields** at compile time
- ✅ **Prevent field order mistakes** through method chaining
- ✅ **Auto-update** when you add/remove fields
- ✅ **Provide excellent IDE support** with autocomplete

## Installation

Install **gobetter** as standalone utility:

```bash
go install github.com/mobiletoly/gobetter@latest
```

or (even better) install **gobetter** to use as tool in your go.mod:

```
tool (
    github.com/mobiletoly/gobetter
)
```


## Quick Start

### 1. Annotate Your Structs

Add annotations to your Go structs:

```go
package main

// Put on top of the file
//go:generate gobetter -input $GOFILE

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
    FirstName("John").
    LastName("Doe").
	Email("john.doe@example.com").
    DOB("01/01/1990").
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

## 📝 Annotations Reference

| Annotation           | Description                              | Example                                   |
|----------------------|------------------------------------------|-------------------------------------------|
| `//+gob:Constructor` | Generate builder for struct              | `type Person struct { //+gob:Constructor` |
| `//+gob:constructor` | Generate package-level builder           | `type person struct { //+gob:constructor` |
| `//+gob:getter`      | Generate getter for private field        | `name string //+gob:getter`               |
| `//+gob:acronym`     | Treat field as acronym (DOB vs Dob)      | `dob string //+gob:getter +gob:acronym`   |
| `//+gob:_`           | Mark field as optional (skip in builder) | `description string //+gob:_`             |

## 🏗️ Nested Structs Support

**gobetter** supports nested structs with clean naming and type aliases:

```go
type Config struct { //+gob:Constructor
    host string //+gob:getter
    port int    //+gob:getter

    Database struct { //+gob:Constructor
        Driver string
        Host   string
        Port   int
    }
}
```

**Generated builders:**
```go
// Clean naming without underscores
database := NewConfigDatabaseBuilder().
    Driver("postgres").
    Host("db.example.com").
    Port(5432).
    Build()

config := NewConfigBuilder().
    Host("api.example.com").
    Port(8080).
    Database(*database).
    Build()
```

Run:

```bash
go generate ./...
go test -bench . -benchmem -run ^$ -count 5 ./...
```

## Configuration Options

### Command Line Flags

| Flag            | Values                     | Description                  |
|-----------------|----------------------------|------------------------------|
| `-input`        | `<path>`                   | Input file or directory      |
| `-generate-for` | `all\|exported\|annotated` | Which structs to process     |
| `-constructor`  | `exported\|package\|none`  | Constructor visibility level |

### Examples

```bash
# Process only annotated structs (default)
gobetter -input=models.go

# Process all exported structs without annotations
gobetter -input=models.go -generate-for=exported

# Generate package-level constructors for all structs
gobetter -input=models.go -generate-for=all -constructor=package
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
