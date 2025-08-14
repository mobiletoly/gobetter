package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestFullWorkflow tests the complete workflow from input to formatted output
func TestFullWorkflow(t *testing.T) {
	testCases := []struct {
		name        string
		input       string
		expectError bool
		checkOutput []string
	}{
		{
			name: "simple struct with constructor",
			input: `package test

//go:generate` + /*to avoid go:generate comment being executed*/ `gobetter -input $GOFILE

type Person struct { //+gob:Constructor
	firstName string //+gob:getter
	lastName  string //+gob:getter
	age       int
}
`,
			expectError: false,
			checkOutput: []string{
				"package test",
				"func NewPersonBuilder()",
				"func (v *Person) FirstName() string",
				"func (v *Person) LastName() string",
				"type Person_Builder_FirstName struct",
				"type Person_Builder_LastName struct",
				"type Person_Builder_Age struct",
				"func (b Person_Builder_GobFinalizer) Build() *Person",
			},
		},
		{
			name: "struct with acronym field",
			input: `package test

type Contact struct { //+gob:Constructor
	firstName string //+gob:getter
	dob       string //+gob:getter +gob:acronym
	email     string
}
`,
			expectError: false,
			checkOutput: []string{
				"func (v *Contact) DOB() string",
				"func (b Contact_Builder_FirstName) FirstName(arg string) Contact_Builder_DOB",
				"func (b Contact_Builder_DOB) DOB(arg string) Contact_Builder_Email",
			},
		},
		{
			name: "struct with optional fields",
			input: `package test

type User struct { //+gob:Constructor
	username string
	email    string
	bio      string //+gob:_
}
`,
			expectError: false,
			checkOutput: []string{
				"func NewUserBuilder()",
				"type User_Builder_Username struct",
				"type User_Builder_Email struct",
				"func (b User_Builder_Email) Email(arg string) User_Builder_GobFinalizer",
			},
		},
		{
			name: "package-level constructor",
			input: `package test

type internal struct { //+gob:constructor
	value string
}
`,
			expectError: false,
			checkOutput: []string{
				"func newInternalBuilder()",
			},
		},
		{
			name: "no constructor annotation",
			input: `package test

type Plain struct {
	value string
}
`,
			expectError: false,
			checkOutput: []string{
				"package test",
				// Should not contain any builder code
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create temporary directory
			tmpDir, err := os.MkdirTemp("", "gobetter_integration")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(tmpDir)

			// Write input file
			inputFile := filepath.Join(tmpDir, "input.go")
			if err := os.WriteFile(inputFile, []byte(tc.input), 0644); err != nil {
				t.Fatal(err)
			}

			// Create config
			config := &Config{
				InputPath:             inputFile,
				GenerateFor:           nil,
				ConstructorVisibility: ConstructorExported,
			}
			outputFile := makeOutputFilename(inputFile)

			// Generate code
			err = generateCode(config)

			if tc.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			// Read output file
			outputContent, err := os.ReadFile(outputFile)
			if err != nil {
				t.Fatal(err)
			}

			output := string(outputContent)

			// Check expected content
			for _, expected := range tc.checkOutput {
				if expected == "package test" {
					// Special case: always expect package declaration
					if !strings.Contains(output, expected) {
						t.Errorf("Output does not contain expected string: %q", expected)
					}
				} else if strings.HasPrefix(expected, "//") {
					// Comment check
					if !strings.Contains(output, expected) {
						t.Errorf("Output does not contain expected comment: %q", expected)
					}
				} else {
					// Function/type check
					if !strings.Contains(output, expected) {
						t.Errorf("Output does not contain expected code: %q", expected)
					}
				}
			}

			// Verify the output compiles
			if err := verifyGoSyntax(outputFile); err != nil {
				t.Errorf("Generated code has syntax errors: %v", err)
				t.Logf("Generated code:\n%s", output)
			}
		})
	}
}

