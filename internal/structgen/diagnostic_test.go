package structgen

import (
	"go/token"
	"reflect"
	"testing"
)

func TestDiagnosticOrderingDoesNotMutateInput(t *testing.T) {
	input := Diagnostics{
		{Package: "b", Position: token.Position{Filename: "z.go", Line: 1, Column: 1}, Code: InvalidOption},
		{Package: "a", Position: token.Position{Filename: "z.go", Line: 1, Column: 1}, Code: DirectiveDuplicate},
		{Package: "a", Position: token.Position{Filename: "a.go", Line: 2, Column: 1}, Code: InvalidOption},
		{Package: "a", Position: token.Position{Filename: "a.go", Line: 1, Column: 2}, Code: UnusedFieldDirective},
		{Package: "a", Position: token.Position{Filename: "a.go", Line: 1, Column: 2}, Code: InvalidOption},
	}
	before := append(Diagnostics(nil), input...)
	want := Diagnostics{input[4], input[3], input[2], input[1], input[0]}
	if got := input.Sorted(); !reflect.DeepEqual(got, want) {
		t.Fatalf("diagnostics: %+v, want %+v", got, want)
	}
	if !reflect.DeepEqual(input, before) {
		t.Fatal("sorting mutated input diagnostics")
	}
}
