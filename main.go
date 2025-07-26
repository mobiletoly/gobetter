package main

import (
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"
)

// Config holds the configuration for the gobetter generator
type Config struct {
	InputFile             string
	OutputFile            string
	GenerateFor           *string
	UsePtrReceiver        bool
	ConstructorVisibility string
}

// fileNameWithoutExt returns the filename without its extension
func fileNameWithoutExt(fileName string) string {
	return strings.TrimSuffix(fileName, filepath.Ext(fileName))
}

// makeOutputFilename generates the output filename based on input filename
func makeOutputFilename(inFilename string) string {
	dir := filepath.Dir(inFilename)
	base := fileNameWithoutExt(filepath.Base(inFilename))
	ext := filepath.Ext(inFilename)
	return filepath.Join(dir, base+GobFileSuffix+ext)
}

// validateGoimports checks if goimports is available
func validateGoimports() error {
	_, err := exec.LookPath("goimports")
	if err != nil {
		return errors.New(ErrGoimportsNotFound + "\n" + ErrGoimportsInstall)
	}
	return nil
}

// parseCommandLineArgs parses and validates command line arguments
func parseCommandLineArgs() (*Config, error) {
	if err := validateGoimports(); err != nil {
		return nil, err
	}

	inputFilePtr := flag.String(FlagInput, "", "go input file path")
	outputFilePtr := flag.String(FlagOutput, "", "go output file path (optional)")
	generateForPtr := flag.String(FlagGenerateFor, GenerateForAnnotated,
		`allows parsing of non-annotated struct types:
|  all       - process exported and package-level classes
|  exported  - process exported classes only
|  annotated - process specifically annotated class only
`)
	receiverTypePtr := flag.String(FlagReceiver, ReceiverValue,
		`specify function receiver type:
|  value     - receiver must be a value type, e.g. { func (v *Class) Name }
|  pointer   - receiver must be a pointer type, e.g. { func (v Class) Name }
`)
	constructorVisibilityPtr := flag.String(FlagConstructor, ConstructorExported,
		`generate exported or package-level constructors:
|  exported  - exported (upper-cased) constructors will be created
|  package   - package-level (lower-cased) constructors will be created
|  none      - no constructors will be created
`)
	versionPtr := flag.Bool(FlagVersion, false, "print current version")

	flag.Parse()

	if *versionPtr {
		fmt.Printf("gobetter version %s\n", Version)
		os.Exit(0)
	}

	config := &Config{
		InputFile: *inputFilePtr,
	}

	if !isFlagPassed(FlagInput) {
		return nil, errors.New(ErrInputRequired)
	}

	if _, err := os.Stat(config.InputFile); os.IsNotExist(err) {
		return nil, fmt.Errorf(ErrFileNotExist, config.InputFile)
	}

	if isFlagPassed(FlagOutput) {
		config.OutputFile = *outputFilePtr
	} else {
		config.OutputFile = makeOutputFilename(config.InputFile)
	}

	// Validate generate-for flag
	switch *generateForPtr {
	case GenerateForAll, GenerateForExported:
		config.GenerateFor = generateForPtr
	case GenerateForAnnotated:
		config.GenerateFor = nil
	default:
		return nil, errors.New(ErrInvalidGenerateFor)
	}

	// Validate receiver flag
	switch *receiverTypePtr {
	case ReceiverPointer:
		config.UsePtrReceiver = true
	case ReceiverValue:
		config.UsePtrReceiver = false
	default:
		return nil, errors.New(ErrInvalidReceiver)
	}

	// Validate constructor flag
	switch *constructorVisibilityPtr {
	case ConstructorExported, ConstructorPackage, ConstructorNone:
		config.ConstructorVisibility = *constructorVisibilityPtr
	default:
		return nil, errors.New(ErrInvalidConstructor)
	}

	fmt.Printf("Input file: %s\n", config.InputFile)
	fmt.Printf("Output file: %s\n", config.OutputFile)
	return config, nil
}

func isFlagPassed(name string) bool {
	found := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}

