package repomap

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// maxScanLines is the maximum number of lines scanned per file.
const maxScanLines = 500

// --- TypeScript / JavaScript patterns ---

var (
	tsExportDecl    = regexp.MustCompile(`export\s+(function|class|interface|type|const|enum)\s+(\w+)`)
	tsExportDefault = regexp.MustCompile(`export\s+default\s+(function|class)\s+(\w+)`)
	tsReExport      = regexp.MustCompile(`export\s+\{([^}]+)\}`)
	tsImportFrom    = regexp.MustCompile(`import\s+.*\s+from\s+['"]([^'"]+)['"]`)
	tsRequire       = regexp.MustCompile(`require\s*\(\s*['"]([^'"]+)['"]\s*\)`)
)

// --- Python patterns ---

var (
	pyFunc    = regexp.MustCompile(`^def\s+([A-Za-z]\w*)\s*\(`)
	pyClass   = regexp.MustCompile(`^class\s+(\w+)`)
	pyConst   = regexp.MustCompile(`^([A-Z][A-Z_0-9]+)\s*=`)
	pyImport  = regexp.MustCompile(`^import\s+(\w+)`)
	pyFrom    = regexp.MustCompile(`^from\s+(\w+)`)
)

// --- Rust patterns ---

var (
	rustPubItem  = regexp.MustCompile(`^pub\s+(fn|struct|enum|trait|type|const|static)\s+(\w+)`)
	rustPubAsync = regexp.MustCompile(`^pub\s+async\s+fn\s+(\w+)`)
	rustImpl     = regexp.MustCompile(`^impl(?:<[^>]*>)?\s+(\w+)`)
	rustUse      = regexp.MustCompile(`^use\s+([^;{]+)`)
)

// --- C / C++ patterns ---

var (
	cFunc    = regexp.MustCompile(`^(?:[\w:*&\s]+)\s+(\w+)\s*\(`)
	cTagDecl = regexp.MustCompile(`^(?:struct|class|enum|typedef)\s+(\w+)`)
	cInclude = regexp.MustCompile(`^#include\s*[<"]([^>"]+)[>"]`)
)

// --- Java patterns ---

var (
	javaTypeDecl   = regexp.MustCompile(`public\s+(?:static\s+)?(?:final\s+)?(?:class|interface|enum|record)\s+(\w+)`)
	javaMethodDecl = regexp.MustCompile(`public\s+(?:static\s+)?(?:[\w<>\[\],\s]+)\s+(\w+)\s*\(`)
	javaImport     = regexp.MustCompile(`^import\s+(?:static\s+)?([^;]+)`)
)

// --- Ruby patterns ---

var rubyDecl = regexp.MustCompile(`^(?:def|class|module)\s+(\w+)`)

// --- PHP patterns ---

var (
	phpClass     = regexp.MustCompile(`^(?:abstract\s+|final\s+)?class\s+(\w+)`)
	phpInterface = regexp.MustCompile(`^interface\s+(\w+)`)
	phpTrait     = regexp.MustCompile(`^trait\s+(\w+)`)
	phpEnum      = regexp.MustCompile(`^enum\s+(\w+)`)
	phpFunction  = regexp.MustCompile(`^(?:public\s+|protected\s+|private\s+)?(?:static\s+)?function\s+(\w+)`)
	phpConst     = regexp.MustCompile(`^(?:public\s+|protected\s+|private\s+)?const\s+(\w+)`)
	phpUse       = regexp.MustCompile(`^use\s+([^;{]+)`)
	phpNamespace = regexp.MustCompile(`^namespace\s+([^;]+)`)
)

// ParseGenericFile extracts symbols from a non-Go source file using regex
// patterns. path is absolute, root is the project root for relative path
// calculation.
func ParseGenericFile(path, root, language string) (*FileSymbols, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	rel, err := filepath.Rel(root, path)
	if err != nil {
		rel = path
	}

	fs := &FileSymbols{
		Path:     rel,
		Language: language,
	}

	lines := strings.Split(string(data), "\n")
	if len(lines) > maxScanLines {
		lines = lines[:maxScanLines]
	}

	switch language {
	case "typescript", "javascript", "tsx", "jsx":
		parseTS(lines, fs)
	case "python":
		parsePython(lines, fs)
	case "rust":
		parseRust(lines, fs)
	case "c", "cpp", "c++", "cxx":
		parseC(lines, fs)
	case "java":
		parseJava(lines, fs)
	case "ruby":
		parseRuby(lines, fs)
	case "php":
		parsePHP(lines, fs)
	// swift, kotlin, lua, zig — unsupported, return empty
	}

	return fs, nil
}

