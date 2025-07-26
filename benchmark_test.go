package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// BenchmarkGenerateCode benchmarks the complete code generation process
func BenchmarkGenerateCode(b *testing.B) {
	// Create a test input file
	inputContent := `package test

//go:generate gobetter -input $GOFILE

type Person struct { //+gob:Constructor
	firstName string //+gob:getter
	lastName  string //+gob:getter
	age       int
	email     string
	phone     string
}

type Address struct { //+gob:Constructor
	street   string //+gob:getter
	city     string //+gob:getter
	state    string //+gob:getter
	zipCode  string //+gob:getter
	country  string //+gob:getter
}

type Company struct { //+gob:Constructor
	name        string //+gob:getter
	address     Address
	employees   []Person
	founded     int
	revenue     float64
}
`

	tmpDir, err := os.MkdirTemp("", "gobetter_bench")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	inputFile := filepath.Join(tmpDir, "test.go")
	if err := os.WriteFile(inputFile, []byte(inputContent), 0644); err != nil {
		b.Fatal(err)
	}

	config := &Config{
		InputFile:             inputFile,
		OutputFile:            filepath.Join(tmpDir, "test_gob.go"),
		GenerateFor:           nil,
		UsePtrReceiver:        false,
		ConstructorVisibility: ConstructorExported,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := generateCode(config)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkStructFieldGeneration benchmarks individual struct field operations
func BenchmarkStructFieldGeneration(b *testing.B) {
	sf := &StructField{
		StructFlags: &StructFlags{
			ProcessStruct: true,
			PtrReceiver:   false,
			Visibility:    ExportedVisibility,
		},
		StructName:    "Person",
		FieldName:     "firstName",
		FieldTypeText: "string",
		Acronym:       false,
	}

	b.Run("GenerateGetter", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = sf.GenerateGetter()
		}
	})

	b.Run("BuilderFieldStructName", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = sf.builderFieldStructName()
		}
	})

	b.Run("GenerateSourceCodeForStructField", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = sf.GenerateSourceCodeForStructField(nil, true)
		}
	})
}

// BenchmarkRegexMatching benchmarks the regex pattern matching
func BenchmarkRegexMatching(b *testing.B) {
	testStrings := []string{
		"//+gob:Constructor",
		"//+gob:constructor",
		"//+gob:getter",
		"//+gob:acronym",
		"//+gob:_",
		"// regular comment",
		"//+gob:Constructor some other text",
	}

	b.Run("ConstructorExported", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			for _, s := range testStrings {
				_ = ConstructorExportedRegexp.MatchString(s)
			}
		}
	})

	b.Run("ConstructorPackage", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			for _, s := range testStrings {
				_ = ConstructorPackageRegexp.MatchString(s)
			}
		}
	})

	b.Run("FlagGetter", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			for _, s := range testStrings {
				_ = FlagGetterRegexp.MatchString(s)
			}
		}
	})
}

// BenchmarkFileOperations benchmarks file I/O operations
func BenchmarkFileOperations(b *testing.B) {
	content := []byte(`package test

type Person struct { //+gob:Constructor
	firstName string //+gob:getter
	lastName  string //+gob:getter
	age       int
}
`)

	tmpDir, err := os.MkdirTemp("", "gobetter_bench_io")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	b.Run("WriteFile", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			filename := filepath.Join(tmpDir, "test_write.go")
			err := os.WriteFile(filename, content, 0644)
			if err != nil {
				b.Fatal(err)
			}
			os.Remove(filename) // Clean up for next iteration
		}
	})

	// Create a file for read benchmarks
	testFile := filepath.Join(tmpDir, "test_read.go")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		b.Fatal(err)
	}

	b.Run("ReadFile", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, err := os.ReadFile(testFile)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkStringBuilding benchmarks string building operations
func BenchmarkStringBuilding(b *testing.B) {
	b.Run("StringsBuilder", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			var builder strings.Builder
			builder.WriteString("package test\n\n")
			builder.WriteString("type Person_Builder_FirstName struct {\n")
			builder.WriteString("\troot *Person\n")
			builder.WriteString("}\n\n")
			_ = builder.String()
		}
	})

	b.Run("StringConcatenation", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			result := "package test\n\n" +
				"type Person_Builder_FirstName struct {\n" +
				"\troot *Person\n" +
				"}\n\n"
			_ = result
		}
	})
}