// StructInfo holds information about a discovered struct
type StructInfo struct {
	Name       string
	StructType *ast.StructType
	TypeSpec   *ast.TypeSpec
	ParentPath string     // For inner structs, this holds the parent path like "OuterStruct.Config"
	Field      *ast.Field // For inner structs, this holds the field that contains the struct
}

// findAllStructs recursively finds all struct definitions in the AST
func findAllStructs(node ast.Node, parentPath string) []*StructInfo {
	var structs []*StructInfo

	ast.Inspect(node, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.TypeSpec:
			if st, ok := node.Type.(*ast.StructType); ok {
				structInfo := &StructInfo{
					Name:       node.Name.Name,
					StructType: st,
					TypeSpec:   node,
					ParentPath: parentPath,
				}
				structs = append(structs, structInfo)

				// Look for inner structs within this struct
				innerStructs := findInnerStructs(st, node.Name.Name)
				structs = append(structs, innerStructs...)
			}
		}
		return true
	})

	return structs
}

// findInnerStructs finds struct definitions within struct fields
func findInnerStructs(st *ast.StructType, parentName string) []*StructInfo {
	var structs []*StructInfo

	for _, field := range st.Fields.List {
		// Check for both direct struct types and pointer to struct types
		var structType *ast.StructType
		var ok bool

		if structType, ok = field.Type.(*ast.StructType); ok {
			// Direct struct type: struct { ... }
		} else if starExpr, starOk := field.Type.(*ast.StarExpr); starOk {
			// Pointer to struct type: *struct { ... }
			structType, ok = starExpr.X.(*ast.StructType)
		}

		if ok && structType != nil {
			// This is an anonymous inner struct (direct or pointer)
			for _, name := range field.Names {
				fieldName := name.Name
				fullName := parentName + fieldName

				structInfo := &StructInfo{
					Name:       fullName,
					StructType: structType,
					TypeSpec:   nil, // Inner structs don't have TypeSpec
					ParentPath: parentName + "." + fieldName,
					Field:      field, // Store the field for comment access
				}
				structs = append(structs, structInfo)

				// Recursively find nested inner structs
				nestedStructs := findInnerStructs(structType, fullName)
				structs = append(structs, nestedStructs...)
			}
		}
	}

	return structs
}

// generateInnerStructTypeDefinition generates a type alias for an inner struct
func generateInnerStructTypeDefinition(structInfo *StructInfo, allStructs []*StructInfo) string {
	var bld strings.Builder

	// Generate type alias instead of type definition for compatibility
	structTypeString := buildStructTypeStringFromAST(structInfo.StructType, allStructs)
	bld.WriteString(fmt.Sprintf("type %s = %s\n\n", structInfo.Name, structTypeString))

	return bld.String()
}

// buildStructTypeStringFromAST builds a struct type string from AST with proper nested type handling
func buildStructTypeStringFromAST(st *ast.StructType, allStructs []*StructInfo) string {
	var fieldParts []string

	for _, field := range st.Fields.List {
		fieldType := getFieldTypeFromASTWithAliases(field.Type, allStructs)

		// Extract struct tag if present
		var tag string
		if field.Tag != nil {
			tag = " " + field.Tag.Value // field.Tag.Value includes the backticks
		}

		for _, name := range field.Names {
			fieldParts = append(fieldParts, fmt.Sprintf("%s %s%s", name.Name, fieldType, tag))
		}
	}

	return fmt.Sprintf("struct { %s }", strings.Join(fieldParts, "; "))
}

