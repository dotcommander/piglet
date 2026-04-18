package repomap

import (
	"go/ast"
	"go/token"
)

// extractFunc extracts a Symbol from a function or method declaration.
// Returns (Symbol, false) if the function is unexported.
func extractFunc(fset *token.FileSet, d *ast.FuncDecl) (Symbol, bool) {
	if !isExported(d.Name.Name) {
		return Symbol{}, false
	}

	sym := Symbol{
		Name:     d.Name.Name,
		Kind:     "function",
		Exported: true,
		Line:     fset.Position(d.Name.Pos()).Line,
	}

	if d.Recv != nil && len(d.Recv.List) > 0 {
		sym.Kind = "method"
		sym.Receiver = receiverString(d.Recv.List[0])
	}

	sym.Signature = funcSignature(d.Type)
	return sym, true
}

// extractGenDecl extracts symbols from a general declaration (type, const, var).
func extractGenDecl(fset *token.FileSet, d *ast.GenDecl) []Symbol {
	var syms []Symbol
	switch d.Tok {
	case token.TYPE:
		for _, spec := range d.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok || !isExported(ts.Name.Name) {
				continue
			}
			kind := "type"
			var signature string
			switch t := ts.Type.(type) {
			case *ast.StructType:
				kind = "struct"
				signature = structFields(t)
			case *ast.InterfaceType:
				kind = "interface"
				signature = interfaceMethods(t)
			}
			syms = append(syms, Symbol{Name: ts.Name.Name, Kind: kind, Exported: true, Signature: signature, Line: fset.Position(ts.Name.Pos()).Line})
		}
	case token.CONST:
		for _, spec := range d.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, name := range vs.Names {
				if isExported(name.Name) {
					syms = append(syms, Symbol{Name: name.Name, Kind: "constant", Exported: true, Line: fset.Position(name.Pos()).Line})
				}
			}
		}
	case token.VAR:
		for _, spec := range d.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, name := range vs.Names {
				if isExported(name.Name) {
					syms = append(syms, Symbol{Name: name.Name, Kind: "variable", Exported: true, Line: fset.Position(name.Pos()).Line})
				}
			}
		}
	}
	return syms
}
