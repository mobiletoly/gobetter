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

Install **gobetter** and its dependency:

```bash
# Install goimports (for code formatting)
go install golang.org/x/tools/cmd/goimports@latest

# Install gobetter
go install github.com/mobiletoly/gobetter@latest
```

## Quick Start

### 1. Annotate Your Structs

Add annotations to your Go structs:

```go
package main

//go:generate gobetter -input $GOFILE

type Person struct { //+gob:Constructor
    firstName   string  //+gob:getter
    lastName    string  //+gob:getter
    dob         string  //+gob:getter +gob:acronym
    Score       int
    Description string  //+gob:_
}
```

### 2. Generate Code

Run the generator:

```bash
go generate ./...
```

### 3. Use the Generated Builder

```go
person := NewPersonBuilder().
    FirstName("John").
    LastName("Doe").
    DOB("01/01/1990").
    Score(85).
    Build()

// Set optional fields after building
person.Description = "Software engineer"

// Access private fields through getters
fmt.Println(person.FirstName()) // "John"
fmt.Println(person.DOB())       // "01/01/1990" (acronym handling)
```

## 📝 Annotations Reference

| Annotation | Description | Example |
|------------|-------------|---------|
| `//+gob:Constructor` | Generate builder for struct | `type Person struct { //+gob:Constructor` |
| `//+gob:constructor` | Generate package-level builder | `type person struct { //+gob:constructor` |
| `//+gob:getter` | Generate getter for private field | `name string //+gob:getter` |
| `//+gob:acronym` | Treat field as acronym (DOB vs Dob) | `dob string //+gob:getter +gob:acronym` |
| `//+gob:_` | Mark field as optional (skip in builder) | `description string //+gob:_` |

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

## Configuration Options

### Command Line Flags

| Flag | Values | Description |
|------|--------|-------------|
| `-input` | `<file>` | Input file to process |
| `-output` | `<file>` | Output file (default: `<input>_gob.go`) |
| `-generate-for` | `all\|exported\|annotated` | Which structs to process |
| `-constructor` | `exported\|package\|none` | Constructor visibility level |

### Examples

```bash
# Process only annotated structs (default)
gobetter -input=models.go

# Process all exported structs without annotations
gobetter -input=models.go -generate-for=exported

# Generate package-level constructors for all structs
gobetter -input=models.go -generate-for=all -constructor=package
```

## IDE Integration

### IntelliJ IDEA / GoLand

Set up a File Watcher for automatic generation:

1. Go to **Preferences → Tools → File Watchers**
2. Add **Custom** watcher:
   - **Name**: `Go Generate`
   - **File type**: `Go files`
   - **Program**: `go`
   - **Arguments**: `generate`
   - **Scope**: Create scope with pattern `file:*.go&&!file:*_gob.go`

Now builders regenerate automatically when you save Go files!