// TestGenerateForFlags tests different generate-for flag values
func TestGenerateForFlags(t *testing.T) {
	input := `package test

type ExportedStruct struct {
	field string
}

type unexportedStruct struct {
	field string
}
`

	testCases := []struct {
		name        string
		generateFor string
		expectCode  []string
	}{
		{
			name:        "generate for all",
			generateFor: GenerateForAll,
			expectCode: []string{
				"func NewExportedStructBuilder()",
				"func newUnexportedStructBuilder()",
			},
		},
		{
			name:        "generate for exported only",
			generateFor: GenerateForExported,
			expectCode: []string{
				"func NewExportedStructBuilder()",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir, err := os.MkdirTemp("", "gobetter_flags")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(tmpDir)

			inputFile := filepath.Join(tmpDir, "input.go")
			if err := os.WriteFile(inputFile, []byte(input), 0644); err != nil {
				t.Fatal(err)
			}

			generateFor := tc.generateFor
			config := &Config{
				InputPath:             inputFile,
				GenerateFor:           &generateFor,
				ConstructorVisibility: ConstructorExported,
			}
			outputFile := makeOutputFilename(inputFile)

			err = generateCode(config)
			if err != nil {
				t.Fatal(err)
			}

			outputContent, err := os.ReadFile(outputFile)
			if err != nil {
				t.Fatal(err)
			}

			output := string(outputContent)

			for _, expected := range tc.expectCode {
				if !strings.Contains(output, expected) {
					t.Errorf("Output does not contain expected code: %q", expected)
				}
			}
		})
	}
}

// verifyGoSyntax checks if the generated Go code has valid syntax
func verifyGoSyntax(filename string) error {
	cmd := exec.Command("go", "fmt", filename)
	return cmd.Run()
}

// TestComplexStruct tests generation for a complex struct with various field types
func TestComplexStruct(t *testing.T) {
	input := `package test

import (
	"time"
)

type ComplexStruct struct { //+gob:Constructor
	id          int64     //+gob:getter
	name        string    //+gob:getter
	tags        []string
	metadata    map[string]interface{}
	createdAt   time.Time
	updatedAt   *time.Time
	isActive    bool
	score       float64
	description *string   //+gob:_
}
`

	tmpDir, err := os.MkdirTemp("", "gobetter_complex")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	inputFile := filepath.Join(tmpDir, "input.go")
	if err := os.WriteFile(inputFile, []byte(input), 0644); err != nil {
		t.Fatal(err)
	}

	config := &Config{
		InputPath:             inputFile,
		GenerateFor:           nil,
		ConstructorVisibility: ConstructorExported,
	}
	outputFile := makeOutputFilename(inputFile)

	err = generateCode(config)
	if err != nil {
		t.Fatal(err)
	}

	outputContent, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatal(err)
	}

	output := string(outputContent)

	expectedElements := []string{
		"\"time\"", // Import can be either "import \"time\"" or "import (\n\"time\"\n)"
		"func (v *ComplexStruct) Id() int64",
		"func (v *ComplexStruct) Name() string",
		"func NewComplexStructBuilder()",
		"type ComplexStruct_Builder_Id struct",
		"type ComplexStruct_Builder_Name struct",
		"type ComplexStruct_Builder_Tags struct",
		"type ComplexStruct_Builder_Metadata struct",
		"type ComplexStruct_Builder_CreatedAt struct",
		"type ComplexStruct_Builder_UpdatedAt struct",
		"type ComplexStruct_Builder_IsActive struct",
		"type ComplexStruct_Builder_Score struct",
		"func (b ComplexStruct_Builder_Score) Score(arg float64) ComplexStruct_Builder_GobFinalizer",
		"func (b ComplexStruct_Builder_GobFinalizer) Build() *ComplexStruct",
	}

	for _, expected := range expectedElements {
		if !strings.Contains(output, expected) {
			t.Errorf("Output does not contain expected element: %q", expected)
		}
	}

	// Verify syntax
	if err := verifyGoSyntax(outputFile); err != nil {
		t.Errorf("Generated code has syntax errors: %v", err)
		t.Logf("Generated code:\n%s", output)
	}
}
