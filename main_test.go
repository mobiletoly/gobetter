package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileNameWithoutExt(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"test.go", "test"},
		{"main.go", "main"},
		{"file.txt", "file"},
		{"noext", "noext"},
		{"path/to/file.go", "path/to/file"},
		{"", ""},
	}

	for _, test := range tests {
		result := fileNameWithoutExt(test.input)
		if result != test.expected {
			t.Errorf("fileNameWithoutExt(%q) = %q, expected %q", test.input, result, test.expected)
		}
	}
}

func TestMakeOutputFilename(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"test.go", "test_gob.go"},
		{"main.go", "main_gob.go"},
		{"path/to/file.go", "path/to/file_gob.go"},
		{"./example/main.go", "example/main_gob.go"},
	}

	for _, test := range tests {
		result := makeOutputFilename(test.input)
		if result != test.expected {
			t.Errorf("makeOutputFilename(%q) = %q, expected %q", test.input, result, test.expected)
		}
	}
}

func TestParseCommandLineArgs(t *testing.T) {
	// Skip this test as it interferes with the global flag state
	// The functionality is tested in integration tests instead
	t.Skip("Skipping flag parsing test to avoid global state interference")
}

func TestGenerateCode(t *testing.T) {
	// Create a temporary input file with test struct
	inputContent := `package test

//go:generate` + /**/ `gobetter -input $GOFILE

type Person struct { //+gob:Constructor
	firstName string //+gob:getter
	lastName  string //+gob:getter
	age       int
}
`

	tmpDir, err := os.MkdirTemp("", "gobetter_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	inputFile := filepath.Join(tmpDir, "test.go")
	if err := os.WriteFile(inputFile, []byte(inputContent), 0644); err != nil {
		t.Fatal(err)
	}

	config := &Config{
		InputPath:             inputFile,
		GenerateFor:           nil,
		ConstructorVisibility: ConstructorExported,
	}
	outputFile := filepath.Join(tmpDir, "test_gob.go")

	err = generateCode(config)
	if err != nil {
		t.Fatalf("generateCode failed: %v", err)
	}

	// Check if output file was created
	if _, err := os.Stat(outputFile); os.IsNotExist(err) {
		t.Error("Output file was not created")
	}

	// Read and verify output content
	outputContent, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatal(err)
	}

	output := string(outputContent)

	// Check for expected content
	expectedStrings := []string{
		"package test",
		"func NewPersonBuilder()",
		"func (v *Person) FirstName() string",
		"func (v *Person) LastName() string",
		"func (b Person_Builder_GobFinalizer) Build() *Person",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(output, expected) {
			t.Errorf("Output does not contain expected string: %q", expected)
		}
	}
}
