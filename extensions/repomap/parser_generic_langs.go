package repomap

import "strings"

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
