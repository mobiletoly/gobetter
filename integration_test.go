package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
				Sort:                  SortSeq,
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
				Sort:                  SortSeq,
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
		Sort:                  SortSeq,
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

func TestGenerateCodeSkipsWritingWhenUnchanged(t *testing.T) {
	tmpDir := t.TempDir()

	const input = `package test

type Person struct { //+gob:Constructor
	firstName string //+gob:getter
	lastName  string //+gob:getter
	age       int
}
`

	inputFile := filepath.Join(tmpDir, "person.go")
	if err := os.WriteFile(inputFile, []byte(input), 0644); err != nil {
		t.Fatal(err)
	}

	config := &Config{
		InputPath:             inputFile,
		GenerateFor:           nil,
		ConstructorVisibility: ConstructorExported,
		Sort:                  SortAbc,
	}

	outputFile := makeOutputFilename(inputFile)

	if err := generateCode(config); err != nil {
		t.Fatalf("first generateCode call failed: %v", err)
	}

	infoBefore, err := os.Stat(outputFile)
	if err != nil {
		t.Fatalf("stat after first generateCode failed: %v", err)
	}

	time.Sleep(150 * time.Millisecond)

	if err := generateCode(config); err != nil {
		t.Fatalf("second generateCode call failed: %v", err)
	}

	infoAfter, err := os.Stat(outputFile)
	if err != nil {
		t.Fatalf("stat after second generateCode failed: %v", err)
	}

	if !infoBefore.ModTime().Equal(infoAfter.ModTime()) {
		t.Fatalf("expected mod time to remain unchanged, before=%v after=%v", infoBefore.ModTime(), infoAfter.ModTime())
	}
}

func TestSignatureChangeOnConfigMutation(t *testing.T) {
	tmpDir := t.TempDir()

	const input = `package test

type Person struct { //+gob:Constructor
	firstName string //+gob:getter
	lastName  string //+gob:getter
	age       int
}
`

	inputFile := filepath.Join(tmpDir, "person.go")
	if err := os.WriteFile(inputFile, []byte(input), 0644); err != nil {
		t.Fatal(err)
	}

	config := &Config{
		InputPath:             inputFile,
		GenerateFor:           nil,
		ConstructorVisibility: ConstructorExported,
		Sort:                  SortAbc,
	}

	outputFile := makeOutputFilename(inputFile)

	firstSig := runAndGetSignature(t, config, outputFile)

	config.Sort = SortSeq
	secondSig := runAndGetSignature(t, config, outputFile)
	if secondSig == firstSig {
		t.Fatalf("expected signature to change after Sort mutation: before=%s after=%s", firstSig, secondSig)
	}

	config.Sort = SortAbc
	thirdSig := runAndGetSignature(t, config, outputFile)
	if thirdSig != firstSig {
		t.Fatalf("expected signature to revert; want %s, got %s", firstSig, thirdSig)
	}

	infoBefore, err := os.Stat(outputFile)
	if err != nil {
		t.Fatalf("stat before final call failed: %v", err)
	}

	time.Sleep(150 * time.Millisecond)
	if err := generateCode(config); err != nil {
		t.Fatalf("generateCode failed after signature stabilization: %v", err)
	}
	infoAfter, err := os.Stat(outputFile)
	if err != nil {
		t.Fatalf("stat after final call failed: %v", err)
	}
	if !infoBefore.ModTime().Equal(infoAfter.ModTime()) {
		t.Fatalf("expected mod time unchanged when signature identical; before=%v after=%v", infoBefore.ModTime(), infoAfter.ModTime())
	}
}