// parseTS processes TypeScript/JavaScript lines.
func parseTS(lines []string, fs *FileSymbols) {
	for lineIdx, line := range lines {
		trimmed := strings.TrimSpace(line)

		if m := tsExportDecl.FindStringSubmatch(trimmed); m != nil {
			fs.Symbols = append(fs.Symbols, Symbol{Name: m[2], Kind: m[1], Line: lineIdx + 1})
			continue
		}
		if m := tsExportDefault.FindStringSubmatch(trimmed); m != nil {
			fs.Symbols = append(fs.Symbols, Symbol{Name: m[2], Kind: m[1], Line: lineIdx + 1})
			continue
		}
		if m := tsReExport.FindStringSubmatch(trimmed); m != nil {
			for _, name := range splitReExportNames(m[1]) {
				fs.Symbols = append(fs.Symbols, Symbol{Name: name, Kind: "reexport", Line: lineIdx + 1})
			}
			continue
		}

		if m := tsImportFrom.FindStringSubmatch(trimmed); m != nil {
			fs.Imports = append(fs.Imports, m[1])
			continue
		}
		if m := tsRequire.FindStringSubmatch(trimmed); m != nil {
			fs.Imports = append(fs.Imports, m[1])
		}
	}
}

// splitReExportNames splits a re-export list like "Foo, Bar as Baz" into
// individual exported names.
func splitReExportNames(raw string) []string {
	parts := strings.Split(raw, ",")
	var names []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		// Handle "Foo as Bar" — take the local name (first word)
		fields := strings.Fields(p)
		if len(fields) > 0 {
			names = append(names, fields[0])
		}
	}
	return names
}

// parsePython processes Python lines, skipping triple-quoted docstrings.
func parsePython(lines []string, fs *FileSymbols) {
	inDocstring := false
	docQuote := ""

	for lineIdx, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Track triple-quoted strings used as block comments / docstrings.
		if inDocstring {
			if strings.Contains(trimmed, docQuote) {
				inDocstring = false
			}
			continue
		}
		for _, q := range []string{`"""`, `'''`} {
			if strings.HasPrefix(trimmed, q) {
				rest := trimmed[len(q):]
				if !strings.Contains(rest, q) {
					inDocstring = true
					docQuote = q
				}
				break
			}
		}
		if inDocstring {
			continue
		}

		if m := pyFunc.FindStringSubmatch(line); m != nil {
			fs.Symbols = append(fs.Symbols, Symbol{Name: m[1], Kind: "function", Line: lineIdx + 1})
			continue
		}
		if m := pyClass.FindStringSubmatch(line); m != nil {
			fs.Symbols = append(fs.Symbols, Symbol{Name: m[1], Kind: "class", Line: lineIdx + 1})
			continue
		}
		if m := pyConst.FindStringSubmatch(line); m != nil {
			fs.Symbols = append(fs.Symbols, Symbol{Name: m[1], Kind: "const", Line: lineIdx + 1})
			continue
		}
		if m := pyImport.FindStringSubmatch(line); m != nil {
			fs.Imports = append(fs.Imports, m[1])
			continue
		}
		if m := pyFrom.FindStringSubmatch(line); m != nil {
			fs.Imports = append(fs.Imports, m[1])
		}
	}
}

// parseRust processes Rust lines.
func parseRust(lines []string, fs *FileSymbols) {
	inBlockComment := false

	for lineIdx, line := range lines {
		trimmed := strings.TrimSpace(line)

		inBlockComment = trackBlockComment(trimmed, inBlockComment)
		if inBlockComment {
			continue
		}

		if m := rustPubAsync.FindStringSubmatch(trimmed); m != nil {
			fs.Symbols = append(fs.Symbols, Symbol{Name: m[1], Kind: "fn", Line: lineIdx + 1})
			continue
		}
		if m := rustPubItem.FindStringSubmatch(trimmed); m != nil {
			fs.Symbols = append(fs.Symbols, Symbol{Name: m[2], Kind: m[1], Line: lineIdx + 1})
			continue
		}
		if m := rustImpl.FindStringSubmatch(trimmed); m != nil {
			fs.Symbols = append(fs.Symbols, Symbol{Name: m[1], Kind: "impl", Line: lineIdx + 1})
			continue
		}
		if m := rustUse.FindStringSubmatch(trimmed); m != nil {
			fs.Imports = append(fs.Imports, strings.TrimSpace(m[1]))
		}
	}
}

// parseC processes C/C++ lines.
func parseC(lines []string, fs *FileSymbols) {
	inBlockComment := false

	for lineIdx, line := range lines {
		trimmed := strings.TrimSpace(line)

		inBlockComment = trackBlockComment(trimmed, inBlockComment)
		if inBlockComment {
			continue
		}

		// Skip preprocessor directives other than #include
		if strings.HasPrefix(trimmed, "#") {
			if m := cInclude.FindStringSubmatch(trimmed); m != nil {
				fs.Imports = append(fs.Imports, m[1])
			}
			continue
		}

		if m := cTagDecl.FindStringSubmatch(trimmed); m != nil {
			kind := strings.Fields(trimmed)[0] // struct / class / enum / typedef
			fs.Symbols = append(fs.Symbols, Symbol{Name: m[1], Kind: kind, Line: lineIdx + 1})
			continue
		}

		// Function declarations: must start at column 0 (no leading whitespace)
		// and contain a '('.
		if line == trimmed && strings.Contains(line, "(") {
			if m := cFunc.FindStringSubmatch(trimmed); m != nil {
				fs.Symbols = append(fs.Symbols, Symbol{Name: m[1], Kind: "function", Line: lineIdx + 1})
			}
		}
	}
}

