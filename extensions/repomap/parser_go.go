package repomap

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// ParseGoFile extracts exported symbols from a Go source file.
// path is absolute, root is the project root for relative path calculation.
func ParseGoFile(path, root string) (*FileSymbols, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	relPath, err := filepath.Rel(root, path)
	if err != nil {
		relPath = path
	}

	fs := &FileSymbols{
		Path:       relPath,
		Language:   "go",
		Package:    file.Name.Name,
		ImportPath: resolveImportPath(path, root),
	}

	// Collect imports.
	for _, imp := range file.Imports {
		impPath := strings.Trim(imp.Path.Value, `"`)
		fs.Imports = append(fs.Imports, impPath)
	}

	// Walk top-level declarations.
	// For package main, also capture unexported main() and init() as entry points.
	isMain := fs.Package == "main"
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if sym, ok := extractFunc(fset, d); ok {
				fs.Symbols = append(fs.Symbols, sym)
			} else if isMain && d.Recv == nil &&
				(d.Name.Name == "main" || d.Name.Name == "init") {
				fs.Symbols = append(fs.Symbols, Symbol{
					Name:     d.Name.Name,
					Kind:     "function",
					Exported: false,
					Line:     fset.Position(d.Name.Pos()).Line,
				})
			}
		case *ast.GenDecl:
			syms := extractGenDecl(fset, d)
			fs.Symbols = append(fs.Symbols, syms...)
		}
	}

	return fs, nil
}

// moduleInfo caches the result of go.mod discovery for a project root.
type moduleInfo struct {
	name   string // module name from go.mod
	modDir string // directory containing go.mod
}

// cachedModules caches go.mod lookups keyed by project root.
// Safe for concurrent use because parseFiles runs in a single errgroup
// and all goroutines share the same root, so the cache is populated
// at most once before concurrent reads.
var cachedModules sync.Map // map[string]*moduleInfo

// resolveImportPath computes the Go import path for absPath relative to
// the module root at root. Caches go.mod discovery per root.
func resolveImportPath(absPath, root string) string {
	mi := getModuleInfo(root)
	if mi == nil {
		return ""
	}

	rel, err := filepath.Rel(mi.modDir, filepath.Dir(absPath))
	if err != nil {
		return ""
	}

	if rel == "." {
		return mi.name
	}
	return mi.name + "/" + filepath.ToSlash(rel)
}

// getModuleInfo returns cached module info for the given root, or discovers it.
func getModuleInfo(root string) *moduleInfo {
	if v, ok := cachedModules.Load(root); ok {
		return v.(*moduleInfo)
	}

	modPath := findGoMod(root)
	if modPath == "" {
		cachedModules.Store(root, (*moduleInfo)(nil))
		return nil
	}

	data, err := os.ReadFile(modPath)
	if err != nil {
		cachedModules.Store(root, (*moduleInfo)(nil))
		return nil
	}

	name := parseModuleName(string(data))
	if name == "" {
		cachedModules.Store(root, (*moduleInfo)(nil))
		return nil
	}

	mi := &moduleInfo{name: name, modDir: filepath.Dir(modPath)}
	cachedModules.Store(root, mi)
	return mi
}