func TestGenericStructBuilder(t *testing.T) {
	tmpDir := t.TempDir()

	const input = `package test

type Box[T any] struct { //+gob:Constructor
	Value     T
	Stream    <-chan T
	Transform func(T) (T, error)
}
`

	inputFile := filepath.Join(tmpDir, "generic.go")
	if err := os.WriteFile(inputFile, []byte(input), 0644); err != nil {
		t.Fatal(err)
	}

	config := &Config{
		InputPath:             inputFile,
		GenerateFor:           nil,
		ConstructorVisibility: ConstructorExported,
		Sort:                  SortSeq,
	}

	if err := generateCode(config); err != nil {
		t.Fatalf("generateCode failed: %v", err)
	}

	outputFile := makeOutputFilename(inputFile)
	content, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatal(err)
	}
	output := string(content)

	checks := []string{
		"func NewBoxBuilder[T any]() Box_Builder_Value[T]",
		"type Box_Builder_Value[T any] struct {\n\troot *Box[T]",
		"func (b Box_Builder_Value[T]) Value(arg T) Box_Builder_Stream[T]",
		"func (b Box_Builder_Stream[T]) Stream(arg <-chan T) Box_Builder_Transform[T]",
		"func (b Box_Builder_GobFinalizer[T]) Build() *Box[T]",
	}

	for _, check := range checks {
		if !strings.Contains(output, check) {
			t.Fatalf("generated output missing %q:\n%s", check, output)
		}
	}

	if err := verifyGoSyntax(outputFile); err != nil {
		t.Fatalf("generated generic builder has syntax errors: %v", err)
	}
}

func TestAliasCollisionGetsResolved(t *testing.T) {
	tmpDir := t.TempDir()

	const input = `package test

type Wrap struct { //+gob:Constructor
	BX struct { //+gob:Constructor
		Value int
	} //+gob:Constructor
	B struct {
		X struct { //+gob:Constructor
			Value int
		} //+gob:Constructor
	}
}
`

	inputFile := filepath.Join(tmpDir, "wrap.go")
	if err := os.WriteFile(inputFile, []byte(input), 0644); err != nil {
		t.Fatal(err)
	}

	config := &Config{
		InputPath:             inputFile,
		GenerateFor:           nil,
		ConstructorVisibility: ConstructorExported,
		Sort:                  SortSeq,
	}

	if err := generateCode(config); err != nil {
		t.Fatalf("generateCode failed: %v", err)
	}

	outputFile := makeOutputFilename(inputFile)
	content, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatal(err)
	}
	output := string(content)

	if !strings.Contains(output, "type WrapBX =") {
		t.Fatalf("expected alias WrapBX in output:\n%s", output)
	}
	if !strings.Contains(output, "type WrapBX_2 =") {
		t.Fatalf("expected alias WrapBX_2 in output:\n%s", output)
	}
}

func runAndGetSignature(t *testing.T, config *Config, outputFile string) string {
	t.Helper()
	if err := generateCode(config); err != nil {
		t.Fatalf("generateCode failed: %v", err)
	}
	content, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("reading output failed: %v", err)
	}
	if !bytes.Contains(content, []byte("// gobetter:signature=")) {
		t.Fatalf("signature header missing in output:\n%s", string(content))
	}
	sig := extractSignature(string(content))
	if sig == "" {
		t.Fatal("signature not found in output")
	}
	return sig
}

func extractSignature(output string) string {
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "// gobetter:signature=") {
			return line
		}
		if strings.HasPrefix(line, "package ") {
			break
		}
	}
	return ""
}

func TestInvalidFlagErrors(t *testing.T) {
	tmpDir := t.TempDir()
	inputFile := filepath.Join(tmpDir, "input.go")
	source := `package test

type Person struct { //+gob:Constructor
	name string
}
`
	if err := os.WriteFile(inputFile, []byte(source), 0644); err != nil {
		t.Fatal(err)
	}

	testCases := []struct {
		name     string
		args     []string
		expected string
	}{
		{
			name:     "invalid generate-for",
			args:     []string{"-input", inputFile, "-generate-for", "unknown"},
			expected: fmt.Sprintf(ErrInvalidGenerateFor, "unknown"),
		},
		{
			name:     "invalid constructor",
			args:     []string{"-input", inputFile, "-constructor", "weird"},
			expected: fmt.Sprintf(ErrInvalidConstructor, "weird"),
		},
		{
			name:     "invalid sort",
			args:     []string{"-input", inputFile, "-sort", "random"},
			expected: fmt.Sprintf(ErrInvalidSort, "random"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command("go", append([]string{"run", "."}, tc.args...)...)
			output, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatalf("expected command to fail, stdout/stderr:\n%s", string(output))
			}
			if !strings.Contains(string(output), tc.expected) {
				t.Fatalf("expected error output to contain %q, got:\n%s", tc.expected, string(output))
			}
		})
	}
}