// parseJava processes Java lines.
func parseJava(lines []string, fs *FileSymbols) {
	inBlockComment := false

	for lineIdx, line := range lines {
		trimmed := strings.TrimSpace(line)

		inBlockComment = trackBlockComment(trimmed, inBlockComment)
		if inBlockComment {
			continue
		}

		if m := javaImport.FindStringSubmatch(trimmed); m != nil {
			fs.Imports = append(fs.Imports, strings.TrimSpace(m[1]))
			continue
		}
		if m := javaTypeDecl.FindStringSubmatch(trimmed); m != nil {
			// Determine the kind from the keyword preceding the name
			kind := "class"
			for _, kw := range []string{"interface", "enum", "record"} {
				if strings.Contains(trimmed, kw) {
					kind = kw
					break
				}
			}
			fs.Symbols = append(fs.Symbols, Symbol{Name: m[1], Kind: kind, Line: lineIdx + 1})
			continue
		}
		if m := javaMethodDecl.FindStringSubmatch(trimmed); m != nil {
			fs.Symbols = append(fs.Symbols, Symbol{Name: m[1], Kind: "method", Line: lineIdx + 1})
		}
	}
}

// parsePHP processes PHP lines.
func parsePHP(lines []string, fs *FileSymbols) {
	inBlockComment := false

	for lineIdx, line := range lines {
		trimmed := strings.TrimSpace(line)

		if trimmed == "<?php" || trimmed == "?>" || trimmed == "<?" {
			continue
		}

		inBlockComment = trackBlockComment(trimmed, inBlockComment)
		if inBlockComment {
			continue
		}

		if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") {
			continue
		}

		if m := phpNamespace.FindStringSubmatch(trimmed); m != nil {
			fs.Package = strings.TrimSpace(m[1])
			continue
		}
		if m := phpUse.FindStringSubmatch(trimmed); m != nil {
			fs.Imports = append(fs.Imports, strings.TrimSpace(m[1]))
			continue
		}
		if m := phpClass.FindStringSubmatch(trimmed); m != nil {
			fs.Symbols = append(fs.Symbols, Symbol{Name: m[1], Kind: "class", Exported: true, Line: lineIdx + 1})
			continue
		}
		if m := phpInterface.FindStringSubmatch(trimmed); m != nil {
			fs.Symbols = append(fs.Symbols, Symbol{Name: m[1], Kind: "interface", Exported: true, Line: lineIdx + 1})
			continue
		}
		if m := phpTrait.FindStringSubmatch(trimmed); m != nil {
			fs.Symbols = append(fs.Symbols, Symbol{Name: m[1], Kind: "trait", Exported: true, Line: lineIdx + 1})
			continue
		}
		if m := phpEnum.FindStringSubmatch(trimmed); m != nil {
			fs.Symbols = append(fs.Symbols, Symbol{Name: m[1], Kind: "enum", Exported: true, Line: lineIdx + 1})
			continue
		}
		if m := phpFunction.FindStringSubmatch(trimmed); m != nil {
			// Skip magic methods and constructors
			if strings.HasPrefix(m[1], "__") {
				continue
			}
			kind := "function"
			// If indented (inside a class), treat as method
			if len(line) > len(trimmed) {
				kind = "method"
			}
			fs.Symbols = append(fs.Symbols, Symbol{Name: m[1], Kind: kind, Exported: true, Line: lineIdx + 1})
			continue
		}
		if m := phpConst.FindStringSubmatch(trimmed); m != nil {
			fs.Symbols = append(fs.Symbols, Symbol{Name: m[1], Kind: "constant", Exported: true, Line: lineIdx + 1})
			continue
		}
	}
}

// parseRuby processes Ruby lines.
func parseRuby(lines []string, fs *FileSymbols) {
	for lineIdx, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		if m := rubyDecl.FindStringSubmatch(trimmed); m != nil {
			kind := strings.Fields(trimmed)[0] // def / class / module
			fs.Symbols = append(fs.Symbols, Symbol{Name: m[1], Kind: kind, Line: lineIdx + 1})
		}
	}
}

// trackBlockComment advances the block-comment state machine for C-family
// languages (/* ... */). It returns the new inBlockComment state.
func trackBlockComment(trimmed string, inBlockComment bool) bool {
	if inBlockComment {
		if idx := strings.Index(trimmed, "*/"); idx >= 0 {
			return false
		}
		return true
	}
	if idx := strings.Index(trimmed, "/*"); idx >= 0 {
		// Only enter block comment if the closing */ is not on the same line.
		rest := trimmed[idx+2:]
		if !strings.Contains(rest, "*/") {
			return true
		}
	}
	return false
}
