package main

import "regexp"

// Version information
const (
	Version = "0.12.1"
)

// File extensions and suffixes
const (
	GoFileExtension = ".go"
	GobFileSuffix   = "_gob"
)

// Command line flag names
const (
	FlagInput       = "input"
	FlagOutput      = "output"
	FlagGenerateFor = "generate-for"
	FlagReceiver    = "receiver"
	FlagConstructor = "constructor"
	FlagVersion     = "print-version"
)

// Generate-for flag values
const (
	GenerateForAll       = "all"
	GenerateForExported  = "exported"
	GenerateForAnnotated = "annotated"
)

// Receiver type values
const (
	ReceiverValue   = "value"
	ReceiverPointer = "pointer"
)

// Constructor visibility values
const (
	ConstructorExported = "exported"
	ConstructorPackage  = "package"
	ConstructorNone     = "none"
)

// Gob annotation patterns
const (
	GobConstructorExported = `\b+gob:Constructor\b`
	GobConstructorPackage  = `\b+gob:constructor\b`
	GobConstructorNone     = `\b+gob:_\b`
	GobFlagOptional        = `\b+gob:_\b`
	GobFlagGetter          = `\b+gob:getter\b`
	GobFlagAcronym         = `\b+gob:acronym\b`
	WhitespacePattern      = `\s+`
)

// Builder-related constants
const (
	BuilderSuffix    = "Builder"
	BuilderPrefix    = "_Builder_"
	GobFinalizerName = "GobFinalizer"
	BuildMethodName  = "Build"
	NewPrefix        = "New"
	LowerNewPrefix   = "new"
)

// Error messages
const (
	ErrGoimportsNotFound  = "Error: \"goimports\" executable does not exist"
	ErrGoimportsInstall   = "You must install it to continue with gobetter:\n    go get golang.org/x/tools/cmd/goimports"
	ErrInputRequired      = "Error: \"input\" flag must be specified"
	ErrFileNotExist       = "File %s does not exist"
	ErrInvalidGenerateFor = "Error: \"generate-for\" flag must be \"all\", \"exported\", or \"annotated\""
	ErrInvalidReceiver    = "Error: \"receiver\" flag must be \"pointer\" or \"value\""
	ErrInvalidConstructor = "Error: \"constructor\" flag must be \"exported\", \"package\", or \"none\""
	ErrReadFile           = "error: failed to read file %s: %v"
)

// Compiled regex patterns (initialized in init function)
var (
	WhitespaceRegexp          *regexp.Regexp
	ConstructorExportedRegexp *regexp.Regexp
	ConstructorPackageRegexp  *regexp.Regexp
	ConstructorNoRegexp       *regexp.Regexp
	FlagOptionalRegexp        *regexp.Regexp
	FlagGetterRegexp          *regexp.Regexp
	FlagAcronymRegexp         *regexp.Regexp
)

func init() {
	// Compile all regex patterns once at startup
	WhitespaceRegexp = regexp.MustCompile(WhitespacePattern)
	ConstructorExportedRegexp = regexp.MustCompile(GobConstructorExported)
	ConstructorPackageRegexp = regexp.MustCompile(GobConstructorPackage)
	ConstructorNoRegexp = regexp.MustCompile(GobConstructorNone)
	FlagOptionalRegexp = regexp.MustCompile(GobFlagOptional)
	FlagGetterRegexp = regexp.MustCompile(GobFlagGetter)
	FlagAcronymRegexp = regexp.MustCompile(GobFlagAcronym)
}