// getFieldTypeFromASTWithAliases extracts field type from AST, using type aliases when available
func getFieldTypeFromASTWithAliases(expr ast.Expr, allStructs []*StructInfo) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + getFieldTypeFromASTWithAliases(t.X, allStructs)
	case *ast.ArrayType:
		return "[]" + getFieldTypeFromASTWithAliases(t.Elt, allStructs)
	case *ast.MapType:
		keyType := getFieldTypeFromASTWithAliases(t.Key, allStructs)
		valueType := getFieldTypeFromASTWithAliases(t.Value, allStructs)
		return fmt.Sprintf("map[%s]%s", keyType, valueType)
	case *ast.SelectorExpr:
		pkg := getFieldTypeFromASTWithAliases(t.X, allStructs)
		return fmt.Sprintf("%s.%s", pkg, t.Sel.Name)
	case *ast.InterfaceType:
		if len(t.Methods.List) == 0 {
			return "interface{}"
		}
		return "interface{}" // Simplified
	case *ast.StructType:
		// For nested inner structs, check if we have a type alias
		// For now, recursively build the struct type
		return buildStructTypeStringFromAST(t, allStructs)
	default:
		return "interface{}" // Fallback
	}
}

// getFieldTypeForInnerStruct gets the correct field type for inner struct fields
func getFieldTypeForInnerStruct(field *ast.Field, parentStructName string, allStructs []*StructInfo) string {
	switch field.Type.(type) {
	case *ast.StructType:
		// This is a nested inner struct - find its generated name
		for _, name := range field.Names {
			expectedName := parentStructName + "_" + name.Name
			for _, s := range allStructs {
				if s.Name == expectedName {
					return expectedName
				}
			}
		}
		return "interface{}" // Fallback
	default:
		return getTypeString(field.Type, parentStructName)
	}
}

// getTypeString recursively extracts type string from ast.Expr
func getTypeString(expr ast.Expr, parentPath string) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + getTypeString(t.X, parentPath)
	case *ast.ArrayType:
		return "[]" + getTypeString(t.Elt, parentPath)
	case *ast.MapType:
		keyType := getTypeString(t.Key, parentPath)
		valueType := getTypeString(t.Value, parentPath)
		return fmt.Sprintf("map[%s]%s", keyType, valueType)
	case *ast.SelectorExpr:
		pkg := getTypeString(t.X, parentPath)
		return fmt.Sprintf("%s.%s", pkg, t.Sel.Name)
	case *ast.InterfaceType:
		if len(t.Methods.List) == 0 {
			return "interface{}"
		}
		return "interface{}" // Simplified
	case *ast.StructType:
		// This is an inner struct - we need to generate a type name for it
		// For now, return a placeholder that will be replaced
		return "INNER_STRUCT_PLACEHOLDER"
	default:
		return "interface{}" // Fallback
	}
}

