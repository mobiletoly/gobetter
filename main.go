package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"
)

func fileNameWithoutExt(fileName string) string {
	return strings.TrimSuffix(fileName, filepath.Ext(fileName))
}

func makeOutputFilename(inFilename string) string {
	path := filepath.Dir(inFilename)
	ext := filepath.Ext(inFilename)
	outFilename := fmt.Sprintf("%s/%s_gob%s", path, fileNameWithoutExt(filepath.Base(inFilename)), ext)
	return outFilename
}

func parseCommandLineArgs() (
	inFilename string,
	outFilename string,
	generateFor *string,
	usePtrReceiver bool,
	constructorVisibility string,
) {
	_, err := exec.LookPath("goimports")
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "Error: \"goimports\" executable does not exist")
		_, _ = fmt.Fprintln(os.Stderr, "You must install it to continue with gobetter:\n"+
			"    go get golang.org/x/tools/cmd/goimports")
		os.Exit(1)
	}

	inputFilePtr := flag.String("input", "", "go input file path")
	outputFilePtr := flag.String("output", "", "go output file path (optional)")
	generateForPtr := flag.String("generate-for", "annotated",
		`allows parsing of non-annotated struct types:
|  all       - process exported and package-level classes
|  exported  - process exported classes only
|  annotated - process specifically annotated class only
`)
	receiverTypePtr := flag.String("receiver", "value",
		`specify function receiver type:
|  value     - receiver must be a value type, e.g. { func (v *Class) Name }
|  pointer   - receiver must be a pointer type, e.g. { func (v Class) Name }
`)
	constructorVisibilityPtr := flag.String("constructor", "exported",
		`generate exported or package-level constructors:
|  exported  - exported (upper-cased) constructors will be created
|  package   - package-level (lower-cased) constructors will be created
|  none      - no constructors will be created
`)
	flag.Bool("print-version", false, "print current version")

	flag.Parse()
	if isFlagPassed("print-version") {
		println("gobetter version 0.7")
	}

	inFilename = *inputFilePtr

	if !isFlagPassed("input") {
		_, _ = fmt.Fprintln(os.Stderr, "Error: \"input\" flag must be specified")
		os.Exit(1)
	}
	if _, err := os.Stat(inFilename); os.IsNotExist(err) {
		_, _ = fmt.Fprintf(os.Stderr, "File %s does not exist\n", inFilename)
		os.Exit(1)
	}

	if isFlagPassed("output") {
		outFilename = *outputFilePtr
	} else {
		outFilename = makeOutputFilename(inFilename)
	}

	if *generateForPtr == "all" || *generateForPtr == "exported" {
		generateFor = generateForPtr
	} else if *generateForPtr == "annotated" {
		generateFor = nil
	} else {
		_, _ = fmt.Fprintln(os.Stderr, "Error: \"generate-for\" flag must be \"all\", \"exported\", or \"annotated\"")
		os.Exit(1)
	}

	if *receiverTypePtr == "pointer" {
		usePtrReceiver = true
	} else if *receiverTypePtr == "value" {
		usePtrReceiver = false
	} else {
		_, _ = fmt.Fprintln(os.Stderr, "Error: \"receiver\" flag must be \"pointer\" or \"value\"")
		os.Exit(1)
	}

	if *constructorVisibilityPtr == "exported" || *constructorVisibilityPtr == "package" || *constructorVisibilityPtr == "none" {
		constructorVisibility = *constructorVisibilityPtr
	} else {
		_, _ = fmt.Fprintln(os.Stderr, "Error: \"constructor\" flag must be \"exported\", \"package\", or \"none\"")
		os.Exit(1)
	}

	println("Input file:", inFilename)
	println("Output file:", outFilename)
	return
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

func main() {

	inFilename, outFilename, defaultTypes, usePtrReceiver, constructorVisibility := parseCommandLineArgs()
	fileContent, err := ioutil.ReadFile(inFilename)
	fset := token.NewFileSet()
	astFile, err := parser.ParseFile(fset, inFilename, nil, parser.ParseComments)
	if err != nil {
		log.Fatal(err)
	}
	sp := NewStructParser(fset, fileContent)

	gobBld := GobBuilder{
		astFile: astFile,
	}
	gobBld.appendPackage()
	gobBld.appendImports()

	ast.Inspect(astFile, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSpec)
		if !ok {
			return true
		}
		st, ok := ts.Type.(*ast.StructType)
		if !ok {
			return true
		}

		structName := ts.Name.Name
		structFlags := sp.constructorFlags(st)
		if !structFlags.ProcessStruct {
			if defaultTypes == nil {
				return true
			}
			if *defaultTypes == "exported" {
				if !unicode.IsUpper(rune(ts.Name.Name[0])) {
					return true
				}
			}
			structFlags.ProcessStruct = true
			structFlags.PtrReceiver = usePtrReceiver
			if constructorVisibility == "exported" {
				structFlags.Visibility = ExportedVisibility
			} else if constructorVisibility == "package" {
				structFlags.Visibility = PackageLevelVisibility
			} else {
				structFlags.Visibility = NoVisibility
			}
		}

		fmt.Printf("Process structure %s\n", structName)

		for _, field := range st.Fields.List {
			fieldTypeText := sp.fieldTypeText(field)
			for _, fieldName := range field.Names {
				if structFlags.Visibility != NoVisibility {
					if !sp.fieldOptional(field) {
						structArgName := gobBld.appendArgStruct(structName, fieldName.Name, fieldTypeText, structFlags)
						if gobBld.constructorValueDef.Len() == 0 {
							gobBld.appendBeginConstructorDef(structName, structFlags)
							gobBld.appendBeginConstructorBody(structName)
						}
						gobBld.appendConstructorArg(fieldName.Name, structArgName)
					}
				}
				if sp.fieldGetter(field) {
					gobBld.appendGetter(structName, fieldName.Name, fieldTypeText, structFlags, false)
				} else if sp.fieldUppercaseGetter(field) {
					gobBld.appendGetter(structName, fieldName.Name, fieldTypeText, structFlags, true)
				}
			}
		}

		gobBld.AcceptStruct(structName)
		return true
	})

	result := gobBld.Build()
	if err = ioutil.WriteFile(outFilename, []byte(result), os.FileMode(0644)); err != nil {
		panic(err)
	}
	z := exec.Command("goimports", "-w", outFilename)
	if err := z.Run(); err != nil {
		log.Fatal(err)
	}
}
