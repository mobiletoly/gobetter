package main

import (
	"fmt"
	"go/ast"
	"strings"
)

// TypeFormatter renders Go AST expressions into source strings.
type TypeFormatter struct {
	StructHandler    func(tf *TypeFormatter, st *ast.StructType) string
	InterfaceHandler func(tf *TypeFormatter, it *ast.InterfaceType) string
	AliasResolver    func(st *ast.StructType) (string, bool)
}

// newDefaultTypeFormatter returns a TypeFormatter with standard handlers for
// struct and interface rendering.
func newDefaultTypeFormatter() *TypeFormatter {
	tf := &TypeFormatter{}
	tf.StructHandler = defaultStructHandler
	tf.InterfaceHandler = defaultInterfaceHandler
	return tf
}

// Format converts an ast.Expr into its string representation.
func (tf *TypeFormatter) Format(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + tf.Format(t.X)
	case *ast.ArrayType:
		return "[]" + tf.Format(t.Elt)
	case *ast.MapType:
		return fmt.Sprintf("map[%s]%s", tf.Format(t.Key), tf.Format(t.Value))
	case *ast.SelectorExpr:
		return fmt.Sprintf("%s.%s", tf.Format(t.X), t.Sel.Name)
	case *ast.IndexExpr:
		return fmt.Sprintf("%s[%s]", tf.Format(t.X), tf.Format(t.Index))
	case *ast.IndexListExpr:
		items := make([]string, 0, len(t.Indices))
		for _, idx := range t.Indices {
			items = append(items, tf.Format(idx))
		}
		return fmt.Sprintf("%s[%s]", tf.Format(t.X), strings.Join(items, ", "))
	case *ast.ChanType:
		return formatChanType(tf, t)
	case *ast.FuncType:
		return formatFuncType(tf, t)
	case *ast.Ellipsis:
		return "..." + tf.Format(t.Elt)
	case *ast.ParenExpr:
		return fmt.Sprintf("(%s)", tf.Format(t.X))
	case *ast.InterfaceType:
		if tf.InterfaceHandler != nil {
			return tf.InterfaceHandler(tf, t)
		}
		return "interface{}"
	case *ast.StructType:
		if tf.AliasResolver != nil {
			if alias, ok := tf.AliasResolver(t); ok {
				return alias
			}
		}
		if tf.StructHandler != nil {
			return tf.StructHandler(tf, t)
		}
		return "struct{}"
	default:
		return "interface{}"
	}
}

func defaultStructHandler(tf *TypeFormatter, st *ast.StructType) string {
	if st == nil {
		return "struct{}"
	}
	parts := make([]string, 0, len(st.Fields.List))
	for _, field := range st.Fields.List {
		fieldType := tf.Format(field.Type)

		tag := ""
		if field.Tag != nil {
			tag = " " + field.Tag.Value
		}

		if len(field.Names) == 0 {
			parts = append(parts, fmt.Sprintf("%s%s", fieldType, tag))
			continue
		}

		for _, name := range field.Names {
			parts = append(parts, fmt.Sprintf("%s %s%s", name.Name, fieldType, tag))
		}
	}

	if len(parts) == 0 {
		return "struct{}"
	}
	return fmt.Sprintf("struct { %s }", strings.Join(parts, "; "))
}

func defaultInterfaceHandler(tf *TypeFormatter, iface *ast.InterfaceType) string {
	if iface == nil || iface.Methods == nil || len(iface.Methods.List) == 0 {
		return "interface{}"
	}

	var parts []string
	for _, m := range iface.Methods.List {
		if len(m.Names) == 0 {
			parts = append(parts, tf.Format(m.Type))
			continue
		}

		if ft, ok := m.Type.(*ast.FuncType); ok {
			parts = append(parts, fmt.Sprintf("%s%s", m.Names[0].Name, formatFuncSignature(tf, ft)))
			continue
		}

		parts = append(parts, m.Names[0].Name)
	}

	return "interface{ " + strings.Join(parts, "; ") + " }"
}

func formatTuple(tf *TypeFormatter, fl *ast.FieldList) string {
	if fl == nil || len(fl.List) == 0 {
		return "()"
	}

	var items []string
	for _, f := range fl.List {
		typ := tf.Format(f.Type)
		count := 1
		if n := len(f.Names); n > 0 {
			for _, name := range f.Names {
				items = append(items, fmt.Sprintf("%s %s", name.Name, typ))
			}
			continue
		}
		for i := 0; i < count; i++ {
			items = append(items, typ)
		}
	}
	return "(" + strings.Join(items, ", ") + ")"
}

func formatResults(tf *TypeFormatter, fl *ast.FieldList) string {
	if fl == nil || len(fl.List) == 0 {
		return ""
	}

	var items []string
	for _, f := range fl.List {
		typ := tf.Format(f.Type)
		count := 1
		if n := len(f.Names); n > 0 {
			for _, name := range f.Names {
				items = append(items, fmt.Sprintf("%s %s", name.Name, typ))
			}
			continue
		}
		for i := 0; i < count; i++ {
			items = append(items, typ)
		}
	}

	if len(items) == 1 {
		return " " + items[0]
	}
	return " (" + strings.Join(items, ", ") + ")"
}

func formatFuncSignature(tf *TypeFormatter, ft *ast.FuncType) string {
	params := formatTuple(tf, ft.Params)
	results := formatResults(tf, ft.Results)
	return params + results
}

func formatFuncType(tf *TypeFormatter, ft *ast.FuncType) string {
	return "func" + formatFuncSignature(tf, ft)
}

func formatChanType(tf *TypeFormatter, ch *ast.ChanType) string {
	value := tf.Format(ch.Value)
	switch ch.Dir {
	case ast.RECV:
		return "<-chan " + value
	case ast.SEND:
		return "chan<- " + value
	default:
		return "chan " + value
	}
}