// generateCode generates the builder code based on the configuration
func generateCode(config *Config) error {
	fileContent, err := os.ReadFile(config.InputFile)
	if err != nil {
		return fmt.Errorf(ErrReadFile, config.InputFile, err)
	}

	fset := token.NewFileSet()
	astFile, err := parser.ParseFile(fset, config.InputFile, nil, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("failed to parse file: %w", err)
	}

	sp := NewStructParser(fset, fileContent)

	var bld strings.Builder
	bld.WriteString(GeneratePackage(astFile))
	bld.WriteString(GenerateImports(astFile))

	// Find all structs (including inner structs)
	allStructs := findAllStructs(astFile, "")

	// Generate type definitions for inner structs that have constructor annotations
	// or are exported when using -generate-for=exported
	for _, structInfo := range allStructs {
		if structInfo.TypeSpec == nil { // This is an inner struct
			// Check if this inner struct has constructor annotation
			var structFlags StructFlags
			if structInfo.Field != nil {
				structFlags = sp.constructorFlagsFromField(structInfo.Field)
			}

			shouldGenerateType := structFlags.ProcessStruct

			// Also generate type alias if using -generate-for=exported and the field is exported
			if !shouldGenerateType && config.GenerateFor != nil && *config.GenerateFor == GenerateForExported {
				if structInfo.Field != nil && len(structInfo.Field.Names) > 0 {
					fieldName := structInfo.Field.Names[0].Name
					if unicode.IsUpper(rune(fieldName[0])) {
						shouldGenerateType = true
					}
				}
			}

			if shouldGenerateType {
				bld.WriteString(generateInnerStructTypeDefinition(structInfo, allStructs))
			}
		}
	}

	// Process all structs (both top-level and inner structs with constructor annotations)
	for _, structInfo := range allStructs {
		structName := structInfo.Name
		st := structInfo.StructType

		// Get constructor flags based on struct type
		var structFlags StructFlags
		if structInfo.TypeSpec != nil {
			// Top-level struct - get flags from TypeSpec
			structFlags = sp.constructorFlags(st)
		} else {
			// Inner struct - get flags from the field that contains it
			structFlags = sp.constructorFlagsFromField(structInfo.Field)
		}

		if !structFlags.ProcessStruct {
			if config.GenerateFor == nil {
				// Skip structs without constructor annotations if no generate-for flag
				continue
			}
			if *config.GenerateFor == GenerateForExported {
				// For inner structs, we need to check if the field name is exported
				// For top-level structs, we check the struct name
				var nameToCheck string
				if structInfo.TypeSpec == nil {
					// Inner struct - check the field name that contains it
					if structInfo.Field != nil && len(structInfo.Field.Names) > 0 {
						nameToCheck = structInfo.Field.Names[0].Name
					}
				} else {
					// Top-level struct - check the struct name
					nameToCheck = structName
				}

				if nameToCheck == "" || !unicode.IsUpper(rune(nameToCheck[0])) {
					continue
				}
			}
			structFlags.ProcessStruct = true
			structFlags.PtrReceiver = config.UsePtrReceiver
			switch config.ConstructorVisibility {
			case ConstructorExported:
				structFlags.Visibility = ExportedVisibility
			case ConstructorPackage:
				structFlags.Visibility = PackageLevelVisibility
			default:
				structFlags.Visibility = NoVisibility
			}
		}

		structFields := make([]*StructField, 0)
		for _, field := range st.Fields.List {
			var fieldTypeText string

			// Handle inner struct fields specially (both direct and pointer types)
			isInnerStruct := false
			if _, ok := field.Type.(*ast.StructType); ok {
				isInnerStruct = true
			} else if starExpr, ok := field.Type.(*ast.StarExpr); ok {
				if _, ok := starExpr.X.(*ast.StructType); ok {
					isInnerStruct = true
				}
			}

			if isInnerStruct {
				// For inner structs, check if we have a generated type alias
				fieldTypeText = sp.getInnerStructFieldTypeWithAlias(field, structName, allStructs)
			} else {
				fieldTypeText = sp.fieldTypeText(field)
			}
			for _, fieldName := range field.Names {
				// When using -generate-for=exported, skip non-exported inner struct fields
				if isInnerStruct && config.GenerateFor != nil && *config.GenerateFor == GenerateForExported {
					if !unicode.IsUpper(rune(fieldName.Name[0])) {
						continue // Skip non-exported inner struct fields
					}
				}

				structField := StructField{
					StructFlags:   &structFlags,
					StructName:    structName,
					FieldName:     fieldName.Name,
					FieldTypeText: fieldTypeText,
					Acronym:       sp.fieldAcronym(field),
				}
				if structFlags.Visibility != NoVisibility {
					if !sp.fieldOptional(field) {
						structFields = append(structFields, &structField)
					}
				}
				// Skip getter generation for inner struct types (type aliases)
				// because Go doesn't allow methods on type aliases
				if sp.fieldGetter(field) && structInfo.TypeSpec != nil {
					bld.WriteString(structField.GenerateGetter())
				}
			}
		}

		for i, field := range structFields {
			isLast := i == len(structFields)-1
			var code string
			if i == 0 {
				code = field.GenerateSourceCodeForStructField(nil, isLast)
			} else {
				code = field.GenerateSourceCodeForStructField(structFields[i-1], isLast)
			}
			bld.WriteString(code)
		}
	}

	result := bld.String()
	if err := os.WriteFile(config.OutputFile, []byte(result), 0644); err != nil {
		return fmt.Errorf("failed to write output file: %w", err)
	}

	// Format the generated code with goimports
	cmd := exec.Command("goimports", "-w", config.OutputFile)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to format generated code: %w", err)
	}

	return nil
}

func main() {
	config, err := parseCommandLineArgs()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	if err := generateCode(config); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Code generation completed successfully!")
}
