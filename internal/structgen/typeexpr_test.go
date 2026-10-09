package structgen

import (
	"go/ast"
	"go/parser"
	"strings"
	"testing"
)

func TestRewriteTypeByIdentity(t *testing.T) {
	expr, err := parser.ParseExpr("struct { T T `json:\"T\"`; Member pkg.T; Call func(T int) T }")
	if err != nil {
		t.Fatal(err)
	}
	st := expr.(*ast.StructType)
	bindings := map[*ast.Ident]string{st.Fields.List[0].Type.(*ast.Ident): "gobetterT0", st.Fields.List[2].Type.(*ast.FuncType).Results.List[0].Type.(*ast.Ident): "gobetterT0"}
	before, err := FormatType(expr, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	after, err := FormatType(expr, bindings, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(strings.Fields(after), " "), "T gobetterT0") || !strings.Contains(after, `json:"T"`) || !strings.Contains(after, "pkg.T") || !strings.Contains(after, "func(T int) gobetterT0") {
		t.Fatalf("binding-aware rewrite: %s", after)
	}
	original, err := FormatType(expr, nil, nil)
	if err != nil || original != before {
		t.Fatal("source AST was mutated")
	}
	if _, err := FormatType(&ast.BadExpr{}, nil, nil); err == nil {
		t.Fatal("invalid type syntax was replaced or accepted")
	}
}

// These ordinary-type cases replace the retired CLI formatter tests.
func TestOrdinaryTypeFormattingCoverage(t *testing.T) {
	for _, text := range []string{"pkg.Type", "[3]string", "*[]string", "map[string]int", "chan int", "chan<- string", "<-chan bool", "func(ctx context.Context, values []string) (int, *Error)", "func(...string)", "List[string]", "Map[string,int]", "(T)", "struct{ A int; Embedded }", "interface{ Read(p []byte) (int,error); Close() error }"} {
		t.Run(text, func(t *testing.T) {
			expr, err := parser.ParseExpr(text)
			if err != nil {
				t.Fatal(err)
			}
			rendered, err := FormatType(expr, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			again, err := parser.ParseExpr(rendered)
			if err != nil {
				t.Fatal(err)
			}
			roundtrip, err := FormatType(again, nil, nil)
			if err != nil || roundtrip != rendered {
				t.Fatalf("formatting changed identity syntax: %s %v", rendered, err)
			}
		})
	}
}

func TestNestedAliasFormattingByIdentity(t *testing.T) {
	expr, err := parser.ParseExpr("struct { Value int }")
	if err != nil {
		t.Fatal(err)
	}
	st := expr.(*ast.StructType)
	text, err := FormatType(st, nil, map[*ast.StructType]ast.Expr{st: ast.NewIdent("AliasType")})
	if err != nil || text != "AliasType" {
		t.Fatalf("alias identity formatting: %s %v", text, err)
	}
}
