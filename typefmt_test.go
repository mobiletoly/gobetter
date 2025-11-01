package main

import (
	"go/ast"
	"testing"
)

func TestTypeFormatter_Rendering(t *testing.T) {
	tf := newDefaultTypeFormatter()

	tests := []struct {
		name string
		expr ast.Expr
		want string
	}{
		{
			name: "basic selector",
			expr: &ast.SelectorExpr{X: ast.NewIdent("pkg"), Sel: ast.NewIdent("Type")},
			want: "pkg.Type",
		},
		{
			name: "pointer to array",
			expr: &ast.StarExpr{X: &ast.ArrayType{Elt: ast.NewIdent("string")}},
			want: "*[]string",
		},
		{
			name: "map type",
			expr: &ast.MapType{Key: ast.NewIdent("string"), Value: ast.NewIdent("int")},
			want: "map[string]int",
		},
		{
			name: "channel bidirectional",
			expr: &ast.ChanType{Dir: ast.SEND | ast.RECV, Value: ast.NewIdent("int")},
			want: "chan int",
		},
		{
			name: "channel send only",
			expr: &ast.ChanType{Dir: ast.SEND, Value: ast.NewIdent("string")},
			want: "chan<- string",
		},
		{
			name: "channel recv only",
			expr: &ast.ChanType{Dir: ast.RECV, Value: ast.NewIdent("bool")},
			want: "<-chan bool",
		},
		{
			name: "function type",
			expr: &ast.FuncType{
				Params: &ast.FieldList{List: []*ast.Field{
					{
						Names: []*ast.Ident{ast.NewIdent("ctx")},
						Type:  &ast.SelectorExpr{X: ast.NewIdent("context"), Sel: ast.NewIdent("Context")},
					},
					{
						Names: []*ast.Ident{ast.NewIdent("values")},
						Type:  &ast.ArrayType{Elt: ast.NewIdent("string")},
					},
				}},
				Results: &ast.FieldList{List: []*ast.Field{
					{Type: ast.NewIdent("int")},
					{Type: &ast.StarExpr{X: ast.NewIdent("Error")}},
				}},
			},
			want: "func(ctx context.Context, values []string) (int, *Error)",
		},
		{
			name: "variadic function",
			expr: &ast.FuncType{
				Params: &ast.FieldList{List: []*ast.Field{{Type: &ast.Ellipsis{Elt: ast.NewIdent("string")}}}},
			},
			want: "func(...string)",
		},
		{
			name: "indexed generic",
			expr: &ast.IndexExpr{X: ast.NewIdent("List"), Index: ast.NewIdent("string")},
			want: "List[string]",
		},
		{
			name: "multi index generic",
			expr: &ast.IndexListExpr{X: ast.NewIdent("Map"), Indices: []ast.Expr{ast.NewIdent("string"), ast.NewIdent("int")}},
			want: "Map[string, int]",
		},
		{
			name: "paren expr",
			expr: &ast.ParenExpr{X: ast.NewIdent("T")},
			want: "(T)",
		},
		{
			name: "struct literal",
			expr: &ast.StructType{Fields: &ast.FieldList{List: []*ast.Field{
				{Names: []*ast.Ident{ast.NewIdent("A")}, Type: ast.NewIdent("int")},
				{Type: ast.NewIdent("Embedded")},
			}}},
			want: "struct { A int; Embedded }",
		},
		{
			name: "interface literal",
			expr: &ast.InterfaceType{Methods: &ast.FieldList{List: []*ast.Field{
				{
					Names: []*ast.Ident{ast.NewIdent("Read")},
					Type: &ast.FuncType{
						Params:  &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ast.NewIdent("p")}, Type: &ast.ArrayType{Elt: ast.NewIdent("byte")}}}},
						Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("int")}, {Type: ast.NewIdent("error")}}},
					},
				},
				{
					Names: []*ast.Ident{ast.NewIdent("Close")},
					Type:  &ast.FuncType{Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("error")}}}},
				},
			}}},
			want: "interface{ Read(p []byte) (int, error); Close() error }",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tf.Format(tc.expr); got != tc.want {
				t.Fatalf("Format() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTypeFormatter_AliasResolver(t *testing.T) {
	fieldStruct := &ast.StructType{}
	aliasMap := map[*ast.StructType]string{fieldStruct: "AliasType"}

	tf := newDefaultTypeFormatter()
	tf.AliasResolver = func(st *ast.StructType) (string, bool) {
		alias, ok := aliasMap[st]
		return alias, ok
	}

	if got := tf.Format(fieldStruct); got != "AliasType" {
		t.Fatalf("expected alias to be used, got %s", got)
	}
}
