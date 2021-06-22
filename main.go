package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

type StructParser struct {
	fileSet            *token.FileSet
	fileContent        []byte
	whitespaceRegexp   *regexp.Regexp
	flagRequiredRegexp *regexp.Regexp
}

type GobBuilder struct {
	common          strings.Builder
	constructorDef  strings.Builder
	constructorBody strings.Builder
	astFile         *ast.File
}

// HelloStruct comment
type HelloStruct struct { //+constructor
	// First and last names
	FirstName string //+required
	// Age
	Age int `json:"age"` //+required
	// Description
	Description *string `json:"description"`
	Tags        []int   `json:"tags"`
	// Pointer to function
	ZZ func(a1, a2 int,
		a3 *string) interface{} //+required

	Test  strings.Builder //+required
	test2 *ast.Scope
}

func (bld *GobBuilder) appendPackage(filename string) {
	bld.common.WriteString("// Code generated by gobetter; DO NOT EDIT.\n\n")
	bld.common.WriteString(fmt.Sprintf("//go:generate goimports -w %s\n\n", filename))
	bld.common.WriteString(fmt.Sprintf("package %s\n\n", bld.astFile.Name.Name))
}

func (bld *GobBuilder) appendImports() {
	bld.common.WriteString("import (\n")
	for _, i := range bld.astFile.Imports {
		fmt.Println(i.Path.Value)
		bld.common.WriteString(fmt.Sprintf("\t%s\n", i.Path.Value))
	}
	bld.common.WriteString(")\n\n")
}

func (bld *GobBuilder) appendArgStruct(structName string, fieldName string, fieldType string) (structArgName string) {
	structArgName = structName + fieldName + "Arg"
	bld.common.WriteString(fmt.Sprintf("// %s represents field %s of struct %s\n", structArgName, fieldName, structName))
	bld.common.WriteString(fmt.Sprintf("type %s struct {\n", structArgName))
	bld.common.WriteString(fmt.Sprintf("\tArg %s\n}\n", fieldType))
	bld.common.WriteString(fmt.Sprintf("// %s%s creates argument for field %s\n", structName, fieldName, fieldName))
	bld.common.WriteString(fmt.Sprintf("func %s%s(arg %s) %s {\n", structName, fieldName,
		fieldType, structArgName))
	bld.common.WriteString(fmt.Sprintf("\treturn %s{Arg: arg}\n}\n\n", structArgName))
	return
}

func (bld *GobBuilder) appendArgStructConstructor(structName string, fieldName string, fieldType string) (structArgName string) {
	structArgName = structName + fieldName + "Arg"
	bld.common.WriteString(fmt.Sprintf("func %s%s(arg %s) %s {\n", structName, fieldName,
		fieldType, structArgName))
	bld.common.WriteString(fmt.Sprintf("\treturn %s{Arg: arg}\n}\n\n", structArgName))
	return
}

func (bld *GobBuilder) appendBeginConstructorDef(structName string) {
	bld.constructorDef.WriteString(fmt.Sprintf("// New%s creates new instance of %s struct\n", structName, structName))
	bld.constructorDef.WriteString(fmt.Sprintf("func New%s(\n", structName))
}

func (bld *GobBuilder) appendBeginConstructorBody(structName string) {
	bld.constructorBody.WriteString(fmt.Sprintf("\treturn %s{\n", structName))
}

func (bld *GobBuilder) appendConstructorArg(fieldName string, structArgName string) {
	argName := "arg" + fieldName
	bld.constructorDef.WriteString(fmt.Sprintf("\t%s %s,\n", argName, structArgName))
	bld.constructorBody.WriteString(fmt.Sprintf("\t\t%s: %s.Arg,\n", fieldName, argName))
}

func (bld *GobBuilder) Build() string {
	return bld.common.String()
}

func (bld *GobBuilder) AcceptStruct(structName string) {
	if bld.constructorDef.Len() > 0 {
		bld.common.WriteString(bld.constructorDef.String())
		bld.common.WriteString(fmt.Sprintf(") %s {\n", structName))
		bld.common.WriteString(bld.constructorBody.String())
		bld.common.WriteString("\t}\n")
		bld.common.WriteString("}\n")
		bld.constructorDef.Reset()
		bld.constructorBody.Reset()
	}
}

func NewStructParser(fileSet *token.FileSet, fileContent []byte) StructParser {
	return StructParser{
		fileSet:            fileSet,
		fileContent:        fileContent,
		whitespaceRegexp:   regexp.MustCompile(`\s+`),
		flagRequiredRegexp: regexp.MustCompile("\\b+required\\b"),
	}
}

func (sp *StructParser) fieldTypeText(field *ast.Field) string {
	begin := sp.fileSet.Position(field.Type.Pos()).Offset
	end := sp.fileSet.Position(field.Type.End()).Offset
	return sp.whitespaceRegexp.ReplaceAllString(string(sp.fileContent[begin:end]), " ")
}

func (sp *StructParser) fieldRequired(field *ast.Field) bool {
	return sp.flagRequiredRegexp.MatchString(field.Comment.Text())
}

func fileNameWithoutExt(fileName string) string {
	return strings.TrimSuffix(fileName, filepath.Ext(fileName))
}

func outputFilename(inFilename string) string {
	path := filepath.Dir(inFilename)
	ext := filepath.Ext(inFilename)
	outFilename := fmt.Sprintf("%s/%s_gob%s", path, fileNameWithoutExt(filepath.Base(inFilename)), ext)
	return outFilename
}

func parseCommandLineArgs() (inFilename string, outFilename string) {
	if len(os.Args) < 2 {
		_, _ = fmt.Fprintln(os.Stderr, "Error: filename is required")
		os.Exit(1)
	}
	inFilename = os.Args[1]
	if _, err := os.Stat(inFilename); os.IsNotExist(err) {
		_, _ = fmt.Fprintf(os.Stderr, "File %s does not exist\n", inFilename)
		os.Exit(1)
	}

	_, err := exec.LookPath("goimports")
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "Error: \"goimports\" executable does not exist")
		_, _ = fmt.Fprintln(os.Stderr, "You must install it to continue with gobetter:\n"+
			"    go get golang.org/x/tools/cmd/goimports")
		os.Exit(1)
	}

	outFilename = outputFilename(inFilename)
	println("Input file: " + inFilename)
	println("Output file: " + outFilename)
	return
}

func main() {

	inFilename, outFilename := parseCommandLineArgs()
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
	gobBld.appendPackage(filepath.Base(outFilename))
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
		fmt.Printf("Struct type declaration found : %s\n", ts.Name.Name)

		structName := ts.Name.Name

		for _, field := range st.Fields.List {
			fieldTypeText := sp.fieldTypeText(field)
			for _, fieldName := range field.Names {
				requiredField := sp.fieldRequired(field)

				if requiredField {
					structArgName := gobBld.appendArgStruct(structName, fieldName.Name, fieldTypeText)
					if gobBld.constructorDef.Len() == 0 {
						gobBld.appendBeginConstructorDef(structName)
						gobBld.appendBeginConstructorBody(structName)
					}
					gobBld.appendConstructorArg(fieldName.Name, structArgName)
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
}