// findGoMod walks up from root looking for go.mod.
func findGoMod(root string) string {
	dir := root
	for {
		candidate := filepath.Join(dir, "go.mod")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// isExported reports whether name begins with an uppercase ASCII letter,
// meaning it is an exported Go identifier.
func isExported(name string) bool {
	return len(name) > 0 && name[0] >= 'A' && name[0] <= 'Z'
}

// parseModuleName extracts the module name from go.mod content.
func parseModuleName(content string) string {
	for line := range strings.SplitSeq(content, "\n") {
		line = strings.TrimSpace(line)
		if name, ok := strings.CutPrefix(line, "module "); ok {
			return strings.TrimSpace(name)
		}
	}
	return ""
}

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

// receiverString formats a receiver field as "*TypeName" or "TypeName".
func receiverString(field *ast.Field) string {
	if field.Type == nil {
		return ""
	}
	return typeString(field.Type)
}

// funcSignature builds "(param, param, ...) ReturnType" from a FuncType.
// Uses param names only when the full signature with types exceeds 40 chars;
// otherwise includes types for clarity.
func funcSignature(ft *ast.FuncType) string {
	paramsShort := paramList(ft.Params, false)
	returnStr := returnString(ft.Results)

	short := "(" + paramsShort + ")"
	if returnStr != "" {
		short += " " + returnStr
	}

	if len(short) <= 40 {
		// Try with types — if it still fits, prefer the richer form.
		paramsLong := paramList(ft.Params, true)
		long := "(" + paramsLong + ")"
		if returnStr != "" {
			long += " " + returnStr
		}
		if len(long) <= 40 {
			return long
		}
	}

	return short
}

// paramList formats parameters. If withTypes is true, includes type
// annotations; otherwise emits only param names (or "_" for unnamed).
func paramList(fl *ast.FieldList, withTypes bool) string {
	if fl == nil {
		return ""
	}

	var parts []string
	for _, field := range fl.List {
		typ := typeString(field.Type)
		if len(field.Names) == 0 {
			// Unnamed parameter — emit type or "_".
			if withTypes {
				parts = append(parts, typ)
			} else {
				parts = append(parts, "_")
			}
			continue
		}
		for _, name := range field.Names {
			if withTypes {
				parts = append(parts, name.Name+" "+typ)
			} else {
				parts = append(parts, name.Name)
			}
		}
	}
	return strings.Join(parts, ", ")
}

// returnString formats the return types of a function.
func returnString(fl *ast.FieldList) string {
	if fl == nil || len(fl.List) == 0 {
		return ""
	}

	var parts []string
	for _, field := range fl.List {
		typ := typeString(field.Type)
		if len(field.Names) == 0 {
			parts = append(parts, typ)
		} else {
			for range field.Names {
				parts = append(parts, typ)
			}
		}
	}

	if len(parts) == 1 {
		return parts[0]
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

// typeString converts an AST type expression to a compact string.
func typeString(expr ast.Expr) string {
	if expr == nil {
		return ""
	}
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + typeString(t.X)
	case *ast.SelectorExpr:
		return typeString(t.X) + "." + t.Sel.Name
	case *ast.ArrayType:
		if t.Len == nil {
			return "[]" + typeString(t.Elt)
		}
		return "[...]" + typeString(t.Elt)
	case *ast.MapType:
		return "map[" + typeString(t.Key) + "]" + typeString(t.Value)
	case *ast.ChanType:
		switch t.Dir {
		case ast.SEND:
			return "chan<- " + typeString(t.Value)
		case ast.RECV:
			return "<-chan " + typeString(t.Value)
		default:
			return "chan " + typeString(t.Value)
		}
	case *ast.Ellipsis:
		return "..." + typeString(t.Elt)
	case *ast.FuncType:
		return "func(" + paramList(t.Params, true) + ")"
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.StructType:
		return "struct{}"
	case *ast.IndexExpr:
		return typeString(t.X) + "[" + typeString(t.Index) + "]"
	case *ast.IndexListExpr:
		var args []string
		for _, idx := range t.Indices {
			args = append(args, typeString(idx))
		}
		return typeString(t.X) + "[" + strings.Join(args, ", ") + "]"
	default:
		return "..."
	}
}

// structFields extracts exported field names from a struct type.
// Returns a compact representation like "{field1, field2, field3}".
func structFields(st *ast.StructType) string {
	if st.Fields == nil {
		return "{}"
	}

	var names []string
	for _, field := range st.Fields.List {
		// Embedded type (no field names)
		if len(field.Names) == 0 {
			if ident, ok := field.Type.(*ast.Ident); ok && isExported(ident.Name) {
				names = append(names, ident.Name)
			}
			continue
		}
		// Named fields
		for _, name := range field.Names {
			if isExported(name.Name) {
				names = append(names, name.Name)
			}
		}
	}

	if len(names) == 0 {
		return "{}"
	}
	return "{" + strings.Join(names, ", ") + "}"
}

// interfaceMethods extracts method names from an interface type.
// Returns a compact representation like "{Method1, Method2}".
func interfaceMethods(it *ast.InterfaceType) string {
	if it.Methods == nil {
		return "{}"
	}

	var names []string
	for _, field := range it.Methods.List {
		// Embedded interface
		if len(field.Names) == 0 {
			if ident, ok := field.Type.(*ast.Ident); ok && isExported(ident.Name) {
				names = append(names, ident.Name)
			}
			continue
		}
		// Methods
		for _, name := range field.Names {
			if isExported(name.Name) {
				names = append(names, name.Name)
			}
		}
	}

	if len(names) == 0 {
		return "{}"
	}
	return "{" + strings.Join(names, ", ") + "}"
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
