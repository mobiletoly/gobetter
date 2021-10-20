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
	defaultTypes *string,
) {
	_, err := exec.LookPath("goimports")
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "Error: \"goimports\" executable does not exist")
		_, _ = fmt.Fprintln(os.Stderr, "You must install it to continue with gobetter:\n"+
			"    go get golang.org/x/tools/cmd/goimports")
		os.Exit(1)
	}

	inputFilePtr := flag.String("input", "filename", "go input file")
	outputFilePtr := flag.String("output", "filename", "go output file (optional)")
	defaultTypes = flag.String("generate-for", "exported", "parse even non-annotated "+
		"struct types (\"all\" for exported and package-level, \"exported\" for exported only)")
	boolPtr := flag.Bool("print-version", true, "a bool")

	flag.Parse()
	if isFlagPassed("print-version") && *boolPtr {
		println("gobetter version 0.3")
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

	if isFlagPassed("generate-for") {
		if *defaultTypes != "all" && *defaultTypes != "exported" {
			_, _ = fmt.Fprintln(os.Stderr, "Error: \"generate-for\" flag must be \"all\" or \"exported\"")
			os.Exit(1)
		}
	} else {
		defaultTypes = nil
	}

	println("Input file: " + inFilename)
	println("Output file: " + outFilename)
	return inFilename, outFilename, defaultTypes
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

	inFilename, outFilename, defaultTypes := parseCommandLineArgs()
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
		processStruct, visibility := sp.constructorVisibility(st)
		if !processStruct {
			if defaultTypes == nil {
				return true
			}
			if *defaultTypes == "exported" {
				if !unicode.IsUpper(rune(ts.Name.Name[0])) {
					return true
				}
			}
		}

		fmt.Printf("Process structure %s\n", structName)

		for _, field := range st.Fields.List {
			fieldTypeText := sp.fieldTypeText(field)
			for _, fieldName := range field.Names {
				if visibility != NoVisibility {
					if !sp.fieldOptional(field) {
						structArgName := gobBld.appendArgStruct(structName, fieldName.Name, fieldTypeText, visibility)
						if gobBld.constructorValueDef.Len() == 0 {
							gobBld.appendBeginConstructorDef(structName, visibility)
							gobBld.appendBeginConstructorBody(structName)
						}
						gobBld.appendConstructorArg(fieldName.Name, structArgName)
					}
				}
				if sp.fieldGetter(field) {
					gobBld.appendGetter(structName, fieldName.Name, fieldTypeText)
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
